#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .scratch
run="$(mktemp -d "$PWD/.scratch/visual-qa.XXXXXX")"
target="${1:-$run/screenshots}"
mkdir -p "$target" "$run/images/parts" "$run/images/machines" "$run/batch" "$run/export" "$run/cancel" "$run/config" "$run/empty" "$run/formats" "$run/format-export/chuli" "$run/extended" "$run/extended-export" "$run/series" "$run/series-export" "$run/compose" "$run/compose-export"
mkdir -p "$run/certification" "$run/certification-export"
mkdir -p "$run/logos"
vips="$PWD/.deps/vips/bin/vips"
export DYLD_LIBRARY_PATH="$PWD/.deps/vips/lib:$PWD/.deps/native/lib"
# Use the credited welcome photos when no local fixtures are supplied.
assets="${IMAGEFLOW_QA_ASSETS:-}"
samples=(internal/ui/assets/welcome/{front,right,left}.jpg)
items=(pump battery machine valve motor gearbox flange cylinder gauge drive impeller filter coupling)
for i in "${!items[@]}"; do
  item="${items[$i]}"
  name="$(printf '%02d-%s.jpg' "$((i+1))" "$item")"
  dir="$run/images"
  if [[ "$i" -gt 8 ]]; then
    dir="$dir/parts"
  elif [[ "$i" -gt 5 ]]; then
    dir="$dir/machines"
  fi
  if [[ -n "$assets" && -f "$assets/$item.jpg" ]]; then
    cp "$assets/$item.jpg" "$dir/$name"
  else
    "$vips" thumbnail "${samples[i % ${#samples[@]}]}" "$dir/$name" \
      "$((984 - 23 * i))" --height "$((656 - 11 * i))" --size both --crop centre
  fi
done
"$vips" thumbnail "$run/images/01-pump.jpg" "$run/compose/01-pump.jpg" \
  984 --height 656 --size both --crop centre
"$vips" extract_area "$run/compose/01-pump.jpg" "$run/compose/02-inlet.jpg" 78 219 444 296
"$vips" extract_area "$run/compose/01-pump.jpg" "$run/compose/03-motor.jpg" 450 106 474 316
"$vips" extract_area "$run/compose/01-pump.jpg" "$run/compose/04-outlet.jpg" 128 21 441 294
for logo in brand-logo dealer-logo; do
  "$vips" copy "internal/imaging/testdata/$logo.svg" "$run/logos/$logo.png"
done
if [[ -n "${IMAGEFLOW_QA_CERT_PHOTO:-}" ]]; then
  "$vips" copy "$IMAGEFLOW_QA_CERT_PHOTO" "$run/certification/01-pump.webp"
else
  "$vips" copy "$run/compose/01-pump.jpg" "$run/certification/01-pump.webp"
fi
if [[ -n "${IMAGEFLOW_QA_CAPTION_PHOTO:-}" ]]; then
  "$vips" copy "$IMAGEFLOW_QA_CAPTION_PHOTO" "$run/caption-photo.webp[lossless]"
fi
cp "$run/compose/02-inlet.jpg" "$run/certification/02-inlet.jpg"
cp "$run/compose/03-motor.jpg" "$run/certification/03-motor.jpg"
cp "$run/compose/04-outlet.jpg" "$run/certification/04-outlet.jpg"
cp "$run/images/02-battery.jpg" "$run/certification/05-battery.jpg"
cp "$run/images/03-machine.jpg" "$run/certification/06-machine.jpg"
"$vips" thumbnail "$run/images/01-pump.jpg" "$run/large.jpg" 1800 --size both
sips -s format heic "$run/images/01-pump.jpg" --out "$run/formats/01-pump.heic" >/dev/null
sips -s format tiff "$run/images/01-pump.jpg" --out "$run/formats/02-pump.tiff" >/dev/null
sips -s format bmp "$run/images/01-pump.jpg" --out "$run/formats/03-pump.bmp" >/dev/null
cp internal/imaging/testdata/two-sizes.ico "$run/formats/04-icon.ico"
cp internal/imaging/testdata/still.gif "$run/formats/05-still.gif"
"$vips" heifsave "$run/images/01-pump.jpg" "$run/formats/06-pump.avif" --compression av1 --effort 3
printf 'Invalid TIFF fixture\n' > "$run/formats/07-corrupt.tiff"
printf 'Invalid AVIF fixture\n' > "$run/formats/08-corrupt.avif"
govips="$(go list -m -f '{{.Dir}}' github.com/davidbyttow/govips/v2)/resources"
cp internal/imaging/testdata/mark.svg "$run/extended/01-mark.svg"
"$vips" jxlsave "$run/images/01-pump.jpg" "$run/extended/02-pump.jxl" --effort 3
cp "$govips/jp2k-orientation-6.jp2" "$run/extended/03-oriented.jp2"
cp internal/imaging/testdata/still.j2k "$run/extended/04-codestream.j2k"
cp "$govips/psd.example.psd" "$run/extended/05-layered.psd"
printf 'Invalid JPEG XL fixture\n' > "$run/extended/07-corrupt.jxl"
cp internal/imaging/testdata/timed.gif "$run/series/01-animation.gif"
cp internal/imaging/testdata/different-pages.tiff "$run/series/02-book.tiff"
if [[ -n "${IMAGEFLOW_QA_RAW:-}" ]]; then
  cp "$IMAGEFLOW_QA_RAW" "$run/extended/06-camera.${IMAGEFLOW_QA_RAW##*.}"
fi
python3 - "$run" <<'PY'
import pathlib, struct, sys, zlib
p = pathlib.Path(sys.argv[1])
psd = (p / "extended/05-layered.psd").read_bytes()
offset = 26
for _ in range(3):  # Color mode, resources and layer/mask sections.
    offset += 4 + struct.unpack_from(">I", psd, offset)[0]
(p / "extended/08-no-composite.psd").write_bytes(psd[:offset])
b = (p / "large.jpg").read_bytes()
for i in range(165):
    (p / "batch" / f"fixture-{i+1:04}.jpg").write_bytes(b)
w,h = 220,72
pixels = b"".join(b"\0" + b"".join(
    bytes((32,32,32,255)) if (x-33)**2+(y-36)**2<29**2 and (x-33)**2+(y-36)**2>13**2
      or 70<x<207 and (15<y<27 or 42<y<54 and x<176) else bytes(4)
    for x in range(w)) for y in range(h))
def chunk(tag,data):
    return struct.pack(">I",len(data))+tag+data+struct.pack(">I",zlib.crc32(tag+data))
(p/"logo.png").write_bytes(b"\x89PNG\r\n\x1a\n"+chunk(b"IHDR",struct.pack(">2I5B",w,h,8,6,0,0,0))+
                           chunk(b"IDAT",zlib.compress(pixels))+chunk(b"IEND",b""))
# Headers remain readable so the native result view can exercise pixel-decode
# failures that are only discovered after import.
damaged = (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR",struct.pack(">2I5B",w,h,8,6,0,0,0)) +
           chunk(b"IDAT",b"not a zlib stream") + chunk(b"IEND",b""))
for name in ("bearing", "drive", "gauge", "housing", "motor"):
    (p / "batch" / f"damaged-{name}.png").write_bytes(damaged)
PY
unset DYLD_LIBRARY_PATH
hash_inputs() {
  find "$run/images" "$run/batch" "$run/formats" "$run/extended" "$run/series" "$run/compose" "$run/certification" "$run/logos" -type f -print0 | xargs -0 shasum -a 256
  if [[ -f "$run/caption-photo.webp" ]]; then
    shasum -a 256 "$run/caption-photo.webp"
  fi
}
hash_inputs > "$run/input.sha256"
scripts/go.sh build -tags imageflowqa -o "$run/ImageFlow-QA" ./cmd/imageflow
FONTCONFIG_FILE="$PWD/scripts/fonts.conf" IMAGEFLOW_CONFIG_DIR="$run/config" MYGO_LOCALE=zh-Hans \
  IMAGEFLOW_VISUAL_FIXTURES="$run" IMAGEFLOW_VISUAL_CAPTURE="$target" "$run/ImageFlow-QA"
shasum -c "$run/input.sha256" > "$run/input-check.log"
printf 'Input hashes unchanged: %s\n' "$(wc -l < "$run/input-check.log" | tr -d ' ')"
printf 'Screenshots: %s\n' "$target"
