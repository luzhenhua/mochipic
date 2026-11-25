#!/bin/bash
# Project-local Apple Silicon toolchain for libvips. No shell/profile changes.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)/$(uname -m)" == Darwin/arm64 ]] || { echo 'Requires Apple Silicon macOS.' >&2; exit 1; }
xcrun --find clang >/dev/null
deps="$PWD/.deps"
mkdir -p "$deps/bootstrap"
mamba="$deps/bootstrap/bin/micromamba"
native="$deps/native"
vips="$deps/vips"
export MAMBA_ROOT_PREFIX="$deps/mamba"

fetch() {
  local url="$1" file="$2" sha="$3"
  if [[ ! -f "$file" ]]; then
    curl --fail --location --silent --show-error --retry 2 "$url" -o "$file.part"
    mv "$file.part" "$file"
  fi
  printf '%s  %s\n' "$sha" "$file" | shasum -a 256 -c
}

if [[ ! -x "$mamba" ]]; then
  archive="$deps/bootstrap/micromamba.tar.bz2"
  fetch https://conda.anaconda.org/conda-forge/osx-arm64/micromamba-2.9.0-0.tar.bz2 "$archive" \
    500f5074feb8d02c4296ef9921c3650ed2874171805a9fbb8fbb53896433646b
  tar -xjf "$archive" -C "$deps/bootstrap" bin/micromamba
fi
lock=scripts/native-osx-arm64.lock
lock_sha="$(shasum -a 256 "$lock" | cut -d ' ' -f 1)"
if [[ ! -x "$native/bin/meson" || "$(cat "$native/.imageflow-lock-sha256" 2>/dev/null || true)" != "$lock_sha" ]]; then
  if [[ -d "$native/conda-meta" ]]; then
    "$mamba" --no-rc install -y -p "$native" --file "$lock"
  else
    "$mamba" --no-rc create -y -p "$native" --file "$lock"
  fi
  printf '%s\n' "$lock_sha" > "$native/.imageflow-lock-sha256"
fi

version=8.18.7
source_dir="$deps/vips-$version"
if [[ ! -d "$source_dir" ]]; then
  archive="$deps/bootstrap/vips-$version.tar.xz"
  fetch "https://github.com/libvips/libvips/releases/download/v$version/vips-$version.tar.xz" "$archive" \
    5baaead3b0bb20ffdb9e9ff09aa9fda08620923df77b63b436654cb5e0b3bf94
  tar -xJf "$archive" -C "$deps"
fi

export PATH="$native/bin:$PATH"
export PKG_CONFIG_LIBDIR="$native/lib/pkgconfig:$native/share/pkgconfig"
unset PKG_CONFIG_PATH
export SDKROOT="$(xcrun --show-sdk-path)"
export MACOSX_DEPLOYMENT_TARGET=13.0
export CC="$(xcrun --find clang)" CXX="$(xcrun --find clang++)"
build="$deps/build-vips-$version-arm64"
setup=
if [[ -f "$build/build.ninja" ]]; then setup=--reconfigure; fi
meson setup ${setup:+$setup} "$build" "$source_dir" --prefix "$vips" --libdir lib --buildtype release \
    "-Dc_args=['-I$native/include']" "-Dcpp_args=['-I$native/include']" \
    "-Dc_link_args=['-L$native/lib','-Wl,-rpath,$native/lib']" \
    "-Dcpp_link_args=['-L$native/lib','-Wl,-rpath,$native/lib']" \
    --auto-features disabled -Ddeprecated=true -Dcplusplus=false -Dexamples=false \
    -Dmodules=disabled -Dnsgif=true -Danalyze=false -Dppm=false -Dradiance=false \
    -Djpeg=enabled -Dpng=enabled -Dwebp=enabled -Dzlib=enabled -Dexif=enabled \
    -Dtiff=enabled -Dheif=enabled -Dheif-module=disabled \
    -Djpeg-xl=enabled -Djpeg-xl-module=disabled -Dopenjpeg=enabled -Drsvg=enabled \
    -Dlcms=enabled -Dpangocairo=enabled -Dfontconfig=enabled
meson compile -C "$build"
meson install -C "$build"
PKG_CONFIG_PATH="$vips/lib/pkgconfig" pkg-config --modversion vips
