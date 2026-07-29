package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
)

func TestRepairAuthorizeRejectsHandoffWithoutPendingRepairTasks(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	writeArtifact(t, repository, blockedCLIDocument(false, 0))
	handoffName := filepath.Join(
		repository,
		"docs",
		"higurashi",
		"WORK-123-repair-1.md",
	)
	writeFile(t, handoffName, validCLIRepairHandoff("ready", 1))

	exitCode, result, _ := runRepairJSON(t, repository, "WORK-123")

	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5", exitCode)
	}
	if result.Kind != "authorization_rejected" {
		t.Errorf("kind = %q, want authorization_rejected", result.Kind)
	}
	if !strings.Contains(result.Message, "no new pending repair tasks") {
		t.Errorf("message = %q", result.Message)
	}
	handoff, err := os.ReadFile(handoffName)
	if err != nil {
		t.Fatalf("read rejected handoff: %v", err)
	}
	if !bytes.Contains(handoff, []byte("Handoff-Status: ready")) {
		t.Error("rejected authorization consumed the handoff")
	}
}

func TestRepairAuthorizeTransitionsAndConsumesExactlyOnce(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	original := blockedCLIDocument(true, 0)
	writeArtifact(t, repository, original)
	handoffName := filepath.Join(
		repository,
		"docs",
		"higurashi",
		"WORK-123-repair-1.md",
	)
	writeFile(t, handoffName, validCLIRepairHandoff("ready", 1))

	exitCode, result, stderr := runRepairJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr:\n%s", exitCode, stderr)
	}
	if result.Kind != "authorized" {
		t.Errorf("kind = %q, want authorized", result.Kind)
	}
	if result.ArtifactStatus != "implementing" {
		t.Errorf("artifactStatus = %q, want implementing", result.ArtifactStatus)
	}
	if result.RepairRound == nil || *result.RepairRound != 1 {
		t.Errorf("repairRound = %v, want 1", result.RepairRound)
	}
	if result.HandoffValidation != "consumed" {
		t.Errorf(
			"handoffValidation = %q, want consumed",
			result.HandoffValidation,
		)
	}
	if result.AuthorizationRequired == nil || *result.AuthorizationRequired {
		t.Errorf(
			"authorizationRequired = %v, want false",
			result.AuthorizationRequired,
		)
	}
	handoff, err := os.ReadFile(handoffName)
	if err != nil {
		t.Fatalf("read consumed handoff: %v", err)
	}
	if !bytes.Contains(handoff, []byte("Handoff-Status: consumed")) {
		t.Error("handoff was not consumed")
	}
	artifact := readArtifact(t, repository)
	if !strings.Contains(
		artifact,
		"Narrative remains unchanged.",
	) && !strings.Contains(
		artifact,
		"Existing narrative remains unchanged.",
	) {
		t.Error("authorization lost existing narrative")
	}
	if !strings.Contains(
		artifact,
		"- [x] **task-001 — Finish initial implementation:** complete.",
	) {
		t.Error("authorization changed completed task evidence")
	}

	secondExit, second, secondStderr := runRepairJSON(
		t,
		repository,
		"WORK-123",
	)
	if secondExit != 0 {
		t.Fatalf(
			"second exit code = %d, want 0\nstderr:\n%s",
			secondExit,
			secondStderr,
		)
	}
	if second.Kind != "unchanged" {
		t.Errorf("second kind = %q, want unchanged", second.Kind)
	}
}

func TestInspectConsumedPartialAuthorizationOffersExactRecoveryCommand(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	writeArtifact(t, repository, blockedCLIDocument(true, 0))
	writeFile(
		t,
		filepath.Join(
			repository,
			"docs",
			"higurashi",
			"WORK-123-repair-1.md",
		),
		validCLIRepairHandoff("consumed", 1),
	)

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "repair_recovery_required" {
		t.Errorf("kind = %q, want repair_recovery_required", result.Kind)
	}
	if result.NextCommand != "higurashi repair authorize WORK-123" {
		t.Errorf("nextCommand = %q", result.NextCommand)
	}
	if result.AuthorizationRequired == nil || !*result.AuthorizationRequired {
		t.Errorf(
			"authorizationRequired = %v, want true",
			result.AuthorizationRequired,
		)
	}

	repairExit, repaired, stderr := runRepairJSON(
		t,
		repository,
		"WORK-123",
	)
	if repairExit != 0 {
		t.Fatalf("recovery exit = %d\nstderr:\n%s", repairExit, stderr)
	}
	if repaired.Kind != "recovered" {
		t.Errorf("repair kind = %q, want recovered", repaired.Kind)
	}
}

func TestBlockedTextInspectionPrintsExactlyOneExecutableNextCommand(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	writeArtifact(t, repository, blockedCLIDocument(true, 0))
	writeFile(
		t,
		filepath.Join(
			repository,
			"docs",
			"higurashi",
			"WORK-123-repair-1.md",
		),
		validCLIRepairHandoff("ready", 1),
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := cli.Run(
		context.Background(),
		[]string{"inspect", "WORK-123"},
		&stdout,
		&stderr,
		cli.Options{
			WorkingDirectory: repository,
			Version:          "test",
			CommandRunner: &doctorRunner{
				projectRoot: repository,
			},
		},
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d\nstderr:\n%s", exitCode, stderr.String())
	}
	command := "higurashi repair authorize WORK-123"
	if strings.Count(stdout.String(), command) != 1 {
		t.Errorf(
			"stdout contains command %d times, want exactly once\n%s",
			strings.Count(stdout.String(), command),
			stdout.String(),
		)
	}
}

func TestInspectImplementingRepairRequiresConsumedHandoff(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	document := strings.Replace(
		blockedCLIDocument(true, 1),
		`Status: blocked
Repair-Round: 1
Blocked-From: verifying
Blocker-Reason: independent-review-blocker`,
		`Status: implementing
Repair-Round: 1`,
		1,
	)
	writeArtifact(t, repository, document)

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "invalid_repair_state" {
		t.Errorf("kind = %q, want invalid_repair_state", result.Kind)
	}
	if result.HandoffValidation != "missing" {
		t.Errorf("handoffValidation = %q, want missing", result.HandoffValidation)
	}
}

type repairJSONResult struct {
	SchemaVersion         int    `json:"schemaVersion"`
	Command               string `json:"command"`
	OK                    bool   `json:"ok"`
	Kind                  string `json:"kind"`
	Message               string `json:"message"`
	WorkItemID            string `json:"workItemId"`
	ArtifactStatus        string `json:"artifactStatus"`
	ArtifactHash          string `json:"artifactHash"`
	RepairRound           *int   `json:"repairRound"`
	HandoffPath           string `json:"handoffPath"`
	HandoffValidation     string `json:"handoffValidation"`
	BlockerCount          int    `json:"blockerCount"`
	AuthorizationRequired *bool  `json:"authorizationRequired"`
	CandidateStrategy     string `json:"candidateStrategy"`
}

func runRepairJSON(
	t *testing.T,
	workingDirectory string,
	workItemID string,
) (int, repairJSONResult, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		[]string{"repair", "authorize", workItemID, "--json"},
		&stdout,
		&stderr,
		cli.Options{
			WorkingDirectory: workingDirectory,
			Version:          "test",
			CommandRunner: &doctorRunner{
				projectRoot: workingDirectory,
			},
		},
	)
	var result repairJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf(
			"decode repair JSON: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return exitCode, result, stderr.String()
}
