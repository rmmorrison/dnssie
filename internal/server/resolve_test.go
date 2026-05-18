package server

import (
	"testing"

	"github.com/miekg/dns"

	"github.com/rmmorrison/dnssie/internal/store"
)

func TestMatches(t *testing.T) {
	rec := store.Record{Type: "A", Name: "WWW.Example.COM.", Value: "1.2.3.4"}
	if !matches(rec, "www.example.com.", dns.TypeA) {
		t.Error("expected case-insensitive FQDN match")
	}
	if matches(rec, "www.example.com.", dns.TypeAAAA) {
		t.Error("type mismatch should not match")
	}
	if matches(rec, "other.example.com.", dns.TypeA) {
		t.Error("name mismatch should not match")
	}
	if matches(store.Record{Type: "WAT", Name: "x.", Value: "y"}, "x.", dns.TypeA) {
		t.Error("unsupported type should not match")
	}
}

func TestBuildRRValid(t *testing.T) {
	const q = "rec.example.com."
	cases := []struct {
		typ, val string
		qtype    uint16
		check    func(dns.RR) bool
	}{
		{"A", "192.0.2.1", dns.TypeA, func(rr dns.RR) bool { return rr.(*dns.A).A.String() == "192.0.2.1" }},
		{"AAAA", "2001:db8::1", dns.TypeAAAA, func(rr dns.RR) bool { return rr.(*dns.AAAA).AAAA.String() == "2001:db8::1" }},
		{"CNAME", "target.example.com", dns.TypeCNAME, func(rr dns.RR) bool { return rr.(*dns.CNAME).Target == "target.example.com." }},
		{"PTR", "host.example.com", dns.TypePTR, func(rr dns.RR) bool { return rr.(*dns.PTR).Ptr == "host.example.com." }},
		{"NS", "ns1.example.com", dns.TypeNS, func(rr dns.RR) bool { return rr.(*dns.NS).Ns == "ns1.example.com." }},
		{"MX", "10 mail.example.com", dns.TypeMX, func(rr dns.RR) bool {
			m := rr.(*dns.MX)
			return m.Preference == 10 && m.Mx == "mail.example.com."
		}},
		{"SOA", "ns.example.com hostmaster.example.com 1 2 3 4 5", dns.TypeSOA, func(rr dns.RR) bool {
			s := rr.(*dns.SOA)
			return s.Ns == "ns.example.com." && s.Mbox == "hostmaster.example.com." &&
				s.Serial == 1 && s.Refresh == 2 && s.Retry == 3 && s.Expire == 4 && s.Minttl == 5
		}},
		{"TXT", `"v=spf1 -all"`, dns.TypeTXT, func(rr dns.RR) bool {
			tx := rr.(*dns.TXT).Txt
			return len(tx) == 1 && tx[0] == "v=spf1 -all"
		}},
		{"TXT", "unquoted value", dns.TypeTXT, func(rr dns.RR) bool {
			tx := rr.(*dns.TXT).Txt
			return len(tx) == 1 && tx[0] == "unquoted value"
		}},
	}
	for _, c := range cases {
		rec := store.Record{Type: c.typ, Name: q, Value: c.val}
		rr := buildRR(rec, q, c.qtype, defaultTTL)
		if rr == nil {
			t.Errorf("%s %q: buildRR returned nil", c.typ, c.val)
			continue
		}
		if rr.Header().Name != q || rr.Header().Ttl != defaultTTL {
			t.Errorf("%s: bad header %+v", c.typ, rr.Header())
		}
		if !c.check(rr) {
			t.Errorf("%s %q: rdata check failed (%s)", c.typ, c.val, rr.String())
		}
	}
}

func TestBuildRRMalformedReturnsNil(t *testing.T) {
	bad := []struct {
		typ, val string
		qtype    uint16
	}{
		{"A", "not-an-ip", dns.TypeA},
		{"A", "2001:db8::1", dns.TypeA},        // v6 in an A
		{"AAAA", "192.0.2.1", dns.TypeAAAA},    // v4 in an AAAA
		{"MX", "mail.example.com", dns.TypeMX}, // missing preference
		{"MX", "ten mail.example.com", dns.TypeMX},
		{"SOA", "ns mbox 1 2 3", dns.TypeSOA},     // too few fields
		{"SOA", "ns mbox a b c d e", dns.TypeSOA}, // non-numeric
		{"CNAME", "", dns.TypeCNAME},
	}
	for _, c := range bad {
		rec := store.Record{Type: c.typ, Name: "x.", Value: c.val}
		if rr := buildRR(rec, "x.", c.qtype, defaultTTL); rr != nil {
			t.Errorf("%s %q: expected nil, got %s", c.typ, c.val, rr.String())
		}
	}
}

func TestSplitTXTLongChunk(t *testing.T) {
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'a'
	}
	chunks := splitTXT(string(long))
	if len(chunks) != 3 || len(chunks[0]) != 255 || len(chunks[2]) != 90 {
		t.Errorf("unexpected chunking: %d chunks, lens %d/%d/%d",
			len(chunks), len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
}
