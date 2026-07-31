package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/jpmartinez/higurashi-loop/internal/artifact"
	"github.com/jpmartinez/higurashi-loop/internal/codegraph"
	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/repair"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

type inspectTask struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type inspectLoop struct {
	MaxApplyBatchesPerRun          int  `json:"maxApplyBatchesPerRun"`
	MaxRepairAttempts              int  `json:"maxRepairAttempts"`
	RequireProgressAfterEveryBatch bool `json:"requireProgressAfterEveryBatch"`
}

func runInspect(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	workItemID, jsonOutput, help, err := parseInspectArguments(args)
	if help {
		writeInspectHelp(stdout)
		return exitSuccess
	}
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			jsonOutput,
			"inspect",
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
			jsonOutput,
			"inspect",
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
			"inspect",
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	if err := workitem.ValidateID(
		configuration.WorkItems.IDPattern,
		workItemID,
	); err != nil {
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"inspect",
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
			return writeInspectFailure(
				stdout,
				stderr,
				jsonOutput,
				root,
				workItemID,
				kind,
				diagnostic.Problem,
				exitCapabilityUnavailable,
				diagnostic,
				warnings,
			)
		}
		if diagnostic.Problem != "" {
			warnings = append(warnings, diagnostic.Problem)
		}
	}

	match, err := workitem.Find(
		root,
		configuration.WorkItems.RequirementSources,
		workItemID,
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
		return writeInspectFailure(
			stdout,
			stderr,
			jsonOutput,
			root,
			workItemID,
			kind,
			err.Error(),
			exitCode,
			diagnostic,
			warnings,
		)
	}

	artifactPath, absoluteArtifactPath, err := artifact.ResolvePath(
		root,
		configuration.Artifacts.Directory,
		workItemID,
	)
	if err != nil {
		return writeInspectFailure(
			stdout,
			stderr,
			jsonOutput,
			root,
			workItemID,
			"unsafe_project_root",
			err.Error(),
			exitUnsafeProject,
			diagnostic,
			warnings,
		)
	}
	_, err = os.Stat(absoluteArtifactPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return writeInspectFailure(
			stdout,
			stderr,
			jsonOutput,
			root,
			workItemID,
			"invalid_artifact",
			fmt.Sprintf("stat artifact: %v", err),
			exitInvalidArtifact,
			diagnostic,
			warnings,
		)
	}

	envelope := result.Envelope{
		Command:           "inspect",
		OK:                true,
		Kind:              "ready",
		ProjectRoot:       root,
		WorkItemID:        workItemID,
		RequirementSource: match.Source,
		ArtifactPath:      artifactPath,
		Progress:          artifact.Progress{},
		Loop: inspectLoop{
			MaxApplyBatchesPerRun: configuration.Loop.MaxApplyBatchesPerRun,
			MaxRepairAttempts:     configuration.Loop.MaxRepairAttempts,
			RequireProgressAfterEveryBatch: configuration.Loop.
				RequireProgressAfterEveryBatch,
		},
		Warnings:  warnings,
		CodeGraph: diagnostic,
	}
	if err == nil {
		document, readErr := artifact.Read(
			absoluteArtifactPath,
			workItemID,
		)
		if readErr != nil {
			return writeInspectFailure(
				stdout,
				stderr,
				jsonOutput,
				root,
				workItemID,
				"invalid_artifact",
				readErr.Error(),
				exitInvalidArtifact,
				diagnostic,
				warnings,
			)
		}
		envelope.Kind = "resume"
		if document.Status == "complete" {
			envelope.Kind = "complete"
		}
		envelope.ArtifactStatus = document.Status
		envelope.ArtifactHash = document.Hash
		envelope.BlockedFrom = document.BlockedFrom
		envelope.CompletionNote = document.CompletionNote
		envelope.Progress = document.Progress
		repairRound := document.RepairRound
		envelope.RepairRound = &repairRound
		authorizationRequired := false
		envelope.AuthorizationRequired = &authorizationRequired
		if task := document.NextPending(); task != nil {
			envelope.NextTask = inspectTask{
				ID:   task.ID,
				Text: task.Text,
			}
		}
		handoffRound := document.RepairRound + 1
		inspectHandoff := document.Status == "blocked"
		if document.Status == "implementing" && document.RepairRound > 0 {
			handoffRound = document.RepairRound
			inspectHandoff = true
		}
		handoffPath, absoluteHandoffPath, pathErr := repair.ResolvePath(
			root,
			configuration.Artifacts.Directory,
			workItemID,
			handoffRound,
		)
		if pathErr != nil {
			return writeInspectFailure(
				stdout,
				stderr,
				jsonOutput,
				root,
				workItemID,
				"unsafe_project_root",
				pathErr.Error(),
				exitUnsafeProject,
				diagnostic,
				warnings,
			)
		}
		envelope.HandoffPath = handoffPath
		envelope.HandoffValidation = "not_required"
		if inspectHandoff {
			validation := repair.ValidateFile(
				absoluteHandoffPath,
				workItemID,
				handoffRound,
			)
			envelope.HandoffValidation = validation.State
			if validation.Handoff.SchemaVersion != 0 {
				envelope.BlockerCount = len(validation.Handoff.Blockers)
				envelope.Blockers = validation.Handoff.Blockers
				envelope.CandidateStrategy =
					validation.Handoff.CandidateStrategy
			}
			if document.Status == "blocked" {
				switch validation.State {
				case "ready":
					if planErr := repair.ValidatePlan(
						document,
						validation.Handoff,
					); planErr != nil {
						envelope.Kind = "repair_plan_required"
						envelope.Message = planErr.Error()
						break
					}
					envelope.Kind = "repair_ready"
					authorizationRequired = true
					envelope.AuthorizationRequired =
						&authorizationRequired
					envelope.NextCommand =
						validation.Handoff.NextCommand
				case "consumed":
					if document.Progress.Pending > 0 {
						envelope.Kind = "repair_recovery_required"
						envelope.Message =
							"Repair handoff was consumed before the artifact transition completed"
						authorizationRequired = true
						envelope.AuthorizationRequired =
							&authorizationRequired
						envelope.NextCommand =
							validation.Handoff.NextCommand
					} else {
						envelope.Kind = "handoff_required"
						envelope.Message =
							"Consumed repair handoff has no pending repair tasks"
					}
				default:
					envelope.Kind = "handoff_required"
					envelope.Message = validation.Reason
				}
			} else if validation.State != "consumed" {
				envelope.Kind = "invalid_repair_state"
				envelope.Message =
					"Implementing repair round requires its consumed handoff: " +
						validation.Reason
			}
		}
	}
	return writeInspectEnvelope(stdout, stderr, jsonOutput, envelope)
}

func parseInspectArguments(args []string) (string, bool, bool, error) {
	jsonOutput := false
	workItemID := ""
	for _, argument := range args {
		switch argument {
		case "--json":
			if jsonOutput {
				return "", true, false, errors.New(
					"--json may be supplied only once",
				)
			}
			jsonOutput = true
		case "-h", "--help":
			return "", jsonOutput, true, nil
		default:
			if len(argument) > 0 && argument[0] == '-' {
				return "", jsonOutput, false, fmt.Errorf(
					"unknown option %q",
					argument,
				)
			}
			if workItemID != "" {
				return "", jsonOutput, false, errors.New(
					"inspect accepts exactly one work item ID",
				)
			}
			workItemID = argument
		}
	}
	if workItemID == "" {
		return "", jsonOutput, false, errors.New(
			"inspect requires a work item ID",
		)
	}
	return workItemID, jsonOutput, false, nil
}

func writeInspectFailure(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	root string,
	workItemID string,
	kind string,
	message string,
	exitCode int,
	diagnostic codegraph.Diagnostic,
	warnings []string,
) int {
	envelope := result.Envelope{
		Command:     "inspect",
		OK:          false,
		Kind:        kind,
		Message:     message,
		ProjectRoot: root,
		WorkItemID:  workItemID,
		Warnings:    warnings,
		CodeGraph:   diagnostic,
	}
	if jsonOutput {
		if err := result.WriteJSON(stdout, envelope); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
	}
	fmt.Fprintf(stderr, "higurashi: %s\n", message)
	return exitCode
}

func writeInspectEnvelope(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	envelope result.Envelope,
) int {
	if jsonOutput {
		if err := result.WriteJSON(stdout, envelope); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
	} else {
		fmt.Fprintf(stdout, "Work item: %s\n", envelope.WorkItemID)
		fmt.Fprintf(stdout, "State: %s\n", envelope.Kind)
		fmt.Fprintf(stdout, "Project root: %s\n", envelope.ProjectRoot)
		fmt.Fprintf(
			stdout,
			"Requirement source: %s\n",
			envelope.RequirementSource,
		)
		fmt.Fprintf(stdout, "Artifact: %s\n", envelope.ArtifactPath)
		if envelope.ArtifactStatus != "" {
			fmt.Fprintf(
				stdout,
				"Artifact status: %s\n",
				envelope.ArtifactStatus,
			)
			fmt.Fprintf(stdout, "Artifact SHA-256: %s\n", envelope.ArtifactHash)
		}
		if progress, ok := envelope.Progress.(artifact.Progress); ok {
			fmt.Fprintf(
				stdout,
				"Progress: %d complete, %d pending, %d total\n",
				progress.Completed,
				progress.Pending,
				progress.Total,
			)
		}
		if task, ok := envelope.NextTask.(inspectTask); ok {
			fmt.Fprintf(stdout, "Next task: %s\n%s\n", task.ID, task.Text)
		}
		if envelope.BlockedFrom != "" {
			fmt.Fprintf(stdout, "Blocked from: %s\n", envelope.BlockedFrom)
		}
		if envelope.CompletionNote != "" {
			fmt.Fprintf(stdout, "Completion note: %s\n", envelope.CompletionNote)
		}
		if envelope.RepairRound != nil {
			fmt.Fprintf(stdout, "Repair round: %d\n", *envelope.RepairRound)
		}
		if envelope.HandoffPath != "" {
			fmt.Fprintf(stdout, "Repair handoff: %s\n", envelope.HandoffPath)
			fmt.Fprintf(
				stdout,
				"Handoff validation: %s\n",
				envelope.HandoffValidation,
			)
		}
		if envelope.BlockerCount > 0 {
			fmt.Fprintf(stdout, "Blockers: %d\n", envelope.BlockerCount)
		}
		if envelope.CandidateStrategy != "" {
			fmt.Fprintf(
				stdout,
				"Candidate strategy: %s\n",
				envelope.CandidateStrategy,
			)
		}
		if envelope.AuthorizationRequired != nil {
			fmt.Fprintf(
				stdout,
				"Explicit authorization required: %t\n",
				*envelope.AuthorizationRequired,
			)
		}
		if envelope.Message != "" {
			fmt.Fprintf(stdout, "Reason: %s\n", envelope.Message)
		}
		if envelope.NextCommand != "" {
			fmt.Fprintf(stdout, "Next command: %s\n", envelope.NextCommand)
		}
		if loop, ok := envelope.Loop.(inspectLoop); ok {
			fmt.Fprintf(
				stdout,
				"Loop limits: %d APPLY batches, %d repair attempts\n",
				loop.MaxApplyBatchesPerRun,
				loop.MaxRepairAttempts,
			)
		}
		if diagnostic, ok := envelope.CodeGraph.(codegraph.Diagnostic); ok {
			fmt.Fprintf(
				stdout,
				"CodeGraph: %s (%s)\n",
				diagnostic.Synchronization,
				diagnostic.Mode,
			)
		}
	}
	for _, warning := range envelope.Warnings {
		fmt.Fprintf(stderr, "higurashi: warning: %s\n", warning)
	}
	return exitSuccess
}

func writeInspectHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi inspect WORK-123 [--json]

Inspects a known work item, its durable artifact, checklist progress, and
CodeGraph readiness without modifying repository files.`)
}
