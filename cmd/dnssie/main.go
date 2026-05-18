// Command dnssie is a terminal UI for managing records served by the dnssie
// DNS server.
//
// Usage:
//
//	dnssie          launch the TUI (default)
//	dnssie serve    run the DNS server in the foreground
//
// The TUI starts the server as a detached `dnssie serve` child process; see
// internal/supervisor.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/server"
	"github.com/rmmorrison/dnssie/internal/tui"
)

func main() {
	if len(os.Args) < 2 {
		if err := tui.Run(); err != nil {
			fail(err)
		}
		return
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fail(err)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "dnssie: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

// runServe runs the DNS server in the foreground until interrupted. This is
// what the detached child process executes.
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 0, "listen port (0 = use config.toml)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Defaults()
	if st, err := config.Default(); err == nil {
		if c, e := st.Load(); e == nil { // missing file -> defaults, no error
			cfg = c
		}
	}
	if *port != 0 {
		cfg.Port = *port
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(server.Options{Port: cfg.Port})
	if err != nil {
		return err
	}
	return srv.Run(ctx)
}

func usage() {
	fmt.Fprint(os.Stderr, `dnssie - dev-friendly DNS server

usage:
  dnssie          launch the TUI
  dnssie serve    run the DNS server in the foreground
                  [--port N]  override the configured listen port
`)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "dnssie:", err)
	os.Exit(1)
}
