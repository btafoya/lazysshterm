{
  description = "lazysshterm — terminal UI for managing SSH connections";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        version = self.shortRev or self.dirtyShortRev or "dev";
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "lazysshterm";
          inherit version;
          src = self;

          vendorHash = "sha256-GNtV6kZa/1tAvIpTEsZg+Z0uE13eGPKaqoKDAnXEvus=";

          subPackages = [ "cmd/lazysshterm" ];

          ldflags = [
            "-X main.version=${version}"
            "-X main.gitCommit=${self.rev or "unknown"}"
          ];

          meta = with pkgs.lib; {
            description = "Terminal UI for managing SSH connections";
            homepage = "https://github.com/btafoya/lazysshterm";
            license = licenses.asl20;
            mainProgram = "lazysshterm";
          };
        };

        apps.default = flake-utils.lib.mkApp { drv = self.packages.${system}.default; };

        devShells.default = pkgs.mkShell {
          buildInputs = [ pkgs.go_1_26 pkgs.golangci-lint ];
        };
      });
}
