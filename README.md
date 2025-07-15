# nynx (nix+sync)

A Flake deployer for distributed NixOS and Darwin clusters. Useful for quickly deploying the same flake revision to multiple machines in parallel.

Mainly a personal project to serve some specific needs of mine (outlined [here](https://aly.codes/blog/2025-05-19-mildly-better-flake-deployments/)), but it may be helpful to others as well. Also, it's my first time writing Go (please be gentle).

## Usage

Build the tool using Nix:

```
nix build .#nynx
```

Run deployments:

```
nix run .#nynx -- --flake <flake-url> --operation <operation>
```

### Flags

- `--build-host`: Specify the host on which to build closures (default: `localhost`).
- `--flake`: Specify the flake path or URL (e.g., `github:alyraffauf/nixcfg`).
- `--jobs`: Comma-separated subset of jobs to run (default: all jobs).
- `--operation`: Operation to perform (`switch`, or `activate` for Darwin; `boot`, `test`, `switch` for NixOS).
- `--skip`: Skip a comma-separated subset of jobs.
- `--verbose`: Enable verbose output.

### Example

Deploy a flake to all jobs defined in `deployments.nix`:

```
nix run .#nynx -- --flake github:alyraffauf/nixcfg --operation switch
```

Run specific deployment jobs:

```
nix run .#nynx -- --flake github:alyraffauf/nixcfg --operation switch --jobs server,workstation
```

### Sample #nynxDeployments

Nynx is configured with a Flake output containing an attrset that defines a set of deployment jobs. Outputs can be declared in the same Flake or in an upstream Flake.

```nix
{
  nynxDeployments = {
    evergrande = {
      hostname = "evergrande"; # Will be assumed from deployment name if not specified.
      output = self.inputs.nixcfg.nixosConfigurations.evergrande.config.system.build.toplevel;
      type = "nixos";
      user = "root";
    };

    fortree = {
      output = self.darwinConfigurations.fortree.config.system.build.toplevel;
      user = "aly";
      type = "darwin";
    };
  };
}
```

## Limitations

- Requires SSH root access or the ability to escalate privileges with sudo without password entry. It won't prompt for a password, it just fails.
- Does not (yet) support other forms of Nix profiles, such as home-manager.

## License

This project is licensed under the GNU General Public License v3.0.

## Contribution

Contributions are welcome! Please open issues or submit pull requests for any improvements you make or bugs you encounter.

## Contact

You can reach me at aly @ aly dot codes.
