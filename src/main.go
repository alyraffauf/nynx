// Copyright 2025 Aly Raffauf
// nynx
// A minimal NixOS deployment tool in Go.
// Usage:
//   go build -o nynx
//   ./nynx --flake github:alyraffauf/nixcfg --operation test --deployments deployments.nix

package main

import (
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
	skipFlag := flag.String("skip", "", "Comma-separated list of deployment jobs to skip.")

	flag.Parse()

	flake := *flakeFlag
	op := *opFlag
	cfg := *cfgFlag
	jobFilter := *jobsFlag
	skipFilter := *skipFlag

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

	// Skip provided jobs
	if skipFilter != "" {
		skipJobs := strings.SplitSeq(skipFilter, ",")
		for skipJob := range skipJobs {
			skipJob = strings.TrimSpace(skipJob)
			if skipJob == "" {
				continue
			}
			if _, exists := jobs[skipJob]; exists {
				delete(jobs, skipJob)
			} else {
				warn("Job '%s' not found in deployment specification.", skipJob)
			}
		}
	}

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

	err = validateOperations(jobs, op)
	if err != nil {
		fatal("Invalid operation: %v", err)
	} else {
		info("✔ Operations validated.")
	}

	info("Building closures for %d job%s...", len(jobs), func() string {
		if len(jobs) != 1 {
			return "s"
		}
		return ""
	}())

	outs := make(map[string]string, len(jobs))
	for name, spec := range jobs {
		info("Building %s#%s...", flake, spec.Output)
		out, err := buildClosure(flake, spec)
		if err != nil {
			fatal("Error building closures: %v", err)
		}
		outs[name] = out
	}
	info("✔ Closures built successfully.")

	// Copy closures
	var wg sync.WaitGroup
	for name, spec := range jobs {
		wg.Add(1)
		go func(name string, spec JobSpec) {
			defer wg.Done()
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
			run("nix", "copy", "--to", "ssh://"+target, path)
			info("✔ Copied %s to %s", path, target)
			info("Deploying %s#%s to %s...", flake, spec.Output, target)

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
