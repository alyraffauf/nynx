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
	Output   string `json:"output"`
	Hostname string `json:"hostname"`
	Type     string `json:"type"`
	User     string `json:"user"`
	DrvPath  string
}

// Output format of nix-eval-jobs
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

func getConfigAttr(cfg string, job string, attr string) string {
	attrPath := fmt.Sprintf("%s#nynxDeployments.%s.%s", cfg, job, attr)
	value, err := runJSON[string]("nix", "eval", "--json", attrPath)
	if err != nil {
		return ""
	}
	return value
}

func evalDeployments(cfg string) (map[string]JobSpec, error) {
	flakeReference := fmt.Sprintf("%s#nynxDeployments", cfg)

	// Get raw output since nix-eval-jobs uses JSON Lines format
	c := exec.Command("nix-eval-jobs", "--force-recurse", "--flake", flakeReference)
	out, err := c.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to run nix-eval-jobs on %s: %s", cfg, string(ee.Stderr))
		}
		return nil, fmt.Errorf("failed to run nix-eval-jobs on %s: %v", cfg, err)
	}

	// Parse JSON Lines format
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	results := make([]NixEvalJobsResult, 0, len(lines))
	for _, line := range lines {
		var result NixEvalJobsResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON line from nix-eval-jobs: %w\nLine: %s", err, line)
		}
		results = append(results, result)
	}

	jobs := make(map[string]JobSpec, len(results))

	// First pass: create basic job specs
	for _, result := range results {
		if len(result.AttrPath) < 2 {
			return nil, fmt.Errorf("invalid attrPath format: %v", result.AttrPath)
		}

		jobName := result.AttrPath[0]
		outputPath, ok := result.Outputs["out"]
		if !ok {
			return nil, fmt.Errorf("missing 'out' output for job: %s", jobName)
		}

		jobs[jobName] = JobSpec{
			Output:   outputPath,
			DrvPath:  result.DrvPath,
			Type:     "",
			Hostname: jobName,
		}
	}

	// Second pass: enrich specs with additional attributes
	for jobName, spec := range jobs {
		if hostname := getConfigAttr(cfg, jobName, "hostname"); hostname != "" {
			spec.Hostname = hostname
		}

		spec.User = getConfigAttr(cfg, jobName, "user")
		spec.Type = getConfigAttr(cfg, jobName, "type")

		if spec.Output == "" {
			return nil, fmt.Errorf("missing 'output' for job: %s", jobName)
		}

		if spec.User == "" {
			return nil, fmt.Errorf("missing 'user' for job: %s", jobName)
		}

		// Infer Type if missing
		if spec.Type == "" {
			// Find the matching job's system from original evaluation
			var systemFound string
			for _, result := range results {
				if len(result.AttrPath) > 0 && result.AttrPath[0] == jobName {
					systemFound = result.System
					break
				}
			}

			switch {
			case strings.Contains(systemFound, "darwin"):
				spec.Type = "darwin"
			case strings.Contains(systemFound, "linux"):
				spec.Type = "nixos"
			default:
				return nil, fmt.Errorf("could not infer system type for job '%s' from system '%s'", jobName, systemFound)
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

func runJSON[T any](cmd string, args ...string) (T, error) {
	var result T
	c := exec.Command(cmd, args...)
	out, err := c.Output() // only capture stdout
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return result, fmt.Errorf("`%s %v` failed: %s", cmd, args, string(ee.Stderr))
		}
		return result, fmt.Errorf("`%s %v` failed: %v", cmd, args, err)
	}

	if err := json.Unmarshal(out, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal JSON output from `%s %v`: %w\nRaw output: %s",
			cmd, args, err, string(out))
	}

	return result, nil
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
