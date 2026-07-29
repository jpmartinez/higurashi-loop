package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/artifact"
	"github.com/jpmartinez/higurashi-loop/internal/codegraph"
	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/repair"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

type repairArguments struct {
	WorkItemID string
	JSON       bool
	Help       bool
}

func runRepair(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	arguments, err := parseRepairArguments(args)
	if arguments.Help {
		writeRepairHelp(stdout)
		return exitSuccess
	}
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			"repair.authorize",
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
			"repair.authorize",
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
			"repair.authorize",
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	if err := workitem.ValidateID(
		configuration.WorkItems.IDPattern,
		arguments.WorkItemID,
	); err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"repair.authorize",
			"unknown",
			err.Error(),
			exitWorkItemUnavailable,
		)
	}

	diagnostic := codegraph.Diagnose(
		ctx,
		root,
		configuration.CodeGraph.Mode,
		options.CommandRunner,
	)
	warnings := append([]string{}, diagnostic.Warnings...)
	if !diagnostic.Healthy() {
		if configuration.CodeGraph.Mode == "required" {
			kind := "codegraph_unhealthy"
			if diagnostic.Unavailable() {
				kind = "codegraph_unavailable"
			}
			return writeRepairFailure(
				stdout,
				stderr,
				arguments.JSON,
				root,
				arguments.WorkItemID,
				"",
				"",
				kind,
				diagnostic.Problem,
				exitCapabilityUnavailable,
				diagnostic,
				warnings,
				nil,
			)
		}
		if diagnostic.Problem != "" {
			warnings = append(warnings, diagnostic.Problem)
		}
	}

	match, err := workitem.Find(
		root,
		configuration.WorkItems.RequirementSources,
		arguments.WorkItemID,
	)
	if err != nil {
		kind := "unknown"
		exitCode := exitWorkItemUnavailable
		switch {
		case errors.Is(err, workitem.ErrMissingSource):
			kind = "missing_requirement_source"
			exitCode = exitInvalidConfiguration
		case errors.Is(err, workitem.ErrConflict):
			kind = "conflict"
			exitCode = exitConflict
		case errors.Is(err, project.ErrPathEscapesRoot):
			kind = "unsafe_project_root"
			exitCode = exitUnsafeProject
		}
		return writeRepairFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			"",
			"",
			kind,
			err.Error(),
			exitCode,
			diagnostic,
			warnings,
			nil,
		)
	}
	artifactPath, absoluteArtifactPath, err := artifact.ResolvePath(
		root,
		configuration.Artifacts.Directory,
		arguments.WorkItemID,
	)
	if err != nil {
		return writeRepairFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			"",
			"",
			"unsafe_project_root",
			err.Error(),
			exitUnsafeProject,
			diagnostic,
			warnings,
			nil,
		)
	}
	artifactMutationPath, err := mutationPath(root, absoluteArtifactPath)
	if err != nil {
		return writeRepairFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			"",
			"invalid_artifact",
			err.Error(),
			exitInvalidArtifact,
			diagnostic,
			warnings,
			nil,
		)
	}
	snapshot, err := artifact.Load(
		artifactMutationPath,
		arguments.WorkItemID,
	)
	if err != nil {
		return writeRepairFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			"",
			"invalid_artifact",
			err.Error(),
			exitInvalidArtifact,
			diagnostic,
			warnings,
			nil,
		)
	}
	handoffRound := snapshot.Document.RepairRound + 1
	if snapshot.Document.Status == "implementing" &&
		snapshot.Document.RepairRound > 0 {
		handoffRound = snapshot.Document.RepairRound
	}
	handoffPath, absoluteHandoffPath, err := repair.ResolvePath(
		root,
		configuration.Artifacts.Directory,
		arguments.WorkItemID,
		handoffRound,
	)
	if err != nil {
		return writeRepairFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			"",
			"unsafe_project_root",
			err.Error(),
			exitUnsafeProject,
			diagnostic,
			warnings,
			&snapshot.Document.RepairRound,
		)
	}
	handoffMutationPath, err := mutationPath(root, absoluteHandoffPath)
	if err != nil {
		return writeRepairFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			handoffPath,
			"invalid_handoff",
			err.Error(),
			exitInvalidArtifact,
			diagnostic,
			warnings,
			&snapshot.Document.RepairRound,
		)
	}

	authorized, err := repair.Authorize(
		artifactMutationPath,
		handoffMutationPath,
		arguments.WorkItemID,
	)
	if err != nil {
		kind := "authorization_rejected"
		exitCode := exitInvalidArtifact
		switch {
		case errors.Is(err, repair.ErrConflict):
			kind = "conflict"
			exitCode = exitConflict
		case errors.Is(err, repair.ErrPartial):
			kind = "repair_recovery_required"
			exitCode = exitInternalError
		case errors.Is(err, repair.ErrInvalidHandoff):
			kind = "invalid_handoff"
		}
		return writeRepairFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			handoffPath,
			kind,
			err.Error(),
			exitCode,
			diagnostic,
			warnings,
			&snapshot.Document.RepairRound,
		)
	}
	kind := "authorized"
	if authorized.Recovered {
		kind = "recovered"
	} else if !authorized.Changed {
		kind = "unchanged"
	}
	repairRound := authorized.Document.RepairRound
	authorizationRequired := false
	envelope := result.Envelope{
		Command:               "repair.authorize",
		OK:                    true,
		Kind:                  kind,
		ProjectRoot:           root,
		WorkItemID:            arguments.WorkItemID,
		RequirementSource:     match.Source,
		ArtifactPath:          artifactPath,
		ArtifactStatus:        authorized.Document.Status,
		ArtifactHash:          authorized.ArtifactHash,
		RepairRound:           &repairRound,
		HandoffPath:           handoffPath,
		HandoffValidation:     authorized.Handoff.Status,
		BlockerCount:          len(authorized.Handoff.Blockers),
		AuthorizationRequired: &authorizationRequired,
		CandidateStrategy:     authorized.Handoff.CandidateStrategy,
		Progress:              authorized.Document.Progress,
		Warnings:              warnings,
		CodeGraph:             diagnostic,
	}
	return writeRepairResult(
		stdout,
		stderr,
		arguments.JSON,
		envelope,
		exitSuccess,
	)
}

func mutationPath(root, name string) (string, error) {
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", fmt.Errorf("resolve mutation path: %w", err)
	}
	if err := project.ValidateMutationPath(root, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func parseRepairArguments(args []string) (repairArguments, error) {
	var parsed repairArguments
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
			if strings.HasPrefix(argument, "-") {
				return parsed, fmt.Errorf("unknown option %q", argument)
			}
			positionals = append(positionals, argument)
		}
	}
	if len(positionals) != 2 || positionals[0] != "authorize" {
		return parsed, errors.New(
			"repair requires exactly: authorize WORK-ITEM-ID",
		)
	}
	parsed.WorkItemID = positionals[1]
	return parsed, nil
}

func writeRepairFailure(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	root string,
	workItemID string,
	artifactPath string,
	handoffPath string,
	kind string,
	message string,
	exitCode int,
	diagnostic codegraph.Diagnostic,
	warnings []string,
	repairRound *int,
) int {
	authorizationRequired := kind == "repair_recovery_required"
	nextCommand := ""
	handoffValidation := ""
	if authorizationRequired {
		nextCommand = repair.ExpectedCommand(workItemID)
		handoffValidation = "consumed"
	}
	envelope := result.Envelope{
		Command:               "repair.authorize",
		OK:                    false,
		Kind:                  kind,
		Message:               message,
		ProjectRoot:           root,
		WorkItemID:            workItemID,
		ArtifactPath:          artifactPath,
		HandoffPath:           handoffPath,
		HandoffValidation:     handoffValidation,
		RepairRound:           repairRound,
		AuthorizationRequired: &authorizationRequired,
		NextCommand:           nextCommand,
		Warnings:              warnings,
		CodeGraph:             diagnostic,
	}
	return writeRepairResult(
		stdout,
		stderr,
		jsonOutput,
		envelope,
		exitCode,
	)
}

func writeRepairResult(
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
		fmt.Fprintf(stdout, "Work item: %s\n", envelope.WorkItemID)
		fmt.Fprintf(stdout, "Result: %s\n", envelope.Kind)
		fmt.Fprintf(stdout, "Artifact: %s\n", envelope.ArtifactPath)
		fmt.Fprintf(stdout, "Status: %s\n", envelope.ArtifactStatus)
		if envelope.RepairRound != nil {
			fmt.Fprintf(stdout, "Repair round: %d\n", *envelope.RepairRound)
		}
		fmt.Fprintf(stdout, "Consumed handoff: %s\n", envelope.HandoffPath)
	}
	if !envelope.OK && envelope.Message != "" {
		fmt.Fprintf(stderr, "higurashi: %s\n", envelope.Message)
	}
	for _, warning := range envelope.Warnings {
		fmt.Fprintf(stderr, "higurashi: warning: %s\n", warning)
	}
	return exitCode
}

func writeRepairHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi repair authorize WORK-123 [--json]

Validates the expected versioned repair handoff and newly appended bounded
repair tasks, consumes the handoff, increments Repair-Round, and recoverably
transitions the blocked artifact to implementing.`)
}
