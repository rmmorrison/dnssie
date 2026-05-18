# dnssie

A dev-friendly DNS server with a terminal UI.

`dnssie` lets you define your own DNS records (point a hostname at an IP,
override a domain locally, mock an MX, etc.) and serves them from a small
local DNS server. Anything you haven't defined is forwarded to your normal
resolvers, so it stays out of the way.

## What this is not

`dnssie` is a local development and testing tool. It is not your new
production DNS server, not an authoritative nameserver for real zones, and
not built to be exposed to a network. Use it on `localhost` while you work.

## Requirements

`dnssie` ships as a single self-contained binary — there's nothing to install
alongside it.

- A 64-bit Linux, macOS, or Windows machine (amd64 or arm64).
- Go 1.26 or newer **only if** you build from source.

Windows is supported on a best-effort basis.

## Install

### Download a release (recommended)

Download the archive for your platform from the
[Releases page](https://github.com/rmmorrison/dnssie/releases), extract it, and
move the `dnssie` binary somewhere on your `PATH`. No Go toolchain required.

```sh
# Example: macOS on Apple silicon
tar -xzf dnssie_0.1.0_darwin_arm64.tar.gz
sudo mv dnssie /usr/local/bin/
dnssie version
```

Linux/macOS archives are `.tar.gz`; Windows archives are `.zip` (extract and
run `dnssie.exe`). Every release also publishes `checksums.txt` so you can
verify your download.

The macOS binary isn't notarized, so the first launch may be blocked by
Gatekeeper. Clear the quarantine flag with
`xattr -d com.apple.quarantine ./dnssie`, or right-click the binary and choose
**Open**.

### Build from source

Requires Go 1.26+:

```sh
go install github.com/rmmorrison/dnssie/cmd/dnssie@latest
```

Or from a clone:

```sh
git clone https://github.com/rmmorrison/dnssie
cd dnssie
go build -o dnssie ./cmd/dnssie
```

## Quick start

1. Launch the terminal UI:

   ```sh
   dnssie
   ```

2. Choose **Create a new record**, pick a type, and enter the fully-qualified
   name (e.g. `app.test.`) and value (e.g. `127.0.0.1`).

3. Open **DNS server** and press `s` to start the server. It listens on
   `127.0.0.1:1053` by default.

4. Query it from another terminal:

   ```sh
   dig @127.0.0.1 -p 1053 app.test
   ```

   Names you haven't defined are forwarded upstream:

   ```sh
   dig @127.0.0.1 -p 1053 example.com
   ```

The server runs independently of the UI, so it keeps serving after you quit.
The next time you launch `dnssie`, the **DNS server** screen shows whether it
is running, streams recent lookups as they happen, and lets you stop it.

## Supported record types

`A`, `AAAA`, `CNAME`, `PTR`, `NS`, `MX`, `SOA`, `TXT`.

Names can be wildcards: a record named `*.app.test.` answers any name under
`app.test.` (e.g. `api.app.test.`, `a.b.app.test.`). An exact record always
wins over a wildcard, a more specific wildcard wins over a broader one, and a
wildcard never answers its own parent name (`app.test.` here) — add an
explicit record for that.

Each record has its own TTL (seconds). Leave it blank when creating or
editing a record to use the default (300); set it explicitly — including `0`
— to test client/resolver caching behavior. Records created by older versions
have no stored TTL and keep using the default.

## Configuration

`dnssie` manages everything through the UI; you don't need to edit files by
hand. For reference, records and settings are stored as TOML in:

- macOS / Linux: `~/.config/dnssie/` (honors `$XDG_CONFIG_HOME`)
- Windows: `%AppData%\dnssie\`

Set `DNSSIE_CONFIG_DIR` to override this with an exact directory of your
choice (takes precedence on every platform).

From the **DNS server** screen you can change the listen port and choose how
unmatched queries are handled: forwarded to your system resolvers, forwarded
to a manual list of upstreams, or not forwarded at all (anything without a
local record returns `NXDOMAIN`).

## Notes

- The default port is `1053` so it runs without root/admin. You can set it to
  the standard DNS port `53` in the UI, but binding `53` requires elevated
  privileges.
- The server only listens on `127.0.0.1` (localhost).
- Record changes take effect immediately on a running server. Changing the
  listen port requires a restart, which the UI will prompt you to do.

## Running the server without the UI

The UI starts the server for you, but you can also run it directly:

```sh
dnssie serve            # uses your saved configuration
dnssie serve --port 53  # override the listen port
```

## License

MIT — see [LICENSE](LICENSE).
