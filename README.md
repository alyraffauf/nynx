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
nix run .#nynx -- --flake <flake-url> --operation <operation> --deployments <deployments-file>
```

### Flags

- `--flake`: Specify the flake URL (e.g., `github:alyraffauf/nixcfg`).
- `--operation`: Operation to perform (`test`, `switch`, or `activate` for Darwin; `test`, `switch`, etc. for NixOS).
- `--deployments`: Path to the `deployments.nix` file (default: `deployments.nix`).

### Example

Deploy a flake to multiple hosts using Nix:

```
nix run .#nynx -- -flake github:alyraffauf/nixcfg -operation switch -deployments deployments.nix
```

### Sample deployments.nix

Nynx is configured with a Nix attrset that defines the hosts and their configurations.

```nix
{
  host1 = {
    output = "host1"; # Will be inferred from job name if not present.
    hostname = "192.168.1.1"; # Also inferred from job name if not present.
    type = "nixos";
    user = "root";
  };

  host2 = {
    output = "host2";
    hostname = "website.com";
    type = "nixos";
    user = "root";
  };

  host3 = {
    output = "host3";
    hostname = "host3.local";
    user = "root";
    type = "darwin";
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
