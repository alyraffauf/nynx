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
          prettier
        ]
        ++ lib.attrValues config.treefmt.build.programs
        ++ [
          self'.packages.nynx
        ];
    };
  };
}
