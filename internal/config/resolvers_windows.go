//go:build windows

package config

import (
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SystemResolvers returns the OS-configured DNS servers by walking the
// network adapters via the Win32 GetAdaptersAddresses API.
func SystemResolvers() ([]string, error) {
	adapters, err := adapterAddresses()
	if err != nil {
		return nil, err
	}

	var out []string
	seen := make(map[string]bool)
	for _, aa := range adapters {
		if aa.OperStatus != windows.IfOperStatusUp {
			continue
		}
		for dns := aa.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			ip := dns.Address.IP()
			if ip == nil || ip.IsMulticast() || ip.IsUnspecified() {
				continue
			}
			// Windows reports well-known site-local placeholders for
			// unconfigured IPv6 DNS (fec0:0:0:ffff::1..3); skip them.
			if strings.HasPrefix(ip.String(), "fec0:") {
				continue
			}
			hostport := NormalizeUpstream(ip.String())
			if hostport == "" || seen[hostport] {
				continue
			}
			seen[hostport] = true
			out = append(out, hostport)
		}
	}
	return out, nil
}

// adapterAddresses returns every network adapter, growing the buffer until
// the API stops reporting an overflow. The returned slice points into a
// single backing buffer that the GC keeps alive via these pointers.
func adapterAddresses() ([]*windows.IpAdapterAddresses, error) {
	const flags = windows.GAA_FLAG_SKIP_UNICAST |
		windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST |
		windows.GAA_FLAG_SKIP_FRIENDLY_NAME

	size := uint32(15000)
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		err := windows.GetAdaptersAddresses(
			windows.AF_UNSPEC,
			flags,
			0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])),
			&size,
		)
		if err == nil {
			var out []*windows.IpAdapterAddresses
			for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); aa != nil; aa = aa.Next {
				out = append(out, aa)
			}
			return out, nil
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return nil, err
		}
		// size now holds the required length; retry with a bigger buffer.
	}
	return nil, errors.New("GetAdaptersAddresses: buffer kept overflowing")
}
