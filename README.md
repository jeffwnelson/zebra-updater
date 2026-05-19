# zebra-updater (`zfw`)

A small Go CLI that upgrades Zebra ZD621 printer firmware over raw TCP port
9100. A cross-platform replacement for the Windows-only ZDownloader.exe.

## Why this exists

Zebra ships firmware as `.zpl` files but only distributes a Windows GUI tool
(ZDownloader.exe) to push them. Under the hood, ZDownloader is just streaming
the firmware file at the printer over a TCP socket. This tool does the same
thing from any platform with a Go toolchain — primary target macOS.

## Install

```
go install github.com/jeffwnelson/zebra-updater@latest
# binary lands in $(go env GOPATH)/bin as `zebra-updater`
# rename or symlink to `zfw` if you prefer
```

Or from source:

```
git clone https://github.com/jeffwnelson/zebra-updater
cd zebra-updater
make build
./zfw --help
```

## Usage

Inspect a printer:

```
zfw info printer.example.com
```

Upgrade firmware (with confirmation prompt):

```
zfw upgrade printer.example.com ./ZD621_V99.21.04Z.zpl
```

Non-interactive:

```
zfw upgrade printer.example.com ./firmware.zpl --yes
```

Pre-flight only (no firmware sent):

```
zfw upgrade printer.example.com ./firmware.zpl --dry-run
```

If the firmware header can't be auto-parsed, supply the targets explicitly:

```
zfw upgrade printer.example.com ./firmware.zpl \
  --target-model ZD621 --target-version V99.21.04Z
```

## Exit codes

| Code | Meaning                                                     |
|------|-------------------------------------------------------------|
| 0    | Success                                                     |
| 2    | Pre-flight failure (no bytes sent)                          |
| 3    | Transfer failure (firmware partially or fully sent, unsafe) |
| 4    | Reboot timeout (firmware sent, printer not back in time)    |

## Safety

- Pre-flight refuses model mismatch, version match (without `--force`), low
  flash, and busy printers.
- On transfer failure, the tool does **not** retry. Run `zfw info` to inspect
  the printer's state before attempting another upgrade.
- Only tested against ZD621. Other Link-OS printers may work but are not
  validated.

## Design and plan

See `docs/superpowers/specs/` and `docs/superpowers/plans/`.
