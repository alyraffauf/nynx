// Copyright 2025 Aly Raffauf
// nynx
// A minimal NixOS deployment tool in Go.
// Usage:
//   go build -o nynx
//   ./nynx --flake github:alyraffauf/nixcfg --operation test

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

	if flakeDefault == "" {
		flakeDefault = "."
	}

	if opDefault == "" {
		opDefault = "test"
	}

	buildHostFlag := flag.String("build-host", "localhost", "Build closures on a specified remote host instead of locally.")
	flakeFlag := flag.String("flake", flakeDefault, "Flake URL or path.")
	jobsFlag := flag.String("jobs", "", "Filtered, comma-separated subset of deployment jobs to run.")
	opFlag := flag.String("operation", opDefault, "Operation to perform.")
	skipFlag := flag.String("skip", "", "Comma-separated list of deployment jobs to skip.")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output.")

	flag.Parse()

	buildHost := *buildHostFlag
	flake := *flakeFlag
	jobFilter := *jobsFlag
	op := *opFlag
	skipFilter := *skipFlag
	verbose := *verboseFlag

	verboseInfo(verbose, "Flake: %s", flake)
	verboseInfo(verbose, "Operation: %s", op)

	jobs, err := loadDeployments(flake)

	if err != nil {
		fatal("Failed to load deployments: %v", err)
	}

	validatedJobs, err := validateJobs(jobs, flake)
	if err != nil {
		fatal("Invalid deployments: %v", err)
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
				warn("Job '%s' not found in flake.nix.", skipJob)
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
				fatal("Job '%s' not found in flake.nix.", job)
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

	verboseInfo(verbose, "Instantiating drvPath(s) for %d job(s)...", len(jobs))

	drvPaths := make(map[string]string, len(jobs))
	for name, _ := range jobs {
		verboseInfo(verbose, "Instantiating drvPath for %s#nynxDeployments.%s.output...", flake, name)

		drv, err := instantiateDrvPath(flake, name, buildHost)
		if err != nil {
			fatal("Failed to instantiate drvPath for job '%s': %v", name, err)
		}
		drvPaths[name] = drv

		info("✔ Instantiated drvPath for %s#nynxDeployments.%s.output.", flake, name)
	}

	verboseInfo(verbose, "Building closures for %d job(s)...", len(jobs))

	outs := make(map[string]string, len(jobs))
	for name, spec := range jobs {
		verboseInfo(verbose, "Building %s#nynxDeployments.%s.output...", flake, name)

		out, err := buildClosure(spec, drvPaths[name], buildHost)
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
			verboseInfo(verbose, "Deploying %s#nynxDeployments.%s.output to %s...", flake, name, target)
			err := deployClosure(name, spec, outs, op)

			if err != nil {
				warn("%v", err)
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			} else {
				info("✔ Deployed %s#nynxDeployments.%s.output to %s.", flake, name, target)
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
