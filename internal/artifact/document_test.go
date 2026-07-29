package artifact_test

import (
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/artifact"
)

func TestParsePreservesMultilineTaskNewlinesAndWhitespace(t *testing.T) {
	document := strings.ReplaceAll(`# WORK-123 — Preserve task text

Higurashi-Schema: 1
Status: implementing

## Ordered vertical TDD checklist

- [ ] **task-001 — Preserve text:** first line.

  indented continuation.
	Tabbed continuation.

## Verification
`, "\n", "\r\n")

	parsed, err := artifact.Parse([]byte(document), "WORK-123")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	task := parsed.NextPending()
	if task == nil {
		t.Fatal("NextPending() = nil")
	}
	want := "Preserve text: first line.\r\n\r\n" +
		"  indented continuation.\r\n\tTabbed continuation."
	if task.Text != want {
		t.Errorf("task.Text = %q, want %q", task.Text, want)
	}
}

func TestParseAcceptsValidBlockedState(t *testing.T) {
	parsed, err := artifact.Parse([]byte(`# WORK-123 — Blocked work

Higurashi-Schema: 1
Status: blocked
Repair-Round: 2
Blocked-From: implementing
Blocker-Reason: waiting-for-explicit-user-decision

## Ordered vertical TDD checklist

- [ ] **task-001 — Continue safely:** pending.
`), "WORK-123")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.BlockedFrom != "implementing" {
		t.Errorf(
			"BlockedFrom = %q, want %q",
			parsed.BlockedFrom,
			"implementing",
		)
	}
	if parsed.RepairRound != 2 {
		t.Errorf("RepairRound = %d, want 2", parsed.RepairRound)
	}
}

func TestParseDefaultsMissingRepairRoundToZeroForCompatibility(t *testing.T) {
	parsed, err := artifact.Parse([]byte(`# WORK-123 — Legacy artifact

Higurashi-Schema: 1
Status: planned

## Ordered vertical TDD checklist

- [ ] **task-001 — Preserve compatibility:** pending.
`), "WORK-123")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.RepairRound != 0 {
		t.Errorf("RepairRound = %d, want 0", parsed.RepairRound)
	}
}

func TestParseAcceptsConfirmedRefinedArtifactWithoutTasks(t *testing.T) {
	parsed, err := artifact.Parse([]byte(`# WORK-123 — Confirmed contract

Higurashi-Schema: 1
Status: refined
Repair-Round: 0

## Contract

The confirmed observable behavior.

## Ordered vertical TDD checklist

## Verification

Not started.
`), "WORK-123")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Status != "refined" {
		t.Errorf("Status = %q, want refined", parsed.Status)
	}
	if parsed.Progress.Total != 0 || parsed.Progress.Pending != 0 {
		t.Errorf("Progress = %+v, want empty checklist", parsed.Progress)
	}
}

func TestParseRejectsNonCanonicalRepairRound(t *testing.T) {
	_, err := artifact.Parse([]byte(`# WORK-123 — Invalid round

Higurashi-Schema: 1
Status: blocked
Repair-Round: 01
Blocked-From: verifying
Blocker-Reason: invalid-round

## Ordered vertical TDD checklist

- [x] **task-001 — Complete work:** complete.
`), "WORK-123")
	if err == nil || !strings.Contains(err.Error(), "canonical integer") {
		t.Fatalf("Parse() error = %v, want canonical integer refusal", err)
	}
}

func TestParseRejectsNarrativeChecklistContent(t *testing.T) {
	_, err := artifact.Parse([]byte(`# WORK-123 — Invalid checklist

Higurashi-Schema: 1
Status: implementing

## Ordered vertical TDD checklist

This unstructured line cannot be resumed deterministically.
`), "WORK-123")
	if err == nil {
		t.Fatal("Parse() error = nil, want invalid checklist")
	}
}

func TestParseRejectsMachineFieldOutsidePreamble(t *testing.T) {
	_, err := artifact.Parse([]byte(`# WORK-123 — Misplaced field

Higurashi-Schema: 1
Status: implementing

## Contract

Status: planned

## Ordered vertical TDD checklist

- [ ] **task-001 — Continue safely:** pending.
`), "WORK-123")
	if err == nil {
		t.Fatal("Parse() error = nil, want misplaced machine field")
	}
}
