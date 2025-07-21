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
	DrvPath  string // store path for derivation
}

// NixEvalJobsResult represents the output format of nix-eval-jobs
type NixEvalJobsResult struct {
	Attr     string            `json:"attr"`
	AttrPath []string          `json:"attrPath"`
	DrvPath  string            `json:"drvPath"`
	Name     string            `json:"name"`
	Outputs  map[string]string `json:"outputs"`
	System   string            `json:"system"`
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Error: "+format+"\n", args...)
	os.Exit(1)
}

func info(format string, args ...any) {
	fmt.Printf("[nynx] "+format+"\n", args...)
}

func evalDeployments(cfg string) (map[string]JobSpec, error) {
	flakeReference := fmt.Sprintf("%s#nynxDeployments", cfg)

	// First evaluate the nix configuration to get user settings
	configData, err := runJSON("nix", "eval", "--json", flakeReference)
	if err != nil {
		return nil, fmt.Errorf("failed to load nix config from %s: %w", cfg, err)
	}

	// Parse the config JSON
	config := make(map[string]JobSpec)
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid config JSON in %s: %w", cfg, err)
	}

	// Evaluate build outputs using nix-eval-jobs
	data, err := runJSON("nix-eval-jobs", "--force-recurse", "--flake", flakeReference)
	if err != nil {
		return nil, fmt.Errorf("failed to run nix-eval-jobs on %s: %w", cfg, err)
	}

	// Parse the newline-delimited JSON
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	jobs := make(map[string]JobSpec)

	for _, line := range lines {
		var result NixEvalJobsResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			return nil, fmt.Errorf("invalid JSON line: %s: %w", line, err)
		}

		// Extract the job name from the attr path (e.g. ["jobname", "output"] -> "jobname")
		if len(result.AttrPath) < 2 {
			return nil, fmt.Errorf("invalid attrPath format: %v", result.AttrPath)
		}
		jobName := result.AttrPath[0]

		// Get the output path
		outputPath, ok := result.Outputs["out"]
		if !ok {
			return nil, fmt.Errorf("missing 'out' output for job: %s", jobName)
		}

		// Get the config for this job
		configSpec, ok := config[jobName]
		if !ok {
			return nil, fmt.Errorf("job '%s' found in evaluation but not in config", jobName)
		}

		// Create and validate the job spec
		spec := JobSpec{
			Output:   outputPath,
			DrvPath:  result.DrvPath,
			Hostname: configSpec.Hostname,
			Type:     configSpec.Type,
			User:     configSpec.User,
		}

		// Validate required fields and set defaults
		if spec.Output == "" {
			return nil, fmt.Errorf("missing 'output' for job: %s", jobName)
		}

		if spec.User == "" {
			return nil, fmt.Errorf("missing 'user' for job: %s", jobName)
		}

		// Set hostname to jobName if not specified
		if spec.Hostname == "" {
			spec.Hostname = jobName
		}

		// Infer Type if missing
		if spec.Type == "" {
			system := result.System
			switch {
			case strings.Contains(system, "darwin"):
				spec.Type = "darwin"
			case strings.Contains(system, "linux"):
				spec.Type = "nixos"
			default:
				return nil, fmt.Errorf("could not infer system type for job '%s' from system '%s'", jobName, system)
			}
		}

		if spec.Type != "nixos" && spec.Type != "darwin" {
			return nil, fmt.Errorf("unsupported system type '%s' for job: %s", spec.Type, jobName)
		}

		jobs[jobName] = spec
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
			case "boot", "switch", "test":
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
