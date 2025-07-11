{
  description = "A simple Flake deployer for distributed NixOS and Darwin clusters.";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs = {
    self,
    nixpkgs,
  }: let
    allSystems = [
      "aarch64-darwin"
      "aarch64-linux"
      "x86_64-darwin"
      "x86_64-linux"
    ];

    forAllSystems = f:
      nixpkgs.lib.genAttrs allSystems (system:
        f {
          pkgs = import self.inputs.nixpkgs {
            inherit system;
          };
        });
  in {
    formatter = self.inputs.nixpkgs.lib.genAttrs allSystems (system: self.packages.${system}.formatter);

    packages = forAllSystems ({pkgs}: rec {
      formatter = pkgs.writeShellApplication {
        name = "formatter";

        runtimeInputs = with pkgs; [
          alejandra
          diffutils
          findutils
          go
          gopls
          nodePackages.prettier
          shfmt
        ];

        text = builtins.readFile ./utils/formatter.sh;
      };

      nynx = pkgs.buildGoModule {
        pname = "nynx";
        src = ./src;
        vendorHash = null;

        version =
          if self ? shortRev
          then "git-${self.shortRev}"
          else "dev";
      };

      default = nynx;
    });

    devShells = forAllSystems ({pkgs}: {
      default = pkgs.mkShell {
        packages = with pkgs;
          [
            go
            gopls
            nixd
            nodePackages.prettier
            shfmt
          ]
          ++ [
            self.packages.${pkgs.system}.formatter
            self.packages.${pkgs.system}.nynx
          ];
      };
    });
  };
}
