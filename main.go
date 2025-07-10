// Copyright 2025 Aly Raffauf
// nynx
// A minimal NixOS deployment tool in Go.
// Usage:
//   go build -o nynx
//   ./nynx --flake github:alyraffauf/nixcfg --operation test --deployments deployments.nix

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
)

func main() {
	flakeFlag := flag.String("flake", "", "Flake specification")
	opFlag := flag.String("operation", "", "Operation to perform")
	cfgFlag := flag.String("deployments", "", "Path to deployments file")
	flag.Parse()

	flake := *flakeFlag
	op := *opFlag
	cfg := *cfgFlag

	if flake == "" {
		flake = os.Getenv("FLAKE")
		if flake == "" {
			flake = "."
		}
	}

	if op == "" {
		op = os.Getenv("OPERATION")
		if op == "" {
			op = "test"
		}
	}

	if cfg == "" {
		cfg = os.Getenv("DEPLOYMENTS")
		if cfg == "" {
			cfg = "deployments.nix"
		}
	}

	info("Flake: %s", flake)
	info("Operation: %s", op)
	info("Config: %s", cfg)

	hosts, err := loadDeploymentSpec(cfg)
	if err != nil {
		fatal("Failed to load deployment specs: %v", err)
	}

	// Build closures
	outs := make(map[string]string, len(hosts))
	for name, spec := range hosts {
		info("Building %s#%s...", flake, spec.Output)

		var expr string

		switch spec.Type {
		case "darwin":
			expr = fmt.Sprintf("%s#darwinConfigurations.%s.config.system.build.toplevel", flake, spec.Output)
		case "nixos":
			expr = fmt.Sprintf("%s#nixosConfigurations.%s.config.system.build.toplevel", flake, spec.Output)
		default:
			fatal("Unsupported system type: %s", spec.Type)
		}

		data, err := runJSON("nix", "build", "--no-link", "--json", expr)
		if err != nil {
			fatal("Failed to build %s: %v", name, err)
		}

		var res []BuildResult
		if err := json.Unmarshal(data, &res); err != nil {
			fatal("Bad build JSON for %s: %v", name, err)
		}
		if len(res) == 0 {
			fatal("No outputs for %s", name)
		}
		out, ok := res[0].Outputs["out"]
		if !ok {
			fatal("Missing 'out' for %s", name)
		}
		outs[name] = out
		info("✔ Built: %s", out)
	}

	// Copy closures
	var wg sync.WaitGroup
	for name, spec := range hosts {
		wg.Add(1)
		go func(name string, spec HostSpec) {
			defer wg.Done()
			target := fmt.Sprintf("%s@%s", spec.User, spec.Hostname)
			path := outs[name]
			info("Copying %s to %s...", path, target)
			run("nix", "copy", "--to", "ssh://"+target, path)
			info("✔ Copied %s to %s", path, target)
			info("Deploying %s#%s to %s...", flake, spec.Output, target)

			switch spec.Type {
			case "darwin":
				switch op {
				case "test":
					warn("nix-darwin does not support 'test' operation, using 'switch' instead.")
					fallthrough
				case "switch":
					run("ssh", target, "sudo", "nix-env", "-p", "/nix/var/nix/profiles/system", "--set", path)
					fallthrough // we always want to activate
				case "activate":
					run("ssh", target, "sudo", path+"/activate")
				default:
					fatal("unsupported darwin operation: %s", op)
				}
			case "nixos":
				run("ssh", target, "sudo", path+"/bin/switch-to-configuration", op)
			default:
				fatal("unsupported system type: %s", spec.Type)
			}

			info("✔ Deployed %s#%s to %s", flake, spec.Output, target)
		}(name, spec)
	}

	wg.Wait()
	info("✔ Deployments complete.")
}
