# zebra-updater (`zfw`)

A Go package and CLI that upgrade Zebra Link-OS printer firmware over raw TCP
port 9100. A cross-platform replacement for the Windows-only ZDownloader.exe.

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

## Go package

The CLI is a thin wrapper over `github.com/jeffwnelson/zebra-updater/zebra`:

```go
fw, err := zebra.OpenFirmware("V93.21.53Z.zpl")
if err != nil { ... }
defer fw.Close()

p := zebra.New("192.0.2.10")
report, err := p.Check(ctx, fw)          // pre-flight: report.Problems, report.Warnings
err = p.Upgrade(ctx, fw, zebra.UpgradeOptions{
    Progress: func(pr zebra.Progress) { ... },
})
```

`Upgrade` re-runs the pre-flight check, streams the image, then waits for the
printer to reboot and report the new version. Failures after the transfer
starts are typed: `*TransferError`, `*RebootError`, `*VersionMismatchError`.
Every call takes a `context.Context`.

## Usage

Inspect a printer:

```
zfw info 192.0.2.10
```

Pre-flight only (no firmware sent):

```
zfw upgrade 192.0.2.10 ./V93.21.53Z.zpl --dry-run
```

Upgrade (with confirmation prompt; `--yes` to skip it):

```
zfw upgrade 192.0.2.10 ./V93.21.53Z.zpl
```

Use the printer's IP address, not its hostname, when upgrading from Link-OS
older than 7.6: the first boot on 7.6+ migrates the printer's network
hostname, so a DNS name may stop resolving mid-upgrade.

## Exit codes

| Code | Meaning                                                          |
|------|------------------------------------------------------------------|
| 0    | Success, or already on the target version                        |
| 1    | Usage error, aborted at the prompt, or printer unreachable       |
| 2    | Pre-flight failure (no bytes sent)                               |
| 3    | Transfer failure (firmware partially sent; printer may be stuck) |
| 4    | Firmware sent, but the printer did not come back on the new version |

## How it works

Firmware images start with a plain-text header:

```
! PROGRAM
V93.21.53Z
```

The version's leading component is the OS branch. One branch covers a model
family (V93: ZD411, ZD421, ZD611, ZD621), so compatibility is checked by
branch, not model name. The image is streamed byte-for-byte to port 9100, the
same way ZDownloader does it; the printer validates the signed payload,
flashes, and reboots.

Pre-flight refuses to send when:

- the image's branch differs from the printer's running branch
- the printer runs non-standard firmware (e.g. a Zebra special build such as
  `SP93-006444A`), which a standard image would replace
- `device.allow_firmware_downloads` is `no`
- the printer has formats queued or labels remaining (`device.host_status`)
- the printer already runs that version (unless `--force`)

It warns, without refusing, on downgrades, a paused printer, an open
printhead, and the hostname migration above.

## Safety

- On transfer failure the tool does **not** retry. Power-cycle the printer
  and run `zfw info` before attempting another upgrade.
- A write that blocks for `--write-timeout` (default 2m) abandons the
  transfer.
- Tested against ZD621 and ZD611 pre-flight; no full flash has been validated
  yet.

## Design and plan

See `docs/superpowers/specs/` and `docs/superpowers/plans/`.
