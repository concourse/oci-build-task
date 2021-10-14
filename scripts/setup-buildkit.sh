if ! which buildctl >/dev/null || ! which buildkitd >/dev/null; then
  BUILDKIT_VERSION=0.9.1
  BUILDKIT_URL=https://github.com/moby/buildkit/releases/download/v$BUILDKIT_VERSION/buildkit-v$BUILDKIT_VERSION.linux-amd64.tar.gz

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
