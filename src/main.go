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
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output.")

	flag.Parse()

	flake := *flakeFlag
	op := *opFlag
	cfg := *cfgFlag
	jobFilter := *jobsFlag
	skipFilter := *skipFlag
	verbose := *verboseFlag

	verboseInfo(verbose, "Flake: %s", flake)
	verboseInfo(verbose, "Operation: %s", op)
	verboseInfo(verbose, "Config: %s", cfg)

	jobs, err := loadDeploymentSpec(cfg)
	if err != nil {
		fatal("Failed to load %s: %v", cfg, err)
	}

	validatedJobs, err := validateJobs(jobs)
	if err != nil {
		fatal("Invalid jobs! Please check %s: %v", err, cfg)
	}

	jobs = validatedJobs
	verboseInfo(verbose, "✔ Deployments validated.")

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
				warn("Job '%s' not found in %s.", skipJob, cfg)
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
				fatal("Job '%s' not found in %s.", job, cfg)
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
		verboseInfo(verbose, "✔ Operations validated.")
	}

	verboseInfo(verbose, "Building closures for %d job(s)...", len(jobs))

	outs := make(map[string]string, len(jobs))
	for name, spec := range jobs {
		verboseInfo(verbose, "Building %s#%s...", flake, spec.Output)
		out, err := buildClosure(flake, spec)
		if err != nil {
			fatal("Failure building closures: %v", err)
		}
		outs[name] = out

		info("✔ Built closure at %s.", out)
	}

	verboseInfo(verbose, "✔ All closures built successfully.")

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
			verboseInfo(verbose, "Deploying %s#%s to %s...", flake, spec.Output, target)
			err := deployClosure(name, spec, outs, op)

			if err != nil {
				warn("%v", err)
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			} else {
				info("✔ Deployed %s#%s to %s.", flake, spec.Output, target)
			}
		}(name, spec)
	}

	wg.Wait()

	errorCount := len(errors)

	if errorCount > 0 {
		fatal("Jobs failed with %d error(s).", errorCount)
	} else {
		verboseInfo(verbose, "✔ Deployments complete.")
	}
}
