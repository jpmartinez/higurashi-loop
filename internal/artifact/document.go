package artifact

import (
	"crypto/sha256"
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
	SchemaVersion          = 1
	maxArtifactBytes       = 4 << 20
	maxBlockerBytes        = 512
	maxCompletionNoteBytes = 2048
	checklistHeading       = "## Ordered vertical TDD checklist"
)

var taskPattern = regexp.MustCompile(
	`^- \[([ xX])\] \*\*([A-Za-z0-9][A-Za-z0-9._-]*) — (.*?)\*\*(.*)$`,
)

type Document struct {
	SchemaVersion  int
	Status         string
	BlockedFrom    string
	BlockerReason  string
	CompletionNote string
	RepairRound    int
	Hash           string
	Tasks          []Task
	Progress       Progress
}

type Task struct {
	ID       string
	Text     string
	Complete bool
}

type Progress struct {
	Completed int `json:"completed"`
	Pending   int `json:"pending"`
	Total     int `json:"total"`
}

type Snapshot struct {
	WorkItemID string
	Document   Document
	Content    []byte
	Mode       fs.FileMode
}

// Read parses and validates an artifact without modifying it.
func Read(name, expectedWorkItemID string) (Document, error) {
	snapshot, err := Load(name, expectedWorkItemID)
	if err != nil {
		return Document{}, err
	}
	return snapshot.Document, nil
}

// Load returns a validated document together with the exact bytes and mode
// required for guarded, narrative-preserving updates.
func Load(name, expectedWorkItemID string) (Snapshot, error) {
	file, err := os.Open(name)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Snapshot{}, fmt.Errorf("stat artifact: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, errors.New("artifact must be a regular file")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read artifact: %w", err)
	}
	if len(data) > maxArtifactBytes {
		return Snapshot{}, fmt.Errorf(
			"artifact exceeds %d bytes",
			maxArtifactBytes,
		)
	}
	document, err := Parse(data, expectedWorkItemID)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		WorkItemID: expectedWorkItemID,
		Document:   document,
		Content:    data,
		Mode:       info.Mode(),
	}, nil
}

// Parse decodes the machine-owned preamble and ordered checklist while
// preserving task continuation bytes and newline style.
func Parse(data []byte, expectedWorkItemID string) (Document, error) {
	lines := splitRawLines(string(data))
	if len(lines) == 0 || !artifactHeadingMatches(lines[0].content, expectedWorkItemID) {
		return Document{}, fmt.Errorf(
			"artifact heading must begin with exact work item ID %s",
			expectedWorkItemID,
		)
	}

	document := Document{
		Hash: fmt.Sprintf("%x", sha256.Sum256(data)),
	}
	fields := map[string]string{}
	checklistIndex := -1
	inPreamble := true
	for index, line := range lines {
		trimmed := strings.TrimSpace(line.content)
		if trimmed == checklistHeading {
			if checklistIndex >= 0 {
				return Document{}, errors.New(
					"duplicate Ordered vertical TDD checklist",
				)
			}
			checklistIndex = index
		}
		if index > 0 && strings.HasPrefix(trimmed, "## ") {
			inPreamble = false
			continue
		}
		name, value, ok := machineField(line.content)
		if !ok {
			continue
		}
		if !inPreamble {
			return Document{}, fmt.Errorf(
				"%s field must appear before the first section",
				name,
			)
		}
		if _, exists := fields[name]; exists {
			return Document{}, fmt.Errorf("duplicate %s field", name)
		}
		if value == "" {
			return Document{}, fmt.Errorf("%s field must not be empty", name)
		}
		fields[name] = value
	}

	if checklistIndex < 0 {
		return Document{}, errors.New(
			"missing Ordered vertical TDD checklist",
		)
	}
	if fields["Higurashi-Schema"] == "" {
		return Document{}, errors.New("missing Higurashi-Schema field")
	}
	if fields["Higurashi-Schema"] != "1" {
		return Document{}, fmt.Errorf(
			"unsupported Higurashi-Schema %q",
			fields["Higurashi-Schema"],
		)
	}
	document.SchemaVersion = SchemaVersion
	document.Status = fields["Status"]
	document.BlockedFrom = fields["Blocked-From"]
	document.BlockerReason = fields["Blocker-Reason"]
	document.CompletionNote = fields["Completion-Note"]
	if value := fields["Repair-Round"]; value != "" {
		round, err := strconv.Atoi(value)
		if err != nil || round < 0 || strconv.Itoa(round) != value {
			return Document{}, errors.New(
				"Repair-Round must be a non-negative canonical integer",
			)
		}
		document.RepairRound = round
	}
	if document.Status == "" {
		return Document{}, errors.New("missing Status field")
	}

	tasks, err := parseTasks(lines, checklistIndex+1)
	if err != nil {
		return Document{}, err
	}
	document.Tasks = tasks
	for _, task := range tasks {
		if task.Complete {
			document.Progress.Completed++
		} else {
			document.Progress.Pending++
		}
	}
	document.Progress.Total = len(tasks)

	if err := validateDocument(document); err != nil {
		return Document{}, err
	}
	return document, nil
}

// NextPending returns the first incomplete task in artifact order.
func (document Document) NextPending() *Task {
	for index := range document.Tasks {
		if !document.Tasks[index].Complete {
			task := document.Tasks[index]
			return &task
		}
	}
	return nil
}

type rawLine struct {
	content string
	ending  string
}

func splitRawLines(data string) []rawLine {
	if data == "" {
		return nil
	}
	var lines []rawLine
	for len(data) > 0 {
		index := strings.IndexByte(data, '\n')
		if index < 0 {
			lines = append(lines, rawLine{content: data})
			break
		}
		content := data[:index]
		ending := "\n"
		if strings.HasSuffix(content, "\r") {
			content = strings.TrimSuffix(content, "\r")
			ending = "\r\n"
		}
		lines = append(lines, rawLine{content: content, ending: ending})
		data = data[index+1:]
	}
	return lines
}

func artifactHeadingMatches(line, id string) bool {
	prefix := "# " + id
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	if len(line) == len(prefix) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(line[len(prefix):])
	return unicode.IsSpace(next) ||
		next == ':' ||
		next == '—' ||
		next == '–' ||
		next == '-'
}

func machineField(line string) (string, string, bool) {
	names := [...]string{
		"Higurashi-Schema",
		"Status",
		"Blocked-From",
		"Blocker-Reason",
		"Completion-Note",
		"Repair-Round",
	}
	for _, name := range names {
		prefix := name + ":"
		if strings.HasPrefix(line, prefix) {
			return name, strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
		}
	}
	return "", "", false
}

func parseTasks(lines []rawLine, start int) ([]Task, error) {
	var tasks []Task
	seen := map[string]struct{}{}
	lastTaskLine := -1
	var pendingBlankLines []int
	for index := start; index < len(lines); index++ {
		line := lines[index]
		if strings.HasPrefix(strings.TrimSpace(line.content), "## ") {
			break
		}
		if match := taskPattern.FindStringSubmatch(line.content); match != nil {
			id := match[2]
			if _, exists := seen[id]; exists {
				return nil, fmt.Errorf("duplicate task ID %q", id)
			}
			seen[id] = struct{}{}
			tasks = append(tasks, Task{
				ID:       id,
				Text:     match[3] + match[4],
				Complete: match[1] == "x" || match[1] == "X",
			})
			lastTaskLine = index
			pendingBlankLines = nil
			continue
		}
		if strings.HasPrefix(line.content, "- [") {
			return nil, fmt.Errorf(
				"malformed checklist task on line %d",
				index+1,
			)
		}
		if strings.TrimSpace(line.content) == "" {
			if len(tasks) > 0 {
				pendingBlankLines = append(pendingBlankLines, index)
			}
			continue
		}
		if len(tasks) == 0 ||
			(!strings.HasPrefix(line.content, "  ") &&
				!strings.HasPrefix(line.content, "\t")) {
			return nil, fmt.Errorf(
				"unexpected content in checklist on line %d",
				index+1,
			)
		}
		for _, blankIndex := range pendingBlankLines {
			tasks[len(tasks)-1].Text += lines[lastTaskLine].ending +
				lines[blankIndex].content
			lastTaskLine = blankIndex
		}
		pendingBlankLines = nil
		tasks[len(tasks)-1].Text += lines[lastTaskLine].ending +
			line.content
		lastTaskLine = index
	}
	return tasks, nil
}

func validateDocument(document Document) error {
	validStatus := map[string]bool{
		"refined":      true,
		"planned":      true,
		"implementing": true,
		"verifying":    true,
		"blocked":      true,
		"complete":     true,
	}
	if !validStatus[document.Status] {
		return fmt.Errorf("unsupported Status %q", document.Status)
	}
	if (document.Status == "planned" || document.Status == "implementing") &&
		document.Progress.Total == 0 {
		return fmt.Errorf(
			"Status %s requires at least one checklist task",
			document.Status,
		)
	}
	if document.Status == "verifying" &&
		document.Progress.Pending != 0 {
		return fmt.Errorf(
			"Status %s requires zero pending tasks",
			document.Status,
		)
	}
	if document.Status == "complete" &&
		document.Progress.Pending != 0 &&
		document.CompletionNote == "" {
		return errors.New(
			"Status complete requires zero pending tasks unless Completion-Note records human-ordered deferral",
		)
	}

	if document.Status == "blocked" {
		validOrigin := map[string]bool{
			"refined":      true,
			"planned":      true,
			"implementing": true,
			"verifying":    true,
		}
		if !validOrigin[document.BlockedFrom] {
			return errors.New(
				"Status blocked requires a nonterminal Blocked-From field",
			)
		}
		if err := validateBlockerReason(document.BlockerReason); err != nil {
			return err
		}
	} else if document.BlockedFrom != "" || document.BlockerReason != "" {
		return errors.New(
			"Blocked-From and Blocker-Reason are valid only for Status blocked",
		)
	}
	if document.CompletionNote != "" {
		if document.Status != "complete" {
			return errors.New(
				"Completion-Note is valid only for Status complete",
			)
		}
		if err := validateCompletionNote(document.CompletionNote); err != nil {
			return err
		}
	}
	return nil
}

func validateBlockerReason(reason string) error {
	if reason == "" {
		return errors.New("Status blocked requires a Blocker-Reason field")
	}
	if len(reason) > maxBlockerBytes {
		return fmt.Errorf(
			"Blocker-Reason exceeds %d bytes",
			maxBlockerBytes,
		)
	}
	if !utf8.ValidString(reason) {
		return errors.New("Blocker-Reason must be valid UTF-8")
	}
	for _, character := range reason {
		if unicode.IsControl(character) {
			return errors.New("Blocker-Reason must not contain control characters")
		}
	}
	return nil
}

func validateCompletionNote(note string) error {
	if len(note) > maxCompletionNoteBytes {
		return fmt.Errorf(
			"Completion-Note exceeds %d bytes",
			maxCompletionNoteBytes,
		)
	}
	if !utf8.ValidString(note) {
		return errors.New("Completion-Note must be valid UTF-8")
	}
	for _, character := range note {
		if unicode.IsControl(character) {
			return errors.New("Completion-Note must not contain control characters")
		}
	}
	return nil
}
