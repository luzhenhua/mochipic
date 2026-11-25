#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ -f .deps/vips/lib/pkgconfig/vips.pc ]]; then
  export PKG_CONFIG="$PWD/.deps/native/bin/pkg-config"
  export PKG_CONFIG_LIBDIR="$PWD/.deps/vips/lib/pkgconfig:$PWD/.deps/native/lib/pkgconfig:$PWD/.deps/native/share/pkgconfig"
  export PKG_CONFIG_PATH=
  export PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1
  export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L$PWD/.deps/native/lib -Wl,-rpath,$PWD/.deps/native/lib -Wl,-rpath,$PWD/.deps/vips/lib"
  export FONTCONFIG_FILE="${FONTCONFIG_FILE:-$PWD/scripts/fonts.conf}"
fi
export SDKROOT="${SDKROOT:-$(xcrun --show-sdk-path)}"
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_ENABLED=1
exec go "$@"
