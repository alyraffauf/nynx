package main

import (
	"fmt"
)

// model 'nix build --json' output.
type BuildResult struct {
	Outputs map[string]string `json:"outputs"`
}

func buildClosure(spec JobSpec, builder string) (string, error) {
	var results []BuildResult
	var err error

	// Build the closure locally or on the remote builder
	if builder != "localhost" {
		if _, err := run("nix", "copy", "--to", "ssh-ng://"+builder, spec.DrvPath); err != nil {
			return "", fmt.Errorf("failed to copy derivation to %s: %v", builder, err)
		}

		results, err = runJSON[[]BuildResult]("nix", "build", "--no-link", "--json", "--store", "ssh-ng://"+builder, spec.DrvPath+"^*")
	} else {
		results, err = runJSON[[]BuildResult]("nix", "build", "--no-link", "--json", spec.DrvPath+"^*")
	}
	if err != nil {
		return "", fmt.Errorf("failed to build %s on %s: %w", spec.Output, builder, err)
	}
	if len(results) == 0 {
		return "", fmt.Errorf("build result for %s was empty", spec.Output)
	}
	out, ok := results[0].Outputs["out"]
	if !ok {
		return "", fmt.Errorf("missing 'out' key in build result for %s", spec.Output)
	}

	if builder != "localhost" {
		if _, err := run("nix", "copy", "--from", "ssh-ng://"+builder, out, "--no-check-sigs"); err != nil {
			return "", fmt.Errorf("could not copy from %s: %v", builder, err)
		}
	}

	return out, nil
}

func deployClosure(name string, spec JobSpec, outs map[string]string, op string) error {
	target := fmt.Sprintf("%s@%s", spec.User, spec.Hostname)
	path := outs[name]
	var cmds [][]string

	switch spec.Type {
	case "darwin":
		switch op {
		case "switch", "test":
			cmds = append(cmds, []string{"ssh", target, "PATH=/run/current-system/sw/bin:$PATH", "sudo", "nix-env", "-p", "/nix/var/nix/profiles/system", "--set", path})
			fallthrough // we always want to activate
		case "activate":
			cmds = append(cmds, []string{"ssh", target, "PATH=/run/current-system/sw/bin:$PATH", "sudo", path + "/activate"})
		}
	case "nixos":
		cmds = append(cmds, []string{"ssh", target, "sudo", path + "/bin/switch-to-configuration", op})
	}

	if _, err := run("nix", "copy", "--to", "ssh-ng://"+target, path, "--no-check-sigs"); err != nil {
		return fmt.Errorf("error copying to %s: %v", target, err)
	}

	for _, cmd := range cmds {
		if _, err := run(cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("failed to activate on %s: %v", target, err)
		}
	}

	return nil
}
