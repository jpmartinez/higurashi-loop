package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

func runConfigRequirements(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	jsonOutput := false
	for _, argument := range args {
		if argument == "--json" {
			if jsonOutput {
				return writeUsageFailure(
					stdout,
					stderr,
					true,
					"config.requirements.set",
					"--json may be supplied only once",
				)
			}
			jsonOutput = true
		}
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		writeConfigRequirementsHelp(stdout)
		return exitSuccess
	}
	if len(args) == 0 || args[0] != "set" {
		return writeUsageFailure(
			stdout,
			stderr,
			jsonOutput,
			"config.requirements.set",
			`config requirements requires the "set" operation`,
		)
	}

	var sources []string
	seen := make(map[string]bool)
	for _, argument := range args[1:] {
		if argument == "--json" {
			continue
		}
		if strings.HasPrefix(argument, "-") {
			return writeUsageFailure(
				stdout,
				stderr,
				jsonOutput,
				"config.requirements.set",
				fmt.Sprintf("unknown option %q", argument),
			)
		}
		source := config.NormalizeProjectPath(argument)
		if strings.TrimSpace(source) == "" || source == "." {
			return writeUsageFailure(
				stdout,
				stderr,
				jsonOutput,
				"config.requirements.set",
				"requirement sources must not be empty",
			)
		}
		if seen[source] {
			return writeUsageFailure(
				stdout,
				stderr,
				jsonOutput,
				"config.requirements.set",
				fmt.Sprintf("duplicate requirement source %q", source),
			)
		}
		seen[source] = true
		sources = append(sources, source)
	}
	if len(sources) == 0 {
		return writeUsageFailure(
			stdout,
			stderr,
			jsonOutput,
			"config.requirements.set",
			"at least one requirement source is required",
		)
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
			"config.requirements.set",
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
			"config.requirements.set",
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	currentSources := append(
		[]string(nil),
		configuration.WorkItems.RequirementSources...,
	)
	configuration.WorkItems.RequirementSources = sources
	encoded, err := config.Encode(configuration)
	if err != nil {
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"config.requirements.set",
			"configuration_invalid",
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	if err := workitem.ValidateSources(root, sources); err != nil {
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"config.requirements.set",
			"requirement_source_invalid",
			err.Error(),
			exitInvalidConfiguration,
		)
	}

	kind := "current"
	var changedPaths []string
	if !slices.Equal(currentSources, sources) {
		if err := replaceProjectConfig(root, encoded); err != nil {
			return writeFailure(
				stdout,
				stderr,
				jsonOutput,
				"config.requirements.set",
				"configuration_update_failed",
				err.Error(),
				exitInternalError,
			)
		}
		kind = "updated"
		changedPaths = []string{".higurashi/config.json"}
	}

	envelope := result.Envelope{
		Command:            "config.requirements.set",
		OK:                 true,
		Kind:               kind,
		ProjectRoot:        root,
		Config:             configuration,
		RequirementSources: sources,
		ChangedPaths:       changedPaths,
	}
	if jsonOutput {
		if err := result.WriteJSON(stdout, envelope); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
		return exitSuccess
	}
	fmt.Fprintf(stdout, "Requirement sources: %s\n", kind)
	fmt.Fprintf(stdout, "Project root: %s\n", root)
	for _, source := range sources {
		fmt.Fprintf(stdout, "Source: %s\n", source)
	}
	return exitSuccess
}

func writeConfigRequirementsHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi config requirements set PATH [PATH ...] [--json]

Atomically replaces the configured project-relative requirement sources after
validating that every source exists. This is an explicit project configuration
change; refinement agents remain read-only and cannot perform it.`)
}
