#!/usr/bin/env bash
# Builds the StartERP Card Bridge setup files into $1 (default ../cardbridge-dist)
# and writes manifest.json, which the API serves at GET /card-bridge/downloads.
#
#   Windows  x64 / ARM64   StartERP-CardBridge-windows-<arch>.exe  (no console window)
#   macOS    Apple / Intel StartERP-CardBridge-macos-<arch>.zip    ("StartERP Card Bridge.app")
#   Ubuntu   x64 / ARM64   starterp-cardbridge_<ver>_<arch>.deb  + .tar.gz with install.sh
#   Raspberry Pi (ARMv7)   StartERP-CardBridge-linux-armv7.tar.gz
#   Android  ARM64         StartERP-CardBridge-android-arm64 (runs in Termux)
#
# Needs Go; dpkg-deb and zip when present (the .deb / .zip are skipped otherwise).
set -euo pipefail
cd "$(dirname "$0")"
OUT="$(realpath -m "${1:-../cardbridge-dist}")"
VERSION="${CARD_BRIDGE_VERSION:-1.0.$(git rev-list --count HEAD 2>/dev/null || echo 0)}"
SERVER="${CARD_BRIDGE_SERVER:-https://startpos-api-v2.gulfunionozone.com/v1/erp}"
rm -rf "$OUT" && mkdir -p "$OUT"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
LD="-s -w -X main.Version=$VERSION -X main.DefaultServer=$SERVER"

build() { # goos goarch out [extra ldflags] [goarm]
  GOOS=$1 GOARCH=$2 GOARM=${5:-} CGO_ENABLED=0 go build -trimpath -ldflags "$LD ${4:-}" -o "$3" .
}

# Windows: a GUI-subsystem exe, so no console window opens
for a in amd64:x64 arm64:arm64; do
  build windows "${a%%:*}" "$OUT/StartERP-CardBridge-windows-${a##*:}.exe" "-H windowsgui"
done

# macOS: an app bundle in a zip
for a in arm64:apple-silicon amd64:intel; do
  app="$WORK/mac-${a##*:}/StartERP Card Bridge.app"
  mkdir -p "$app/Contents/MacOS"
  build darwin "${a%%:*}" "$app/Contents/MacOS/cardbridge"
  cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>StartERP Card Bridge</string>
<key>CFBundleDisplayName</key><string>StartERP Card Bridge</string>
<key>CFBundleIdentifier</key><string>com.starterp.cardbridge</string>
<key>CFBundleVersion</key><string>$VERSION</string>
<key>CFBundleShortVersionString</key><string>$VERSION</string>
<key>CFBundleExecutable</key><string>cardbridge</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>11.0</string>
<key>LSUIElement</key><true/>
</dict></plist>
EOF
  if command -v zip >/dev/null; then
    (cd "$WORK/mac-${a##*:}" && zip -qry "$OUT/StartERP-CardBridge-macos-${a##*:}.zip" "StartERP Card Bridge.app")
  fi
done

# Linux: tarball with an installer, and a .deb for Ubuntu / Debian
for a in amd64:x64:amd64 arm64:arm64:arm64 arm:armv7:armhf; do
  goarch=${a%%:*}; rest=${a#*:}; label=${rest%%:*}; debarch=${rest##*:}
  dir="$WORK/linux-$label/starterp-cardbridge"
  mkdir -p "$dir"
  build linux "$goarch" "$dir/cardbridge" "" "$([ "$goarch" = arm ] && echo 7)"
  cat > "$dir/install.sh" <<'EOF'
#!/bin/sh
# Installs the StartERP Card Bridge for this user and starts it with the computer.
set -e
mkdir -p "$HOME/.local/bin"
cp "$(dirname "$0")/cardbridge" "$HOME/.local/bin/starterp-cardbridge"
chmod +x "$HOME/.local/bin/starterp-cardbridge"
"$HOME/.local/bin/starterp-cardbridge" autostart on || true
(systemctl --user start starterp-cardbridge.service 2>/dev/null || nohup "$HOME/.local/bin/starterp-cardbridge" run >/dev/null 2>&1 &) || true
echo "StartERP Card Bridge installed. Open http://127.0.0.1:17777 to pair it with your store."
(xdg-open http://127.0.0.1:17777 >/dev/null 2>&1 &) || true
EOF
  chmod +x "$dir/install.sh"
  tar -C "$WORK/linux-$label" -czf "$OUT/StartERP-CardBridge-linux-$label.tar.gz" starterp-cardbridge
  if command -v dpkg-deb >/dev/null && [ "$label" != armv7 ]; then
    deb="$WORK/deb-$label"
    mkdir -p "$deb/DEBIAN" "$deb/usr/bin" "$deb/usr/share/applications"
    cp "$dir/cardbridge" "$deb/usr/bin/starterp-cardbridge"
    cat > "$deb/DEBIAN/control" <<EOF
Package: starterp-cardbridge
Version: $VERSION
Architecture: $debarch
Maintainer: StartERP <support@gulfunionozone.com>
Section: misc
Priority: optional
Description: StartERP Card Bridge
 Connects the shop's card machines to StartERP so the tills can send
 amounts to them. Open http://127.0.0.1:17777 after installing to pair it.
EOF
    cat > "$deb/usr/share/applications/starterp-cardbridge.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=StartERP Card Bridge
Comment=Connect card machines to StartERP
Exec=/usr/bin/starterp-cardbridge run
Terminal=false
Categories=Office;Finance;
EOF
    dpkg-deb --root-owner-group --build "$deb" "$OUT/starterp-cardbridge_${VERSION}_${debarch}.deb" >/dev/null
  fi
done

# Android (advanced): a Termux binary
build android arm64 "$OUT/StartERP-CardBridge-android-arm64"

# manifest
python3 - "$OUT" "$VERSION" <<'EOF'
import hashlib, json, os, sys
out, ver = sys.argv[1], sys.argv[2]
kinds = [
    ("windows", "x64", "StartERP-CardBridge-windows-x64.exe", "Windows 10 / 11 (64-bit)", "ويندوز 10 / 11 (64 بت)"),
    ("windows", "arm64", "StartERP-CardBridge-windows-arm64.exe", "Windows on ARM", "ويندوز على معالج ARM"),
    ("macos", "apple-silicon", "StartERP-CardBridge-macos-apple-silicon.zip", "macOS, Apple silicon (M1 and later)", "ماك، معالج Apple (M1 وما بعده)"),
    ("macos", "intel", "StartERP-CardBridge-macos-intel.zip", "macOS, Intel", "ماك، معالج Intel"),
    ("ubuntu", "x64", f"starterp-cardbridge_{ver}_amd64.deb", "Ubuntu / Debian (64-bit, .deb)", "أوبونتو / ديبيان (64 بت، ‎.deb)"),
    ("ubuntu", "arm64", f"starterp-cardbridge_{ver}_arm64.deb", "Ubuntu / Debian on ARM64 (.deb)", "أوبونتو / ديبيان على ARM64 (‎.deb)"),
    ("linux", "x64", "StartERP-CardBridge-linux-x64.tar.gz", "Other Linux (64-bit)", "لينكس آخر (64 بت)"),
    ("linux", "arm64", "StartERP-CardBridge-linux-arm64.tar.gz", "Linux ARM64 (Raspberry Pi 4/5 64-bit)", "لينكس ARM64 (راسبيري باي 4/5)"),
    ("linux", "armv7", "StartERP-CardBridge-linux-armv7.tar.gz", "Linux ARMv7 (Raspberry Pi 32-bit)", "لينكس ARMv7 (راسبيري باي 32 بت)"),
    ("android", "arm64", "StartERP-CardBridge-android-arm64", "Android (advanced, runs in Termux)", "أندرويد (متقدم، يعمل داخل Termux)"),
]
files = []
for os_, arch, name, en, ar in kinds:
    p = os.path.join(out, name)
    if not os.path.exists(p):
        continue
    h = hashlib.sha256(open(p, "rb").read()).hexdigest()
    files.append({"os": os_, "arch": arch, "file": name, "size": os.path.getsize(p), "sha256": h, "labelEn": en, "labelAr": ar})
json.dump({"version": ver, "files": files}, open(os.path.join(out, "manifest.json"), "w"), indent=1)
print(f"Card Bridge {ver}: {len(files)} setup files in {out}")
EOF
