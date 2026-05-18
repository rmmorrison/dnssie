package server

import (
	"errors"
	"time"

	"github.com/miekg/dns"

	"github.com/rmmorrison/dnssie/internal/config"
)

// errNoUpstreams means there is nowhere to forward an unmatched query.
var errNoUpstreams = errors.New("no upstream resolvers configured")

// upstreamsFor returns the resolvers to forward to. Manual mode uses the
// configured list; otherwise the OS resolvers. All returned strings are
// already normalized to host:port (NormalizeUpstream / SystemResolvers).
func upstreamsFor(cfg config.Config) ([]string, error) {
	if cfg.Resolvers.Mode == config.ModeManual {
		return cfg.Resolvers.Upstream, nil
	}
	return config.SystemResolvers()
}

// forward sends req to each upstream in order and returns the first reply.
// proto mirrors the inbound transport; a truncated UDP reply is retried over
// TCP against the same upstream.
func forward(req *dns.Msg, upstreams []string, proto string, timeout time.Duration) (*dns.Msg, error) {
	if len(upstreams) == 0 {
		return nil, errNoUpstreams
	}
	c := &dns.Client{Net: proto, Timeout: timeout}

	var lastErr error
	for _, up := range upstreams {
		resp, _, err := c.Exchange(req, up)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.Truncated && proto == "udp" {
			tcp := &dns.Client{Net: "tcp", Timeout: timeout}
			if r2, _, err2 := tcp.Exchange(req, up); err2 == nil {
				return r2, nil
			}
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = errors.New("all upstream resolvers failed")
	}
	return nil, lastErr
}
