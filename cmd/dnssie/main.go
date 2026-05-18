// Command dnssie is a terminal UI for managing records served by the dnssie
// DNS server.
package main

import (
	"fmt"
	"os"

	"github.com/rmmorrison/dnssie/internal/tui"
)

func main() {
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "dnssie:", err)
		os.Exit(1)
	}
}
