_: {
  perSystem = {
    config,
    lib,
    pkgs,
    self',
    ...
  }: {
    devShells.default = pkgs.mkShell {
      packages = with pkgs;
        [
          go
          gopls
          nixd
          nodePackages.prettier
        ]
        ++ lib.attrValues config.treefmt.build.programs
        ++ [
          self'.packages.nynx
        ];
    };
  };
}
