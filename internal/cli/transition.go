package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/artifact"
	"github.com/jpmartinez/higurashi-loop/internal/codegraph"
	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/repair"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

type transitionArguments struct {
	WorkItemID   string
	Target       string
	ExpectedHash string
	Reason       string
	Deferrals    []repair.Deferral
	JSON         bool
	Help         bool
}

func runTransition(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	arguments, err := parseTransitionArguments(args)
	if arguments.Help {
		writeTransitionHelp(stdout)
		return exitSuccess
	}
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			"transition",
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
			"transition",
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
			"transition",
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
			"transition",
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
			return writeTransitionFailure(
				stdout,
				stderr,
				arguments.JSON,
				root,
				arguments.WorkItemID,
				"",
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
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			"",
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
		arguments.WorkItemID,
	)
	if err != nil {
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			"",
			"unsafe_project_root",
			err.Error(),
			exitUnsafeProject,
			diagnostic,
			warnings,
		)
	}
	mutationPath, err := filepath.EvalSymlinks(absoluteArtifactPath)
	if err != nil {
		kind := "invalid_artifact"
		exitCode := exitInvalidArtifact
		if !errors.Is(err, os.ErrNotExist) {
			kind = "unsafe_project_root"
			exitCode = exitUnsafeProject
		}
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			kind,
			fmt.Sprintf("resolve artifact for mutation: %v", err),
			exitCode,
			diagnostic,
			warnings,
		)
	}
	if err := project.ValidateMutationPath(root, mutationPath); err != nil {
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			"unsafe_project_root",
			err.Error(),
			exitUnsafeProject,
			diagnostic,
			warnings,
		)
	}

	snapshot, err := artifact.Load(mutationPath, arguments.WorkItemID)
	if err != nil {
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			"invalid_artifact",
			err.Error(),
			exitInvalidArtifact,
			diagnostic,
			warnings,
		)
	}
	if snapshot.Document.Hash != arguments.ExpectedHash {
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			"stale_hash",
			"artifact content hash does not match --expected-hash",
			exitConflict,
			diagnostic,
			warnings,
		)
	}
	handoffPath, absoluteHandoffPath, err := repair.ResolvePath(
		root,
		configuration.Artifacts.Directory,
		arguments.WorkItemID,
		snapshot.Document.RepairRound+1,
	)
	if err != nil {
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			"unsafe_project_root",
			err.Error(),
			exitUnsafeProject,
			diagnostic,
			warnings,
		)
	}

	completionNote := ""
	followUpRequirements := []string(nil)
	changedPaths := []string(nil)
	var change artifact.Change
	if len(arguments.Deferrals) > 0 {
		validation := repair.ValidateFile(
			absoluteHandoffPath,
			arguments.WorkItemID,
			snapshot.Document.RepairRound+1,
		)
		if err := repair.ValidateDeferrals(
			validation.Handoff,
			arguments.Deferrals,
		); err != nil {
			return writeTransitionFailure(
				stdout,
				stderr,
				arguments.JSON,
				root,
				arguments.WorkItemID,
				artifactPath,
				"invalid_repair_state",
				err.Error(),
				exitInvalidArtifact,
				diagnostic,
				warnings,
			)
		}
		for _, deferral := range arguments.Deferrals {
			if err := workitem.ValidateID(
				configuration.WorkItems.IDPattern,
				deferral.FollowUpID,
			); err != nil {
				return writeTransitionFailure(
					stdout,
					stderr,
					arguments.JSON,
					root,
					arguments.WorkItemID,
					artifactPath,
					"invalid_repair_state",
					err.Error(),
					exitInvalidArtifact,
					diagnostic,
					warnings,
				)
			}
		}
		completionNote = repair.CompletionNote(
			validation.Handoff,
			arguments.Deferrals,
		)
		change, err = artifact.CompleteWithNote(snapshot, completionNote)
		if err == nil {
			changedPaths, err = materializeDeferredFollowUps(
				root,
				configuration,
				arguments.WorkItemID,
				validation.Handoff,
				arguments.Deferrals,
			)
		}
		followUpRequirements = make([]string, 0, len(arguments.Deferrals))
		for _, deferral := range arguments.Deferrals {
			followUpRequirements = append(
				followUpRequirements,
				deferral.FollowUpID,
			)
		}
	} else {
		change, err = artifact.Transition(
			snapshot,
			arguments.Target,
			arguments.Reason,
		)
	}
	if err != nil {
		kind := "invalid_artifact"
		if errors.Is(err, artifact.ErrIllegalTransition) {
			kind = "illegal_transition"
		}
		if len(arguments.Deferrals) > 0 &&
			!errors.Is(err, artifact.ErrIllegalTransition) {
			kind = "follow_up_write_failed"
			if errors.Is(err, errRequirementConflict) ||
				errors.Is(err, workitem.ErrConflict) {
				kind = "conflict"
			}
			if errors.Is(err, repair.ErrInvalidDeferral) {
				kind = "invalid_repair_state"
			}
			if errors.Is(err, project.ErrPathEscapesRoot) ||
				errors.Is(err, project.ErrUnsafeMutationPath) {
				kind = "unsafe_project_root"
			}
		}
		return writeTransitionFailure(
			stdout,
			stderr,
			arguments.JSON,
			root,
			arguments.WorkItemID,
			artifactPath,
			kind,
			err.Error(),
			exitInvalidArtifact,
			diagnostic,
			warnings,
		)
	}
	if change.Changed {
		if err := artifact.WriteAtomic(
			mutationPath,
			change.Content,
			snapshot.Mode,
		); err != nil {
			return writeTransitionFailure(
				stdout,
				stderr,
				arguments.JSON,
				root,
				arguments.WorkItemID,
				artifactPath,
				"write_failed",
				err.Error(),
				exitInternalError,
				diagnostic,
				warnings,
			)
		}
	}

	kind := "transitioned"
	if !change.Changed {
		kind = "unchanged"
	}
	repairRound := change.Document.RepairRound
	authorizationRequired := false
	handoffValidation := "not_required"
	blockerCount := 0
	candidateStrategy := ""
	nextCommand := ""
	if change.Document.Status == "blocked" {
		validation := repair.ValidateFile(
			absoluteHandoffPath,
			arguments.WorkItemID,
			change.Document.RepairRound+1,
		)
		handoffValidation = validation.State
		blockerCount = len(validation.Handoff.Blockers)
		candidateStrategy = validation.Handoff.CandidateStrategy
		if validation.State == "ready" {
			authorizationRequired = true
			nextCommand = validation.Handoff.NextCommand
		}
	}
	envelope := result.Envelope{
		Command:               "transition",
		OK:                    true,
		Kind:                  kind,
		ProjectRoot:           root,
		WorkItemID:            arguments.WorkItemID,
		RequirementSource:     match.Source,
		ArtifactPath:          artifactPath,
		ArtifactStatus:        change.Document.Status,
		ArtifactHash:          change.Document.Hash,
		BlockedFrom:           change.Document.BlockedFrom,
		RepairRound:           &repairRound,
		HandoffPath:           handoffPath,
		HandoffValidation:     handoffValidation,
		BlockerCount:          blockerCount,
		AuthorizationRequired: &authorizationRequired,
		NextCommand:           nextCommand,
		CandidateStrategy:     candidateStrategy,
		CompletionNote:        change.Document.CompletionNote,
		FollowUpRequirements:  followUpRequirements,
		ChangedPaths:          changedPaths,
		Progress:              change.Document.Progress,
		Warnings:              warnings,
		CodeGraph:             diagnostic,
	}
	return writeTransitionResult(
		stdout,
		stderr,
		arguments.JSON,
		envelope,
	)
}

func materializeDeferredFollowUps(
	root string,
	configuration config.Config,
	workItemID string,
	handoff repair.Document,
	deferrals []repair.Deferral,
) ([]string, error) {
	managedDirectory := path.Join(
		configuration.Artifacts.Directory,
		"requirements",
	)
	nextConfiguration := configuration
	configurationChanged := !slices.Contains(
		nextConfiguration.WorkItems.RequirementSources,
		managedDirectory,
	)
	if configurationChanged {
		nextConfiguration.WorkItems.RequirementSources = append(
			append([]string(nil), nextConfiguration.WorkItems.RequirementSources...),
			managedDirectory,
		)
	}

	blockers := make(map[string]repair.Blocker, len(handoff.Blockers))
	for _, blocker := range handoff.Blockers {
		blockers[blocker.ID] = blocker
	}
	type followUp struct {
		path    string
		content []byte
	}
	followUps := make([]followUp, 0, len(deferrals))
	for _, deferral := range deferrals {
		blocker := blockers[deferral.BlockerID]
		requirementPath := path.Join(
			managedDirectory,
			deferral.FollowUpID+".md",
		)
		body := repair.FollowUpBody(
			workItemID,
			deferral.FollowUpID,
			blocker,
		)
		sourceLabel := workItemID + "#" + blocker.ID
		sourceHash := fmt.Sprintf("%x", sha256.Sum256(body))
		content := importedRequirementDocument(
			deferral.FollowUpID,
			"blocker-follow-up",
			sourceLabel,
			sourceHash,
			body,
		)

		match, err := workitem.Find(
			root,
			configuration.WorkItems.RequirementSources,
			deferral.FollowUpID,
		)
		if err == nil && match.Source != requirementPath {
			return nil, fmt.Errorf(
				"follow-up work item %s already exists at %s",
				deferral.FollowUpID,
				match.Source,
			)
		}
		if err != nil && !errors.Is(err, workitem.ErrUnknown) {
			return nil, err
		}
		followUps = append(followUps, followUp{
			path:    requirementPath,
			content: content,
		})
	}

	var changedPaths []string
	for _, followUp := range followUps {
		created, err := createRequirementSnapshot(
			root,
			followUp.path,
			followUp.content,
		)
		if err != nil {
			return nil, fmt.Errorf("create deferred follow-up: %w", err)
		}
		if created {
			changedPaths = append(changedPaths, followUp.path)
		}
	}
	if configurationChanged {
		encoded, err := config.Encode(nextConfiguration)
		if err != nil {
			return nil, fmt.Errorf("encode follow-up configuration: %w", err)
		}
		if err := replaceProjectConfig(root, encoded); err != nil {
			return nil, fmt.Errorf("update requirement sources for follow-ups: %w", err)
		}
		changedPaths = append(changedPaths, ".higurashi/config.json")
	}
	slices.Sort(changedPaths)
	return changedPaths, nil
}

func parseTransitionArguments(args []string) (transitionArguments, error) {
	var parsed transitionArguments
	var positionals []string
	expectedHashSeen := false
	reasonSeen := false
	seenDeferrals := map[string]bool{}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--json":
			if parsed.JSON {
				return parsed, errors.New("--json may be supplied only once")
			}
			parsed.JSON = true
		case "-h", "--help":
			parsed.Help = true
			return parsed, nil
		case "--expected-hash":
			if expectedHashSeen {
				return parsed, errors.New(
					"--expected-hash may be supplied only once",
				)
			}
			if index+1 >= len(args) {
				return parsed, errors.New("--expected-hash requires a value")
			}
			index++
			parsed.ExpectedHash = args[index]
			expectedHashSeen = true
		case "--reason":
			if reasonSeen {
				return parsed, errors.New("--reason may be supplied only once")
			}
			if index+1 >= len(args) {
				return parsed, errors.New("--reason requires a value")
			}
			index++
			parsed.Reason = args[index]
			reasonSeen = true
		case "--defer-blocker":
			if index+1 >= len(args) {
				return parsed, errors.New("--defer-blocker requires a value")
			}
			index++
			blockerID, followUpID, ok := strings.Cut(args[index], "=")
			if !ok || blockerID == "" || followUpID == "" {
				return parsed, errors.New(
					"--defer-blocker requires BLOCKER-ID=FOLLOW-UP-ID",
				)
			}
			if seenDeferrals[blockerID] {
				return parsed, fmt.Errorf(
					"--defer-blocker may name %q only once",
					blockerID,
				)
			}
			seenDeferrals[blockerID] = true
			parsed.Deferrals = append(parsed.Deferrals, repair.Deferral{
				BlockerID:  blockerID,
				FollowUpID: followUpID,
			})
		default:
			if strings.HasPrefix(argument, "-") {
				return parsed, fmt.Errorf("unknown option %q", argument)
			}
			positionals = append(positionals, argument)
		}
	}
	if len(positionals) != 2 {
		return parsed, errors.New(
			"transition requires exactly one work item ID and target status",
		)
	}
	parsed.WorkItemID = positionals[0]
	parsed.Target = positionals[1]
	if len(parsed.Deferrals) > 0 && parsed.Target != "complete" {
		return parsed, errors.New(
			"--defer-blocker is valid only when targeting complete",
		)
	}
	if len(parsed.Deferrals) > 0 && reasonSeen {
		return parsed, errors.New(
			"--defer-blocker cannot be combined with --reason",
		)
	}
	if !expectedHashSeen {
		return parsed, errors.New("--expected-hash is required")
	}
	if !validArtifactHash(parsed.ExpectedHash) {
		return parsed, errors.New(
			"--expected-hash must be 64 lowercase hexadecimal characters",
		)
	}
	return parsed, nil
}

func validArtifactHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, character := range hash {
		if !(character >= '0' && character <= '9') &&
			!(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func writeTransitionFailure(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	root string,
	workItemID string,
	artifactPath string,
	kind string,
	message string,
	exitCode int,
	diagnostic codegraph.Diagnostic,
	warnings []string,
) int {
	envelope := result.Envelope{
		Command:      "transition",
		OK:           false,
		Kind:         kind,
		Message:      message,
		ProjectRoot:  root,
		WorkItemID:   workItemID,
		ArtifactPath: artifactPath,
		Warnings:     warnings,
		CodeGraph:    diagnostic,
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

func writeTransitionResult(
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
		fmt.Fprintf(stdout, "Result: %s\n", envelope.Kind)
		fmt.Fprintf(stdout, "Artifact: %s\n", envelope.ArtifactPath)
		fmt.Fprintf(stdout, "Status: %s\n", envelope.ArtifactStatus)
		fmt.Fprintf(stdout, "Artifact SHA-256: %s\n", envelope.ArtifactHash)
		if envelope.CompletionNote != "" {
			fmt.Fprintf(stdout, "Completion note: %s\n", envelope.CompletionNote)
		}
		for _, followUp := range envelope.FollowUpRequirements {
			fmt.Fprintf(stdout, "Follow-up requirement: %s\n", followUp)
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
	}
	for _, warning := range envelope.Warnings {
		fmt.Fprintf(stderr, "higurashi: warning: %s\n", warning)
	}
	return exitSuccess
}

func writeTransitionHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi transition WORK-123 STATUS --expected-hash HASH [--reason TEXT]
    [--defer-blocker BLOCKER-ID=FOLLOW-UP-ID ...] [--json]

Validates the current artifact and expected SHA-256 hash, enforces the legal
state graph and checklist invariants, then atomically updates only machine-owned
fields. Use --reason when entering blocked. To complete blocked work, defer
every classified blocker explicitly with a named follow-up work item; Higurashi
will create those follow-up requirements and retain the unresolved decision.`)
}
