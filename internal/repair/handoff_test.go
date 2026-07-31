package repair

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/artifact"
)

func TestParseRejectsIncompletePlaceholderAndConflictingHandoffs(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		workItemID string
		round      int
		contains   string
	}{
		{
			name:       "placeholder evidence",
			content:    strings.Replace(validHandoff("ready", 1), "internal/x.go:42", "TBD", 1),
			workItemID: "WORK-123",
			round:      1,
			contains:   "placeholder Evidence-Location",
		},
		{
			name: "duplicate blocker",
			content: validHandoff("ready", 1) + `
## Blocker B-001
Severity: low
Originating-Reviewer: risk
Violated-Contract: duplicate
Evidence-Location: internal/y.go:9
Reproduction: go test ./internal/y
Minimum-Acceptance: focused test passes
`,
			workItemID: "WORK-123",
			round:      1,
			contains:   "duplicate blocker ID",
		},
		{
			name:       "wrong work item",
			content:    validHandoff("ready", 1),
			workItemID: "WORK-999",
			round:      1,
			contains:   "exact work item",
		},
		{
			name:       "wrong round",
			content:    validHandoff("ready", 1),
			workItemID: "WORK-123",
			round:      2,
			contains:   "does not match expected round",
		},
		{
			name: "wrong next command",
			content: strings.Replace(
				validHandoff("ready", 1),
				"higurashi repair authorize WORK-123",
				"higurashi inspect WORK-123",
				1,
			),
			workItemID: "WORK-123",
			round:      1,
			contains:   "Next-Command must be exactly",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(
				[]byte(test.content),
				test.workItemID,
				test.round,
			)
			if !errors.Is(err, ErrInvalidHandoff) {
				t.Fatalf("Parse() error = %v, want ErrInvalidHandoff", err)
			}
			if !strings.Contains(err.Error(), test.contains) {
				t.Errorf("error = %q, want it to contain %q", err, test.contains)
			}
		})
	}
}

func TestParseRejectsUnclassifiedBlockerSeverity(t *testing.T) {
	content := strings.Replace(
		validHandoff("ready", 1),
		"Severity: high",
		"Severity: urgent",
		1,
	)

	_, err := Parse([]byte(content), "WORK-123", 1)

	if !errors.Is(err, ErrInvalidHandoff) {
		t.Fatalf("Parse() error = %v, want ErrInvalidHandoff", err)
	}
	if !strings.Contains(err.Error(), "unsupported Severity") {
		t.Errorf("Parse() error = %q, want severity diagnostic", err)
	}
}

func TestAuthorizeRequiresNewPendingRepairTasks(t *testing.T) {
	directory := t.TempDir()
	artifactName := filepath.Join(directory, "WORK-123.md")
	handoffName := filepath.Join(directory, "WORK-123-repair-1.md")
	writeRepairFile(t, artifactName, blockedArtifact(false, 0))
	writeRepairFile(t, handoffName, validHandoff("ready", 1))

	_, err := Authorize(artifactName, handoffName, "WORK-123")

	if !errors.Is(err, ErrAuthorization) {
		t.Fatalf("Authorize() error = %v, want ErrAuthorization", err)
	}
	handoff, loadErr := Load(handoffName, "WORK-123", 1)
	if loadErr != nil {
		t.Fatalf("Load() handoff error = %v", loadErr)
	}
	if handoff.Document.Status != "ready" {
		t.Errorf("handoff status = %q, want ready", handoff.Document.Status)
	}
}

func TestAuthorizeConsumesHandoffIncrementsRoundAndPreservesNarrative(t *testing.T) {
	directory := t.TempDir()
	artifactName := filepath.Join(directory, "WORK-123.md")
	handoffName := filepath.Join(directory, "WORK-123-repair-1.md")
	original := blockedArtifact(true, 0)
	writeRepairFile(t, artifactName, original)
	writeRepairFile(t, handoffName, validHandoff("ready", 1))

	result, err := Authorize(artifactName, handoffName, "WORK-123")

	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if result.Document.Status != "implementing" || result.Document.RepairRound != 1 {
		t.Errorf(
			"artifact state = %s round %d, want implementing round 1",
			result.Document.Status,
			result.Document.RepairRound,
		)
	}
	if result.Handoff.Status != "consumed" {
		t.Errorf("handoff status = %q, want consumed", result.Handoff.Status)
	}
	actual, err := os.ReadFile(artifactName)
	if err != nil {
		t.Fatalf("read authorized artifact: %v", err)
	}
	want := strings.Replace(
		original,
		`Status: blocked
Repair-Round: 0
Blocked-From: verifying
Blocker-Reason: independent-review-blocker`,
		`Status: implementing
Repair-Round: 1`,
		1,
	)
	if string(actual) != want {
		t.Errorf(
			"authorization changed narrative or tasks\nwant:\n%s\nactual:\n%s",
			want,
			actual,
		)
	}
}

func TestAuthorizeIsIdempotentAfterCompletion(t *testing.T) {
	directory := t.TempDir()
	artifactName := filepath.Join(directory, "WORK-123.md")
	handoffName := filepath.Join(directory, "WORK-123-repair-1.md")
	writeRepairFile(t, artifactName, blockedArtifact(true, 0))
	writeRepairFile(t, handoffName, validHandoff("ready", 1))
	if _, err := Authorize(artifactName, handoffName, "WORK-123"); err != nil {
		t.Fatalf("first Authorize() error = %v", err)
	}
	before, err := os.ReadFile(artifactName)
	if err != nil {
		t.Fatalf("read artifact before retry: %v", err)
	}

	result, err := Authorize(artifactName, handoffName, "WORK-123")

	if err != nil {
		t.Fatalf("second Authorize() error = %v", err)
	}
	if result.Changed {
		t.Error("second authorization changed files")
	}
	after, err := os.ReadFile(artifactName)
	if err != nil {
		t.Fatalf("read artifact after retry: %v", err)
	}
	if string(after) != string(before) {
		t.Error("idempotent authorization changed artifact bytes")
	}
}

func TestConsumedHandoffCannotAuthorizeALaterRound(t *testing.T) {
	directory := t.TempDir()
	artifactName := filepath.Join(directory, "WORK-123.md")
	handoffName := filepath.Join(directory, "WORK-123-repair-1.md")
	writeRepairFile(t, artifactName, blockedArtifact(true, 1))
	writeRepairFile(t, handoffName, validHandoff("consumed", 1))

	_, err := Authorize(artifactName, handoffName, "WORK-123")

	if !errors.Is(err, ErrInvalidHandoff) {
		t.Fatalf("Authorize() error = %v, want ErrInvalidHandoff", err)
	}
	if !strings.Contains(err.Error(), "expected round 2") {
		t.Errorf("error = %q, want wrong-round refusal", err)
	}
}

func TestAuthorizeRecoversConsumedHandoffAfterInterruptedArtifactWrite(t *testing.T) {
	directory := t.TempDir()
	artifactName := filepath.Join(directory, "WORK-123.md")
	handoffName := filepath.Join(directory, "WORK-123-repair-1.md")
	writeRepairFile(t, artifactName, blockedArtifact(true, 0))
	writeRepairFile(t, handoffName, validHandoff("ready", 1))
	writes := 0
	interruptArtifact := func(name string, content []byte, mode os.FileMode) error {
		writes++
		if writes == 2 {
			return errors.New("simulated interruption")
		}
		return artifact.WriteAtomic(name, content, mode)
	}

	_, err := authorizeWithWriter(
		artifactName,
		handoffName,
		"WORK-123",
		interruptArtifact,
	)
	if !errors.Is(err, ErrPartial) {
		t.Fatalf("interrupted Authorize() error = %v, want ErrPartial", err)
	}
	if strings.Count(err.Error(), ExpectedCommand("WORK-123")) != 1 {
		t.Errorf("partial error does not contain exactly one recovery command: %v", err)
	}
	partialHandoff, err := Load(handoffName, "WORK-123", 1)
	if err != nil {
		t.Fatalf("load partial handoff: %v", err)
	}
	if partialHandoff.Document.Status != "consumed" {
		t.Errorf(
			"partial handoff status = %q, want consumed",
			partialHandoff.Document.Status,
		)
	}
	partialArtifact, err := artifact.Read(artifactName, "WORK-123")
	if err != nil {
		t.Fatalf("read partial artifact: %v", err)
	}
	if partialArtifact.Status != "blocked" || partialArtifact.RepairRound != 0 {
		t.Errorf(
			"partial artifact = %s round %d, want blocked round 0",
			partialArtifact.Status,
			partialArtifact.RepairRound,
		)
	}

	recovered, err := Authorize(artifactName, handoffName, "WORK-123")

	if err != nil {
		t.Fatalf("recovery Authorize() error = %v", err)
	}
	if !recovered.Recovered {
		t.Error("recovery result did not report consumed-handoff recovery")
	}
	if recovered.Document.Status != "implementing" ||
		recovered.Document.RepairRound != 1 {
		t.Errorf(
			"recovered artifact = %s round %d",
			recovered.Document.Status,
			recovered.Document.RepairRound,
		)
	}
}

func validHandoff(status string, round int) string {
	return `# Repair handoff for WORK-123

Higurashi-Repair-Handoff: 1
Work-Item: WORK-123
Handoff-Status: ` + status + `
Repair-Round: ` + strconv.Itoa(round) + `
Next-Command: higurashi repair authorize WORK-123
Candidate-Strategy: uncommitted

## Blocker B-001
Severity: high
Originating-Reviewer: contract
Violated-Contract: authorization must preserve durable evidence
Evidence-Location: internal/x.go:42
Reproduction: go test ./internal/x -run TestBlocked
Minimum-Acceptance: the focused blocked-state test passes
`
}

func blockedArtifact(withRepairTask bool, round int) string {
	pending := ""
	if withRepairTask {
		pending = `- [ ] **repair-r1-B-001 — Repair the authorization blocker:** bounded repair.
  Blocker-ID: B-001
  Reproduction: go test ./internal/x -run TestBlocked
  Minimum-Acceptance: the focused blocked-state test passes
`
	}
	return `# WORK-123 — Durable repair

Higurashi-Schema: 1
Status: blocked
Repair-Round: ` + strconv.Itoa(round) + `
Blocked-From: verifying
Blocker-Reason: independent-review-blocker

## Contract

Narrative sentinel must remain byte-for-byte unchanged.

## Ordered vertical TDD checklist

- [x] **task-001 — Preserve completed work:** existing evidence.
` + pending + `
## Evidence

Previous reviewer evidence remains here.
`
}

func writeRepairFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0o640); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(name), err)
	}
}
