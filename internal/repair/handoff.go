package repair

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion   = 1
	maxHandoffBytes = 1 << 20
)

var (
	ErrInvalidHandoff = errors.New("invalid repair handoff")
	blockerIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	blockerHeading    = regexp.MustCompile(`^## Blocker ([A-Za-z0-9][A-Za-z0-9._-]*)$`)
)

type Blocker struct {
	ID                  string
	OriginatingReviewer string
	ViolatedContract    string
	EvidenceLocation    string
	Reproduction        string
	MinimumAcceptance   string
}

type Document struct {
	SchemaVersion     int
	WorkItemID        string
	Status            string
	Round             int
	NextCommand       string
	CandidateStrategy string
	Blockers          []Blocker
}

type Snapshot struct {
	Document Document
	Content  []byte
	Mode     fs.FileMode
}

type Validation struct {
	State   string
	Reason  string
	Handoff Document
}

// ExpectedCommand returns the only command that may authorize a repair round.
func ExpectedCommand(workItemID string) string {
	return "higurashi repair authorize " + workItemID
}

// ValidateFile classifies the expected sidecar without modifying it.
func ValidateFile(name, expectedWorkItemID string, expectedRound int) Validation {
	snapshot, err := Load(name, expectedWorkItemID, expectedRound)
	if err != nil {
		state := "invalid"
		if errors.Is(err, os.ErrNotExist) {
			state = "missing"
		}
		return Validation{State: state, Reason: err.Error()}
	}
	return Validation{
		State:   snapshot.Document.Status,
		Handoff: snapshot.Document,
	}
}

// Load reads and strictly validates one repair handoff.
func Load(name, expectedWorkItemID string, expectedRound int) (Snapshot, error) {
	file, err := os.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, fmt.Errorf(
				"%w: expected repair handoff is missing",
				os.ErrNotExist,
			)
		}
		return Snapshot{}, fmt.Errorf("open repair handoff: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Snapshot{}, fmt.Errorf("stat repair handoff: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, fmt.Errorf(
			"%w: repair handoff must be a regular file",
			ErrInvalidHandoff,
		)
	}
	content, err := io.ReadAll(io.LimitReader(file, maxHandoffBytes+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read repair handoff: %w", err)
	}
	if len(content) > maxHandoffBytes {
		return Snapshot{}, fmt.Errorf(
			"%w: repair handoff exceeds %d bytes",
			ErrInvalidHandoff,
			maxHandoffBytes,
		)
	}
	document, err := Parse(content, expectedWorkItemID, expectedRound)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Document: document,
		Content:  content,
		Mode:     info.Mode(),
	}, nil
}

// Parse validates the versioned header and structured blocker evidence.
func Parse(
	content []byte,
	expectedWorkItemID string,
	expectedRound int,
) (Document, error) {
	if !utf8.Valid(content) {
		return Document{}, invalid("repair handoff must be valid UTF-8")
	}
	for _, character := range string(content) {
		if unicode.IsControl(character) &&
			character != '\n' &&
			character != '\r' {
			return Document{}, invalid(
				"repair handoff contains a control character",
			)
		}
	}
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if strings.ContainsRune(normalized, '\r') {
		return Document{}, invalid("repair handoff contains a bare carriage return")
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 ||
		lines[0] != "# Repair handoff for "+expectedWorkItemID {
		return Document{}, invalid(
			"repair handoff heading must identify exact work item " +
				expectedWorkItemID,
		)
	}

	header := map[string]string{}
	var blockers []Blocker
	seenBlockers := map[string]struct{}{}
	var currentID string
	currentFields := map[string]string{}
	inBlockers := false
	finishBlocker := func() error {
		if currentID == "" {
			return nil
		}
		required := []string{
			"Originating-Reviewer",
			"Violated-Contract",
			"Evidence-Location",
			"Reproduction",
			"Minimum-Acceptance",
		}
		for _, name := range required {
			value := currentFields[name]
			if placeholder(value) {
				return invalid(
					fmt.Sprintf("blocker %s has missing or placeholder %s", currentID, name),
				)
			}
		}
		blockers = append(blockers, Blocker{
			ID:                  currentID,
			OriginatingReviewer: currentFields["Originating-Reviewer"],
			ViolatedContract:    currentFields["Violated-Contract"],
			EvidenceLocation:    currentFields["Evidence-Location"],
			Reproduction:        currentFields["Reproduction"],
			MinimumAcceptance:   currentFields["Minimum-Acceptance"],
		})
		return nil
	}

	for index, line := range lines[1:] {
		lineNumber := index + 2
		if line == "" {
			continue
		}
		if match := blockerHeading.FindStringSubmatch(line); match != nil {
			if err := finishBlocker(); err != nil {
				return Document{}, err
			}
			currentID = match[1]
			if !blockerIDPattern.MatchString(currentID) || placeholder(currentID) {
				return Document{}, invalid(
					fmt.Sprintf("invalid blocker ID %q", currentID),
				)
			}
			if _, duplicate := seenBlockers[currentID]; duplicate {
				return Document{}, invalid(
					fmt.Sprintf("duplicate blocker ID %q", currentID),
				)
			}
			seenBlockers[currentID] = struct{}{}
			currentFields = map[string]string{}
			inBlockers = true
			continue
		}
		if strings.HasPrefix(line, "#") {
			return Document{}, invalid(
				fmt.Sprintf("unexpected heading on line %d", lineNumber),
			)
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != name || !strings.HasPrefix(value, " ") {
			return Document{}, invalid(
				fmt.Sprintf("malformed field on line %d", lineNumber),
			)
		}
		value = strings.TrimPrefix(value, " ")
		fields := header
		allowed := map[string]bool{
			"Higurashi-Repair-Handoff": true,
			"Work-Item":                true,
			"Handoff-Status":           true,
			"Repair-Round":             true,
			"Next-Command":             true,
			"Candidate-Strategy":       true,
		}
		if inBlockers {
			fields = currentFields
			allowed = map[string]bool{
				"Originating-Reviewer": true,
				"Violated-Contract":    true,
				"Evidence-Location":    true,
				"Reproduction":         true,
				"Minimum-Acceptance":   true,
			}
		}
		if !allowed[name] {
			return Document{}, invalid(
				fmt.Sprintf("unknown field %q on line %d", name, lineNumber),
			)
		}
		if _, duplicate := fields[name]; duplicate {
			return Document{}, invalid(fmt.Sprintf("duplicate %s field", name))
		}
		fields[name] = value
	}
	if err := finishBlocker(); err != nil {
		return Document{}, err
	}

	requiredHeader := []string{
		"Higurashi-Repair-Handoff",
		"Work-Item",
		"Handoff-Status",
		"Repair-Round",
		"Next-Command",
		"Candidate-Strategy",
	}
	for _, name := range requiredHeader {
		if placeholder(header[name]) {
			return Document{}, invalid("missing or placeholder " + name)
		}
	}
	if header["Higurashi-Repair-Handoff"] != strconv.Itoa(SchemaVersion) {
		return Document{}, invalid(
			"unsupported Higurashi-Repair-Handoff " +
				strconv.Quote(header["Higurashi-Repair-Handoff"]),
		)
	}
	if header["Work-Item"] != expectedWorkItemID {
		return Document{}, invalid(
			fmt.Sprintf(
				"Work-Item %q does not match %q",
				header["Work-Item"],
				expectedWorkItemID,
			),
		)
	}
	round, err := strconv.Atoi(header["Repair-Round"])
	if err != nil || round <= 0 || strconv.Itoa(round) != header["Repair-Round"] {
		return Document{}, invalid("Repair-Round must be a positive canonical integer")
	}
	if round != expectedRound {
		return Document{}, invalid(
			fmt.Sprintf("Repair-Round %d does not match expected round %d", round, expectedRound),
		)
	}
	if header["Handoff-Status"] != "ready" &&
		header["Handoff-Status"] != "consumed" {
		return Document{}, invalid("Handoff-Status must be ready or consumed")
	}
	expectedCommand := ExpectedCommand(expectedWorkItemID)
	if header["Next-Command"] != expectedCommand {
		return Document{}, invalid(
			fmt.Sprintf("Next-Command must be exactly %q", expectedCommand),
		)
	}
	if header["Candidate-Strategy"] != "uncommitted" {
		return Document{}, invalid(
			"Candidate-Strategy must be uncommitted",
		)
	}
	if len(blockers) == 0 {
		return Document{}, invalid("repair handoff requires at least one blocker")
	}
	return Document{
		SchemaVersion:     SchemaVersion,
		WorkItemID:        expectedWorkItemID,
		Status:            header["Handoff-Status"],
		Round:             round,
		NextCommand:       expectedCommand,
		CandidateStrategy: header["Candidate-Strategy"],
		Blockers:          blockers,
	}, nil
}

func placeholder(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed != value {
		return true
	}
	switch strings.ToLower(trimmed) {
	case "todo", "tbd", "unknown", "n/a", "none", "placeholder", "...":
		return true
	}
	return strings.Contains(trimmed, "{{") ||
		strings.Contains(trimmed, "}}") ||
		(strings.Contains(trimmed, "<") && strings.Contains(trimmed, ">"))
}

func invalid(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidHandoff, message)
}
