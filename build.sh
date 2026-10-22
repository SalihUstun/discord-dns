#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
APP_NAME="Discord DNS"

{
  echo "discord-dns aşağıdaki açık kaynak yazılımları içerir."
  echo
  echo "================================================================"
  echo "Go (https://go.dev)"
  echo "================================================================"
  GOROOT_DIR="$(go env GOROOT)"
  cat "$GOROOT_DIR/LICENSE" 2>/dev/null || cat "$GOROOT_DIR/../LICENSE"
  for target in darwin windows; do
    GOOS=$target go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}{{end}}' .
  done | sort -u | while IFS='|' read -r path dir; do
    echo
    echo "================================================================"
    echo "$path"
    echo "================================================================"
    cat "$dir/LICENSE"
  done
} > THIRD_PARTY_LICENSES.txt

rm -rf dist
mkdir -p dist
FLAGS=(-trimpath -ldflags "-s -w -X main.version=$VERSION")

echo "==> macOS (Apple Silicon + Intel) $VERSION"
GOOS=darwin GOARCH=arm64 go build "${FLAGS[@]}" -o dist/discord-dns-mac-arm64 .
GOOS=darwin GOARCH=amd64 go build "${FLAGS[@]}" -o dist/discord-dns-mac-amd64 .
lipo -create -output dist/discord-dns-mac dist/discord-dns-mac-arm64 dist/discord-dns-mac-amd64
rm dist/discord-dns-mac-arm64 dist/discord-dns-mac-amd64

APP="dist/$APP_NAME.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
mv dist/discord-dns-mac "$APP/Contents/MacOS/discord-dns"
cp LICENSE THIRD_PARTY_LICENSES.txt "$APP/Contents/Resources/"
ICONSET="dist/AppIcon.iconset"
mkdir -p "$ICONSET"
for s in 16 32 128 256 512; do
  sips -z $s $s assets/icon.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  sips -z $((s * 2)) $((s * 2)) assets/icon.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
rm -rf "$ICONSET"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$APP_NAME</string>
  <key>CFBundleDisplayName</key><string>$APP_NAME</string>
  <key>CFBundleIdentifier</key><string>com.salihustun.discord-dns</string>
  <key>CFBundleExecutable</key><string>discord-dns</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundleShortVersionString</key><string>${VERSION#v}</string>
  <key>CFBundleVersion</key><string>${VERSION#v}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHumanReadableCopyright</key><string>Copyright (c) 2026 SalihUstun. MIT License.</string>
</dict>
</plist>
PLIST
xattr -cr "$APP"
codesign --force --deep --sign - "$APP"
(cd dist && ditto -c -k --norsrc --noextattr --noacl --keepParent "$APP_NAME.app" discord-dns-mac.zip)

echo "==> Windows (x64 + ARM) $VERSION"
WINVER="$(echo "${VERSION#v}" | grep -oE '^[0-9]+(\.[0-9]+){0,3}' || echo 0.0.0)"
go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --out rsrc --manifest cli \
  --icon assets/icon.png --product-name "$APP_NAME" --file-description "$APP_NAME" \
  --product-version "$WINVER" --file-version "$WINVER" --original-filename discord-dns.exe \
  --copyright "Copyright (c) 2026 SalihUstun. MIT License."
trap 'rm -f rsrc_windows_*.syso' EXIT
GOOS=windows GOARCH=amd64 go build "${FLAGS[@]}" -o dist/discord-dns-windows-x64.exe .
GOOS=windows GOARCH=arm64 go build "${FLAGS[@]}" -o dist/discord-dns-windows-arm64.exe .

cp LICENSE dist/LICENSE.txt
cp THIRD_PARTY_LICENSES.txt dist/
(cd dist && shasum -a 256 discord-dns-mac.zip discord-dns-windows-*.exe > SHA256SUMS.txt)
ls -lh dist
