package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
	"github.com/jpmartinez/higurashi-loop/internal/config"
)

func TestModelsSetPersistsAssignmentsAndRegeneratesAgents(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	if code, _, stderr := runAdapterJSON(
		t,
		repository,
		"install",
		"opencode",
	); code != 0 {
		t.Fatalf("adapter install code = %d\n%s", code, stderr)
	}

	code, response, stderr := runModelsJSON(
		t,
		repository,
		nil,
		"models",
		"set",
		"--orchestrator",
		"openai/control-model",
		"--apply",
		"openai/coding-model",
		"--apply-effort",
		"max",
		"--verify-contract",
		"openai/contract-model",
		"--verify-risk",
		"other/risk-model",
		"--json",
	)
	if code != 0 {
		t.Fatalf("models set code = %d\n%s", code, stderr)
	}
	if response.Kind != "updated" {
		t.Errorf("kind = %q, want updated", response.Kind)
	}

	configuration, err := config.Load(repository)
	if err != nil {
		t.Fatalf("load updated config: %v", err)
	}
	if configuration.Runners.OpenCode.Models.Apply != "openai/coding-model#max" {
		t.Errorf(
			"apply model = %q",
			configuration.Runners.OpenCode.Models.Apply,
		)
	}
	if configuration.Runners.OpenCode.Models.Plan != "inherit" {
		t.Errorf(
			"unspecified plan model = %q, want inherit",
			configuration.Runners.OpenCode.Models.Plan,
		)
	}
	assertFileContains(
		t,
		filepath.Join(
			repository,
			".opencode",
			"agents",
			"higurashi-apply.md",
		),
		"model: openai/coding-model\nvariant: max\n",
	)
	assertFileContains(
		t,
		filepath.Join(
			repository,
			".opencode",
			"agents",
			"higurashi-verify-risk.md",
		),
		"model: other/risk-model\n",
	)

	code, _, stderr = runModelsJSON(
		t,
		repository,
		nil,
		"models",
		"set",
		"--apply-effort",
		"inherit",
		"--json",
	)
	if code != 0 {
		t.Fatalf("clear APPLY effort code = %d\n%s", code, stderr)
	}
	configuration, err = config.Load(repository)
	if err != nil {
		t.Fatalf("load config after clearing effort: %v", err)
	}
	if configuration.Runners.OpenCode.Models.Apply != "openai/coding-model" {
		t.Errorf(
			"apply model after clearing effort = %q",
			configuration.Runners.OpenCode.Models.Apply,
		)
	}
}

func TestModelsSetPreflightsGeneratedConflictBeforeConfigMutation(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	if code, _, stderr := runAdapterJSON(
		t,
		repository,
		"install",
		"opencode",
	); code != 0 {
		t.Fatalf("adapter install code = %d\n%s", code, stderr)
	}
	configName := filepath.Join(repository, ".higurashi", "config.json")
	before, err := os.ReadFile(configName)
	if err != nil {
		t.Fatalf("read config before conflict: %v", err)
	}
	agentName := filepath.Join(
		repository,
		".opencode",
		"agents",
		"higurashi-apply.md",
	)
	file, err := os.OpenFile(agentName, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open generated agent: %v", err)
	}
	if _, err := file.WriteString("\nlocal edit\n"); err != nil {
		t.Fatalf("modify generated agent: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close generated agent: %v", err)
	}

	code, response, _ := runModelsJSON(
		t,
		repository,
		nil,
		"models",
		"set",
		"--apply",
		"openai/coding-model",
		"--json",
	)
	if code != 8 || response.Kind != "generated_file_conflict" {
		t.Errorf(
			"result = code:%d kind:%q, want conflict",
			code,
			response.Kind,
		)
	}
	after, err := os.ReadFile(configName)
	if err != nil {
		t.Fatalf("read config after conflict: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("models set changed config after generated-file conflict")
	}
}

func TestModelsValidateChecksOpenCodeCatalog(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	configuration, err := config.Load(repository)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	configuration.Runners.OpenCode.Models.Apply = "openai/coding-model#max"
	content, err := config.Encode(configuration)
	if err != nil {
		t.Fatalf("encode config: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(repository, ".higurashi", "config.json"),
		content,
		0o600,
	); err != nil {
		t.Fatalf("write configured models: %v", err)
	}

	code, response, stderr := runModelsJSON(
		t,
		repository,
		&modelsRunner{
			root:    repository,
			catalog: "openai/coding-model\n{\"variants\":{\"low\":{},\"max\":{}}}\nother/reviewer\n{}\n",
		},
		"models",
		"validate",
		"--json",
	)
	if code != 0 || response.Kind != "valid" {
		t.Fatalf(
			"valid catalog result = code:%d kind:%q\n%s",
			code,
			response.Kind,
			stderr,
		)
	}

	code, response, _ = runModelsJSON(
		t,
		repository,
		&modelsRunner{root: repository, catalog: "other/reviewer\n"},
		"models",
		"validate",
		"--json",
	)
	if code != 6 || response.Kind != "models_unavailable" {
		t.Errorf(
			"missing model result = code:%d kind:%q",
			code,
			response.Kind,
		)
	}

	code, response, _ = runModelsJSON(
		t,
		repository,
		&modelsRunner{
			root:    repository,
			catalog: "openai/coding-model\n{\"variants\":{\"low\":{}}}\n",
		},
		"models",
		"validate",
		"--json",
	)
	if code != 6 || response.Kind != "models_unavailable" {
		t.Errorf(
			"missing variant result = code:%d kind:%q",
			code,
			response.Kind,
		)
	}
}

func TestModelsSetRejectsUnsafeValue(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")

	code, response, _ := runModelsJSON(
		t,
		repository,
		nil,
		"models",
		"set",
		"--apply",
		"not a model",
		"--json",
	)
	if code != 3 || response.Kind != "models_invalid" {
		t.Errorf(
			"result = code:%d kind:%q, want models_invalid",
			code,
			response.Kind,
		)
	}

	code, response, _ = runModelsJSON(
		t,
		repository,
		nil,
		"models",
		"set",
		"--apply-effort",
		"max",
		"--json",
	)
	if code != 3 || response.Kind != "models_invalid" {
		t.Errorf(
			"effort without model = code:%d kind:%q, want models_invalid",
			code,
			response.Kind,
		)
	}
}

type modelsJSONResult struct {
	Kind         string       `json:"kind"`
	Message      string       `json:"message"`
	Runner       string       `json:"runner"`
	Models       []modelEntry `json:"models"`
	ChangedPaths []string     `json:"changedPaths"`
}

type modelEntry struct {
	Role    string `json:"role"`
	Model   string `json:"model"`
	Variant string `json:"variant"`
}

func runModelsJSON(
	t *testing.T,
	workingDirectory string,
	runner *modelsRunner,
	args ...string,
) (int, modelsJSONResult, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	options := cli.Options{
		WorkingDirectory: workingDirectory,
		Version:          "test",
	}
	if runner != nil {
		options.CommandRunner = runner
	}
	code := cli.Run(
		context.Background(),
		args,
		&stdout,
		&stderr,
		options,
	)
	var response modelsJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf(
			"decode models JSON: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return code, response, stderr.String()
}

type modelsRunner struct {
	root    string
	catalog string
}

func (runner *modelsRunner) Output(
	_ context.Context,
	_ int,
	name string,
	args ...string,
) ([]byte, error) {
	switch {
	case name == "git":
		return []byte(runner.root + "\n"), nil
	case name == "opencode" &&
		len(args) == 2 &&
		args[0] == "models" &&
		args[1] == "--verbose":
		return []byte(runner.catalog), nil
	default:
		return nil, errors.New("unexpected command")
	}
}

func assertFileContains(t *testing.T, name, expected string) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if !strings.Contains(string(content), expected) {
		t.Errorf("%s does not contain %q", name, expected)
	}
}
