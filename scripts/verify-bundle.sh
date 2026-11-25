#!/bin/bash
# Relocate the bundle and verify its loader without project environment variables.
set -euo pipefail
cd "$(dirname "$0")/.."
app="$PWD/dist/MochiPic.app"
mkdir -p .scratch
dir="$(mktemp -d "$PWD/.scratch/bundle-verification.XXXXXX")"
relocated="$dir/Relocated MochiPic.app"
ditto "$app" "$relocated"
codesign --verify --deep --strict "$relocated"
plutil -lint "$relocated/Contents/Info.plist"
otool -L "$relocated/Contents/MacOS/MochiPic" > "$dir/otool.txt"
for locale in zh-Hans en ja es fr pt-BR; do
  label="$(plutil -extract CFBundleDisplayName raw "$relocated/Contents/Resources/$locale.lproj/InfoPlist.strings")"
  if [[ "$locale" == zh-Hans ]]; then
    test "$label" = '麻薯图图'
  else
    test "$label" = MochiPic
  fi
done
test -s "$relocated/Contents/Resources/MochiPic.icns"
testdata="$PWD/internal/imaging/testdata"
samples=("$testdata/apple.heic" "$testdata/still.jxl" "$testdata/still.jp2" "$testdata/still.j2k" "$testdata/mark.svg" "$testdata/merged.psd")
samples+=("$testdata/timed.gif" "$testdata/poster.apng" "$testdata/timed.jxl" "$testdata/timed.avis" "$testdata/timed.heics" "$testdata/different-pages.tiff")
if [[ -n "${IMAGEFLOW_VERIFY_RAW:-}" ]]; then
  samples+=("$IMAGEFLOW_VERIFY_RAW")
fi
env -i PATH=/usr/bin:/bin IMAGEFLOW_CONFIG_DIR="$dir/preferences" DYLD_PRINT_LIBRARIES=1 \
  "$relocated/Contents/MacOS/MochiPic" --verify-window "${samples[@]}" > "$dir/launch.log" 2>&1
python3 - "$dir/launch.log" "${IMAGEFLOW_VERIFY_RAW:-}" <<'PY'
from pathlib import Path
import sys
log = Path(sys.argv[1]).read_text()
for line in log.splitlines():
    if "native frame rendered" in line or " verified:" in line or ("libvips " in line and "→" in line):
        print(line)
assert "native frame rendered with Lobe MyGo" in log
formats = ["HEIF", "JPEG XL", "JPEG 2000", "SVG", "PSD"] + (["RAW"] if sys.argv[2] else [])
for kind in formats:
    assert f"{kind} → WebP" in log and f"{kind} → AVIF" in log, kind
for name in ("timed.gif", "poster.apng", "timed.jxl", "timed.avis", "timed.heics"):
    assert f"series verified: {name} → WebP (2 frames)" in log, name
assert "series verified: different-pages.tiff → TIFF (2 pages)" in log
assert "crop verified:" in log and "collage verified: 2 stills → 159 × 101 PNG" in log
libs = {line.split("Contents/Frameworks/")[-1].split()[0] for line in log.splitlines()
        if "Relocated MochiPic.app/Contents/Frameworks/" in line}
for stem in ("libvips", "libwebp", "libwebpmux", "libpango", "libjpeg", "libpng", "libtiff", "libheif", "libaom", "libde265", "libjxl", "libopenjp2", "librsvg", "libxml2"):
    assert any(lib.startswith(stem) for lib in libs), stem
assert "/opt/homebrew/" not in log and "/.deps/" not in log
print(f"dyld loaded {len(libs)} packaged libraries from Contents/Frameworks")
PY
printf 'Logs: %s\n' "$dir"
