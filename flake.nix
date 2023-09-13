# If using a flake-enabled version of Nix (minimum 2.4, with experimental
# features enabled), ''nix develop'' will spawn an environment in which
# ''./scripts/test'' will work as intended.

# For older versions of Nix, ''nix-shell'' will invoke this same code via the
# shell.nix compatibility layer.

{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/release-21.11";
    flake-compat.url = "github:edolstra/flake-compat";
    flake-compat.flake = false;
    flake-utils.url = "github:numtide/flake-utils";
  };
  outputs = { self, nixpkgs, flake-utils, ... }:
  flake-utils.lib.eachDefaultSystem (system: 
    let pkgs = import nixpkgs { inherit system; };
  in rec {
    devShell = pkgs.mkShell {
      buildInputs = with pkgs; [
        buildkit
        go
        rootlesskit
        runc
      ];
      shellHook = ''
        if ! type newuidmap >/dev/null 2>&1; then {
          echo "WARNING: newuidmap and newgid map are required but not found"
          echo "         Because these tools require a setuid bit to operate,"
          echo "         they cannot be installed in a local Nix shell."
          echo
        } >&2; fi
        PS1='[oci-build-task devshell] '"$PS1"
      '';
    };
  });
}


