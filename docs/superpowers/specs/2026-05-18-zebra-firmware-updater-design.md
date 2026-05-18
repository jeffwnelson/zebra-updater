# Zebra Firmware Updater (`zfw`) — Design

**Date:** 2026-05-18
**Repo:** github.com/jeffwnelson/zebra-updater
**Binary:** `zfw` (short, easy to type; rename later if desired)

## Background and motivation

Zebra distributes printer firmware as `.zpl` files and provides `ZDownloader.exe`,
a Windows-only utility, to push them. The user has two networked ZD621 desktop
printers (`printer.example.com`, `printer-2.example.com`) and no Windows machine.

ZDownloader is fundamentally a transport: it opens a channel to the printer and
streams the firmware file. Link-OS printers expose several channels (raw TCP on
port 9100, FTP, embedded web UI, SFTP). For a networked ZD621, raw TCP to port
9100 is the simplest and most reliable mechanism — no auth, no HTML scraping, and
the same socket handles both firmware delivery and pre/post diagnostic queries
via SGD (Set/Get/Do) commands.

This document specifies a small Go CLI that performs the upgrade end-to-end with
pre-flight safety checks and post-reboot verification.

## Goals

- One-shot firmware upgrade of a single ZD621 by hostname.
- Refuse to send firmware unless basic safety checks pass (model match, free
  flash, not busy, version differs from current).
- Confirm the printer comes back on the expected firmware version.
- Cross-platform Go binary, primary target macOS.

## Non-goals (v1)

- Fleet upgrade (parallel or sequential across multiple printers). Run the
  command twice for two printers.
- Automatic rollback between firmware slots.
- USB or Bluetooth transports.
- Firmware signature verification on the client side. The printer enforces
  whatever it enforces; we surface failures via post-reboot version check.
- A config file. Hostnames are positional arguments.

## Architecture

Single Go module, three packages.

```
zebra-updater/
├── main.go                # thin: cmd.Execute()
├── cmd/                   # Cobra CLI wiring
│   ├── root.go
│   ├── info.go
│   └── upgrade.go
└── internal/
    ├── printer/           # TCP + SGD protocol
    │   ├── printer.go
    │   ├── sgd.go
    │   └── printer_test.go
    └── firmware/          # .zpl file inspection
        ├── firmware.go
        ├── firmware_test.go
        └── testdata/
            └── zd621-sample-header.bin
```

- **`internal/printer`** — `Printer{Host string}` with `Connect()`, `GetVar(name)
  (string, error)`, `Send(io.Reader) error`, `Ping() error`. SGD framing
  (`! U1 getvar "<name>"\r\n` → quoted-string response) lives in `sgd.go`.
- **`internal/firmware`** — `File` type that reads a `.zpl` firmware file, parses
  the header to expose `Model()`, `Version()`, `Size()`. Used by pre-flight.
- **`cmd`** — Cobra commands. Wires flags, prints output, sets exit codes.

Only third-party runtime dependency is Cobra. Everything else is stdlib.

## CLI surface

```
zfw info <host>
zfw upgrade <host> <firmware.zpl>
zfw upgrade <host> <firmware.zpl> --dry-run    # pre-flight only
zfw upgrade <host> <firmware.zpl> --yes        # skip confirmation
zfw upgrade <host> <firmware.zpl> --force      # upgrade even if versions match
```

Global flags: `--port` (default 9100), `--connect-timeout` (5s),
`--reboot-timeout` (5m).

Exit codes:

| Code | Meaning                                                     |
|------|-------------------------------------------------------------|
| 0    | Success                                                     |
| 2    | Pre-flight failure (no bytes sent)                          |
| 3    | Transfer failure (firmware partially or fully sent, unsafe) |
| 4    | Reboot timeout (firmware sent, printer not back in time)    |

## Pre-flight checks (`upgrade`)

Performed in order; first failure aborts before any firmware bytes leave the
host.

1. **TCP connect** to `host:port` within `--connect-timeout`.
2. **Model match** — `! U1 getvar "device.product.name"` must contain the model
   declared in the firmware file's header.
3. **Version compare** — `! U1 getvar "appl.name"` (e.g. `V95.21.04Z`) vs.
   firmware file's declared version. If equal, exit 0 with "already on this
   version" unless `--force`.
4. **Free flash** — `! U1 getvar "memory.flash_free"` must be ≥ firmware size
   × 1.2.
5. **Not busy** — `! U1 getvar "device.host_status"` must not indicate an
   active print job.

`zfw info` runs queries 1–5 and prints the values without any upgrade action,
giving the user a sanity-check command.

## Upgrade flow

1. Run pre-flight checks above.
2. Confirmation prompt (skipped with `--yes`): show `before → after` version and
   ask `y/N`.
3. Open TCP to `host:port`. Stream firmware file straight through, printing a
   byte-progress line on a ticker (e.g. `1.2 / 4.8 MB`).
4. Close socket cleanly.
5. **Reboot detection loop** — every 2s attempt a TCP connect. Track phase:
   - *Still up*: keep polling.
   - *Down* (refused/timeout): flashing in progress.
   - *Back up* after observing at least one *down* sample: rebooted.
   The "at least one down sample" rule prevents declaring success during the
   brief window between socket close and flash start.
6. After back-up, issue one `appl.name` query and confirm it matches the
   firmware file's declared version. Print `before → after`.
7. Total wall time for steps 4–6 bounded by `--reboot-timeout`.

## Error handling

- **Pre-flight failures** (exit 2): print one-line reason and the observed SGD
  value (e.g. `pre-flight: model mismatch — file targets ZD621, printer reports
  ZD420`). No firmware sent.
- **Transfer failures** (exit 3): TCP write error mid-stream or socket dies
  before EOF. Do **not** retry automatically. Print explicit warning that the
  printer may be in a partially-flashed state and instruct user to run `zfw
  info` to inspect before deciding next steps.
- **Reboot timeout** (exit 4): firmware sent, no return within
  `--reboot-timeout`. Print instruction to power-cycle and run `zfw info`
  manually. Not necessarily a fatal failure — large firmware bumps can take
  longer than expected.

## Testing strategy

1. **SGD protocol unit tests** — `internal/printer` tested against an in-process
   fake TCP server bound to `127.0.0.1:0` that speaks the SGD wire format.
   Coverage: well-formed responses, quoted strings with escapes, partial reads
   across packet boundaries, timeouts, mid-response disconnects.
2. **Firmware header parser** — `internal/firmware` tested against a small
   corpus of real ZD621 header bytes (first few KB only) in `testdata/`. If real
   samples aren't available, fall back to crafted fixtures matching the
   documented `! CISDFCRC16` download header format.
3. **End-to-end** — `make e2e` runs `zfw info $ZFW_E2E_HOST` against a real
   printer, gated by env var so CI doesn't run it. No e2e test that actually
   flashes firmware — repeated flashing wears flash memory.

## Repository layout

```
zebra-updater/
├── README.md
├── go.mod                 # module github.com/jeffwnelson/zebra-updater
├── main.go
├── cmd/
│   ├── root.go
│   ├── info.go
│   └── upgrade.go
├── internal/
│   ├── printer/
│   │   ├── printer.go
│   │   ├── sgd.go
│   │   └── printer_test.go
│   └── firmware/
│       ├── firmware.go
│       ├── firmware_test.go
│       └── testdata/
│           └── zd621-sample-header.bin
├── Makefile               # build, test, e2e
├── docs/
│   └── superpowers/
│       └── specs/
│           └── 2026-05-18-zebra-firmware-updater-design.md
└── .github/workflows/
    └── ci.yml             # go test, go vet, staticcheck
```

## Open questions / things to verify during implementation

- Exact byte layout of the ZD621 firmware header — confirm `CISDFCRC16` or
  similar marker and the model/version fields. May require inspecting an actual
  firmware file before writing the parser.
- Whether `appl.name` returns a value comparable byte-for-byte to the version
  string embedded in the firmware file, or whether normalization is needed.
- Behavior of `device.host_status` SGD variable on ZD621 specifically — confirm
  the documented response format matches reality.
- Whether the support notice at https://support.zebra.com/article/000035294
  introduces any constraint (e.g. signed firmware enforced on raw 9100) that
  changes this design. User to paste contents if so.
