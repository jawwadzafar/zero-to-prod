# sandbox: the Linux behind the in-browser terminal

The `<Terminal />` component (used in Part 2's labs) boots a real Linux inside
the web page with [v86](https://github.com/copy/v86), a 32-bit PC emulated in
WebAssembly. This folder builds that Linux with
[Buildroot](https://buildroot.org): one file, `zero-to-prod-linux.bin`, a
kernel with the whole root filesystem inside it.

What's in it: Linux 6.12 (long-term support), BusyBox (the everyday commands),
bash, jq, nano and sudo. It logs in automatically as `learner` (password
`learner`, member of `wheel`, so `sudo` works); `root`'s password is `sandbox`.
There is no network: nothing typed in it leaves the browser tab.

Why 6.12 and not the newer 6.18 series: under v86, a 6.18 kernel panics within
two seconds of booting ("stack-protector: Kernel stack is corrupted in
`__schedule`"), while 6.12 boots in about four. v86's own test image uses 6.8.
Retry newer kernels when v86 or Linux changes.

## Build

```bash
sandbox/build.sh     # needs Docker; ~10–20 minutes the first time
ls sandbox/out/      # zero-to-prod-linux.bin, legal-info.tar.gz
```

To try a build in the site locally, copy the image to
`static/sandbox/zero-to-prod-linux.bin` and run `npm start`.

On every push that changes this folder, `.github/workflows/sandbox.yml` builds
the image and publishes it as a `sandbox-<commit>` release. The site deploy
copies the newest one into the published site.

## Files

| File | What it does |
|---|---|
| `configs/zero2prod_v86_defconfig` | The Buildroot configuration: CPU, toolchain, kernel version, packages |
| `board/v86/linux.fragment` | Changes to the kernel's standard i386 config for the emulated PC |
| `board/v86/rootfs_overlay/` | Files copied into the image: `inittab` (auto-login on the screen), `motd`, `autologin`, `profile.d/prompt.sh` (the `learner@sandbox:~$` prompt) |
| `board/v86/users.txt` | The `learner` account |
| `board/v86/post-build.sh` | Lets the `wheel` group use `sudo` (via `/etc/sudoers.d/wheel`) |
| `Dockerfile`, `build.sh` | The build machine and the build |

## Licenses

The image contains free software under its own licenses (Linux and BusyBox are
GPL-2.0; bash and nano GPL-3.0; jq MIT; sudo ISC; musl MIT). Every release
carries `legal-info.tar.gz`, Buildroot's bundle of the exact source code and
license texts of everything in the image. The BIOS files the emulator loads
(SeaBIOS, LGPL-3.0) come from the v86 repository, pinned to one commit.

`make legal-info` warns about two things, both expected: Buildroot's own source
isn't saved (Buildroot is the build tool, not part of the image), and the
prebuilt Bootlin toolchain declares no license file. The only parts of that
toolchain inside the image are the musl C library (MIT) and GCC's runtime
library (GPL-3.0 with the GCC Runtime Library Exception).

The approach (a Buildroot external tree for v86 with an auto-login `inittab`)
follows [humphd/browser-vm](https://github.com/humphd/browser-vm) (MIT,
Copyright (c) David Humphrey), whose files were the starting point.
