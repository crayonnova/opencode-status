{
  description = "opencode free-models status monitor: TUI + HTTP + NixOS module + Docker";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      version = "0.1.2";
    in
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in {
        packages.default = pkgs.buildGoModule {
          pname = "opencode-status";
          inherit version;
          src = ./.;
          # vendorHash: compute with `nix-prefetch-github-deps` or set to lib.fakeSha256 + first build.
          vendorHash = "sha256-3mlIHKNKMixZICQ63HMQR0fSt3tU3KFvsjWgowSWBnA=";
          modRoot = ".";
          subPackages = [ "cmd/opencode-status" ];
          ldflags = [ "-s" "-w" "-X main.version=${version}" ];
          meta = with pkgs.lib; {
            description = "Monitor opencode free models: TUI + HTTP API + SQLite history";
            homepage = "https://github.com/nova/opencode-status";
            license = licenses.mit;
            mainProgram = "opencode-status";
            platforms = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ go go-task gopls gotools sqlite ];
          shellHook = ''
            export GOPROXY="https://proxy.golang.org,direct"
            echo "opencode-status dev shell ready"
          '';
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/opencode-status";
        };

        # Standalone Docker image (built from this flake).
        apps.docker = {
          type = "app";
          program = toString (pkgs.writeShellScript "opencode-status-docker" ''
            cd ${./deploy}
            docker build -t opencode-status:${version} -t opencode-status:latest .
          '');
        };
      }) // {
        nixosModules.default = ./nix/nixos-module.nix;
      };
}
