#!/bin/bash
# Verify real sample codecs from the pinned upstream tests and local encoders.
set -euo pipefail
cd "$(dirname "$0")/.."
command -v ffmpeg >/dev/null || { echo 'This optional QA run needs FFmpeg with the SVT-AV1 encoder.' >&2; exit 1; }
mkdir -p .scratch
run="$(mktemp -d "$PWD/.scratch/format-corpus.XXXXXX")"
govips="$(go list -m -f '{{.Dir}}' github.com/davidbyttow/govips/v2)/resources"
vips=".deps/vips-8.18.7/test/test-suite/images"
cp "$govips/heic-24bit-exif.heic" "$run/photo.heic"
cp "$govips/heic-24bit.heic" "$run/photo-alt.heif"
cp "$govips/tif-16bit.tif" "$run/photo-16bit.tif"
cp "$govips/avif-8bit.avif" "$run/photo.avif"
cp "$govips/bmp+alpha.bmp" "$run/alpha.bmp"
cp "$govips/koala-rle.bmp" "$run/rle.bmp"
cp "$govips/gif-animated.gif" "$run/animated.gif"
cp "$vips/avif-orientation-6.avif" "$run/oriented.avif"
cp "$vips/favicon.ico" "$run/favicon.ico"
sips -z 6000 8000 -s format heic -s formatOptions 87 internal/imaging/testdata/raster.bmp --out "$run/48mp.heic" >/dev/null
ffmpeg -hide_banner -loglevel error -nostdin -n -f lavfi -i 'testsrc2=size=96x96:rate=4' \
  -frames:v 3 -c:v libsvtav1 -preset 11 -threads 1 -f avif "$run/animated.avif" 2>"$run/ffmpeg-avif.log"
ffmpeg -hide_banner -loglevel error -nostdin -n -f lavfi -i 'testsrc2=size=96x96:rate=4' \
  -frames:v 3 -c:v apng -f apng "$run/animated.png"
scripts/go.sh test -c -o "$run/imaging.test" ./internal/imaging
IMAGEFLOW_FORMAT_FIXTURES="$run" IMAGEFLOW_ADVANCED_FIXTURES="$govips" FONTCONFIG_FILE="$PWD/scripts/fonts.conf" \
  /usr/bin/time -l "$run/imaging.test" -test.run '^(TestExternalFormatCorpus|TestUpstreamAdvancedSamples)$' -test.count=1 -test.v
printf 'Local QA samples: %s\n' "$run"
