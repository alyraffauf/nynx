package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DebugInfo contains command execution details for debugging
type DebugInfo struct {
	Command    string // The command that was run
	StdOut     string // Standard output
	StdErr     string // Standard error
	WasSuccess bool   // Whether the command succeeded
}

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

// getConfigAttr evaluates a configuration attribute from the nix flake.
// Returns empty string if evaluation fails.
func getConfigAttr(cfg string, job string, attr string) (string, *DebugInfo, error) {
	attrPath := fmt.Sprintf("%s#nynxDeployments.%s.%s", cfg, job, attr)
	value, debug, err := runJSON[string]("nix", "eval", "--json", attrPath)
	if err != nil {
		return "", debug, fmt.Errorf("failed to evaluate attribute %s for job %s: %w", attr, job, err)
	}
	return value, debug, nil
}

func evalDeployments(cfg string) (map[string]JobSpec, []*DebugInfo, error) {
	var debugInfos []*DebugInfo
	flakeReference := fmt.Sprintf("%s#nynxDeployments", cfg)

	// Get raw output since nix-eval-jobs uses JSON Lines format
	cmdStr := fmt.Sprintf("nix-eval-jobs --force-recurse --flake %s", flakeReference)
	c := exec.Command("nix-eval-jobs", "--force-recurse", "--flake", flakeReference)
	out, err := c.Output()

	debug := &DebugInfo{
		Command:    cmdStr,
		WasSuccess: err == nil,
	}

	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			debug.StdErr = string(ee.Stderr)
			debugInfos = append(debugInfos, debug)
			return nil, debugInfos, fmt.Errorf("failed to run nix-eval-jobs on %s: %s", cfg, string(ee.Stderr))
		}
		debugInfos = append(debugInfos, debug)
		return nil, debugInfos, fmt.Errorf("failed to run nix-eval-jobs on %s: %v", cfg, err)
	}

	debug.StdOut = string(out)
	debugInfos = append(debugInfos, debug)

	// Parse JSON Lines format
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	results := make([]NixEvalJobsResult, 0, len(lines))
	for _, line := range lines {
		var result NixEvalJobsResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			return nil, debugInfos, fmt.Errorf("invalid JSON output: %w", err)
		}
		results = append(results, result)
	}

	jobs := make(map[string]JobSpec, len(results))

	// First pass: create basic job specs
	for _, result := range results {
		if len(result.AttrPath) < 2 {
			return nil, debugInfos, fmt.Errorf("malformed attrPath: %v", result.AttrPath)
		}

		jobName := result.AttrPath[0]
		outputPath, ok := result.Outputs["out"]
		if !ok {
			return nil, debugInfos, fmt.Errorf("job %s: missing 'out' output", jobName)
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
		if hostname, debug, err := getConfigAttr(cfg, jobName, "hostname"); err == nil && hostname != "" {
			spec.Hostname = hostname
			debugInfos = append(debugInfos, debug)
		}

		if user, debug, err := getConfigAttr(cfg, jobName, "user"); err == nil {
			spec.User = user
			debugInfos = append(debugInfos, debug)
		}

		if sysType, debug, err := getConfigAttr(cfg, jobName, "type"); err == nil {
			spec.Type = sysType
			debugInfos = append(debugInfos, debug)
		}

		if spec.Output == "" {
			return nil, debugInfos, fmt.Errorf("job %s: missing output path", jobName)
		}

		if spec.User == "" {
			return nil, debugInfos, fmt.Errorf("job %s: missing user attribute", jobName)
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
				return nil, debugInfos, fmt.Errorf("job %s: unknown system type %s", jobName, systemFound)
			}
		}

		if spec.Type != "nixos" && spec.Type != "darwin" {
			return nil, debugInfos, fmt.Errorf("job %s: unsupported system type %s", jobName, spec.Type)
		}

		jobs[jobName] = spec
	}

	return jobs, debugInfos, nil
}

func run(cmd string, args ...string) ([]byte, *DebugInfo, error) {
	cmdStr := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
	debug := &DebugInfo{
		Command: cmdStr,
	}

	out, err := exec.Command(cmd, args...).CombinedOutput()
	debug.StdOut = string(out)
	debug.WasSuccess = err == nil

	if err != nil {
		return out, debug, fmt.Errorf("%s failed: %s", cmdStr, strings.TrimSpace(string(out)))
	}
	return out, debug, nil
}

func runJSON[T any](cmd string, args ...string) (T, *DebugInfo, error) {
	var result T
	cmdStr := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))
	debug := &DebugInfo{
		Command: cmdStr,
	}

	c := exec.Command(cmd, args...)
	out, err := c.Output() // only capture stdout
	debug.StdOut = string(out)
	debug.WasSuccess = err == nil

	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			debug.StdErr = string(ee.Stderr)
			return result, debug, fmt.Errorf("%s failed: %s", cmdStr, strings.TrimSpace(string(ee.Stderr)))
		}
		return result, debug, fmt.Errorf("%s failed: %v", cmdStr, err)
	}

	if err := json.Unmarshal(out, &result); err != nil {
		return result, debug, fmt.Errorf("failed to unmarshal JSON output from %s: %w\nRaw output: %s",
			cmdStr, err, strings.TrimSpace(string(out)))
	}

	return result, debug, nil
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
