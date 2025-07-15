{
  description = "A Flake-native deployment tool for distributed NixOS and Darwin clusters.";

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

    # darwinConfigurations = {
    #   darwintest =  self.inputs.nix-darwin.lib.darwinSystem {
    #     system = "aarch64-darwin";
    #     modules = [
    #       {
    #         networking.hostName = "darwintest";
    #       }
    #     ];
    #   };
    # };

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

    nixosConfigurations = {
      nixostest = self.inputs.nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          ({
            config,
            pkgs,
            ...
          }: {
            imports = [];

            boot.loader.systemd-boot.enable = true;
            boot.loader.efi.canTouchEfiVariables = true;

            fileSystems."/" = {
              device = "/dev/sda1";
              fsType = "ext4";
            };

            networking.hostName = "nixostest";
            services.openssh.enable = true;
            users.users.root.initialPassword = "changeme";
            system.stateVersion = "25.05";
          })
        ];
      };
    };

    nynxDeployments = {
      nixostest = {
        hostname = "nixostest"; # Will be assumed from deployment name if not specified.
        output = self.nixosConfigurations.nixostest.config.system.build.toplevel;
        # type = "nixos"; # Will be inferred based on the output.
        user = "root";
      };

      # fortree = {
      #   output = self.inputs.nixcfg.darwinConfigurations.fortree.config.system.build.toplevel;
      #   user = "aly";
      #   type = "darwin";
      # };
    };
  };
}
