# The tools Buildroot needs on the build machine (its "host"), per
# https://buildroot.org/downloads/manual/manual.html#requirement
FROM ubuntu:24.04
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bc bzip2 ca-certificates cpio file g++ gcc git make patch perl python3 rsync unzip wget xz-utils \
  && rm -rf /var/lib/apt/lists/*
