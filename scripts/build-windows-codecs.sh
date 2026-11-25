#!/bin/bash
# Build just the HEIC decoder and AV1 support needed by the Windows package.
set -euo pipefail
cd "$(dirname "$0")/.."
root="$PWD"
sdk="${MOCHIPIC_WINDOWS_SDK:-$root/.deps/windows-cross}"
codec="$sdk/codecs"
llvm="$sdk/llvm-mingw-20261117-ucrt-macos-universal/bin"
base="$sdk/vips-dev-8.18"
ninja="${MOCHIPIC_NINJA:-$root/.deps/native/bin/ninja}"
aom_headers="${MOCHIPIC_AOM_HEADERS:-$root/.deps/native/include}"
if [[ ! -f "$aom_headers/aom/aom_encoder.h" || ! -x "$ninja" ]]; then
  printf 'Set MOCHIPIC_AOM_HEADERS to an AOM 3.15 include directory and MOCHIPIC_NINJA to a local Ninja executable.\n' >&2
  exit 1
fi
mkdir -p "$codec"
cd "$codec"
fetch() {
  local name=$1 digest=$2 url=$3
  if [[ ! -f "$name.tar.gz" ]]; then
    curl -fLsS --retry 2 --proto '=https' --proto-redir '=https' -o "$name.tar.gz.part" "$url"
    printf '%s  %s\n' "$digest" "$name.tar.gz.part" | shasum -a 256 -c -
    mv "$name.tar.gz.part" "$name.tar.gz"
  fi
  printf '%s  %s\n' "$digest" "$name.tar.gz" | shasum -a 256 -c -
  [[ -d "$name" ]] || tar -xf "$name.tar.gz"
}
fetch libheif-1.23.7 f7483e818503ea6224c8863e4a56ac02c7089021a98472c811c12aa19faf2475 \
  https://github.com/strukturag/libheif/releases/download/v1.23.7/libheif-1.23.7.tar.gz
fetch libde265-1.1.3 554228bd17788c99a7e63b37ab5634722190e6e2bf60c1dcb01cef328e133905 \
  https://github.com/strukturag/libde265/releases/download/v1.1.3/libde265-1.1.3.tar.gz

cat > llvm-windows.cmake <<MAKE
set(CMAKE_SYSTEM_NAME Windows)
set(CMAKE_SYSTEM_PROCESSOR x86_64)
set(CMAKE_C_COMPILER "$llvm/x86_64-w64-mingw32-clang")
set(CMAKE_CXX_COMPILER "$llvm/x86_64-w64-mingw32-clang++")
set(CMAKE_RC_COMPILER "$llvm/x86_64-w64-mingw32-windres")
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)
MAKE
cmake=(uv run --no-project --with cmake==4.4.5 cmake)
common=(-G Ninja "-DCMAKE_MAKE_PROGRAM=$ninja" "-DCMAKE_TOOLCHAIN_FILE=$codec/llvm-windows.cmake" -DCMAKE_BUILD_TYPE=Release)
"${cmake[@]}" -S libde265-1.1.3 -B de265-build "${common[@]}" "-DCMAKE_INSTALL_PREFIX=$codec/de265-install" \
  -DENABLE_SDL=OFF -DENABLE_DECODER=OFF -DENABLE_AVX512=OFF
"${cmake[@]}" --build de265-build --target install -j 4
ln -sf libaom.lib "$base/lib/libaom.dll.a"
"${cmake[@]}" -S libheif-1.23.7 -B heif-build "${common[@]}" "-DCMAKE_INSTALL_PREFIX=$codec/heif-install" \
  "-DCMAKE_FIND_ROOT_PATH=$codec/de265-install" "-DAOM_INCLUDE_DIR=$aom_headers" "-DAOM_LIBRARY=$base/lib/libaom.dll.a" \
  "-DLIBDE265_INCLUDE_DIR=$codec/de265-install/include" "-DLIBDE265_LIBRARY=$codec/de265-install/lib/libde265.dll.a" \
  -DWITH_X265=OFF -DWITH_LIBDE265=ON -DWITH_AOM_ENCODER=ON -DWITH_AOM_DECODER=ON -DWITH_EXAMPLES=OFF \
  -DBUILD_TESTING=OFF -DWITH_GDK_PIXBUF=OFF -DENABLE_PLUGIN_LOADING=OFF -DWITH_JPEG_ENCODER=OFF \
  -DWITH_JPEG_DECODER=OFF -DWITH_OpenJPEG_ENCODER=OFF -DWITH_OpenJPEG_DECODER=OFF
"${cmake[@]}" --build heif-build --target install -j 5
