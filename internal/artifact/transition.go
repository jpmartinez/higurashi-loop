package artifact

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrIllegalTransition = errors.New("illegal artifact transition")

type Change struct {
	Document Document
	Content  []byte
	Changed  bool
}

// Transition validates and renders one guarded state transition. Narrative
// content and newline bytes are preserved exactly.
func Transition(
	snapshot Snapshot,
	targetStatus string,
	reason string,
) (Change, error) {
	current := snapshot.Document
	if !supportedStatus(targetStatus) {
		return Change{}, fmt.Errorf(
			"%w: unsupported target status %q",
			ErrIllegalTransition,
			targetStatus,
		)
	}
	if targetStatus != "blocked" && reason != "" {
		return Change{}, fmt.Errorf(
			"%w: a reason is valid only when targeting blocked",
			ErrIllegalTransition,
		)
	}
	if reason != strings.TrimSpace(reason) {
		return Change{}, fmt.Errorf(
			"%w: reason must not have leading or trailing whitespace",
			ErrIllegalTransition,
		)
	}

	if targetStatus == current.Status {
		if current.Status == "blocked" &&
			reason != "" &&
			reason != current.BlockerReason {
			return Change{}, fmt.Errorf(
				"%w: an idempotent blocked transition cannot change its reason",
				ErrIllegalTransition,
			)
		}
		return Change{
			Document: current,
			Content:  append([]byte(nil), snapshot.Content...),
			Changed:  false,
		}, nil
	}

	if !legalTransition(current, targetStatus) {
		return Change{}, fmt.Errorf(
			"%w: %s to %s",
			ErrIllegalTransition,
			current.Status,
			targetStatus,
		)
	}

	next := current
	next.Status = targetStatus
	if targetStatus == "blocked" {
		if err := validateBlockerReason(reason); err != nil {
			return Change{}, fmt.Errorf("%w: %v", ErrIllegalTransition, err)
		}
		next.BlockedFrom = current.Status
		next.BlockerReason = reason
	} else {
		next.BlockedFrom = ""
		next.BlockerReason = ""
		next.CompletionNote = ""
	}
	if err := validateDocument(next); err != nil {
		return Change{}, fmt.Errorf("%w: %v", ErrIllegalTransition, err)
	}

	content, err := rewriteMachineFields(
		snapshot.Content,
		next.Status,
		next.BlockedFrom,
		next.BlockerReason,
		next.CompletionNote,
		next.RepairRound,
	)
	if err != nil {
		return Change{}, err
	}
	parsed, err := Parse(content, snapshot.WorkItemID)
	if err != nil {
		return Change{}, fmt.Errorf("validate rendered artifact: %w", err)
	}
	return Change{
		Document: parsed,
		Content:  content,
		Changed:  true,
	}, nil
}

// CompleteWithNote records an explicit human decision to complete blocked
// work while retaining the unresolved blocker decision in machine-owned text.
// The caller must validate the follow-up disposition before invoking it.
func CompleteWithNote(snapshot Snapshot, note string) (Change, error) {
	current := snapshot.Document
	if current.Status == "complete" && current.CompletionNote != "" {
		return Change{
			Document: current,
			Content:  append([]byte(nil), snapshot.Content...),
			Changed:  false,
		}, nil
	}
	if current.Status != "blocked" {
		return Change{}, fmt.Errorf(
			"%w: human-ordered completion requires Status blocked",
			ErrIllegalTransition,
		)
	}
	if err := validateCompletionNote(note); err != nil {
		return Change{}, fmt.Errorf("%w: %v", ErrIllegalTransition, err)
	}

	next := current
	next.Status = "complete"
	next.BlockedFrom = ""
	next.BlockerReason = ""
	next.CompletionNote = note
	if err := validateDocument(next); err != nil {
		return Change{}, fmt.Errorf("%w: %v", ErrIllegalTransition, err)
	}
	content, err := rewriteMachineFields(
		snapshot.Content,
		next.Status,
		next.BlockedFrom,
		next.BlockerReason,
		next.CompletionNote,
		next.RepairRound,
	)
	if err != nil {
		return Change{}, err
	}
	parsed, err := Parse(content, snapshot.WorkItemID)
	if err != nil {
		return Change{}, fmt.Errorf("validate human-ordered artifact: %w", err)
	}
	return Change{
		Document: parsed,
		Content:  content,
		Changed:  true,
	}, nil
}

func supportedStatus(status string) bool {
	switch status {
	case "refined", "planned", "implementing", "verifying", "blocked", "complete":
		return true
	default:
		return false
	}
}

func legalTransition(current Document, target string) bool {
	switch current.Status {
	case "refined":
		return target == "planned" || target == "blocked"
	case "planned":
		return target == "implementing" || target == "blocked"
	case "implementing":
		return target == "verifying" || target == "blocked"
	case "verifying":
		return target == "complete" || target == "blocked"
	case "blocked":
		return false
	case "complete":
		return false
	default:
		return false
	}
}

func rewriteMachineFields(
	content []byte,
	status string,
	blockedFrom string,
	blockerReason string,
	completionNote string,
	repairRound int,
) ([]byte, error) {
	lines := splitRawLines(string(content))
	statusIndex := -1
	blockedFromIndex := -1
	blockerReasonIndex := -1
	completionNoteIndex := -1
	repairRoundIndex := -1
	for index, line := range lines {
		name, _, ok := machineField(line.content)
		if !ok {
			continue
		}
		switch name {
		case "Status":
			statusIndex = index
		case "Blocked-From":
			blockedFromIndex = index
		case "Blocker-Reason":
			blockerReasonIndex = index
		case "Completion-Note":
			completionNoteIndex = index
		case "Repair-Round":
			repairRoundIndex = index
		}
	}
	if statusIndex < 0 {
		return nil, errors.New("render artifact: missing Status field")
	}

	lines[statusIndex].content = "Status: " + status
	remove := map[int]bool{}
	if blockedFromIndex >= 0 {
		remove[blockedFromIndex] = true
	}
	if blockerReasonIndex >= 0 {
		remove[blockerReasonIndex] = true
	}
	if completionNoteIndex >= 0 {
		remove[completionNoteIndex] = true
	}
	if repairRoundIndex >= 0 {
		remove[repairRoundIndex] = true
	}

	var rendered strings.Builder
	for index, line := range lines {
		if remove[index] {
			continue
		}
		rendered.WriteString(line.content)
		rendered.WriteString(line.ending)
		if index == statusIndex {
			ending := line.ending
			if ending == "" {
				ending = preferredNewline(content)
			}
			rendered.WriteString(
				"Repair-Round: " + strconv.Itoa(repairRound) + ending,
			)
			if blockedFrom != "" {
				rendered.WriteString("Blocked-From: " + blockedFrom + ending)
				rendered.WriteString("Blocker-Reason: " + blockerReason + ending)
			}
			if completionNote != "" {
				rendered.WriteString("Completion-Note: " + completionNote + ending)
			}
		}
	}
	return []byte(rendered.String()), nil
}

// AuthorizeRepair renders the deterministic artifact half of a repair
// authorization. Callers must validate and consume the matching handoff first.
func AuthorizeRepair(snapshot Snapshot, round int) (Change, error) {
	current := snapshot.Document
	if current.Status != "blocked" {
		return Change{}, fmt.Errorf(
			"%w: repair authorization requires Status blocked",
			ErrIllegalTransition,
		)
	}
	if round != current.RepairRound+1 {
		return Change{}, fmt.Errorf(
			"%w: repair round %d must follow current round %d",
			ErrIllegalTransition,
			round,
			current.RepairRound,
		)
	}
	if current.Progress.Pending == 0 {
		return Change{}, fmt.Errorf(
			"%w: repair authorization requires pending repair tasks",
			ErrIllegalTransition,
		)
	}

	next := current
	next.Status = "implementing"
	next.BlockedFrom = ""
	next.BlockerReason = ""
	next.RepairRound = round
	if err := validateDocument(next); err != nil {
		return Change{}, fmt.Errorf("%w: %v", ErrIllegalTransition, err)
	}
	content, err := rewriteMachineFields(
		snapshot.Content,
		next.Status,
		next.BlockedFrom,
		next.BlockerReason,
		next.CompletionNote,
		next.RepairRound,
	)
	if err != nil {
		return Change{}, err
	}
	parsed, err := Parse(content, snapshot.WorkItemID)
	if err != nil {
		return Change{}, fmt.Errorf("validate authorized artifact: %w", err)
	}
	return Change{
		Document: parsed,
		Content:  content,
		Changed:  true,
	}, nil
}

func preferredNewline(content []byte) string {
	if bytes.Contains(content, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// WriteAtomic replaces an artifact through a same-directory temporary file.
func WriteAtomic(name string, content []byte, mode fs.FileMode) error {
	return writeAtomic(name, bytes.NewReader(content), mode)
}

func writeAtomic(name string, content io.Reader, mode fs.FileMode) error {
	directory := filepath.Dir(name)
	temporary, err := os.CreateTemp(
		directory,
		"."+filepath.Base(name)+".tmp-*",
	)
	if err != nil {
		return fmt.Errorf("create temporary artifact: %w", err)
	}
	temporaryName := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryName)
		}
	}()

	if _, err := io.Copy(temporary, content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary artifact: %w", err)
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary artifact mode: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary artifact: %w", err)
	}
	if err := os.Rename(temporaryName, name); err != nil {
		return fmt.Errorf("replace artifact atomically: %w", err)
	}
	cleanup = false
	return nil
}
