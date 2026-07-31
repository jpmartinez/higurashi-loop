package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/result"
)

const (
	exitSuccess               = 0
	exitInvalidUsage          = 2
	exitInvalidConfiguration  = 3
	exitWorkItemUnavailable   = 4
	exitInvalidArtifact       = 5
	exitCapabilityUnavailable = 6
	exitConflict              = 7
	exitUnsafeProject         = 8
	exitInternalError         = 9
)

// Options contains process state that is explicit for testability.
type Options struct {
	WorkingDirectory string
	Version          string
	CommandRunner    project.CommandRunner
	Input            io.Reader
}

// Run executes the CLI and returns its process exit code.
func Run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	if options.Version == "" {
		options.Version = "dev"
	}
	if options.CommandRunner == nil {
		options.CommandRunner = project.ExecRunner{}
	}
	if options.Input == nil {
		options.Input = os.Stdin
	}

	if len(args) == 0 || isHelp(args) {
		writeRootHelp(stdout)
		return exitSuccess
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "higurashi %s\n", options.Version)
		return exitSuccess
	}
	if len(args) >= 1 && args[0] == "init" {
		return runInit(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "doctor" {
		return runDoctor(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "inspect" {
		return runInspect(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "transition" {
		return runTransition(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "repair" {
		return runRepair(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "adapter" {
		return runAdapter(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "models" {
		return runModels(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "requirements" {
		return runRequirements(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 1 && args[0] == "verification" {
		return runVerification(
			ctx,
			args[1:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 2 && args[0] == "config" && args[1] == "requirements" {
		return runConfigRequirements(
			ctx,
			args[2:],
			stdout,
			stderr,
			options,
		)
	}
	if len(args) >= 2 && args[0] == "config" && args[1] == "validate" {
		return runConfigValidate(
			ctx,
			args[2:],
			stdout,
			stderr,
			options,
		)
	}

	fmt.Fprintf(stderr, "higurashi: unknown command %q\n", strings.Join(args, " "))
	fmt.Fprintln(stderr, "Run 'higurashi help' for usage.")
	return exitInvalidUsage
}

func runConfigValidate(
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
					"config.validate",
					"--json may be supplied only once",
				)
			}
			jsonOutput = true
		case "-h", "--help":
			writeConfigValidateHelp(stdout)
			return exitSuccess
		default:
			return writeUsageFailure(
				stdout,
				stderr,
				jsonOutput,
				"config.validate",
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
			"config.validate",
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
			"config.validate",
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}

	if jsonOutput {
		if err := result.WriteJSON(stdout, result.Envelope{
			Command:     "config.validate",
			OK:          true,
			Kind:        "valid",
			ProjectRoot: root,
			Config:      configuration,
		}); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
		return exitSuccess
	}

	fmt.Fprintln(stdout, "Configuration valid")
	fmt.Fprintf(stdout, "Project root: %s\n", root)
	fmt.Fprintf(
		stdout,
		"Artifact directory: %s\n",
		configuration.Artifacts.Directory,
	)
	fmt.Fprintf(
		stdout,
		"Maximum APPLY batches per run: %d\n",
		configuration.Loop.MaxApplyBatchesPerRun,
	)
	fmt.Fprintf(
		stdout,
		"Maximum repair attempts: %d\n",
		configuration.Loop.MaxRepairAttempts,
	)
	fmt.Fprintf(stdout, "CodeGraph mode: %s\n", configuration.CodeGraph.Mode)
	return exitSuccess
}

func writeFailure(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	command string,
	kind string,
	message string,
	exitCode int,
) int {
	if jsonOutput {
		if err := result.WriteJSON(stdout, result.Envelope{
			Command: command,
			OK:      false,
			Kind:    kind,
			Message: message,
		}); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
	}
	fmt.Fprintf(stderr, "higurashi: %s\n", message)
	return exitCode
}

func writeUsageFailure(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	command string,
	message string,
) int {
	return writeFailure(
		stdout,
		stderr,
		jsonOutput,
		command,
		"invalid_usage",
		message,
		exitInvalidUsage,
	)
}

func isHelp(args []string) bool {
	return len(args) == 1 &&
		(args[0] == "help" || args[0] == "-h" || args[0] == "--help")
}

func writeRootHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Higurashi Loop

Usage:
  higurashi <command>

Commands:
	  higurashi init --runner <opencode|claude-code|pi> [--runner NAME ...] [--requirement-source PATH ...] [--project-root PATH] [--force-generated] [--json]
      Initialize project configuration, artifact directory, and selected adapters
  higurashi inspect WORK-123 [--json]
      Inspect requirement and artifact state without modifying files
  higurashi transition WORK-123 STATUS --expected-hash HASH [--reason TEXT] [--defer-blocker BLOCKER=FOLLOW-UP ...] [--json]
      Guard and atomically update an artifact status
  higurashi repair authorize WORK-123 [--json]
      Authorize one validated durable repair round
  higurashi adapter <install|diff|update> <opencode|claude-code|pi> [--json]
      Safely render or inspect runner adapter files
  higurashi models <show|set|validate> [--runner opencode] [OPTIONS]
      Show, configure, or validate role-specific runner models
  higurashi doctor [--json]
      Check project configuration and CodeGraph health
  higurashi config validate [--json]
      Validate and show effective project configuration
  higurashi config requirements set PATH [PATH ...] [--json]
      Set and validate project requirement sources
  higurashi requirements import WORK-123 (--from PATH | --stdin) [--json]
      Import one durable, project-managed requirement snapshot
  higurashi verification suggest [--json]
      Suggest exact project-owned verification commands without executing them
  higurashi version
      Print the Higurashi version
  higurashi help
      Show this help`)
}

func writeConfigValidateHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi config validate [--json]

Validates .higurashi/config.json and prints the normalized effective
configuration. This command does not modify the repository.`)
}
