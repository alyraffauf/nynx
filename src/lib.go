package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// Deployment spec for a single host.
type HostSpec struct {
	Output   string `json:"output"`   // flake output
	Hostname string `json:"hostname"` // ssh host
	Type     string `json:"type"`     // type (nixos, darwin)
	User     string `json:"user"`     // ssh user
}

// model 'nix build --json' output.
type BuildResult struct {
	Outputs map[string]string `json:"outputs"`
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Warning: "+format+"\n", args...)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Error: "+format+"\n", args...)
	os.Exit(1)
}

func info(format string, args ...any) {
	fmt.Printf("[nynx] "+format+"\n", args...)
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

func run(cmd string, args ...string) ([]byte, error) {
	c := exec.Command(cmd, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("`%s %v` failed: %s", cmd, args, string(out))
	}
	return out, nil
}

func loadDeploymentSpec(cfg string) (map[string]HostSpec, error) {
	// Nix -> JSON
	data, err := runJSON("nix", "eval", "--json", "-f", cfg)
	if err != nil {
		return nil, fmt.Errorf("Failed to run nix eval on %s: %w", cfg, err)
	}
	hosts := make(map[string]HostSpec)
	if err := json.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("Invalid JSON in %s: %w", cfg, err)
	}
	return hosts, nil
}

func validateJobs(hosts map[string]HostSpec) (map[string]HostSpec, error) {
	validated := make(map[string]HostSpec)

	for name, spec := range hosts {
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
