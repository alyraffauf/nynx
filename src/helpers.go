package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Deployment spec for a single job.
type JobSpec struct {
	Output   string `json:"output"`   // flake output
	Hostname string `json:"hostname"` // ssh host
	Type     string `json:"type"`     // type (nixos, darwin)
	User     string `json:"user"`     // ssh user
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Error: "+format+"\n", args...)
	os.Exit(1)
}

func info(format string, args ...any) {
	fmt.Printf("[nynx] "+format+"\n", args...)
}

func loadDeployments(cfg string) (map[string]JobSpec, error) {
	flakeReference := fmt.Sprintf("%s#nynxDeployments", cfg)

	// Nix -> JSON
	data, err := runJSON("nix", "eval", "--json", flakeReference)
	if err != nil {
		return nil, fmt.Errorf("failed to run nix eval on %s: %w", cfg, err)
	}
	jobs := make(map[string]JobSpec)
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", cfg, err)
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

func validateJobs(jobs map[string]JobSpec, flake string) (map[string]JobSpec, error) {
	validated := make(map[string]JobSpec)

	for name, spec := range jobs {

		// Infer Hostname if missing
		if spec.Hostname == "" {
			spec.Hostname = name
		}

		if spec.Output == "" {
			return nil, fmt.Errorf("missing 'output' for job: %s", name)
		}

		if spec.User == "" {
			return nil, fmt.Errorf("missing 'user' for job: %s", name)
		}

		// Infer Type if missing
		if spec.Type == "" {
			expr := fmt.Sprintf("%s#nynxDeployments.%s.output.system", flake, name)
			data, err := run("nix", "eval", "--raw", expr)
			if err != nil {
				return nil, fmt.Errorf("failed to infer type for job '%s': %w", name, err)
			}
			system := strings.TrimSpace(string(data))

			switch {
			case strings.Contains(system, "darwin"):
				spec.Type = "darwin"
			case strings.Contains(system, "linux"):
				spec.Type = "nixos"
			default:
				return nil, fmt.Errorf("could not infer system type for job '%s' from system '%s'", name, system)
			}
		}

		if spec.Type != "nixos" && spec.Type != "darwin" {
			return nil, fmt.Errorf("unsupported system type '%s' for job: %s", spec.Type, name)
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

func verboseInfo(verbose bool, format string, args ...any) {
	if verbose {
		info(format, args...)
	}
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Warning: "+format+"\n", args...)
}
