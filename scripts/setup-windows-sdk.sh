#!/bin/bash
# Install the pinned macOS-to-Windows x64 toolchain under this project.
set -euo pipefail
cd "$(dirname "$0")/.."
sdk="${MOCHIPIC_WINDOWS_SDK:-$PWD/.deps/windows-cross}"
mkdir -p "$sdk"
cd "$sdk"

fetch() {
  local name=$1 digest=$2 url=$3
  if [[ ! -f "$name" ]]; then
    curl -fLsS --retry 2 --proto '=https' --proto-redir '=https' -o "$name.part" "$url"
    printf '%s  %s\n' "$digest" "$name.part" | shasum -a 256 -c -
    mv "$name.part" "$name"
  fi
  printf '%s  %s\n' "$digest" "$name" | shasum -a 256 -c -
}

llvm=llvm-mingw-20261117-ucrt-macos-universal
fetch "$llvm.tar.xz" 98d340bbb8d46b09ad6886ffba88bd9d5415e131c316de48e6bd7647377446aa \
  "https://github.com/mstorsjo/llvm-mingw/releases/download/20261117/$llvm.tar.xz"
[[ -d "$llvm" ]] || tar -xf "$llvm.tar.xz"
vips=vips-dev-x64-all-8.18.7.zip
fetch "$vips" 78e56ba30c267ba32ff965200534bd5721364cadbead8c4d595e44b58dfeff15 \
  "https://github.com/libvips/build-win64-mxe/releases/download/v8.18.7/$vips"
[[ -d vips-dev-8.18 ]] || unzip -q "$vips"

headers() {
  local name=$1 digest=$2
  fetch "$name" "$digest" "https://mirror.msys2.org/mingw/ucrt64/$name"
  tar -xf "$name" -C vips-dev-8.18 --strip-components 1 'ucrt64/include/'
}
headers mingw-w64-ucrt-x86_64-libwebp-1.6.0-1-any.pkg.tar.zst 34ff8396c403d484c8777ad49ad0fc06dfb94fadf6f14bcf84d50c45bc6fd17e
headers mingw-w64-ucrt-x86_64-libtiff-4.7.2-2-any.pkg.tar.zst 258e08a91e6b1703d41730ffe6080c2a40968fe90896b45716c1b41042775076

# Homebrew's Apple Silicon bottle can be run here without a system install.
nsis_sha=fb902fdc6f21f2d0dee4f78d6a1350263901624217ed2ae591b94d107e665a38
if [[ ! -f nsis-3.13.tar.gz ]]; then
  curl -fLsS --retry 2 --proto '=https' --proto-redir '=https' -H 'Authorization: Bearer QQ==' \
    "https://ghcr.io/v2/homebrew/core/makensis/blobs/sha256:$nsis_sha" -o nsis-3.13.tar.gz.part
  printf '%s  %s\n' "$nsis_sha" nsis-3.13.tar.gz.part | shasum -a 256 -c -
  mv nsis-3.13.tar.gz.part nsis-3.13.tar.gz
fi
printf '%s  %s\n' "$nsis_sha" nsis-3.13.tar.gz | shasum -a 256 -c -
[[ -d makensis/3.13 ]] || tar -xf nsis-3.13.tar.gz
