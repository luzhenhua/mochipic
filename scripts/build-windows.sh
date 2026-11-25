#!/bin/bash
# Cross-compile MochiPic for Windows x64 from macOS.
set -euo pipefail
cd "$(dirname "$0")/.."
sdk="${MOCHIPIC_WINDOWS_SDK:-$PWD/.deps/windows-cross}"
vips="$sdk/vips-dev-8.18"
export GOOS=windows GOARCH=amd64 CGO_ENABLED=1
export CC="$sdk/llvm-mingw-20261117-ucrt-macos-universal/bin/x86_64-w64-mingw32-clang"
export PKG_CONFIG="$PWD/scripts/windows-pkg-config.sh"
export PKG_CONFIG_LIBDIR="$sdk/codecs/heif-install/lib/pkgconfig:$sdk/codecs/de265-install/lib/pkgconfig:$vips/lib/pkgconfig" PKG_CONFIG_PATH=
for dependency in "$CC" "$vips/include/webp/mux.h" "$vips/include/tiffio.h" "$sdk/codecs/heif-install/include/libheif/heif_sequences.h"; do
  if [[ ! -f "$dependency" ]]; then
    printf 'Missing Windows SDK component: %s\n' "$dependency" >&2
    exit 1
  fi
done
# LLVM's MinGW driver expects GNU-style names for the SDK's COFF import libraries.
for library in "$vips/lib"/*.lib; do
  alias_file="${library%.lib}.dll.a"
  if [[ ! -e "$alias_file" ]]; then ln -s "$(basename "$library")" "$alias_file"; fi
done
exec go "$@"
