#!/bin/sh
# Runs after the root filesystem is assembled, before it is packed.
set -e
TARGET="$1"
# Members of wheel may use sudo (with their own password), as on most servers.
# /etc/sudoers already reads /etc/sudoers.d; sudo requires 0440 on these files.
mkdir -p "$TARGET/etc/sudoers.d"
rm -f "$TARGET/etc/sudoers.d/wheel"   # read-only from a previous build
echo '%wheel ALL=(ALL:ALL) ALL' > "$TARGET/etc/sudoers.d/wheel"
chmod 0440 "$TARGET/etc/sudoers.d/wheel"
