# zebra-updater (`zfw`)

A cross-platform CLI that upgrades Zebra ZD621 printer firmware over raw TCP
port 9100. Replacement for the Windows-only ZDownloader.exe.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for
the implementation plan.

## Usage (planned)

```
zfw info <hostname>
zfw upgrade <hostname> <firmware.zpl>
```

## Build

```
make build
```
