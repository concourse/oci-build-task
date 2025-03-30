#!/usr/bin/env bash
# this is sourced, not executed; the shebang above is a hint for shellcheck and/or editors

uname_arch=$(uname -m)
case $uname_arch in
  x86_64) arch=amd64;;
  aarch64) arch=arm64;;
  *) arch=$uname_arch;;
esac

if ! command -v buildctl >/dev/null || ! command -v buildkitd >/dev/null; then
  BUILDKIT_VERSION=0.9.1
  BUILDKIT_URL=https://github.com/moby/buildkit/releases/download/v$BUILDKIT_VERSION/buildkit-v$BUILDKIT_VERSION.linux-${arch}.tar.gz

  curl -fL "$BUILDKIT_URL" | tar zxf -
fi

if [ "$UID" != "0" ]; then
  if ! command -v newuidmap >/dev/null || ! command -v newgidmap >/dev/null; then
    echo "newuidmap and newgidmap must be installed" >&2
    exit 1
  fi

  if ! command -v rootlesskit >/dev/null; then
    (cd rootlesskit && exec make)
    cp rootlesskit/bin/* bin/
  fi
fi

# prevents failure to create /run/runc
export XDG_RUNTIME_DIR=/tmp/buildkitd
