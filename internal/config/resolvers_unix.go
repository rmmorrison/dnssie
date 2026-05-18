//go:build linux || darwin

package config

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// SystemResolvers reads the OS resolvers from /etc/resolv.conf.
func SystemResolvers() ([]string, error) {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseResolvConf(f), nil
}

// parseResolvConf extracts nameserver entries from a resolv.conf-formatted
// stream, normalized to host:port.
func parseResolvConf(r io.Reader) []string {
	var servers []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			if ns := NormalizeUpstream(fields[1]); ns != "" {
				servers = append(servers, ns)
			}
		}
	}
	return servers
}
