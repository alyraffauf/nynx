{
  # Darwin example deployments file for nynx

  fortree = {
    # Deployment job name, can be arbitrary and unique
    output = "fortree"; # This corresponds to the darwinConfiguration output in your flake
    hostname = "fortree"; # The hostname or IP of the target Darwin/macOS machine
    user = "aly"; # The SSH user to connect as; typically root or a user with passwordless sudo
    type = "darwin"; # Indicates that this host is a Darwin/macOS system
  };
}
