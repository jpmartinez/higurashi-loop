package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
)

func TestTransitionMovesPlannedArtifactToImplementing(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	original := `# WORK-123 — Guard state transitions

Higurashi-Schema: 1
Status: planned
Repair-Round: 0

## Contract

Narrative content must remain byte-for-byte unchanged.

## Ordered vertical TDD checklist

- [ ] **task-001 — Implement the behavior:** pending.
	`
	writeArtifact(t, repository, original)
	artifactName := filepath.Join(
		repository,
		"docs",
		"higurashi",
		"WORK-123.md",
	)
	if err := os.Chmod(artifactName, 0o640); err != nil {
		t.Fatalf("set artifact mode: %v", err)
	}

	exitCode, result, stderr := runTransitionJSON(
		t,
		repository,
		"WORK-123",
		"implementing",
		hashText(original),
		"",
	)

	if exitCode != 0 {
		t.Fatalf(
			"exit code = %d, want 0\nstderr:\n%s",
			exitCode,
			stderr,
		)
	}
	if !result.OK {
		t.Error("ok = false, want true")
	}
	if result.Kind != "transitioned" {
		t.Errorf("kind = %q, want %q", result.Kind, "transitioned")
	}
	if result.ArtifactStatus != "implementing" {
		t.Errorf(
			"artifactStatus = %q, want %q",
			result.ArtifactStatus,
			"implementing",
		)
	}

	want := strings.Replace(
		original,
		"Status: planned",
		"Status: implementing",
		1,
	)
	actual := readArtifact(t, repository)
	if actual != want {
		t.Errorf("artifact changed unexpectedly\nwant:\n%s\nactual:\n%s", want, actual)
	}
	if result.ArtifactHash != hashText(want) {
		t.Errorf(
			"artifactHash = %q, want %q",
			result.ArtifactHash,
			hashText(want),
		)
	}
	info, err := os.Stat(artifactName)
	if err != nil {
		t.Fatalf("stat transitioned artifact: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf(
			"artifact mode = %o, want 640",
			info.Mode().Perm(),
		)
	}
}

func TestTransitionRejectsStaleExpectedHashWithoutWriting(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	original := `# WORK-123 — Guard state transitions

Higurashi-Schema: 1
Status: planned

## Ordered vertical TDD checklist

- [ ] **task-001 — Implement the behavior:** pending.
`
	writeArtifact(t, repository, original)

	exitCode, result, _ := runTransitionJSON(
		t,
		repository,
		"WORK-123",
		"implementing",
		strings.Repeat("0", 64),
		"",
	)

	if exitCode != 7 {
		t.Errorf("exit code = %d, want 7", exitCode)
	}
	if result.Kind != "stale_hash" {
		t.Errorf("kind = %q, want %q", result.Kind, "stale_hash")
	}
	if actual := readArtifact(t, repository); actual != original {
		t.Errorf("artifact was modified after stale hash\nactual:\n%s", actual)
	}
}

func TestTransitionAcceptsEveryLegalStateChange(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		blockedFrom string
		target      string
		allComplete bool
	}{
		{"refined to planned", "refined", "", "planned", false},
		{"refined to blocked", "refined", "", "blocked", false},
		{"planned to implementing", "planned", "", "implementing", false},
		{"planned to blocked", "planned", "", "blocked", false},
		{"implementing to verifying", "implementing", "", "verifying", true},
		{"implementing to blocked", "implementing", "", "blocked", false},
		{"verifying to complete", "verifying", "", "complete", true},
		{"verifying to blocked", "verifying", "", "blocked", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newGitRepository(t)
			writeInspectFixture(t, repository)
			document := transitionArtifact(
				test.current,
				test.blockedFrom,
				test.allComplete,
			)
			writeArtifact(t, repository, document)
			reason := ""
			if test.target == "blocked" {
				reason = "bounded-test-blocker"
			}

			exitCode, result, stderr := runTransitionJSON(
				t,
				repository,
				"WORK-123",
				test.target,
				hashText(document),
				reason,
			)

			if exitCode != 0 {
				t.Fatalf(
					"exit code = %d, want 0\nstderr:\n%s",
					exitCode,
					stderr,
				)
			}
			if result.Kind != "transitioned" {
				t.Errorf(
					"kind = %q, want %q",
					result.Kind,
					"transitioned",
				)
			}
			if result.ArtifactStatus != test.target {
				t.Errorf(
					"artifactStatus = %q, want %q",
					result.ArtifactStatus,
					test.target,
				)
			}
			if test.target == "blocked" &&
				result.BlockedFrom != test.current {
				t.Errorf(
					"blockedFrom = %q, want %q",
					result.BlockedFrom,
					test.current,
				)
			}
			if test.target == "blocked" {
				if result.RepairRound == nil || *result.RepairRound != 0 {
					t.Errorf("repairRound = %v, want 0", result.RepairRound)
				}
				if result.HandoffPath !=
					"docs/higurashi/WORK-123-repair-1.md" {
					t.Errorf("handoffPath = %q", result.HandoffPath)
				}
				if result.HandoffValidation != "missing" {
					t.Errorf(
						"handoffValidation = %q, want missing",
						result.HandoffValidation,
					)
				}
				if result.AuthorizationRequired == nil ||
					*result.AuthorizationRequired {
					t.Errorf(
						"authorizationRequired = %v, want false",
						result.AuthorizationRequired,
					)
				}
			}
			if !strings.Contains(
				readArtifact(t, repository),
				"Narrative sentinel: preserve exactly.",
			) {
				t.Error("narrative sentinel was not preserved")
			}
		})
	}
}

func TestTransitionRejectsIllegalStateChangesWithoutWriting(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		blockedFrom string
		target      string
		allComplete bool
	}{
		{"refined skips planning", "refined", "", "implementing", false},
		{"planned skips implementation", "planned", "", "verifying", true},
		{"completion outside verification", "implementing", "", "complete", true},
		{"verifying moves backward", "verifying", "", "planned", true},
		{"complete is terminal", "complete", "", "blocked", true},
		{"blocked resumes to wrong state", "blocked", "planned", "implementing", false},
		{"blocked normal resume requires repair authorization", "blocked", "planned", "planned", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newGitRepository(t)
			writeInspectFixture(t, repository)
			document := transitionArtifact(
				test.current,
				test.blockedFrom,
				test.allComplete,
			)
			writeArtifact(t, repository, document)
			reason := ""
			if test.target == "blocked" {
				reason = "bounded-test-blocker"
			}

			exitCode, result, _ := runTransitionJSON(
				t,
				repository,
				"WORK-123",
				test.target,
				hashText(document),
				reason,
			)

			if exitCode != 5 {
				t.Errorf("exit code = %d, want 5", exitCode)
			}
			if result.Kind != "illegal_transition" {
				t.Errorf(
					"kind = %q, want %q",
					result.Kind,
					"illegal_transition",
				)
			}
			if actual := readArtifact(t, repository); actual != document {
				t.Error("illegal transition modified the artifact")
			}
		})
	}
}

func TestTransitionRejectsVerifyingWithPendingTask(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	document := transitionArtifact("implementing", "", false)
	writeArtifact(t, repository, document)

	exitCode, result, _ := runTransitionJSON(
		t,
		repository,
		"WORK-123",
		"verifying",
		hashText(document),
		"",
	)

	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5", exitCode)
	}
	if result.Kind != "illegal_transition" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"illegal_transition",
		)
	}
	if !strings.Contains(result.Message, "zero pending tasks") {
		t.Errorf(
			"message = %q, want pending-task invariant",
			result.Message,
		)
	}
	if actual := readArtifact(t, repository); actual != document {
		t.Error("failed verification transition modified the artifact")
	}
}

func TestTransitionRequiresReasonWhenEnteringBlocked(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	document := transitionArtifact("implementing", "", false)
	writeArtifact(t, repository, document)

	exitCode, result, _ := runTransitionJSON(
		t,
		repository,
		"WORK-123",
		"blocked",
		hashText(document),
		"",
	)

	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5", exitCode)
	}
	if result.Kind != "illegal_transition" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"illegal_transition",
		)
	}
	if !strings.Contains(result.Message, "Blocker-Reason") {
		t.Errorf(
			"message = %q, want missing reason diagnostic",
			result.Message,
		)
	}
	if actual := readArtifact(t, repository); actual != document {
		t.Error("missing-reason transition modified the artifact")
	}
}

func TestTransitionIdempotentStatusDoesNotReplaceArtifact(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	document := transitionArtifact("planned", "", false)
	writeArtifact(t, repository, document)
	name := filepath.Join(
		repository,
		"docs",
		"higurashi",
		"WORK-123.md",
	)
	before, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat artifact before transition: %v", err)
	}

	exitCode, result, _ := runTransitionJSON(
		t,
		repository,
		"WORK-123",
		"planned",
		hashText(document),
		"",
	)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "unchanged" {
		t.Errorf("kind = %q, want %q", result.Kind, "unchanged")
	}
	after, err := os.Stat(name)
	if err != nil {
		t.Fatalf("stat artifact after transition: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Error("idempotent transition replaced the artifact file")
	}
	if actual := readArtifact(t, repository); actual != document {
		t.Error("idempotent transition changed artifact content")
	}
}

func TestTransitionRequiresExpectedHashArgument(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := cli.Run(
		context.Background(),
		[]string{"transition", "WORK-123", "planned", "--json"},
		&stdout,
		&stderr,
		cli.Options{Version: "test"},
	)

	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2", exitCode)
	}
	var result transitionJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf(
			"decode JSON result: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	if result.Kind != "invalid_usage" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"invalid_usage",
		)
	}
}

type transitionJSONResult struct {
	SchemaVersion         int    `json:"schemaVersion"`
	Command               string `json:"command"`
	OK                    bool   `json:"ok"`
	Kind                  string `json:"kind"`
	Message               string `json:"message"`
	ProjectRoot           string `json:"projectRoot"`
	WorkItemID            string `json:"workItemId"`
	ArtifactPath          string `json:"artifactPath"`
	ArtifactStatus        string `json:"artifactStatus"`
	ArtifactHash          string `json:"artifactHash"`
	BlockedFrom           string `json:"blockedFrom"`
	RepairRound           *int   `json:"repairRound"`
	HandoffPath           string `json:"handoffPath"`
	HandoffValidation     string `json:"handoffValidation"`
	AuthorizationRequired *bool  `json:"authorizationRequired"`
	Progress              struct {
		Completed int `json:"completed"`
		Pending   int `json:"pending"`
		Total     int `json:"total"`
	} `json:"progress"`
}

func runTransitionJSON(
	t *testing.T,
	workingDirectory string,
	workItemID string,
	targetStatus string,
	expectedHash string,
	reason string,
) (int, transitionJSONResult, string) {
	t.Helper()

	args := []string{
		"transition",
		workItemID,
		targetStatus,
		"--expected-hash",
		expectedHash,
	}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	args = append(args, "--json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		args,
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

	var result transitionJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf(
			"decode JSON result: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return exitCode, result, stderr.String()
}

func hashText(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func readArtifact(t *testing.T, repository string) string {
	t.Helper()
	name := filepath.Join(
		repository,
		"docs",
		"higurashi",
		"WORK-123.md",
	)
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	return string(data)
}

func transitionArtifact(
	status string,
	blockedFrom string,
	allComplete bool,
) string {
	fields := "Status: " + status + "\n"
	fields += "Repair-Round: 0\n"
	if status == "blocked" {
		fields += "Blocked-From: " + blockedFrom + "\n"
		fields += "Blocker-Reason: bounded-test-blocker\n"
	}
	marker := " "
	if allComplete {
		marker = "x"
	}
	return `# WORK-123 — Guard state transitions

Higurashi-Schema: 1
` + fields + `
## Contract

Narrative sentinel: preserve exactly.

## Ordered vertical TDD checklist

- [` + marker + `] **task-001 — Guard the transition:** observable evidence.
`
}
