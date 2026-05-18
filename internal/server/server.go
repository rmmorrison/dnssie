package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/paths"
	"github.com/rmmorrison/dnssie/internal/store"
)

// maxQueryLog is how many recent lookups the server keeps for the TUI.
const maxQueryLog = 200

// configCache mirrors recordCache for config.toml so resolver/upstream edits
// take effect on a running server without a restart. A missing file yields
// the built-in defaults.
type configCache struct {
	st *config.Store

	mu     sync.RWMutex
	mod    time.Time
	loaded bool
	cfg    config.Config
}

func newConfigCache(st *config.Store) *configCache {
	return &configCache{st: st, cfg: config.Defaults()}
}

func (c *configCache) snapshot() config.Config {
	fi, err := os.Stat(c.st.Path())
	if err != nil {
		return config.Defaults()
	}
	mod := fi.ModTime()

	c.mu.RLock()
	if c.loaded && mod.Equal(c.mod) {
		cfg := c.cfg
		c.mu.RUnlock()
		return cfg
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded && mod.Equal(c.mod) {
		return c.cfg
	}
	cfg, err := c.st.Load()
	if err != nil {
		return c.cfg
	}
	c.cfg, c.mod, c.loaded = cfg, mod, true
	return c.cfg
}

// Options configures a Server. Zero values fall back to the standard stores
// and Port 0 (an OS-assigned port, useful for tests; see Addr).
type Options struct {
	Port    int
	Records *store.Store
	Config  *config.Store
	Logf    func(string, ...any)
}

// Server answers DNS queries from stored records and forwards the rest.
type Server struct {
	opts  Options
	recs  *recordCache
	cfg   *configCache
	udp   *dns.Server
	tcp   *dns.Server
	qlog  *queryLog
	ready chan struct{}

	mu   sync.RWMutex
	addr string
}

// New builds a Server, defaulting the record/config stores to the standard
// configuration directory when not supplied.
func New(opts Options) (*Server, error) {
	if opts.Records == nil {
		st, err := store.Default()
		if err != nil {
			return nil, err
		}
		opts.Records = st
	}
	if opts.Config == nil {
		cs, err := config.Default()
		if err != nil {
			return nil, err
		}
		opts.Config = cs
	}
	if opts.Logf == nil {
		opts.Logf = log.Printf
	}
	var qlog *queryLog
	if dir, err := paths.ConfigDir(); err == nil {
		qlog = newQueryLog(dir, maxQueryLog)
	}
	return &Server{
		opts:  opts,
		recs:  newRecordCache(opts.Records),
		cfg:   newConfigCache(opts.Config),
		qlog:  qlog,
		ready: make(chan struct{}),
		addr:  net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.Port)),
	}, nil
}

// Addr is the bound listen address. Meaningful once Ready is closed (it
// reflects the OS-assigned port when Options.Port was 0).
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

// Ready is closed once the listeners are bound and serving.
func (s *Server) Ready() <-chan struct{} { return s.ready }

// Run binds UDP+TCP on loopback and serves until ctx is cancelled, then
// shuts down gracefully. Bind failures (port in use, privileged port) are
// returned synchronously.
func (s *Server) Run(ctx context.Context) error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)
	s.qlog.reset() // start each run with a clean lookup log

	pc, err := net.ListenPacket("udp", s.Addr())
	if err != nil {
		return err
	}
	resolved := pc.LocalAddr().String()
	l, err := net.Listen("tcp", resolved)
	if err != nil {
		pc.Close()
		return err
	}

	s.mu.Lock()
	s.addr = resolved
	s.mu.Unlock()

	s.udp = &dns.Server{PacketConn: pc, Handler: mux}
	s.tcp = &dns.Server{Listener: l, Handler: mux}

	errc := make(chan error, 2)
	go func() { errc <- s.udp.ActivateAndServe() }()
	go func() { errc <- s.tcp.ActivateAndServe() }()

	s.opts.Logf("dnssie: serving DNS on %s (udp/tcp)", resolved)
	close(s.ready)

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errc:
		_ = s.shutdown()
		return err
	}
}

func (s *Server) shutdown() error {
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.udp.ShutdownContext(sctx)
	_ = s.tcp.ShutdownContext(sctx)
	return nil
}

func (s *Server) handle(w dns.ResponseWriter, req *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(req)

	if len(req.Question) != 1 {
		m.SetRcode(req, dns.RcodeFormatError)
		_ = w.WriteMsg(m)
		return
	}
	q := req.Question[0]
	qname := dns.CanonicalName(q.Name)

	outcome := "servfail"
	defer func() { s.logQuery(q, outcome) }()

	var answers []dns.RR
	for _, rec := range answerRecords(s.recs.snapshot(), qname, q.Qtype) {
		if rr := buildRR(rec, qname, q.Qtype, rec.TTLOr(defaultTTL)); rr != nil {
			answers = append(answers, rr)
		}
	}
	if len(answers) > 0 {
		m.Answer = answers
		m.Authoritative = true
		outcome = "local"
		_ = w.WriteMsg(m)
		return
	}

	// No local match.
	cfg := s.cfg.snapshot()
	if cfg.Resolvers.Mode == config.ModeOff {
		// Forwarding disabled: dnssie only answers for its own records, so
		// everything else simply doesn't exist here.
		m.SetRcode(req, dns.RcodeNameError)
		outcome = "nxdomain"
		_ = w.WriteMsg(m)
		return
	}

	// Forward.
	ups, err := upstreamsFor(cfg)
	if err != nil || len(ups) == 0 {
		m.SetRcode(req, dns.RcodeServerFailure)
		_ = w.WriteMsg(m)
		return
	}
	proto := "udp"
	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		proto = "tcp"
	}
	reply, ferr := forward(req, ups, proto, 2*time.Second)
	if ferr != nil || reply == nil {
		m.SetRcode(req, dns.RcodeServerFailure)
		_ = w.WriteMsg(m)
		return
	}
	outcome = "forwarded " + dns.RcodeToString[reply.Rcode]
	reply.Id = req.Id
	_ = w.WriteMsg(reply)
}

// logQuery appends a display-ready line to the recent-lookups log the TUI
// tails. The server binds loopback only, so the client is always local and
// omitted to keep lines short.
func (s *Server) logQuery(q dns.Question, outcome string) {
	if s.qlog == nil {
		return
	}
	qtype := dns.TypeToString[q.Qtype]
	if qtype == "" {
		qtype = fmt.Sprintf("TYPE%d", q.Qtype)
	}
	s.qlog.add(fmt.Sprintf("%s  %-28s %-5s %s",
		time.Now().Format("15:04:05"), dns.CanonicalName(q.Name), qtype, outcome))
}
