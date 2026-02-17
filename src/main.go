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
	"time"
)

var debugEnabled bool

func info(format string, args ...any) {
	fmt.Printf("[nynx] %s\n", fmt.Sprintf(format, args...))
}

func debugLog(format string, args ...any) {
	fmt.Printf("[nynx debug] %s\n", fmt.Sprintf(format, args...))
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Warning: %s\n", fmt.Sprintf(format, args...))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[nynx] Error: %s\n", fmt.Sprintf(format, args...))
	os.Exit(1)
}

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
	debugFlag := flag.Bool("debug", false, "Enable debug output showing all commands and their results.")
	flakeFlag := flag.String("flake", flakeDefault, "Flake URL or path.")
	jobsFlag := flag.String("jobs", "", "Filtered, comma-separated subset of deployment jobs to run.")
	opFlag := flag.String("operation", opDefault, "Operation to perform.")
	skipFlag := flag.String("skip", "", "Comma-separated list of deployment jobs to skip.")
	flag.Parse()

	debugEnabled = *debugFlag
	startTime := time.Now()

	info("Deploying from %s", *flakeFlag)
	jobs, debugInfos, err := evalDeployments(*flakeFlag)
	if debugEnabled && len(debugInfos) > 0 {
		for _, d := range debugInfos {
			debugLog("$ %s", d.Command)
			if d.StdOut != "" {
				debugLog("Output:\n%s", strings.TrimSpace(d.StdOut))
			}
			if d.StdErr != "" {
				debugLog("Error:\n%s", strings.TrimSpace(d.StdErr))
			}
		}
	}
	if err != nil {
		fatal("Failed to evaluate flake configuration: %v", err)
	}

	// Skip provided jobs
	if *skipFlag != "" {
		skipJobs := strings.SplitSeq(*skipFlag, ",")
		for skipJob := range skipJobs {
			skipJob = strings.TrimSpace(skipJob)
			if skipJob == "" {
				continue
			}
			if _, exists := jobs[skipJob]; exists {
				delete(jobs, skipJob)
				info("Skipping system '%s' as requested", skipJob)
			} else {
				warn("Ignoring unknown system '%s'", skipJob)
			}
		}
	}

	// Filter jobs if --jobs is provided.
	if *jobsFlag != "" {
		selectedJobs := make(map[string]JobSpec)
		jobList := strings.SplitSeq(*jobsFlag, ",")
		for job := range jobList {
			job = strings.TrimSpace(job)
			if job == "" {
				continue
			}
			spec, ok := jobs[job]
			if !ok {
				fatal("Job '%s' not found in flake.nix", job)
			}
			selectedJobs[job] = spec
		}
		jobs = selectedJobs
	}

	info("Evaluating %d deployment%s...",
		len(jobs), plural(len(jobs)))

	warnings, err := validateOperations(jobs, *opFlag)
	for _, warning := range warnings {
		warn("%s", warning)
	}
	if err != nil {
		fatal("Invalid operation '%s': %v", *opFlag, err)
	}

	info("Building %d output%s...",
		len(jobs), plural(len(jobs)))

	outs := make(map[string]string, len(jobs))
	for name, spec := range jobs {
		out, debug, err := buildClosure(spec, *buildHostFlag)
		if debugEnabled && debug != nil {
			debugLog("$ %s", debug.Command)
			if debug.StdOut != "" {
				debugLog("Output:\n%s", strings.TrimSpace(debug.StdOut))
			}
			if debug.StdErr != "" {
				debugLog("Error:\n%s", strings.TrimSpace(debug.StdErr))
			}
		}
		if err != nil {
			fatal("Failed to build system '%s': %v", name, err)
		} else {
			info(" ✔ %s (%s)", name, spec.Type)

		}
		outs[name] = out
	}

	info("Deploying %d output%s...",
		len(jobs), plural(len(jobs)))

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

			debug, err := deployClosure(name, spec, outs, *opFlag)
			if debugEnabled && debug != nil {
				debugLog("$ %s", debug.Command)
				if debug.StdOut != "" {
					debugLog("Output:\n%s", strings.TrimSpace(debug.StdOut))
				}
				if debug.StdErr != "" {
					debugLog("Error:\n%s", strings.TrimSpace(debug.StdErr))
				}
			}
			if err != nil {
				warn("Failed to deploy to %s: %v", target, err)
				mu.Lock()
				errors = append(errors, fmt.Errorf("Failed to deploy to %s: %v", target, err))
				mu.Unlock()
			} else {
				info(" ✔ %s (%s) -> %s", name, spec.Type, target)
			}
		}(name, spec)
	}

	wg.Wait()

	errorCount := len(errors)
	duration := time.Since(startTime).Round(time.Second)

	if errorCount > 0 {
		fatal("Deployment failed with %d error%s (%s)",
			errorCount,
			map[bool]string{true: "s", false: ""}[errorCount != 1],
			duration)
	} else {
		info("Completed successfully in %s", duration)
	}
}
