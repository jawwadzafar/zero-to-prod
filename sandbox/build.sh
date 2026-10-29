#!/usr/bin/env bash
# Builds the sandbox's Linux image (out/zero-to-prod-linux.bin) and the source
# and license bundle that must accompany it (out/legal-info.tar.gz), inside a
# Docker container so the host needs nothing but Docker.
#
#   sandbox/build.sh            # takes ~10-20 minutes the first time
#
# Downloads and build output are cached in sandbox/.work (git-ignored).
set -euo pipefail
cd "$(dirname "$0")"
BUILDROOT=2026.02.3
WORK="$PWD/.work"
mkdir -p "$WORK" out
docker build -q -t zero2prod-buildroot .

docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp \
  -v "$PWD:/sandbox" -v "$WORK:/work" -w /work \
  zero2prod-buildroot bash -euo pipefail -c "
    [ -d buildroot-$BUILDROOT ] || { wget -q https://buildroot.org/downloads/buildroot-$BUILDROOT.tar.xz; tar xf buildroot-$BUILDROOT.tar.xz; }
    cd buildroot-$BUILDROOT
    make -s BR2_EXTERNAL=/sandbox O=/work/output BR2_DL_DIR=/work/dl zero2prod_v86_defconfig
    make -s O=/work/output BR2_DL_DIR=/work/dl
    make -s O=/work/output BR2_DL_DIR=/work/dl legal-info
  "
cp "$WORK/output/images/bzImage" out/zero-to-prod-linux.bin
tar czf out/legal-info.tar.gz -C "$WORK/output" legal-info
ls -lh out/
