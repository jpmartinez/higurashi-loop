package artifact_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/artifact"
)

func TestTransitionStateGraphRejectsEveryUnlistedEdge(t *testing.T) {
	statuses := []string{
		"refined",
		"planned",
		"implementing",
		"verifying",
		"blocked",
		"complete",
	}
	allowed := map[string]map[string]bool{
		"refined": {
			"refined": true,
			"planned": true,
			"blocked": true,
		},
		"planned": {
			"planned":      true,
			"implementing": true,
			"blocked":      true,
		},
		"implementing": {
			"implementing": true,
			"verifying":    true,
			"blocked":      true,
		},
		"verifying": {
			"verifying": true,
			"complete":  true,
			"blocked":   true,
		},
		"blocked": {
			"blocked": true,
		},
		"complete": {
			"complete": true,
		},
	}

	for _, current := range statuses {
		for _, target := range statuses {
			t.Run(current+"_to_"+target, func(t *testing.T) {
				content := graphArtifact(current)
				document, err := artifact.Parse([]byte(content), "WORK-123")
				if err != nil {
					t.Fatalf("Parse() error = %v", err)
				}
				reason := ""
				if target == "blocked" && current != "blocked" {
					reason = "bounded-graph-test"
				}

				_, err = artifact.Transition(artifact.Snapshot{
					WorkItemID: "WORK-123",
					Document:   document,
					Content:    []byte(content),
					Mode:       0o600,
				}, target, reason)

				if allowed[current][target] && err != nil {
					t.Errorf("Transition() error = %v, want success", err)
				}
				if !allowed[current][target] &&
					!errors.Is(err, artifact.ErrIllegalTransition) {
					t.Errorf(
						"Transition() error = %v, want ErrIllegalTransition",
						err,
					)
				}
			})
		}
	}
}

func TestTransitionPreservesCRLFWhenEnteringBlocked(t *testing.T) {
	content := strings.ReplaceAll(graphArtifact("planned"), "\n", "\r\n")
	document, err := artifact.Parse([]byte(content), "WORK-123")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	change, err := artifact.Transition(artifact.Snapshot{
		WorkItemID: "WORK-123",
		Document:   document,
		Content:    []byte(content),
		Mode:       0o600,
	}, "blocked", "bounded-crlf-test")
	if err != nil {
		t.Fatalf("Transition() error = %v", err)
	}

	if strings.Contains(
		strings.ReplaceAll(string(change.Content), "\r\n", ""),
		"\n",
	) {
		t.Error("transition introduced a bare LF into a CRLF artifact")
	}
	if !strings.Contains(
		string(change.Content),
		"Status: blocked\r\n"+
			"Repair-Round: 0\r\n"+
			"Blocked-From: planned\r\n"+
			"Blocker-Reason: bounded-crlf-test\r\n",
	) {
		t.Errorf(
			"blocked fields did not preserve CRLF:\n%q",
			change.Content,
		)
	}
}

func TestCompleteWithNotePreservesAnUnresolvedBlockedDecision(t *testing.T) {
	content := graphArtifact("blocked")
	content = strings.Replace(
		content,
		"- [x] **task-001 — Prove the graph:** complete.",
		"- [ ] **task-001 — Prove the graph:** pending.",
		1,
	)
	document, err := artifact.Parse([]byte(content), "WORK-123")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	note := "Human-ordered completion; unresolved blocker B-001 deferred as follow-up WORK-456"

	change, err := artifact.CompleteWithNote(artifact.Snapshot{
		WorkItemID: "WORK-123",
		Document:   document,
		Content:    []byte(content),
		Mode:       0o600,
	}, note)
	if err != nil {
		t.Fatalf("CompleteWithNote() error = %v", err)
	}
	if change.Document.Status != "complete" {
		t.Errorf("status = %q, want complete", change.Document.Status)
	}
	if change.Document.CompletionNote != note {
		t.Errorf("completion note = %q, want %q", change.Document.CompletionNote, note)
	}
	if change.Document.Progress.Pending != 1 {
		t.Errorf("pending = %d, want 1", change.Document.Progress.Pending)
	}
	if strings.Contains(string(change.Content), "Blocked-From:") ||
		strings.Contains(string(change.Content), "Blocker-Reason:") {
		t.Errorf("blocked fields survived completion:\n%s", change.Content)
	}
}

func graphArtifact(status string) string {
	fields := "Status: " + status + "\n"
	if status == "blocked" {
		fields += "Blocked-From: planned\n"
		fields += "Blocker-Reason: bounded-graph-test\n"
	}
	return `# WORK-123 — State graph

Higurashi-Schema: 1
Repair-Round: 0
` + fields + `
## Ordered vertical TDD checklist

- [x] **task-001 — Prove the graph:** complete.
`
}
