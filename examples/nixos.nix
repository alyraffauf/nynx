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

  lavaridge = mkHost "nixos" "root"; # Use a helper function to create a host.

  lilycove =
    mkHost "nixos" "root"
    // {
      output = "lilycove";
      hostname = "lilycove";
    }; # Override your helper function's attributes.

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
