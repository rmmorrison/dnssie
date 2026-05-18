//go:build linux || darwin

package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseResolvConf(t *testing.T) {
	in := strings.NewReader(`
# a comment
; another comment
nameserver 1.1.1.1
nameserver 8.8.8.8:5353
search example.com
options edns0
nameserver 2001:4860:4860::8888
`)
	got := parseResolvConf(in)
	want := []string{"1.1.1.1:53", "8.8.8.8:5353", "[2001:4860:4860::8888]:53"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseResolvConf = %v, want %v", got, want)
	}
}

func TestParseResolvConfEmpty(t *testing.T) {
	if got := parseResolvConf(strings.NewReader("# nothing here\n")); got != nil {
		t.Errorf("parseResolvConf = %v, want nil", got)
	}
}
