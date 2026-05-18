package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/store"
)

// startStubUpstream runs a throwaway resolver that answers every query with
// the given A record, and returns its address.
func startStubUpstream(t *testing.T, ip string) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("stub listen: %v", err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = append(m.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   net.ParseIP(ip).To4(),
		})
		_ = w.WriteMsg(m)
	})}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

// runServer starts the server under test on an ephemeral port and returns its
// address.
func runServer(t *testing.T, recDir, cfgDir string) string {
	t.Helper()
	srv, err := New(Options{
		Port:    0,
		Records: store.New(recDir),
		Config:  config.New(cfgDir),
		Logf:    func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Run(ctx) }()
	select {
	case <-srv.Ready():
	case <-time.After(3 * time.Second):
		t.Fatal("server did not become ready")
	}
	return srv.Addr()
}

func query(t *testing.T, addr, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	resp, _, err := (&dns.Client{Net: "udp", Timeout: 2 * time.Second}).Exchange(m, addr)
	if err != nil {
		t.Fatalf("exchange %s: %v", name, err)
	}
	return resp
}

func TestServerAnswersLocalRecord(t *testing.T) {
	recDir := t.TempDir()
	if err := store.New(recDir).Save([]store.Record{
		{Type: "A", Name: "test.example.com.", Value: "192.0.2.9"},
	}); err != nil {
		t.Fatalf("seed records: %v", err)
	}
	addr := runServer(t, recDir, t.TempDir())

	resp := query(t, addr, "test.example.com", dns.TypeA)
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("rcode=%d answers=%d", resp.Rcode, len(resp.Answer))
	}
	if a, ok := resp.Answer[0].(*dns.A); !ok || a.A.String() != "192.0.2.9" {
		t.Errorf("answer = %v, want A 192.0.2.9", resp.Answer[0])
	}
	if !resp.Authoritative {
		t.Error("local answer should be authoritative")
	}
}

func TestServerForwardsUnmatched(t *testing.T) {
	stub := startStubUpstream(t, "203.0.113.5")
	cfgDir := t.TempDir()
	if err := config.New(cfgDir).Save(config.Config{
		Port:      5353,
		Resolvers: config.Resolvers{Mode: config.ModeManual, Upstream: []string{stub}},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	addr := runServer(t, t.TempDir(), cfgDir)

	resp := query(t, addr, "nowhere.example.org", dns.TypeA)
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("rcode=%d answers=%d", resp.Rcode, len(resp.Answer))
	}
	if a, ok := resp.Answer[0].(*dns.A); !ok || a.A.String() != "203.0.113.5" {
		t.Errorf("forwarded answer = %v, want 203.0.113.5", resp.Answer[0])
	}
}

func TestServerServfailWhenNoUpstreams(t *testing.T) {
	cfgDir := t.TempDir()
	if err := config.New(cfgDir).Save(config.Config{
		Port:      5353,
		Resolvers: config.Resolvers{Mode: config.ModeManual, Upstream: nil},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	addr := runServer(t, t.TempDir(), cfgDir)

	resp := query(t, addr, "nowhere.example.org", dns.TypeA)
	if resp.Rcode != dns.RcodeServerFailure {
		t.Errorf("rcode = %d, want SERVFAIL (%d)", resp.Rcode, dns.RcodeServerFailure)
	}
}
