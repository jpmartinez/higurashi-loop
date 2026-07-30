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

func TestInspectReportsReadyForKnownWorkItemWithoutArtifact(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectConfig(t, repository)
	writeFile(
		t,
		filepath.Join(repository, "docs", "requirements.md"),
		"# Requirements\n\n## WORK-123 — Add deterministic inspection\n",
	)

	exitCode, result, stderr := runInspectJSON(t, repository, "WORK-123")

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
	if result.Kind != "ready" {
		t.Errorf("kind = %q, want %q", result.Kind, "ready")
	}
	if result.WorkItemID != "WORK-123" {
		t.Errorf("workItemId = %q, want %q", result.WorkItemID, "WORK-123")
	}
	if result.RequirementSource != "docs/requirements.md" {
		t.Errorf(
			"requirementSource = %q, want %q",
			result.RequirementSource,
			"docs/requirements.md",
		)
	}
	if result.ArtifactPath != "docs/higurashi/WORK-123.md" {
		t.Errorf(
			"artifactPath = %q, want %q",
			result.ArtifactPath,
			"docs/higurashi/WORK-123.md",
		)
	}
	if result.Progress.Total != 0 ||
		result.Progress.Completed != 0 ||
		result.Progress.Pending != 0 {
		t.Errorf("progress = %#v, want all zero", result.Progress)
	}
	if result.Loop.MaxApplyBatchesPerRun != 8 {
		t.Errorf(
			"loop.maxApplyBatchesPerRun = %d, want 8",
			result.Loop.MaxApplyBatchesPerRun,
		)
	}
	if result.Loop.MaxRepairAttempts != 1 {
		t.Errorf(
			"loop.maxRepairAttempts = %d, want 1",
			result.Loop.MaxRepairAttempts,
		)
	}
	if !result.Loop.RequireProgressAfterEveryBatch {
		t.Error("loop.requireProgressAfterEveryBatch = false, want true")
	}
	if len(result.Warnings) == 0 {
		t.Error("warnings is empty for preferred CodeGraph without an index")
	}
}

func TestInspectReportsResumeWithProgressHashAndMultilineNextTask(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	document := `# WORK-123 — Add deterministic inspection

Higurashi-Schema: 1
Status: implementing

## Ordered vertical TDD checklist

- [x] **task-001 — Return ready:** prove the absent-artifact path.
- [ ] **task-002 — Parse the artifact:** preserve all task text.
  This continuation remains part of task-002.
- [ ] **task-003 — Report completion:** return a terminal result.

## Verification

Pending.
`
	writeFile(
		t,
		filepath.Join(repository, "docs", "higurashi", "WORK-123.md"),
		document,
	)

	exitCode, result, stderr := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf(
			"exit code = %d, want 0\nstderr:\n%s",
			exitCode,
			stderr,
		)
	}
	if result.Kind != "resume" {
		t.Errorf("kind = %q, want %q", result.Kind, "resume")
	}
	if result.ArtifactStatus != "implementing" {
		t.Errorf(
			"artifactStatus = %q, want %q",
			result.ArtifactStatus,
			"implementing",
		)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(document)))
	if result.ArtifactHash != hash {
		t.Errorf("artifactHash = %q, want %q", result.ArtifactHash, hash)
	}
	if result.Progress.Completed != 1 ||
		result.Progress.Pending != 2 ||
		result.Progress.Total != 3 {
		t.Errorf("progress = %#v, want 1 complete and 2 pending", result.Progress)
	}
	if result.NextTask == nil {
		t.Fatal("nextTask is nil")
	}
	if result.NextTask.ID != "task-002" {
		t.Errorf("nextTask.id = %q, want %q", result.NextTask.ID, "task-002")
	}
	wantText := "Parse the artifact: preserve all task text.\n" +
		"  This continuation remains part of task-002."
	if result.NextTask.Text != wantText {
		t.Errorf("nextTask.text = %q, want %q", result.NextTask.Text, wantText)
	}
	if result.RepairRound == nil || *result.RepairRound != 0 {
		t.Errorf("repairRound = %v, want 0", result.RepairRound)
	}
	if result.HandoffPath != "docs/higurashi/WORK-123-repair-1.md" {
		t.Errorf("handoffPath = %q", result.HandoffPath)
	}
	if result.HandoffValidation != "not_required" {
		t.Errorf(
			"handoffValidation = %q, want not_required",
			result.HandoffValidation,
		)
	}
}

func TestInspectReportsTerminalComplete(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	writeArtifact(t, repository, `# WORK-123 — Add deterministic inspection

Higurashi-Schema: 1
Status: complete

## Ordered vertical TDD checklist

- [x] **task-001 — Finish the behavior:** all evidence is green.
`)

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "complete" {
		t.Errorf("kind = %q, want %q", result.Kind, "complete")
	}
	if result.Progress.Pending != 0 || result.NextTask != nil {
		t.Errorf(
			"progress = %#v, nextTask = %#v; want no pending task",
			result.Progress,
			result.NextTask,
		)
	}
}

func TestInspectBlockedWithoutHandoffRequiresDurableHandoff(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	writeArtifact(t, repository, blockedCLIDocument(false, 0))

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "handoff_required" {
		t.Errorf("kind = %q, want handoff_required", result.Kind)
	}
	if result.RepairRound == nil || *result.RepairRound != 0 {
		t.Errorf("repairRound = %v, want 0", result.RepairRound)
	}
	if result.HandoffPath != "docs/higurashi/WORK-123-repair-1.md" {
		t.Errorf("handoffPath = %q", result.HandoffPath)
	}
	if result.HandoffValidation != "missing" {
		t.Errorf("handoffValidation = %q, want missing", result.HandoffValidation)
	}
	if result.AuthorizationRequired == nil || *result.AuthorizationRequired {
		t.Errorf(
			"authorizationRequired = %v, want false",
			result.AuthorizationRequired,
		)
	}
	if result.NextCommand != "" {
		t.Errorf("nextCommand = %q, want empty", result.NextCommand)
	}
}

func TestInspectValidHandoffReturnsRepairReadyWithOneCommand(t *testing.T) {
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

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "repair_ready" {
		t.Errorf("kind = %q, want repair_ready", result.Kind)
	}
	if result.HandoffValidation != "ready" {
		t.Errorf("handoffValidation = %q, want ready", result.HandoffValidation)
	}
	if result.BlockerCount != 1 {
		t.Errorf("blockerCount = %d, want 1", result.BlockerCount)
	}
	if result.AuthorizationRequired == nil || !*result.AuthorizationRequired {
		t.Errorf(
			"authorizationRequired = %v, want true",
			result.AuthorizationRequired,
		)
	}
	if result.NextCommand != "higurashi repair authorize WORK-123" {
		t.Errorf("nextCommand = %q", result.NextCommand)
	}
}

func TestInspectInvalidHandoffFailsClosed(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	writeArtifact(t, repository, blockedCLIDocument(false, 0))
	writeFile(
		t,
		filepath.Join(
			repository,
			"docs",
			"higurashi",
			"WORK-123-repair-1.md",
		),
		strings.Replace(
			validCLIRepairHandoff("ready", 1),
			"internal/repair/authorize.go:1",
			"TBD",
			1,
		),
	)

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "handoff_required" {
		t.Errorf("kind = %q, want handoff_required", result.Kind)
	}
	if result.HandoffValidation != "invalid" {
		t.Errorf("handoffValidation = %q, want invalid", result.HandoffValidation)
	}
	if !strings.Contains(result.Message, "placeholder Evidence-Location") {
		t.Errorf("message = %q", result.Message)
	}
}

func TestInspectSecondBlockedRoundExpectsNextVersionedHandoff(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	writeArtifact(t, repository, blockedCLIDocument(false, 1))

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if result.HandoffPath != "docs/higurashi/WORK-123-repair-2.md" {
		t.Errorf("handoffPath = %q, want round 2", result.HandoffPath)
	}
}

func TestInspectFailsClosedForInvalidArtifacts(t *testing.T) {
	tests := []struct {
		name     string
		document string
		contains string
	}{
		{
			name: "duplicate status",
			document: `# WORK-123 — Duplicate status

Higurashi-Schema: 1
Status: planned
Status: implementing

## Ordered vertical TDD checklist

- [ ] **task-001 — Do the work:** pending.
`,
			contains: "duplicate Status",
		},
		{
			name: "duplicate task ID",
			document: `# WORK-123 — Duplicate task

Higurashi-Schema: 1
Status: implementing

## Ordered vertical TDD checklist

- [x] **task-001 — First:** complete.
- [ ] **task-001 — Second:** pending.
`,
			contains: "duplicate task ID",
		},
		{
			name: "verifying with pending task",
			document: `# WORK-123 — Pending verification

Higurashi-Schema: 1
Status: verifying

## Ordered vertical TDD checklist

- [ ] **task-001 — Finish:** pending.
`,
			contains: "requires zero pending tasks",
		},
		{
			name: "complete with pending task",
			document: `# WORK-123 — Pending completion

Higurashi-Schema: 1
Status: complete

## Ordered vertical TDD checklist

- [ ] **task-001 — Finish:** pending.
`,
			contains: "requires zero pending tasks",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newGitRepository(t)
			writeInspectFixture(t, repository)
			writeArtifact(t, repository, test.document)

			exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

			if exitCode != 5 {
				t.Errorf("exit code = %d, want 5", exitCode)
			}
			if result.Kind != "invalid_artifact" {
				t.Errorf(
					"kind = %q, want %q",
					result.Kind,
					"invalid_artifact",
				)
			}
			if !strings.Contains(result.Message, test.contains) {
				t.Errorf(
					"message = %q, want it to contain %q",
					result.Message,
					test.contains,
				)
			}
		})
	}
}

func TestInspectRejectsArtifactDirectorySymlinkEscape(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectFixture(t, repository)
	external := t.TempDir()
	writeFile(
		t,
		filepath.Join(external, "WORK-123.md"),
		"outside the project",
	)
	if err := os.MkdirAll(filepath.Join(repository, "docs"), 0o755); err != nil {
		t.Fatalf("create docs directory: %v", err)
	}
	if err := os.Symlink(
		external,
		filepath.Join(repository, "docs", "higurashi"),
	); err != nil {
		t.Skipf("create artifact directory symlink: %v", err)
	}

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 8 {
		t.Errorf("exit code = %d, want 8", exitCode)
	}
	if result.Kind != "unsafe_project_root" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"unsafe_project_root",
		)
	}
}

func TestInspectRefusesUnknownWorkItem(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectConfig(t, repository)
	writeFile(
		t,
		filepath.Join(repository, "docs", "requirements.md"),
		"# Requirements\n\n## WORK-1234 — Similar but not exact\n",
	)

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4", exitCode)
	}
	if result.Kind != "unknown" {
		t.Errorf("kind = %q, want %q", result.Kind, "unknown")
	}
}

func TestInspectReportsMissingRequirementSource(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectConfig(t, repository)

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3", exitCode)
	}
	if result.Kind != "missing_requirement_source" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"missing_requirement_source",
		)
	}
}

func TestInspectReportsConflictingRequirementHeadings(t *testing.T) {
	repository := newGitRepository(t)
	writeInspectConfig(t, repository)
	writeFile(
		t,
		filepath.Join(repository, "docs", "requirements.md"),
		`# Requirements

## WORK-123 — First definition
## WORK-123 — Second definition
`,
	)

	exitCode, result, _ := runInspectJSON(t, repository, "WORK-123")

	if exitCode != 7 {
		t.Errorf("exit code = %d, want 7", exitCode)
	}
	if result.Kind != "conflict" {
		t.Errorf("kind = %q, want %q", result.Kind, "conflict")
	}
}

type inspectJSONResult struct {
	SchemaVersion         int      `json:"schemaVersion"`
	Command               string   `json:"command"`
	OK                    bool     `json:"ok"`
	Kind                  string   `json:"kind"`
	Message               string   `json:"message"`
	ProjectRoot           string   `json:"projectRoot"`
	WorkItemID            string   `json:"workItemId"`
	RequirementSource     string   `json:"requirementSource"`
	ArtifactPath          string   `json:"artifactPath"`
	ArtifactStatus        string   `json:"artifactStatus"`
	ArtifactHash          string   `json:"artifactHash"`
	BlockedFrom           string   `json:"blockedFrom"`
	RepairRound           *int     `json:"repairRound"`
	HandoffPath           string   `json:"handoffPath"`
	HandoffValidation     string   `json:"handoffValidation"`
	BlockerCount          int      `json:"blockerCount"`
	AuthorizationRequired *bool    `json:"authorizationRequired"`
	NextCommand           string   `json:"nextCommand"`
	Warnings              []string `json:"warnings"`
	Progress              struct {
		Completed int `json:"completed"`
		Pending   int `json:"pending"`
		Total     int `json:"total"`
	} `json:"progress"`
	NextTask *struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"nextTask"`
	Loop struct {
		MaxApplyBatchesPerRun          int  `json:"maxApplyBatchesPerRun"`
		MaxRepairAttempts              int  `json:"maxRepairAttempts"`
		RequireProgressAfterEveryBatch bool `json:"requireProgressAfterEveryBatch"`
	} `json:"loop"`
}

func blockedCLIDocument(withRepairTask bool, round int) string {
	task := ""
	if withRepairTask {
		task = `- [ ] **repair-r1-B-001 — Repair the durable blocker:** bounded.
  Blocker-ID: B-001
  Reproduction: go test ./internal/repair -run TestAuthorize
  Minimum-Acceptance: the focused authorization test passes
`
	}
	return fmt.Sprintf(`# WORK-123 — Durable blocked handoff

Higurashi-Schema: 1
Status: blocked
Repair-Round: %d
Blocked-From: verifying
Blocker-Reason: independent-review-blocker

## Contract

Existing narrative remains unchanged.

## Ordered vertical TDD checklist

- [x] **task-001 — Finish initial implementation:** complete.
%s
## Evidence

Existing reviewer evidence remains unchanged.
`, round, task)
}

func validCLIRepairHandoff(status string, round int) string {
	return fmt.Sprintf(`# Repair handoff for WORK-123

Higurashi-Repair-Handoff: 1
Work-Item: WORK-123
Handoff-Status: %s
Repair-Round: %d
Next-Command: higurashi repair authorize WORK-123
Candidate-Strategy: uncommitted

## Blocker B-001
Originating-Reviewer: contract
Violated-Contract: durable evidence must survive sessions
Evidence-Location: internal/repair/authorize.go:1
Reproduction: go test ./internal/repair -run TestAuthorize
Minimum-Acceptance: the focused authorization test passes
`, status, round)
}

func runInspectJSON(
	t *testing.T,
	workingDirectory string,
	workItemID string,
) (int, inspectJSONResult, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		[]string{"inspect", workItemID, "--json"},
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

	var result inspectJSONResult
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

func writeInspectConfig(t *testing.T, repository string) {
	t.Helper()
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{
  "schemaVersion": 1,
  "workItems": {
    "requirementSources": ["docs/requirements.md"]
  },
  "codegraph": {
    "mode": "preferred"
  }
}`,
	)
}

func writeInspectFixture(t *testing.T, repository string) {
	t.Helper()
	writeInspectConfig(t, repository)
	writeFile(
		t,
		filepath.Join(repository, "docs", "requirements.md"),
		"# Requirements\n\n## WORK-123 — Add deterministic inspection\n",
	)
}

func writeArtifact(t *testing.T, repository, document string) {
	t.Helper()
	writeFile(
		t,
		filepath.Join(repository, "docs", "higurashi", "WORK-123.md"),
		document,
	)
}
