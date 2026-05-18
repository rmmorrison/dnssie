package server

import (
	"net"
	"strconv"
	"strings"

	"github.com/miekg/dns"

	"github.com/rmmorrison/dnssie/internal/store"
)

const defaultTTL uint32 = 300

// supportedQType maps the record types dnssie can store to their DNS type
// codes. It is deliberately limited so we never serve a type the UI can't
// create.
var supportedQType = map[string]uint16{
	"A":     dns.TypeA,
	"AAAA":  dns.TypeAAAA,
	"CNAME": dns.TypeCNAME,
	"PTR":   dns.TypePTR,
	"NS":    dns.TypeNS,
	"MX":    dns.TypeMX,
	"SOA":   dns.TypeSOA,
	"TXT":   dns.TypeTXT,
}

// recType returns the DNS type code for rec, or false if dnssie doesn't serve
// that record type.
func recType(rec store.Record) (uint16, bool) {
	t, ok := supportedQType[strings.ToUpper(strings.TrimSpace(rec.Type))]
	return t, ok
}

// matches reports whether rec answers a question for qname/qtype by exact
// name. qname must already be canonical (lowercase, trailing dot); store
// names are FQDNs.
func matches(rec store.Record, qname string, qtype uint16) bool {
	t, ok := recType(rec)
	if !ok || t != qtype {
		return false
	}
	return dns.CanonicalName(rec.Name) == qname
}

// wildcardBase reports whether canonical name cn is a wildcard ("*.something")
// and returns the suffix it stands in for, keeping the trailing dot. "*."
// itself yields an empty base — a catch-all that matches every name.
func wildcardBase(cn string) (string, bool) {
	if !strings.HasPrefix(cn, "*.") {
		return "", false
	}
	return cn[2:], true
}

// underWildcard reports whether qname is covered by a wildcard whose base is
// the given suffix. qname must sit strictly below base (at least one label in
// place of "*"); the wildcard never matches base itself, its parent name.
func underWildcard(qname, base string) bool {
	return qname != base && strings.HasSuffix(qname, "."+base)
}

// answerRecords selects the stored records that should answer qname/qtype.
//
// Wildcard precedence follows RFC 4592, simplified for a local dev tool: an
// exact name match always wins; otherwise the most specific ("longest")
// wildcard whose suffix the query falls under is used. We deliberately skip
// the closest-encloser empty-non-terminal rule — for local development,
// "*.app.test." catching "a.b.app.test." is the least surprising behavior.
//
// qname must already be canonical (lowercase, trailing dot).
func answerRecords(recs []store.Record, qname string, qtype uint16) []store.Record {
	var exact, wild []store.Record
	bestBase := ""
	haveWild := false
	for _, rec := range recs {
		t, ok := recType(rec)
		if !ok || t != qtype {
			continue
		}
		cn := dns.CanonicalName(rec.Name)
		if cn == qname {
			exact = append(exact, rec)
			continue
		}
		base, ok := wildcardBase(cn)
		if !ok || !underWildcard(qname, base) {
			continue
		}
		// A query name has exactly one suffix of any given length, so a
		// longer base is unambiguously the more specific wildcard.
		switch {
		case !haveWild || len(base) > len(bestBase):
			bestBase, wild, haveWild = base, []store.Record{rec}, true
		case base == bestBase:
			wild = append(wild, rec)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return wild
}

// buildRR converts a stored record into a dns.RR for the question. A
// malformed value yields nil so the record is simply treated as non-matching
// rather than crashing the server.
func buildRR(rec store.Record, qname string, qtype uint16, ttl uint32) dns.RR {
	hdr := dns.RR_Header{Name: qname, Rrtype: qtype, Class: dns.ClassINET, Ttl: ttl}
	v := strings.TrimSpace(rec.Value)

	switch qtype {
	case dns.TypeA:
		ip := net.ParseIP(v)
		if ip == nil || ip.To4() == nil {
			return nil
		}
		return &dns.A{Hdr: hdr, A: ip.To4()}

	case dns.TypeAAAA:
		ip := net.ParseIP(v)
		if ip == nil || ip.To4() != nil || ip.To16() == nil {
			return nil
		}
		return &dns.AAAA{Hdr: hdr, AAAA: ip.To16()}

	case dns.TypeCNAME:
		if v == "" {
			return nil
		}
		return &dns.CNAME{Hdr: hdr, Target: dns.Fqdn(v)}

	case dns.TypePTR:
		if v == "" {
			return nil
		}
		return &dns.PTR{Hdr: hdr, Ptr: dns.Fqdn(v)}

	case dns.TypeNS:
		if v == "" {
			return nil
		}
		return &dns.NS{Hdr: hdr, Ns: dns.Fqdn(v)}

	case dns.TypeMX:
		f := strings.Fields(v)
		if len(f) != 2 {
			return nil
		}
		pref, err := strconv.ParseUint(f[0], 10, 16)
		if err != nil {
			return nil
		}
		return &dns.MX{Hdr: hdr, Preference: uint16(pref), Mx: dns.Fqdn(f[1])}

	case dns.TypeSOA:
		// ns mbox serial refresh retry expire minimum
		f := strings.Fields(v)
		if len(f) != 7 {
			return nil
		}
		var n [5]uint32
		for i := 0; i < 5; i++ {
			x, err := strconv.ParseUint(f[2+i], 10, 32)
			if err != nil {
				return nil
			}
			n[i] = uint32(x)
		}
		return &dns.SOA{
			Hdr: hdr, Ns: dns.Fqdn(f[0]), Mbox: dns.Fqdn(f[1]),
			Serial: n[0], Refresh: n[1], Retry: n[2], Expire: n[3], Minttl: n[4],
		}

	case dns.TypeTXT:
		if v == "" {
			return nil
		}
		return &dns.TXT{Hdr: hdr, Txt: splitTXT(v)}
	}
	return nil
}

// splitTXT turns a stored TXT value into character-strings. A value that
// starts with a quote is tokenized as one or more "quoted" strings;
// otherwise the whole value is one string. Any chunk longer than the 255-byte
// DNS limit is split.
func splitTXT(v string) []string {
	s := strings.TrimSpace(v)
	var chunks []string

	if strings.HasPrefix(s, `"`) {
		var cur strings.Builder
		inQuote, esc := false, false
		for _, r := range s {
			switch {
			case esc:
				cur.WriteRune(r)
				esc = false
			case r == '\\':
				esc = true
			case r == '"':
				if inQuote {
					chunks = append(chunks, cur.String())
					cur.Reset()
				}
				inQuote = !inQuote
			case inQuote:
				cur.WriteRune(r)
			}
		}
		if len(chunks) == 0 {
			chunks = []string{v}
		}
	} else {
		chunks = []string{v}
	}

	var out []string
	for _, c := range chunks {
		for len(c) > 255 {
			out = append(out, c[:255])
			c = c[255:]
		}
		out = append(out, c)
	}
	return out
}
