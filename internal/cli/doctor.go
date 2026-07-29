package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jpmartinez/higurashi-loop/internal/codegraph"
	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

func runDoctor(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	jsonOutput := false
	for _, argument := range args {
		switch argument {
		case "--json":
			if jsonOutput {
				return writeUsageFailure(
					stdout,
					stderr,
					true,
					"doctor",
					"--json may be supplied only once",
				)
			}
			jsonOutput = true
		case "-h", "--help":
			writeDoctorHelp(stdout)
			return exitSuccess
		default:
			return writeUsageFailure(
				stdout,
				stderr,
				jsonOutput,
				"doctor",
				fmt.Sprintf("unknown option %q", argument),
			)
		}
	}

	root, err := project.ResolveRoot(
		ctx,
		options.WorkingDirectory,
		options.CommandRunner,
	)
	if err != nil {
		kind := "not_git_project"
		if errors.Is(err, project.ErrUnsafeProjectRoot) {
			kind = "unsafe_project_root"
		}
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"doctor",
			kind,
			err.Error(),
			exitUnsafeProject,
		)
	}

	configuration, err := config.Load(root)
	if err != nil {
		kind := "configuration_invalid"
		if errors.Is(err, config.ErrMissing) {
			kind = "configuration_missing"
		}
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"doctor",
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	if err := workitem.ValidateSources(
		root,
		configuration.WorkItems.RequirementSources,
	); err != nil {
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"doctor",
			"requirement_source_invalid",
			fmt.Sprintf("requirement sources: %v", err),
			exitInvalidConfiguration,
		)
	}

	diagnostic := codegraph.Diagnose(
		ctx,
		root,
		configuration.CodeGraph.Mode,
		options.CommandRunner,
	)
	return writeDoctorResult(
		stdout,
		stderr,
		jsonOutput,
		root,
		configuration.WorkItems.RequirementSources,
		diagnostic,
	)
}

func writeDoctorResult(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	root string,
	requirementSources []string,
	diagnostic codegraph.Diagnostic,
) int {
	ok := diagnostic.Healthy()
	kind := "healthy"
	exitCode := exitSuccess
	message := diagnostic.Problem
	warnings := append([]string{}, diagnostic.Warnings...)

	if !ok && diagnostic.Mode == "preferred" {
		ok = true
		kind = "degraded"
		if diagnostic.Problem != "" {
			warnings = append(warnings, diagnostic.Problem)
		}
	} else if !ok {
		exitCode = exitCapabilityUnavailable
		kind = "codegraph_unhealthy"
		if diagnostic.Unavailable() {
			kind = "codegraph_unavailable"
		}
		if message == "" {
			message = "required CodeGraph capability is unhealthy"
		}
	}

	if jsonOutput {
		if err := result.WriteJSON(stdout, result.Envelope{
			Command:            "doctor",
			OK:                 ok,
			Kind:               kind,
			Message:            message,
			ProjectRoot:        root,
			RequirementSources: requirementSources,
			Warnings:           warnings,
			CodeGraph:          diagnostic,
		}); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
	} else {
		status := kind
		fmt.Fprintf(stdout, "Higurashi doctor: %s\n", status)
		fmt.Fprintf(stdout, "Project root: %s\n", root)
		for _, source := range requirementSources {
			fmt.Fprintf(stdout, "Requirement source: %s\n", source)
		}
		fmt.Fprintf(stdout, "CodeGraph mode: %s\n", diagnostic.Mode)
		fmt.Fprintf(
			stdout,
			"CodeGraph CLI available: %t\n",
			diagnostic.CLIAvailable,
		)
		fmt.Fprintf(
			stdout,
			"Project index present: %t\n",
			diagnostic.IndexPresent,
		)
		fmt.Fprintf(
			stdout,
			"CodeGraph status healthy: %t\n",
			diagnostic.StatusHealthy,
		)
		fmt.Fprintf(
			stdout,
			"CodeGraph root matches: %t\n",
			diagnostic.RootMatches,
		)
		fmt.Fprintf(
			stdout,
			"Synchronization: %s\n",
			diagnostic.Synchronization,
		)
	}

	if !ok && message != "" {
		fmt.Fprintf(stderr, "higurashi: %s\n", message)
	}
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "higurashi: warning: %s\n", warning)
	}
	return exitCode
}

func writeDoctorHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi doctor [--json]

Checks the Git project root, strict project configuration, requirement sources,
CodeGraph CLI, project-local index, status health, root consistency, and synchronization.
This command does not modify the repository.`)
}
