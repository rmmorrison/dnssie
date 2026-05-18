# dnssie

A developer-friendly DNS server with a terminal UI for straightforward management.

## Why not just edit /etc/hosts?

* It serves real record types (`CNAME`, `MX`, `TXT`, `SOA`, `NS`, `PTR`), not just name-to-IP mappings.
* It supports wildcards, so `*.app.test.` resolves without listing every subdomain.
* Names you haven't defined are forwarded to your normal resolvers instead of failing.
* No root needed: records live in your own config directory, not a system-wide file.
* Per-record TTLs and an opt-in "erratic mode" let you test client caching and failure handling.
* It's a real DNS server, so tools and libraries that ignore `/etc/hosts` still see it.
* A terminal UI manages records and shows lookups live, instead of hand-editing a file.

## What this is not

This isn't [dnsmasq](https://thekelleys.org.uk/dnsmasq/doc.html) or [Unbound](https://nlnetlabs.nl/projects/unbound/about/) and it's _definitely_ not your new production DNS server.

## Quick Start

Prebuilt binaries for Linux, macOS, and Windows are on the
[Releases page](https://github.com/rmmorrison/dnssie/releases).

1. Download the archive for your platform and extract it.
2. Move the `dnssie` binary somewhere on your `PATH`.
3. Run `dnssie`.

```sh
# Example: macOS on Apple silicon
tar -xzf dnssie_0.1.0_darwin_arm64.tar.gz
sudo mv dnssie /usr/local/bin/
dnssie
```

(Windows archives are `.zip`; run `dnssie.exe`.)

In the UI, choose **Create a new record** to add e.g. `app.test.` → `127.0.0.1`,
then open **DNS server** and press `s` to start it. Query it from another
terminal:

```sh
dig @127.0.0.1 -p 1053 app.test
```

The server keeps running after you quit the UI — it listens on `127.0.0.1:1053`
by default. Relaunch `dnssie` to see its status and recent lookups, or to stop
it.

## Build from source

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

## License

MIT — see [LICENSE](LICENSE).
