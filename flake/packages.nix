_: {
  perSystem = {
    pkgs,
    self',
    ...
  }: {
    packages = rec {
      nynx = pkgs.buildGoModule {
        pname = "nynx";
        src = ../src;
        vendorHash = null;

        nativeBuildInputs = with pkgs; [
          makeWrapper
        ];

        postInstall = ''
          wrapProgram $out/bin/nynx \
            --prefix PATH : ${pkgs.lib.makeBinPath [pkgs.nix-eval-jobs]}
        '';

        version =
          if self' ? shortRev
          then "git-${self'.shortRev}"
          else "dev";
      };

      nynx-lix = pkgs.buildGoModule {
        pname = "nynx";
        src = ../src;
        vendorHash = null;

        nativeBuildInputs = with pkgs; [
          makeWrapper
        ];

        postInstall = ''
          wrapProgram $out/bin/nynx \
            --prefix PATH : ${pkgs.lib.makeBinPath [pkgs.lixPackageSets.latest.nix-eval-jobs]}
        '';

        version =
          if self' ? shortRev
          then "git-${self'.shortRev}"
          else "dev";
      };

      default = nynx;
    };
  };
}
