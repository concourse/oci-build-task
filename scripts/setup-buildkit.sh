#!/usr/bin/env bash

if ! which buildctl >/dev/null || ! which buildkitd >/dev/null; then
  arch="$(uname -m)"
  case "$arch" in
    "aarch64")
      arch="arm64"
      ;;

    "x86_64")
      arch="amd64"
      ;;

    *)
      ;;
  esac

  BUILDKIT_VERSION="0.17.2"
  BUILDKIT_URL="https://github.com/moby/buildkit/releases/download/v${BUILDKIT_VERSION}/buildkit-v${BUILDKIT_VERSION}.linux-${arch}.tar.gz"

  curl -fL "$BUILDKIT_URL" | tar zxf -
fi

if [ "$(id -u)" != "0" ]; then
  if ! which newuidmap >/dev/null || ! which newgidmap >/dev/null; then
    echo "newuidmap and newgidmap must be installed"
    exit 1
  fi

  if ! which rootlesskit >/dev/null; then
    pushd rootlesskit
      make
    popd

    cp rootlesskit/bin/* bin/
  fi
fi

# prevents failure to create /run/runc
export XDG_RUNTIME_DIR=/tmp/buildkitd
