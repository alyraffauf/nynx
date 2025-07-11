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

	warnings, err := validateOperations(jobs, op)
	for _, warning := range warnings {
		warn(warning)
	}
	if err != nil {
		fatal("Invalid operation: %v", err)
	} else {
		info("✔ Operations validated.")
	}

	info("Building closures for %d job(s)...", len(jobs))

	outs := make(map[string]string, len(jobs))
	for name, spec := range jobs {
		info("Building %s#%s...", flake, spec.Output)
		out, err := buildClosure(flake, spec)
		if err != nil {
			fatal("Error building closures: %v", err)
		}
		outs[name] = out

		info("✔ Built %s#%s at %s.", flake, spec.Output, out)
	}

	info("✔ Closures built successfully.")

	// Deploy closures and report errors as warnings.
	// We don't want to immediately run fatal() on the first error,
	// Because this could leave hosts in even more inconsistent states.
	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex

	for name, spec := range jobs {
		wg.Add(1)
		go func(name string, spec JobSpec) {
			defer wg.Done()

			target := fmt.Sprintf("%s@%s", spec.User, spec.Hostname)
			info("Deploying %s#%s to %s...", flake, spec.Output, target)
			err := deployClosure(name, spec, outs, op)

			if err != nil {
				warn("%v", err)
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			} else {
				info("✔ Deployed %s#%s to %s", flake, spec.Output, target)
			}
		}(name, spec)
	}

	wg.Wait()

	errorCount := len(errors)

	if errorCount > 0 {
		fatal("Jobs failed with %d error(s).", errorCount)
	} else {
		info("✔ Deployments complete.")
	}
}
