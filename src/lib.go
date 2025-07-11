package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// model 'nix build --json' output.
type BuildResult struct {
	Outputs map[string]string `json:"outputs"`
}

// Deployment spec for a single job.
type JobSpec struct {
	Output   string `json:"output"`   // flake output
	Hostname string `json:"hostname"` // ssh host
	Type     string `json:"type"`     // type (nixos, darwin)
	User     string `json:"user"`     // ssh user
}

func buildClosure(flake string, spec JobSpec) (string, error) {
	expr := fmt.Sprintf("%s#%sConfigurations.%s.config.system.build.toplevel", flake, spec.Type, spec.Output)

	data, err := runJSON("nix", "build", "--no-link", "--json", expr)
	if err != nil {
		return "", fmt.Errorf("Failed to build %s: %v", spec.Output, err)
	}

	var res []BuildResult
	if err := json.Unmarshal(data, &res); err != nil {
		return "", fmt.Errorf("Bad build JSON for %s: %v", spec.Output, err)
	}

	out, ok := res[0].Outputs["out"]
	if !ok {
		return "", fmt.Errorf("Missing 'out' for %s", spec.Output)
	}

	return out, nil
}

func deployClosure(name string, spec JobSpec, outs map[string]string, flake string, op string) error {
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

	info("Copying %s to %s...", path, target)
	if _, err := run("nix", "copy", "--to", "ssh://"+target, path); err != nil {
		return fmt.Errorf("error copying to %s: %v", target, err)
	}
	info("✔ Copied %s to %s", path, target)
	info("Deploying %s#%s to %s...", flake, spec.Output, target)

	for _, cmd := range cmds {
		if _, err := run(cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("failed to activate on %s: %v", target, err)
		}
	}

	info("✔ Deployed %s#%s to %s", flake, spec.Output, target)
	return nil
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Error: "+format+"\n", args...)
	os.Exit(1)
}

func info(format string, args ...any) {
	fmt.Printf("[nynx] "+format+"\n", args...)
}

func loadDeploymentSpec(cfg string) (map[string]JobSpec, error) {
	// Nix -> JSON
	data, err := runJSON("nix", "eval", "--json", "-f", cfg)
	if err != nil {
		return nil, fmt.Errorf("Failed to run nix eval on %s: %w", cfg, err)
	}
	jobs := make(map[string]JobSpec)
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("Invalid JSON in %s: %w", cfg, err)
	}
	return jobs, nil
}

func run(cmd string, args ...string) ([]byte, error) {
	c := exec.Command(cmd, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("`%s %v` failed: %s", cmd, args, string(out))
	}
	return out, nil
}

func runJSON(cmd string, args ...string) ([]byte, error) {
	c := exec.Command(cmd, args...)
	out, err := c.Output() // only capture stdout
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("`%s %v` failed: %s", cmd, args, string(ee.Stderr))
		}
		return nil, fmt.Errorf("`%s %v` failed: %v", cmd, args, err)
	}
	return out, nil
}

func validateJobs(jobs map[string]JobSpec) (map[string]JobSpec, error) {
	validated := make(map[string]JobSpec)

	for name, spec := range jobs {
		// Infer Output if missing
		if spec.Output == "" {
			spec.Output = name
		}
		// Infer Hostname if missing
		if spec.Hostname == "" {
			spec.Hostname = name
		}

		if spec.User == "" {
			return nil, fmt.Errorf("Missing user for job: %s", name)
		}
		if spec.Type != "nixos" && spec.Type != "darwin" {
			return nil, fmt.Errorf("Unsupported system type '%s' for job: %s", spec.Type, name)
		}

		validated[name] = spec
	}

	return validated, nil
}

func validateOperations(jobs map[string]JobSpec, op string) ([]string, error) {
	var warnings []string
	for _, spec := range jobs {
		switch spec.Type {
		case "darwin":
			switch op {
			case "test", "switch", "activate":
				// Since "test" is not supported, we treat it equivalent to "switch"
				if op == "test" {
					warnings = append(warnings, "Nix-darwin does not support 'test' operation, using 'switch' instead.")
				}
			default:
				return warnings, fmt.Errorf("unsupported darwin operation: %s", op)
			}
		case "nixos":
			switch op {
			case "test", "switch":
				continue
			default:
				return warnings, fmt.Errorf("unsupported NixOS operation: %s", op)
			}
		default:
			return warnings, fmt.Errorf("unsupported system type: %s", spec.Type)
		}
	}
	return warnings, nil
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Warning: "+format+"\n", args...)
}
