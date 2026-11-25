#!/bin/sh
# The Windows SDK's GLib supplies its own gettext implementation.
args=$(pkg-config --define-prefix "$@") || exit $?
printf '%s\n' "$args" | sed -E 's/(^| )-lintl( |$)/\1\2/g'
