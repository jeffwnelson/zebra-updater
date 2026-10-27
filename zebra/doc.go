// Package zebra talks to Zebra Link-OS printers over raw TCP (port 9100) and
// upgrades their firmware.
//
// Firmware is delivered the same way Zebra's ZDownloader does it: the .zpl
// image is streamed byte-for-byte to the printer, which validates the signed
// payload, flashes it, and reboots. This package adds what ZDownloader lacks:
// pre-flight checks over SGD (Set/Get/Do) queries, a stall timeout on the
// transfer, and confirmation that the printer came back on the new version.
//
// Typical use:
//
//	fw, err := zebra.OpenFirmware("V93.21.53Z.zpl")
//	...
//	defer fw.Close()
//	p := zebra.New("192.0.2.10")
//	report, err := p.Check(ctx, fw)
//	...
//	err = p.Upgrade(ctx, fw, zebra.UpgradeOptions{Progress: func(pr zebra.Progress) { ... }})
//
// Compatibility is decided by OS branch: a firmware image declares a version
// such as V93.21.53Z, and the leading "V93" must match the branch of the
// version the printer is running. One branch covers a model family (V93 is
// the ZD411, ZD421, ZD611, and ZD621), so the image itself names no model.
package zebra
