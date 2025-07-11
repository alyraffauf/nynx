let
  mkHost = type: user: {
    inherit type user;
  };
in {
  # Example NixOS deployments

  evergrande = rec {
    output = "evergrande";
    hostname = "${output}"; # Interpolate strings.
    type = "nixos";
    user = "root";
  };

  # Create a job with a helper function.
  lavaridge = mkHost "nixos" "root";

  lilycove =
    # Override attributes.
    mkHost "nixos" "root"
    // {
      output = "lilycove";
      hostname = "lilycove";
    };

  mauville = {
    output = "mauville";
    hostname = "mauville";
    type = "nixos";
    user = "root";
  };

  mossdeep = {
    output = "mossdeep";
    hostname = "mossdeep";
    type = "nixos";
    user = "root";
  };
}
