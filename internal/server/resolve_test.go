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

func TestAnswerRecordsWildcard(t *testing.T) {
	recs := []store.Record{
		{Type: "A", Name: "*.app.test.", Value: "10.0.0.1"},
		{Type: "A", Name: "api.app.test.", Value: "10.0.0.2"},
		{Type: "A", Name: "*.test.", Value: "10.0.0.9"},
		{Type: "AAAA", Name: "*.app.test.", Value: "2001:db8::1"},
	}
	only := func(rs []store.Record) string {
		t.Helper()
		if len(rs) != 1 {
			t.Fatalf("want 1 record, got %d (%v)", len(rs), rs)
		}
		return rs[0].Value
	}

	if got := only(answerRecords(recs, "api.app.test.", dns.TypeA)); got != "10.0.0.2" {
		t.Errorf("exact beats wildcard: got %s, want 10.0.0.2", got)
	}
	if got := only(answerRecords(recs, "x.app.test.", dns.TypeA)); got != "10.0.0.1" {
		t.Errorf("most specific wildcard wins: got %s, want 10.0.0.1", got)
	}
	if got := only(answerRecords(recs, "a.b.app.test.", dns.TypeA)); got != "10.0.0.1" {
		t.Errorf("multi-label under wildcard: got %s, want 10.0.0.1", got)
	}
	if got := only(answerRecords(recs, "other.test.", dns.TypeA)); got != "10.0.0.9" {
		t.Errorf("falls back to broader wildcard: got %s, want 10.0.0.9", got)
	}
	if got := only(answerRecords(recs, "x.app.test.", dns.TypeAAAA)); got != "2001:db8::1" {
		t.Errorf("wildcard honors qtype: got %s, want 2001:db8::1", got)
	}
}

func TestAnswerRecordsWildcardExcludesParent(t *testing.T) {
	recs := []store.Record{{Type: "A", Name: "*.app.test.", Value: "10.0.0.1"}}
	if rs := answerRecords(recs, "app.test.", dns.TypeA); len(rs) != 0 {
		t.Errorf("wildcard must not answer its parent name, got %v", rs)
	}
	if rs := answerRecords(recs, "x.app.test.", dns.TypeA); len(rs) != 1 {
		t.Errorf("wildcard should answer a child, got %v", rs)
	}
}

func ip(v int) *int { return &v }

func TestMaxErraticPct(t *testing.T) {
	if got := maxErraticPct(nil); got != 0 {
		t.Errorf("maxErraticPct(nil) = %d, want 0", got)
	}
	recs := []store.Record{
		{Type: "A", Name: "a.", Value: "1"},                     // unset -> 0
		{Type: "A", Name: "a.", Value: "2", ErraticPct: ip(30)}, // highest wins
		{Type: "A", Name: "a.", Value: "3", ErraticPct: ip(10)},
	}
	if got := maxErraticPct(recs); got != 30 {
		t.Errorf("maxErraticPct = %d, want 30 (the most erratic record)", got)
	}
	if got := maxErraticPct([]store.Record{{ErraticPct: ip(250)}}); got != 100 {
		t.Errorf("maxErraticPct clamps high = %d, want 100", got)
	}
	if got := maxErraticPct([]store.Record{{ErraticPct: ip(-5)}}); got != 0 {
		t.Errorf("maxErraticPct clamps low = %d, want 0", got)
	}
}

func TestAnswerRecordsCatchAll(t *testing.T) {
	recs := []store.Record{{Type: "A", Name: "*.", Value: "127.0.0.1"}}
	rs := answerRecords(recs, "anything.example.org.", dns.TypeA)
	if len(rs) != 1 || rs[0].Value != "127.0.0.1" {
		t.Errorf(`"*." should match everything, got %v`, rs)
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
