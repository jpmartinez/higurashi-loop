package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/initialize"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/render"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/verification"
)

type initArguments struct {
	Runners            []string
	RequirementSources []string
	ProjectRoot        string
	ForceGenerated     bool
	JSON               bool
	Help               bool
}

func runInit(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	arguments, err := parseInitArguments(args)
	if arguments.Help {
		writeInitHelp(stdout)
		return exitSuccess
	}
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			"init",
			err.Error(),
		)
	}

	workingDirectory := options.WorkingDirectory
	if arguments.ProjectRoot != "" {
		workingDirectory = arguments.ProjectRoot
		if !filepath.IsAbs(workingDirectory) {
			workingDirectory = filepath.Join(
				options.WorkingDirectory,
				workingDirectory,
			)
		}
	}
	root, err := project.ResolveRoot(
		ctx,
		workingDirectory,
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
			arguments.JSON,
			"init",
			kind,
			err.Error(),
			exitUnsafeProject,
		)
	}

	initialization, err := initialize.Apply(root, initialize.Request{
		Runners:            arguments.Runners,
		RequirementSources: arguments.RequirementSources,
		GeneratorVersion:   options.Version,
		ForceGenerated:     arguments.ForceGenerated,
	})
	if err != nil {
		envelope := result.Envelope{
			Command:      "init",
			OK:           false,
			Kind:         "initialization_failed",
			Message:      err.Error(),
			ProjectRoot:  root,
			Adapters:     arguments.Runners,
			Conflicts:    initialization.Conflicts,
			ChangedPaths: initialization.ChangedPaths,
		}
		exitCode := exitInternalError
		switch {
		case errors.Is(err, render.ErrConflict):
			envelope.Kind = "generated_file_conflict"
			exitCode = exitConflict
		case errors.Is(err, initialize.ErrConfigurationConflict):
			envelope.Kind = "configuration_conflict"
			exitCode = exitConflict
		case errors.Is(err, initialize.ErrInvalidConfiguration):
			envelope.Kind = "configuration_invalid"
			exitCode = exitInvalidConfiguration
		case errors.Is(err, project.ErrPathEscapesRoot),
			errors.Is(err, project.ErrUnsafeMutationPath):
			envelope.Kind = "unsafe_project_root"
			exitCode = exitUnsafeProject
		}
		return writeInitResult(
			stdout,
			stderr,
			arguments.JSON,
			envelope,
			exitCode,
		)
	}

	kind := "initialized"
	if len(initialization.ChangedPaths) == 0 {
		kind = "current"
	}
	suggestionReport := verification.Suggest(
		root,
		initialization.Config.Verification,
	)
	nextCommands := []string{"higurashi doctor"}
	if len(suggestionReport.Suggestions) != 0 {
		nextCommands = append(
			[]string{"higurashi verification suggest"},
			nextCommands...,
		)
	}
	for _, adapter := range arguments.Runners {
		switch adapter {
		case "opencode":
			nextCommands = append(nextCommands, "opencode")
		case "claude-code":
			nextCommands = append(nextCommands, "claude --plugin-dir .")
		case "pi":
			nextCommands = append(nextCommands, "pi")
		}
	}
	envelope := result.Envelope{
		Command:                 "init",
		OK:                      true,
		Kind:                    kind,
		ProjectRoot:             root,
		Config:                  initialization.Config,
		Adapters:                arguments.Runners,
		ChangedPaths:            initialization.ChangedPaths,
		NextCommands:            nextCommands,
		VerificationSuggestions: suggestionReport.Suggestions,
		SuggestedVerification:   suggestionReport.ConfigFragment,
		Warnings:                suggestionReport.Warnings,
	}
	return writeInitResult(
		stdout,
		stderr,
		arguments.JSON,
		envelope,
		exitSuccess,
	)
}

func parseInitArguments(args []string) (initArguments, error) {
	var parsed initArguments
	jsonCount := 0
	for _, argument := range args {
		if argument == "--json" {
			jsonCount++
		}
	}
	if jsonCount > 0 {
		parsed.JSON = true
	}
	if jsonCount > 1 {
		return parsed, errors.New("--json may be supplied only once")
	}
	seenRunners := map[string]bool{}
	seenRequirementSources := map[string]bool{}
	projectRootSeen := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--runner":
			if index+1 >= len(args) {
				return parsed, errors.New("--runner requires a value")
			}
			index++
			runner := args[index]
			if runner != "opencode" && runner != "claude-code" && runner != "pi" {
				return parsed, fmt.Errorf("unsupported runner %q", runner)
			}
			if seenRunners[runner] {
				return parsed, fmt.Errorf(
					"--runner %s may be supplied only once",
					runner,
				)
			}
			seenRunners[runner] = true
			parsed.Runners = append(parsed.Runners, runner)
		case "--requirement-source":
			if index+1 >= len(args) {
				return parsed, errors.New(
					"--requirement-source requires a value",
				)
			}
			index++
			source := config.NormalizeProjectPath(args[index])
			if strings.TrimSpace(source) == "" || source == "." {
				return parsed, errors.New(
					"--requirement-source must not be empty",
				)
			}
			if seenRequirementSources[source] {
				return parsed, fmt.Errorf(
					"--requirement-source %q may be supplied only once",
					source,
				)
			}
			seenRequirementSources[source] = true
			parsed.RequirementSources = append(
				parsed.RequirementSources,
				source,
			)
		case "--project-root":
			if projectRootSeen {
				return parsed, errors.New(
					"--project-root may be supplied only once",
				)
			}
			if index+1 >= len(args) {
				return parsed, errors.New("--project-root requires a value")
			}
			index++
			parsed.ProjectRoot = args[index]
			if strings.TrimSpace(parsed.ProjectRoot) == "" {
				return parsed, errors.New("--project-root must not be empty")
			}
			projectRootSeen = true
		case "--force-generated":
			if parsed.ForceGenerated {
				return parsed, errors.New(
					"--force-generated may be supplied only once",
				)
			}
			parsed.ForceGenerated = true
		case "--json":
			continue
		case "-h", "--help":
			parsed.Help = true
			return parsed, nil
		default:
			return parsed, fmt.Errorf("unknown option %q", argument)
		}
	}
	if len(parsed.Runners) == 0 {
		return parsed, errors.New("init requires at least one --runner")
	}
	return parsed, nil
}

func writeInitResult(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	envelope result.Envelope,
	exitCode int,
) int {
	if jsonOutput {
		if err := result.WriteJSON(stdout, envelope); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
	} else if envelope.OK {
		fmt.Fprintf(stdout, "Higurashi project: %s\n", envelope.Kind)
		fmt.Fprintf(stdout, "Project root: %s\n", envelope.ProjectRoot)
		for _, adapter := range envelope.Adapters {
			fmt.Fprintf(stdout, "Runner: %s\n", adapter)
		}
		for _, name := range envelope.ChangedPaths {
			fmt.Fprintf(stdout, "Changed: %s\n", name)
		}
		writeEnvelopeVerificationSuggestions(stdout, envelope)
		fmt.Fprintln(stdout, "Next commands:")
		for _, command := range envelope.NextCommands {
			fmt.Fprintf(stdout, "  %s\n", command)
		}
		for _, adapter := range envelope.Adapters {
			switch adapter {
			case "opencode":
				fmt.Fprintln(
					stdout,
					"OpenCode: /higurashi-refine WORK-123 or /higurashi-deliver WORK-123",
				)
			case "claude-code":
				fmt.Fprintln(
					stdout,
					"Claude Code: /higurashi-loop:refine WORK-123 or /higurashi-loop:deliver WORK-123",
				)
			case "pi":
				fmt.Fprintln(
					stdout,
					"Pi: /higurashi-refine WORK-123 or /higurashi-deliver WORK-123",
				)
			}
		}
	}
	if !envelope.OK && envelope.Message != "" {
		fmt.Fprintf(stderr, "higurashi: %s\n", envelope.Message)
	}
	for _, warning := range envelope.Warnings {
		fmt.Fprintf(stderr, "higurashi: warning: %s\n", warning)
	}
	return exitCode
}

func writeInitHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi init --runner <opencode|claude-code|pi> [--runner NAME ...]
      [--requirement-source PATH ...] [--project-root PATH]
      [--force-generated] [--json]

Creates strict project-local configuration, the artifact directory, and only
the selected runner adapters. Repeat --requirement-source to configure multiple
existing Markdown files or directories instead of the managed
docs/higurashi/requirements directory.
Existing user-owned files are never overwritten.
--force-generated replaces only files whose prior Higurashi ownership is
validated through the generated manifest and embedded ownership marker.`)
}
