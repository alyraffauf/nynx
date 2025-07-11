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
	"strings"
	"sync"
)

func main() {
	flakeDefault := os.Getenv("FLAKE")
	opDefault := os.Getenv("OPERATION")
	cfgDefault := os.Getenv("DEPLOYMENTS")

	if flakeDefault == "" {
		flakeDefault = "."
	}

	if opDefault == "" {
		opDefault = "test"
	}

	if cfgDefault == "" {
		cfgDefault = "deployments.nix"
	}

	flakeFlag := flag.String("flake", flakeDefault, "Flake URL or path.")
	opFlag := flag.String("operation", opDefault, "Operation to perform.")
	cfgFlag := flag.String("deployments", cfgDefault, "Path to deployments file.")
	jobsFlag := flag.String("jobs", "", "Filtered, comma-separated subset of deployment jobs to run.")
	flag.Parse()

	flake := *flakeFlag
	op := *opFlag
	cfg := *cfgFlag
	jobFilter := *jobsFlag

	info("Flake: %s", flake)
	info("Operation: %s", op)
	info("Config: %s", cfg)

	jobs, err := loadDeploymentSpec(cfg)
	if err != nil {
		fatal("Failed to load deployment specs: %v", err)
	}

	validatedJobs, err := validateJobs(jobs)
	if err != nil {
		fatal("Invalid jobs! Please check your deployments: %v", err)
	}
	jobs = validatedJobs
	info("✔ Deployments validated.")

	// Filter jobs if --jobs is provided.
	if jobFilter != "" {
		selectedJobs := make(map[string]JobSpec)
		jobList := strings.SplitSeq(jobFilter, ",")
		for job := range jobList {
			job = strings.TrimSpace(job)
			if job == "" {
				continue
			}
			spec, ok := jobs[job]
			if !ok {
				fatal("Job '%s' not found in deployment specification", job)
			}
			selectedJobs[job] = spec
		}
		jobs = selectedJobs
	}

	// Build closures
	outs := make(map[string]string, len(jobs))
	for name, spec := range jobs {
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
	for name, spec := range jobs {
		wg.Add(1)
		go func(name string, spec JobSpec) {
			defer wg.Done()
			target := fmt.Sprintf("%s@%s", spec.User, spec.Hostname)
			path := outs[name]
			info("Copying %s to %s...", path, target)
			run("nix", "copy", "--to", "ssh://"+target, path)
			info("✔ Copied %s to %s", path, target)
			info("Deploying %s#%s to %s...", flake, spec.Output, target)

			var cmds [][]string

			switch spec.Type {
			case "darwin":
				switch op {
				case "test":
					warn("Nix-darwin does not support 'test' operation, using 'switch' instead.")
					fallthrough
				case "switch":
					cmds = append(cmds, []string{"ssh", target, "PATH=/run/current-system/sw/bin:$PATH", "sudo", "nix-env", "-p", "/nix/var/nix/profiles/system", "--set", path})
					fallthrough // we always want to activate
				case "activate":
					cmds = append(cmds, []string{"ssh", target, "PATH=/run/current-system/sw/bin:$PATH", "sudo", path + "/activate"})
				default:
					fatal("Unsupported darwin operation: %s", op)
				}
			case "nixos":
				cmds = append(cmds, []string{"ssh", target, "sudo", path + "/bin/switch-to-configuration", op})
			default:
				fatal("Unsupported system type: %s", spec.Type)
			}

			for _, cmd := range cmds {
				_, err := run(cmd[0], cmd[1:]...)
				if err != nil {
					fatal("Failed to activate: %v", err)
				}
			}

			info("✔ Deployed %s#%s to %s", flake, spec.Output, target)
		}(name, spec)
	}

	wg.Wait()
	info("✔ Deployments complete.")
}
