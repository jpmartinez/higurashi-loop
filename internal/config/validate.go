package config

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

var modelIDPattern = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._:/+-]*(#[A-Za-z0-9][A-Za-z0-9._+-]*)?$`,
)

// Validate checks all deterministic schema version 1 invariants.
func Validate(configuration Config) error {
	if configuration.SchemaVersion != SchemaVersion {
		return fmt.Errorf(
			"schemaVersion must be %d",
			SchemaVersion,
		)
	}

	pattern, err := regexp.Compile(configuration.WorkItems.IDPattern)
	if err != nil {
		return fmt.Errorf("workItems.idPattern: %w", err)
	}
	if pattern.MatchString("WORK/123") || pattern.MatchString(`WORK\123`) {
		return errors.New("workItems.idPattern must reject path separators")
	}

	if len(configuration.WorkItems.RequirementSources) == 0 {
		return errors.New("workItems.requirementSources must not be empty")
	}
	seenRequirementSources := make(map[string]bool)
	for index, source := range configuration.WorkItems.RequirementSources {
		if err := validateProjectRelativePath(source, true); err != nil {
			return fmt.Errorf(
				"workItems.requirementSources[%d]: %w",
				index,
				err,
			)
		}
		normalized := normalizePath(source)
		if seenRequirementSources[normalized] {
			return fmt.Errorf(
				"workItems.requirementSources[%d]: duplicate source %q",
				index,
				normalized,
			)
		}
		seenRequirementSources[normalized] = true
	}

	if err := validateProjectRelativePath(
		configuration.Artifacts.Directory,
		false,
	); err != nil {
		return fmt.Errorf("artifacts.directory: %w", err)
	}

	if configuration.Loop.MaxApplyBatchesPerRun < 1 ||
		configuration.Loop.MaxApplyBatchesPerRun > 32 {
		return errors.New("loop.maxApplyBatchesPerRun must be between 1 and 32")
	}
	if configuration.Loop.MaxRepairAttempts < 0 ||
		configuration.Loop.MaxRepairAttempts > 1 {
		return errors.New("loop.maxRepairAttempts must be 0 or 1")
	}

	if configuration.CodeGraph.Mode != "required" &&
		configuration.CodeGraph.Mode != "preferred" {
		return errors.New(`codegraph.mode must be "required" or "preferred"`)
	}

	for index, name := range configuration.Context.InstructionFiles {
		if err := validateProjectRelativePath(name, true); err != nil {
			return fmt.Errorf("context.instructionFiles[%d]: %w", index, err)
		}
	}
	for index, name := range configuration.Context.AuthoritativeFiles {
		if err := validateProjectRelativePath(name, true); err != nil {
			return fmt.Errorf("context.authoritativeFiles[%d]: %w", index, err)
		}
	}

	commandGroups := []struct {
		name     string
		commands []Command
	}{
		{"normalizationCommands", configuration.Verification.NormalizationCommands},
		{"requiredCommands", configuration.Verification.RequiredCommands},
		{"candidateFormatCommands", configuration.Verification.CandidateFormatCommands},
	}
	for _, group := range commandGroups {
		for index, command := range group.commands {
			if err := validateCommand(command); err != nil {
				return fmt.Errorf(
					"verification.%s[%d]: %w",
					group.name,
					index,
					err,
				)
			}
		}
	}

	runners := []struct {
		name   string
		runner Runner
	}{
		{"opencode", configuration.Runners.OpenCode},
		{"claudeCode", configuration.Runners.ClaudeCode},
	}
	for _, item := range runners {
		if err := ValidateModelAssignments(item.runner.Models); err != nil {
			return fmt.Errorf("runners.%s.models: %w", item.name, err)
		}
	}

	return nil
}

func normalize(configuration *Config) {
	for index, source := range configuration.WorkItems.RequirementSources {
		configuration.WorkItems.RequirementSources[index] = normalizePath(source)
	}
	configuration.Artifacts.Directory = normalizePath(
		configuration.Artifacts.Directory,
	)
	for index, name := range configuration.Context.InstructionFiles {
		configuration.Context.InstructionFiles[index] = normalizePath(name)
	}
	for index, name := range configuration.Context.AuthoritativeFiles {
		configuration.Context.AuthoritativeFiles[index] = normalizePath(name)
	}
}

func normalizePath(name string) string {
	return path.Clean(strings.ReplaceAll(name, `\`, "/"))
}

// NormalizeProjectPath canonicalizes a user-supplied project-relative path.
func NormalizeProjectPath(name string) string {
	return normalizePath(name)
}

func validateProjectRelativePath(name string, allowRoot bool) error {
	if name == "" {
		return errors.New("must not be empty")
	}
	if strings.ContainsRune(name, 0) {
		return errors.New("must not contain a NUL byte")
	}

	normalized := strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(normalized, "/") ||
		strings.HasPrefix(normalized, "//") ||
		hasWindowsVolume(normalized) {
		return errors.New("must be project-relative")
	}

	cleaned := path.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return errors.New("must not escape the project root")
	}
	if !allowRoot && cleaned == "." {
		return errors.New("must not be the project root")
	}
	return nil
}

func hasWindowsVolume(name string) bool {
	return len(name) >= 2 &&
		((name[0] >= 'A' && name[0] <= 'Z') ||
			(name[0] >= 'a' && name[0] <= 'z')) &&
		name[1] == ':'
}

func validateCommand(command Command) error {
	if len(command.Argv) == 0 {
		return errors.New("argv must not be empty")
	}
	for index, argument := range command.Argv {
		if strings.ContainsRune(argument, 0) {
			return fmt.Errorf("argv[%d] must not contain a NUL byte", index)
		}
	}
	if command.TimeoutSeconds <= 0 {
		return errors.New("timeoutSeconds must be greater than zero")
	}
	return nil
}

// ValidateModelAssignments checks model IDs before they enter runner files.
func ValidateModelAssignments(assignments ModelAssignments) error {
	values := []struct {
		name  string
		value string
	}{
		{"orchestrator", assignments.Orchestrator},
		{"refine", assignments.Refine},
		{"plan", assignments.Plan},
		{"apply", assignments.Apply},
		{"verifyContract", assignments.VerifyContract},
		{"verifyRisk", assignments.VerifyRisk},
	}
	for _, item := range values {
		if err := ValidateModelReference(item.value); err != nil {
			return fmt.Errorf("%s: %w", item.name, err)
		}
	}
	return nil
}

// ValidateModelReference checks one runner-native model selector.
func ValidateModelReference(reference string) error {
	if reference == "inherit" {
		return nil
	}
	if len(reference) > 200 {
		return errors.New("model reference exceeds 200 characters")
	}
	if !modelIDPattern.MatchString(reference) {
		return errors.New(
			`must be "inherit" or a safe provider/model#variant reference`,
		)
	}
	return nil
}

// SplitModelReference separates an optional runner-native variant.
func SplitModelReference(reference string) (model, variant string) {
	model, variant, _ = strings.Cut(reference, "#")
	return model, variant
}

// WithModelVariant replaces the optional variant on one model reference.
func WithModelVariant(reference, variant string) (string, error) {
	model, _ := SplitModelReference(reference)
	if model == "inherit" {
		if variant == "" || variant == "inherit" {
			return "inherit", nil
		}
		return "", errors.New(
			"an explicit model is required before setting its effort",
		)
	}
	if variant == "" || variant == "inherit" {
		return model, nil
	}
	candidate := model + "#" + variant
	if err := ValidateModelReference(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}
