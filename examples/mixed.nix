{
  # Mixed Darwin and NixOS deployments

  job1 = {
    output = "host1"; # the nixosConfiguration from your flake.nix
    hostname = "192.168.1.1"; # reference by IP
    type = "nixos";
    user = "root"; # usually root, but cany user with passwordless sudo escalation
  };

  job2 = {
    output = "host2";
    hostname = "website.com"; # or reference by domain
    type = "nixos";
    user = "root";
  };

  job2 = {
    output = "host3";
    hostname = "host3.local"; # or reference by any other accessible hostname
    user = "root";
    type = "darwin";
  };
}
