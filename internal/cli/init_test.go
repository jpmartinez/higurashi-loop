package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
	"github.com/jpmartinez/higurashi-loop/internal/config"
)

func TestInitCreatesSelectedOpenCodeProjectFiles(t *testing.T) {
	repository := newGitRepository(t)

	exitCode, result, stderr := runInitJSON(
		t,
		repository,
		"init",
		"--runner",
		"opencode",
		"--json",
	)

	if exitCode != 0 {
		t.Fatalf("init exit code = %d, want 0\nstderr:\n%s", exitCode, stderr)
	}
	if !result.OK || result.Kind != "initialized" {
		t.Errorf("result = ok:%v kind:%q, want initialized", result.OK, result.Kind)
	}
	if !slices.Equal(result.Adapters, []string{"opencode"}) {
		t.Errorf("adapters = %v, want [opencode]", result.Adapters)
	}
	for _, relative := range []string{
		".higurashi/config.json",
		".higurashi/generated.json",
		".agents/skills/higurashi-refine/SKILL.md",
		".opencode/commands/higurashi-refine.md",
		".opencode/commands/higurashi-deliver.md",
		".opencode/agents/higurashi-orchestrator.md",
	} {
		if _, err := os.Stat(filepath.Join(
			repository,
			filepath.FromSlash(relative),
		)); err != nil {
			t.Errorf("expected initialized path %s: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repository, "skills")); !os.IsNotExist(err) {
		t.Errorf("Claude plugin path exists after OpenCode-only init: %v", err)
	}
	info, err := os.Stat(filepath.Join(repository, "docs", "higurashi"))
	if err != nil || !info.IsDir() {
		t.Errorf("artifact directory is not present: %v", err)
	}

	configuration, err := config.Load(repository)
	if err != nil {
		t.Fatalf("load initialized config: %v", err)
	}
	if !configuration.Runners.OpenCode.Enabled ||
		configuration.Runners.ClaudeCode.Enabled {
		t.Errorf("initialized runners = %+v, want only OpenCode", configuration.Runners)
	}
	if configuration.CodeGraph.Mode != "required" {
		t.Errorf("CodeGraph mode = %q, want required", configuration.CodeGraph.Mode)
	}
	if len(result.NextCommands) == 0 ||
		!slices.Contains(result.NextCommands, "higurashi doctor") {
		t.Errorf("nextCommands = %v, want doctor", result.NextCommands)
	}
}

func TestInitAcceptsExplicitRequirementSource(t *testing.T) {
	repository := newGitRepository(t)
	requirementSource := filepath.Join(
		"requirements",
		"Product requirements.md",
	)
	writeFile(
		t,
		filepath.Join(repository, requirementSource),
		"### WORK-123: Export audit records\n",
	)

	exitCode, result, stderr := runInitJSON(
		t,
		repository,
		"init",
		"--runner",
		"opencode",
		"--requirement-source",
		requirementSource,
		"--json",
	)
	if exitCode != 0 {
		t.Fatalf("init exit code = %d\n%s", exitCode, stderr)
	}
	configuration, err := config.Load(repository)
	if err != nil {
		t.Fatalf("load initialized config: %v", err)
	}
	if !slices.Equal(
		configuration.WorkItems.RequirementSources,
		[]string{filepath.ToSlash(requirementSource)},
	) {
		t.Errorf(
			"requirement sources = %v",
			configuration.WorkItems.RequirementSources,
		)
	}
	if result.Kind != "initialized" {
		t.Errorf("kind = %q, want initialized", result.Kind)
	}
}

func TestInitIsIdempotentForSameRunnerSelection(t *testing.T) {
	repository := newGitRepository(t)
	args := []string{"init", "--runner", "opencode", "--json"}
	firstExitCode, _, firstStderr := runInitJSON(t, repository, args...)
	if firstExitCode != 0 {
		t.Fatalf("first init exit code = %d\n%s", firstExitCode, firstStderr)
	}
	before, err := os.ReadFile(
		filepath.Join(repository, ".higurashi", "config.json"),
	)
	if err != nil {
		t.Fatalf("read config before repeated init: %v", err)
	}

	exitCode, result, stderr := runInitJSON(t, repository, args...)

	if exitCode != 0 {
		t.Fatalf("second init exit code = %d\n%s", exitCode, stderr)
	}
	if result.Kind != "current" {
		t.Errorf("kind = %q, want current", result.Kind)
	}
	if len(result.ChangedPaths) != 0 {
		t.Errorf("changedPaths = %v, want empty", result.ChangedPaths)
	}
	after, err := os.ReadFile(
		filepath.Join(repository, ".higurashi", "config.json"),
	)
	if err != nil {
		t.Fatalf("read config after repeated init: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("repeated init rewrote configuration")
	}
}

func TestInitPreflightsAdapterConflictBeforeCreatingProjectState(t *testing.T) {
	repository := newGitRepository(t)
	conflict := filepath.Join(
		repository,
		".opencode",
		"commands",
		"higurashi-deliver.md",
	)
	writeFile(t, conflict, "user-owned command\n")

	exitCode, result, _ := runInitJSON(
		t,
		repository,
		"init",
		"--runner",
		"opencode",
		"--json",
	)

	if exitCode != 7 {
		t.Errorf("exit code = %d, want conflict code 7", exitCode)
	}
	if result.Kind != "generated_file_conflict" {
		t.Errorf("kind = %q, want generated_file_conflict", result.Kind)
	}
	if _, err := os.Stat(filepath.Join(
		repository,
		".higurashi",
		"config.json",
	)); !os.IsNotExist(err) {
		t.Errorf("configuration exists after failed preflight: %v", err)
	}
	if _, err := os.Stat(filepath.Join(
		repository,
		"docs",
		"higurashi",
	)); !os.IsNotExist(err) {
		t.Errorf("artifact directory exists after failed preflight: %v", err)
	}
	actual, err := os.ReadFile(conflict)
	if err != nil {
		t.Fatalf("read conflict after init: %v", err)
	}
	if string(actual) != "user-owned command\n" {
		t.Error("init overwrote user-owned command")
	}
}

func TestInitForceGeneratedReplacesOnlyRecognizedGeneratedFiles(t *testing.T) {
	repository := newGitRepository(t)
	args := []string{"init", "--runner", "opencode", "--json"}
	if exitCode, _, stderr := runInitJSON(t, repository, args...); exitCode != 0 {
		t.Fatalf("first init exit code = %d\n%s", exitCode, stderr)
	}
	commandName := filepath.Join(
		repository,
		".opencode",
		"commands",
		"higurashi-deliver.md",
	)
	original, err := os.ReadFile(commandName)
	if err != nil {
		t.Fatalf("read generated command: %v", err)
	}
	modified := append(append([]byte(nil), original...), []byte("\nlocal edit\n")...)
	if err := os.WriteFile(commandName, modified, 0o600); err != nil {
		t.Fatalf("modify generated command: %v", err)
	}

	exitCode, result, stderr := runInitJSON(t, repository, args...)
	if exitCode != 7 || result.Kind != "generated_file_conflict" {
		t.Fatalf(
			"ordinary repeated init = code:%d kind:%q, want conflict",
			exitCode,
			result.Kind,
		)
	}

	exitCode, result, stderr = runInitJSON(
		t,
		repository,
		"init",
		"--runner",
		"opencode",
		"--force-generated",
		"--json",
	)
	if exitCode != 0 {
		t.Fatalf("forced init exit code = %d\n%s", exitCode, stderr)
	}
	if !slices.Contains(
		result.ChangedPaths,
		".opencode/commands/higurashi-deliver.md",
	) {
		t.Errorf("forced changedPaths = %v, want repaired command", result.ChangedPaths)
	}
	repaired, err := os.ReadFile(commandName)
	if err != nil {
		t.Fatalf("read repaired command: %v", err)
	}
	if !bytes.Equal(repaired, original) {
		t.Error("--force-generated did not restore canonical generated content")
	}

	unrecognized := filepath.Join(
		repository,
		".opencode",
		"commands",
		"higurashi-refine.md",
	)
	if err := os.WriteFile(unrecognized, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatalf("replace generated file with unrecognized content: %v", err)
	}
	exitCode, result, _ = runInitJSON(
		t,
		repository,
		"init",
		"--runner",
		"opencode",
		"--force-generated",
		"--json",
	)
	if exitCode != 7 || result.Kind != "generated_file_conflict" {
		t.Errorf(
			"forced unrecognized init = code:%d kind:%q, want conflict",
			exitCode,
			result.Kind,
		)
	}
}

func TestInitSupportsBothRunnersAndExplicitProjectRoot(t *testing.T) {
	repository := newGitRepository(t)

	exitCode, result, stderr := runInitJSON(
		t,
		t.TempDir(),
		"init",
		"--project-root",
		repository,
		"--runner",
		"claude-code",
		"--runner",
		"opencode",
		"--json",
	)

	if exitCode != 0 {
		t.Fatalf("init exit code = %d\n%s", exitCode, stderr)
	}
	if !slices.Equal(result.Adapters, []string{"claude-code", "opencode"}) {
		t.Errorf("adapters = %v, want stable requested order", result.Adapters)
	}
	for _, relative := range []string{
		".opencode/commands/higurashi-deliver.md",
		"skills/deliver/SKILL.md",
		"skills/refine/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(
			repository,
			filepath.FromSlash(relative),
		)); err != nil {
			t.Errorf("missing selected runner path %s: %v", relative, err)
		}
	}
	configuration, err := config.Load(repository)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !configuration.Runners.OpenCode.Enabled ||
		!configuration.Runners.ClaudeCode.Enabled {
		t.Errorf("runners = %+v, want both enabled", configuration.Runners)
	}
}

func TestInitRefusesRunnerDisabledByExistingConfiguration(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(t, filepath.Join(repository, ".higurashi", "config.json"), `{
  "schemaVersion": 1,
  "runners": {
    "opencode": {
      "enabled": false
    },
    "claudeCode": {
      "enabled": true
    }
  }
}`)
	before, err := os.ReadFile(
		filepath.Join(repository, ".higurashi", "config.json"),
	)
	if err != nil {
		t.Fatalf("read existing config: %v", err)
	}

	exitCode, result, _ := runInitJSON(
		t,
		repository,
		"init",
		"--runner",
		"opencode",
		"--json",
	)

	if exitCode != 7 || result.Kind != "configuration_conflict" {
		t.Errorf(
			"result = code:%d kind:%q, want configuration conflict",
			exitCode,
			result.Kind,
		)
	}
	after, err := os.ReadFile(
		filepath.Join(repository, ".higurashi", "config.json"),
	)
	if err != nil {
		t.Fatalf("read config after refused init: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("refused init rewrote existing configuration")
	}
	if _, err := os.Stat(filepath.Join(
		repository,
		".opencode",
	)); !os.IsNotExist(err) {
		t.Errorf("OpenCode adapter exists after refused init: %v", err)
	}
}

func TestInitRejectsMissingDuplicateAndUnknownRunners(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing", []string{"init", "--json"}, "at least one --runner"},
		{
			"duplicate",
			[]string{"init", "--runner", "opencode", "--runner", "opencode", "--json"},
			"may be supplied only once",
		},
		{
			"unknown",
			[]string{"init", "--runner", "other", "--json"},
			"unsupported runner",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exitCode, result, _ := runInitJSON(
				t,
				newGitRepository(t),
				test.args...,
			)
			if exitCode != 2 {
				t.Errorf("exit code = %d, want 2", exitCode)
			}
			if result.Kind != "invalid_usage" ||
				!strings.Contains(result.Message, test.want) {
				t.Errorf("result = %+v, want message containing %q", result, test.want)
			}
		})
	}
}

type initJSONResult struct {
	SchemaVersion int      `json:"schemaVersion"`
	Command       string   `json:"command"`
	OK            bool     `json:"ok"`
	Kind          string   `json:"kind"`
	Message       string   `json:"message"`
	ProjectRoot   string   `json:"projectRoot"`
	Adapters      []string `json:"adapters"`
	ChangedPaths  []string `json:"changedPaths"`
	Conflicts     []string `json:"conflicts"`
	NextCommands  []string `json:"nextCommands"`
}

func runInitJSON(
	t *testing.T,
	workingDirectory string,
	args ...string,
) (int, initJSONResult, string) {
	t.Helper()
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
		},
	)
	var result initJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf(
			"decode init JSON: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return exitCode, result, stderr.String()
}
