// Package verification discovers project-owned verification commands without
// executing them or granting workflow authority.
package verification

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/config"
)

const manifestLimit = 1 << 20

const (
	categoryNormalization = "normalization"
	categoryRequired      = "required"
	categoryCandidate     = "candidate-format"
)

// Suggestion is one exact, untrusted command inferred from project metadata.
// It becomes workflow authority only after a user adds it to configuration.
type Suggestion struct {
	ID             string         `json:"id"`
	Category       string         `json:"category"`
	ConfigField    string         `json:"configField"`
	Command        config.Command `json:"command"`
	DisplayCommand string         `json:"displayCommand"`
	Source         string         `json:"source"`
	Confidence     string         `json:"confidence"`
	RequiresReview bool           `json:"requiresReview"`
	Reason         string         `json:"reason,omitempty"`
}

// Report is the deterministic, read-only discovery result.
type Report struct {
	Suggestions    []Suggestion        `json:"suggestions"`
	ConfigFragment config.Verification `json:"configFragment"`
	Warnings       []string            `json:"warnings,omitempty"`
}

// Suggest reads known project manifests and returns commands not already
// configured. It never executes commands or modifies project files.
func Suggest(root string, existing config.Verification) Report {
	var report Report
	report.Suggestions = append(
		report.Suggestions,
		suggestPackageScripts(root, &report.Warnings)...,
	)
	report.Suggestions = append(
		report.Suggestions,
		suggestGoCommands(root)...,
	)

	report.Suggestions = slices.DeleteFunc(
		report.Suggestions,
		func(suggestion Suggestion) bool {
			return containsArgv(existing, suggestion.Command.Argv)
		},
	)
	sort.Slice(report.Suggestions, func(left, right int) bool {
		leftRank := categoryRank(report.Suggestions[left].Category)
		rightRank := categoryRank(report.Suggestions[right].Category)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return report.Suggestions[left].ID < report.Suggestions[right].ID
	})
	sort.Strings(report.Warnings)
	for _, suggestion := range report.Suggestions {
		command := cloneCommand(suggestion.Command)
		switch suggestion.Category {
		case categoryNormalization:
			report.ConfigFragment.NormalizationCommands = append(
				report.ConfigFragment.NormalizationCommands,
				command,
			)
		case categoryRequired:
			report.ConfigFragment.RequiredCommands = append(
				report.ConfigFragment.RequiredCommands,
				command,
			)
		case categoryCandidate:
			report.ConfigFragment.CandidateFormatCommands = append(
				report.ConfigFragment.CandidateFormatCommands,
				command,
			)
		}
	}
	report.ConfigFragment.NormalizationCommands = nonNil(
		report.ConfigFragment.NormalizationCommands,
	)
	report.ConfigFragment.RequiredCommands = nonNil(
		report.ConfigFragment.RequiredCommands,
	)
	report.ConfigFragment.CandidateFormatCommands = nonNil(
		report.ConfigFragment.CandidateFormatCommands,
	)
	return report
}

func suggestPackageScripts(root string, warnings *[]string) []Suggestion {
	name := filepath.Join(root, "package.json")
	content, err := readBounded(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		*warnings = append(
			*warnings,
			fmt.Sprintf("inspect package.json for verification suggestions: %v", err),
		)
		return nil
	}
	var manifest struct {
		PackageManager string            `json:"packageManager"`
		Scripts        map[string]string `json:"scripts"`
	}
	// package.json permits many unrelated fields, so decode through an
	// unrestricted intermediate value before extracting the two known fields.
	var document map[string]json.RawMessage
	if err := json.Unmarshal(content, &document); err != nil {
		*warnings = append(
			*warnings,
			fmt.Sprintf("decode package.json for verification suggestions: %v", err),
		)
		return nil
	}
	if value, ok := document["packageManager"]; ok {
		_ = json.Unmarshal(value, &manifest.PackageManager)
	}
	if value, ok := document["scripts"]; ok {
		if err := json.Unmarshal(value, &manifest.Scripts); err != nil {
			*warnings = append(
				*warnings,
				fmt.Sprintf("decode package.json scripts: %v", err),
			)
			return nil
		}
	}
	if len(manifest.Scripts) == 0 {
		return nil
	}

	manager := detectPackageManager(root, manifest.PackageManager)
	var suggestions []Suggestion
	for scriptName, body := range manifest.Scripts {
		category, ok := scriptCategory(scriptName)
		if !ok || strings.TrimSpace(body) == "" {
			continue
		}
		argv := packageScriptArgv(manager, scriptName)
		requiresReview, reason := scriptRisk(body)
		suggestions = append(suggestions, newSuggestion(
			"package-json-"+categoryID(category)+"-"+slug(scriptName),
			category,
			argv,
			scriptTimeout(scriptName, category, requiresReview),
			"package.json#scripts."+scriptName,
			requiresReview,
			reason,
		))
	}
	return suggestions
}

func suggestGoCommands(root string) []Suggestion {
	info, err := os.Stat(filepath.Join(root, "go.mod"))
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return []Suggestion{
		newSuggestion(
			"go-mod-required-test",
			categoryRequired,
			[]string{"go", "test", "./..."},
			300,
			"go.mod",
			false,
			"",
		),
		newSuggestion(
			"go-mod-required-vet",
			categoryRequired,
			[]string{"go", "vet", "./..."},
			300,
			"go.mod",
			false,
			"",
		),
	}
}

func newSuggestion(
	id, category string,
	argv []string,
	timeout int,
	source string,
	requiresReview bool,
	reason string,
) Suggestion {
	command := config.Command{
		Argv:           append([]string(nil), argv...),
		TimeoutSeconds: timeout,
	}
	return Suggestion{
		ID:             id,
		Category:       category,
		ConfigField:    configField(category),
		Command:        command,
		DisplayCommand: displayCommand(argv),
		Source:         source,
		Confidence:     "high",
		RequiresReview: requiresReview,
		Reason:         reason,
	}
}

func readBounded(name string) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, manifestLimit+1))
	if err != nil {
		return nil, err
	}
	if len(content) > manifestLimit {
		return nil, fmt.Errorf("manifest exceeds %d bytes", manifestLimit)
	}
	return content, nil
}

func detectPackageManager(root, declared string) string {
	declared = strings.ToLower(strings.TrimSpace(declared))
	for _, manager := range []string{"bun", "pnpm", "yarn", "npm"} {
		if declared == manager || strings.HasPrefix(declared, manager+"@") {
			return manager
		}
	}
	for _, candidate := range []struct {
		name    string
		manager string
	}{
		{"bun.lock", "bun"},
		{"bun.lockb", "bun"},
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"package-lock.json", "npm"},
	} {
		if info, err := os.Stat(filepath.Join(root, candidate.name)); err == nil &&
			info.Mode().IsRegular() {
			return candidate.manager
		}
	}
	return "npm"
}

func packageScriptArgv(manager, scriptName string) []string {
	switch manager {
	case "yarn":
		return []string{"yarn", scriptName}
	default:
		return []string{manager, "run", scriptName}
	}
}

func scriptCategory(name string) (string, bool) {
	switch name {
	case "format":
		return categoryNormalization, true
	case "format:check", "format-check", "check:format":
		return categoryCandidate, true
	case "build", "lint", "test", "typecheck", "type-check":
		return categoryRequired, true
	}
	if strings.HasSuffix(name, ":test") || strings.HasPrefix(name, "test:") {
		return categoryRequired, true
	}
	return "", false
}

func scriptRisk(body string) (bool, string) {
	lower := strings.ToLower(body)
	risks := []struct {
		needle string
		reason string
	}{
		{"reset", "script contains a reset operation that may replace local state"},
		{" drop ", "script contains a drop operation"},
		{"deploy", "script may deploy external state"},
		{"publish", "script may publish external state"},
		{" push", "script may push external state"},
		{"--force", "script contains a force option"},
		{"rm -", "script contains a removal operation"},
	}
	padded := " " + lower + " "
	for _, risk := range risks {
		if strings.Contains(padded, risk.needle) {
			return true, risk.reason
		}
	}
	return false, ""
}

func scriptTimeout(
	name, category string,
	requiresReview bool,
) int {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "supabase"),
		strings.Contains(lower, "database"),
		strings.Contains(lower, "db"),
		requiresReview:
		return 600
	case name == "build", name == "test",
		strings.Contains(name, ":test"), strings.HasPrefix(name, "test:"):
		return 300
	case category == categoryNormalization || category == categoryCandidate:
		return 120
	default:
		return 180
	}
}

func containsArgv(existing config.Verification, argv []string) bool {
	groups := [][]config.Command{
		existing.NormalizationCommands,
		existing.RequiredCommands,
		existing.CandidateFormatCommands,
	}
	for _, commands := range groups {
		for _, command := range commands {
			if slices.Equal(command.Argv, argv) {
				return true
			}
		}
	}
	return false
}

func categoryRank(category string) int {
	switch category {
	case categoryNormalization:
		return 0
	case categoryRequired:
		return 1
	case categoryCandidate:
		return 2
	default:
		return 3
	}
}

func categoryID(category string) string {
	if category == categoryCandidate {
		return "candidate"
	}
	return category
}

func configField(category string) string {
	switch category {
	case categoryNormalization:
		return "verification.normalizationCommands"
	case categoryRequired:
		return "verification.requiredCommands"
	case categoryCandidate:
		return "verification.candidateFormatCommands"
	default:
		return "verification"
	}
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slug(value string) string {
	value = strings.ToLower(value)
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

var simpleShellArgument = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

func displayCommand(argv []string) string {
	parts := make([]string, len(argv))
	for index, argument := range argv {
		if simpleShellArgument.MatchString(argument) {
			parts[index] = argument
			continue
		}
		parts[index] = "'" + strings.ReplaceAll(argument, "'", `'\"'\"'`) + "'"
	}
	return strings.Join(parts, " ")
}

func cloneCommand(command config.Command) config.Command {
	command.Argv = append([]string(nil), command.Argv...)
	return command
}

func nonNil(commands []config.Command) []config.Command {
	if commands == nil {
		return []config.Command{}
	}
	return commands
}
