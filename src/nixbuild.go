package main

import (
	"fmt"
)

// model 'nix build --json' output.
type BuildResult struct {
	Outputs map[string]string `json:"outputs"`
}

func buildClosure(spec JobSpec, builder string) (string, *DebugInfo, error) {
	var results []BuildResult
	var err error
	var debug *DebugInfo

	// Build the closure locally or on the remote builder
	if builder != "localhost" {
		if _, debug, err = run("nix", "copy", "--to", "ssh-ng://"+builder, spec.DrvPath); err != nil {
			return "", debug, fmt.Errorf("copy to %s: %v", builder, err)
		}

		results, debug, err = runJSON[[]BuildResult]("nix", "build", "--no-link", "--json", "--store", "ssh-ng://"+builder, spec.DrvPath+"^*")
	} else {
		results, debug, err = runJSON[[]BuildResult]("nix", "build", "--no-link", "--json", spec.DrvPath+"^*")
	}
	if err != nil {
		return "", debug, fmt.Errorf("build on %s: %w", builder, err)
	}
	if len(results) == 0 {
		return "", debug, fmt.Errorf("empty build result")
	}
	out, ok := results[0].Outputs["out"]
	if !ok {
		return "", debug, fmt.Errorf("missing out key in build result")
	}

	if builder != "localhost" {
		if _, debug, err := run("nix", "copy", "--from", "ssh-ng://"+builder, out, "--no-check-sigs"); err != nil {
			return "", debug, fmt.Errorf("copy from %s: %v", builder, err)
		}
	}

	return out, debug, nil
}

func deployClosure(name string, spec JobSpec, outs map[string]string, op string) (*DebugInfo, error) {
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
		switch op {
		case "switch", "boot":
			cmds = append(cmds, []string{"ssh", target, "sudo", "nix-env", "-p", "/nix/var/nix/profiles/system", "--set", path})
			fallthrough // we always want to activate
		case "test":
			cmds = append(cmds, []string{"ssh", target, "sudo", path + "/bin/switch-to-configuration", op})
		}

	}

	var debug *DebugInfo
	var err error
	if _, debug, err = run("nix", "copy", "--to", "ssh-ng://"+target, path, "--no-check-sigs"); err != nil {
		return debug, fmt.Errorf("copy to %s: %v", target, err)
	}

	for _, cmd := range cmds {
		if _, debug, err := run(cmd[0], cmd[1:]...); err != nil {
			return debug, fmt.Errorf("activation on %s: %v", target, err)
		}
	}

	return debug, nil
}
