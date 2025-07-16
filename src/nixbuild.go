package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// model 'nix build --json' output.
type BuildResult struct {
	Outputs map[string]string `json:"outputs"`
}

func buildClosure(spec JobSpec, drvPath string, builder string) (string, error) {
	var buildOut []byte
	var err error

	// Build the closure locally or on the remote builder
	if builder != "localhost" {
		buildOut, err = runJSON("nix", "build", "--no-link", "--json", "--store", "ssh-ng://"+builder, drvPath+"^*")
	} else {
		buildOut, err = runJSON("nix", "build", "--no-link", "--json", drvPath+"^*")
	}
	if err != nil {
		return "", fmt.Errorf("failed to build %s on %s: %w", spec.Output, builder, err)
	}

	// Parse the output path
	var results []BuildResult
	if err := json.Unmarshal(buildOut, &results); err != nil {
		return "", fmt.Errorf("invalid build JSON for %s: %w\nRaw: %s", spec.Output, err, string(buildOut))
	}
	if len(results) == 0 {
		return "", fmt.Errorf("build result for %s was empty", spec.Output)
	}
	out, ok := results[0].Outputs["out"]
	if !ok {
		return "", fmt.Errorf("missing 'out' key in build result for %s", spec.Output)
	}

	// Step 5: Copy built closure back from builder to local (if needed)
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

func instantiateDrvPath(flake string, name string, builder string) (string, error) {
	expr := fmt.Sprintf("%s#nynxDeployments.%s.output", flake, name)
	drvExpr := expr + ".drvPath"

	// Evaluate the .drv path locally
	data, err := runJSON("nix", "eval", "--raw", drvExpr)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate drvPath for job '%s': %w", name, err)
	}

	drvPath := strings.TrimSpace(string(data))

	// Copy the .drv to the remote builder (if needed)
	if builder != "localhost" {
		if _, err := run("nix", "copy", "--to", "ssh-ng://"+builder, drvPath); err != nil {
			return "", fmt.Errorf("failed to copy .drv for job '%s' to %s: %w", name, builder, err)
		}
	}

	return drvPath, nil
}
