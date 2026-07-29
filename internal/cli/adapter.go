package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/render"
	"github.com/jpmartinez/higurashi-loop/internal/result"
)

type adapterArguments struct {
	Operation string
	Adapter   string
	JSON      bool
	Help      bool
}

func runAdapter(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	arguments, err := parseAdapterArguments(args)
	if arguments.Help {
		writeAdapterHelp(stdout)
		return exitSuccess
	}
	command := "adapter"
	if arguments.Operation != "" {
		command += "." + arguments.Operation
	}
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			err.Error(),
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
			arguments.JSON,
			command,
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
			arguments.JSON,
			command,
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	runner, ok := configuration.Runner(arguments.Adapter)
	if !ok {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			fmt.Sprintf("unsupported adapter %q", arguments.Adapter),
		)
	}
	bundle, err := render.BuildConfigured(
		arguments.Adapter,
		options.Version,
		runner.Models,
	)
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			err.Error(),
		)
	}

	var report render.Report
	if arguments.Operation == "diff" {
		report, err = render.Diff(root, bundle)
	} else {
		report, err = render.Apply(root, bundle)
	}
	if err != nil {
		kind := "render_failed"
		exitCode := exitInternalError
		switch {
		case errors.Is(err, render.ErrConflict):
			kind = "generated_file_conflict"
			exitCode = exitUnsafeProject
		case errors.Is(err, project.ErrPathEscapesRoot),
			errors.Is(err, project.ErrUnsafeMutationPath):
			kind = "unsafe_project_root"
			exitCode = exitUnsafeProject
		}
		envelope := adapterEnvelope(
			command,
			false,
			kind,
			root,
			report,
		)
		envelope.Message = err.Error()
		return writeAdapterResult(
			stdout,
			stderr,
			arguments.JSON,
			envelope,
			exitCode,
		)
	}

	kind := "clean"
	if arguments.Operation == "diff" {
		switch {
		case len(report.Conflicts) != 0:
			kind = "conflict"
		case len(report.ChangedPaths) != 0 || len(report.Stale) != 0:
			kind = "drift"
		}
	} else if len(report.Stale) != 0 && len(report.ChangedPaths) == 0 {
		kind = "stale"
	} else if len(report.ChangedPaths) == 0 {
		kind = "current"
	} else if arguments.Operation == "install" {
		kind = "installed"
	} else {
		kind = "updated"
	}
	envelope := adapterEnvelope(
		command,
		true,
		kind,
		root,
		report,
	)
	return writeAdapterResult(
		stdout,
		stderr,
		arguments.JSON,
		envelope,
		exitSuccess,
	)
}

func parseAdapterArguments(args []string) (adapterArguments, error) {
	var parsed adapterArguments
	var positionals []string
	for _, argument := range args {
		switch argument {
		case "--json":
			if parsed.JSON {
				return parsed, errors.New("--json may be supplied only once")
			}
			parsed.JSON = true
		case "-h", "--help":
			parsed.Help = true
			return parsed, nil
		default:
			if len(argument) > 0 && argument[0] == '-' {
				return parsed, fmt.Errorf("unknown option %q", argument)
			}
			positionals = append(positionals, argument)
		}
	}
	if len(positionals) != 2 {
		return parsed, errors.New(
			"adapter requires an operation and adapter name",
		)
	}
	parsed.Operation = positionals[0]
	parsed.Adapter = positionals[1]
	switch parsed.Operation {
	case "install", "diff", "update":
	default:
		return parsed, fmt.Errorf(
			"unsupported adapter operation %q",
			parsed.Operation,
		)
	}
	switch parsed.Adapter {
	case "opencode", "claude-code":
	default:
		return parsed, fmt.Errorf(
			"unsupported adapter %q",
			parsed.Adapter,
		)
	}
	return parsed, nil
}

func adapterEnvelope(
	command string,
	ok bool,
	kind string,
	root string,
	report render.Report,
) result.Envelope {
	return result.Envelope{
		Command:      command,
		OK:           ok,
		Kind:         kind,
		ProjectRoot:  root,
		Adapter:      report.Adapter,
		Files:        report.Files,
		ChangedPaths: report.ChangedPaths,
		Conflicts:    report.Conflicts,
		StalePaths:   report.Stale,
	}
}

func writeAdapterResult(
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
	} else {
		fmt.Fprintf(stdout, "Adapter: %s\n", envelope.Adapter)
		fmt.Fprintf(stdout, "Result: %s\n", envelope.Kind)
		for _, name := range envelope.ChangedPaths {
			fmt.Fprintf(stdout, "Changed: %s\n", name)
		}
		for _, name := range envelope.Conflicts {
			fmt.Fprintf(stdout, "Conflict: %s\n", name)
		}
		for _, name := range envelope.StalePaths {
			fmt.Fprintf(stdout, "Stale: %s\n", name)
		}
	}
	if !envelope.OK && envelope.Message != "" {
		fmt.Fprintf(stderr, "higurashi: %s\n", envelope.Message)
	}
	return exitCode
}

func writeAdapterHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi adapter install <opencode|claude-code> [--json]
  higurashi adapter diff <opencode|claude-code> [--json]
  higurashi adapter update <opencode|claude-code> [--json]

Renders or inspects the canonical runner-neutral protocol. Existing
unrecognized or locally modified generated files are never overwritten.`)
}
