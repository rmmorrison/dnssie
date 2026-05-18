package tui

import "testing"

func TestFQDN(t *testing.T) {
	cases := map[string]string{
		"www.example.com":  "www.example.com.",
		"www.example.com.": "www.example.com.",
		"example.com":      "example.com.",
		"  example.com  ":  "example.com.",
		"":                 "",
		"   ":              "",
	}
	for in, want := range cases {
		if got := fqdn(in); got != want {
			t.Errorf("fqdn(%q) = %q, want %q", in, got, want)
		}
	}
}
