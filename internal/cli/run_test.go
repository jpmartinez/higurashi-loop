package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
)

type jsonResult struct {
	SchemaVersion     int            `json:"schemaVersion"`
	Command           string         `json:"command"`
	OK                bool           `json:"ok"`
	Kind              string         `json:"kind"`
	Message           string         `json:"message"`
	ProjectRoot       string         `json:"projectRoot"`
	RequirementSource string         `json:"requirementSource"`
	SourceKind        string         `json:"sourceKind"`
	SourceHash        string         `json:"sourceHash"`
	ArtifactPath      string         `json:"artifactPath"`
	Config            map[string]any `json:"config"`
	Warnings          []string       `json:"warnings"`
	CodeGraph         struct {
		Mode               string `json:"mode"`
		CLIAvailable       bool   `json:"cliAvailable"`
		IndexPresent       bool   `json:"indexPresent"`
		StatusHealthy      bool   `json:"statusHealthy"`
		RootMatches        bool   `json:"rootMatches"`
		Synchronization    string `json:"synchronization"`
		WatcherState       string `json:"watcherState"`
		Version            string `json:"version"`
		IndexState         string `json:"indexState"`
		PendingChanges     int    `json:"pendingChanges"`
		ReindexRecommended bool   `json:"reindexRecommended"`
	} `json:"codegraph"`
}

func TestConfigValidateAppliesDefaults(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(t, filepath.Join(repository, ".higurashi", "config.json"), `{
  "schemaVersion": 1
}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		[]string{"config", "validate", "--json"},
		&stdout,
		&stderr,
		cli.Options{
			WorkingDirectory: repository,
			Version:          "test",
		},
	)

	if exitCode != 0 {
		t.Fatalf(
			"config validate exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}

	var result struct {
		SchemaVersion int    `json:"schemaVersion"`
		Command       string `json:"command"`
		OK            bool   `json:"ok"`
		Kind          string `json:"kind"`
		Config        struct {
			Artifacts struct {
				Directory string `json:"directory"`
			} `json:"artifacts"`
			Loop struct {
				MaxApplyBatchesPerRun   int  `json:"maxApplyBatchesPerRun"`
				MaxRepairAttempts       int  `json:"maxRepairAttempts"`
				RequireProgressPerBatch bool `json:"requireProgressAfterEveryBatch"`
			} `json:"loop"`
			CodeGraph struct {
				Mode string `json:"mode"`
			} `json:"codegraph"`
		} `json:"config"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON result: %v\nstdout:\n%s", err, stdout.String())
	}

	if result.SchemaVersion != 1 {
		t.Errorf("schemaVersion = %d, want 1", result.SchemaVersion)
	}
	if result.Command != "config.validate" {
		t.Errorf("command = %q, want %q", result.Command, "config.validate")
	}
	if !result.OK {
		t.Error("ok = false, want true")
	}
	if result.Kind != "valid" {
		t.Errorf("kind = %q, want %q", result.Kind, "valid")
	}
	if result.Config.Artifacts.Directory != "docs/higurashi" {
		t.Errorf(
			"artifacts.directory = %q, want %q",
			result.Config.Artifacts.Directory,
			"docs/higurashi",
		)
	}
	if result.Config.Loop.MaxApplyBatchesPerRun != 8 {
		t.Errorf(
			"loop.maxApplyBatchesPerRun = %d, want 8",
			result.Config.Loop.MaxApplyBatchesPerRun,
		)
	}
	if result.Config.Loop.MaxRepairAttempts != 1 {
		t.Errorf(
			"loop.maxRepairAttempts = %d, want 1",
			result.Config.Loop.MaxRepairAttempts,
		)
	}
	if !result.Config.Loop.RequireProgressPerBatch {
		t.Error("loop.requireProgressAfterEveryBatch = false, want true")
	}
	if result.Config.CodeGraph.Mode != "required" {
		t.Errorf(
			"codegraph.mode = %q, want %q",
			result.Config.CodeGraph.Mode,
			"required",
		)
	}
}

func TestConfigValidateReportsMissingConfiguration(t *testing.T) {
	repository := newGitRepository(t)

	exitCode, result, stderr := runJSON(
		t,
		repository,
		"config",
		"validate",
		"--json",
	)

	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3", exitCode)
	}
	if result.OK {
		t.Error("ok = true, want false")
	}
	if result.Kind != "configuration_missing" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"configuration_missing",
		)
	}
	if result.Message == "" {
		t.Error("message is empty")
	}
	if stderr == "" {
		t.Error("stderr is empty")
	}
}

func TestConfigValidateRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		configuration string
	}{
		{
			name:          "missing schema version",
			configuration: `{}`,
		},
		{
			name: "unsupported schema version",
			configuration: `{
  "schemaVersion": 2
}`,
		},
		{
			name: "unknown top-level field",
			configuration: `{
  "schemaVersion": 1,
  "surprise": true
}`,
		},
		{
			name: "unknown nested field",
			configuration: `{
  "schemaVersion": 1,
  "loop": {
    "surprise": true
  }
}`,
		},
		{
			name: "null object",
			configuration: `{
  "schemaVersion": 1,
  "loop": null
}`,
		},
		{
			name: "null array",
			configuration: `{
  "schemaVersion": 1,
  "context": {
    "authoritativeFiles": null
  }
}`,
		},
		{
			name: "artifact directory escapes root",
			configuration: `{
  "schemaVersion": 1,
  "artifacts": {
    "directory": "../outside"
  }
}`,
		},
		{
			name: "artifact directory is root",
			configuration: `{
  "schemaVersion": 1,
  "artifacts": {
    "directory": "."
  }
}`,
		},
		{
			name: "absolute requirement source",
			configuration: `{
  "schemaVersion": 1,
  "workItems": {
    "requirementSources": ["/tmp/requirements.md"]
  }
}`,
		},
		{
			name: "ID pattern accepts separators",
			configuration: `{
  "schemaVersion": 1,
  "workItems": {
    "idPattern": ".*"
  }
}`,
		},
		{
			name: "apply limit too low",
			configuration: `{
  "schemaVersion": 1,
  "loop": {
    "maxApplyBatchesPerRun": 0
  }
}`,
		},
		{
			name: "apply limit too high",
			configuration: `{
  "schemaVersion": 1,
  "loop": {
    "maxApplyBatchesPerRun": 33
  }
}`,
		},
		{
			name: "repair limit too high",
			configuration: `{
  "schemaVersion": 1,
  "loop": {
    "maxRepairAttempts": 2
  }
}`,
		},
		{
			name: "invalid CodeGraph mode",
			configuration: `{
  "schemaVersion": 1,
  "codegraph": {
    "mode": "disabled"
  }
}`,
		},
		{
			name: "shell command string",
			configuration: `{
  "schemaVersion": 1,
  "verification": {
    "requiredCommands": ["go test ./..."]
  }
}`,
		},
		{
			name:          "multiple JSON values",
			configuration: `{"schemaVersion": 1} {"schemaVersion": 1}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newGitRepository(t)
			writeFile(
				t,
				filepath.Join(repository, ".higurashi", "config.json"),
				test.configuration,
			)

			exitCode, result, _ := runJSON(
				t,
				repository,
				"config",
				"validate",
				"--json",
			)

			if exitCode != 3 {
				t.Errorf("exit code = %d, want 3", exitCode)
			}
			if result.OK {
				t.Error("ok = true, want false")
			}
			if result.Kind != "configuration_invalid" {
				t.Errorf(
					"kind = %q, want %q",
					result.Kind,
					"configuration_invalid",
				)
			}
			if result.Message == "" {
				t.Error("message is empty")
			}
		})
	}
}

func TestConfigValidateNormalizesProjectRelativePaths(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(t, filepath.Join(repository, ".higurashi", "config.json"), `{
  "schemaVersion": 1,
  "workItems": {
    "requirementSources": ["docs\\requirements.md"]
  },
  "artifacts": {
    "directory": "docs\\delivery\\..\\higurashi"
  }
}`)

	exitCode, result, _ := runJSON(
		t,
		repository,
		"config",
		"validate",
		"--json",
	)

	if exitCode != 0 {
		t.Fatalf(
			"exit code = %d, want 0; result = %#v",
			exitCode,
			result,
		)
	}
	artifacts, ok := result.Config["artifacts"].(map[string]any)
	if !ok {
		t.Fatalf("config.artifacts = %#v, want object", result.Config["artifacts"])
	}
	if artifacts["directory"] != "docs/higurashi" {
		t.Errorf(
			"artifacts.directory = %#v, want %q",
			artifacts["directory"],
			"docs/higurashi",
		)
	}
}

func TestConfigValidateRejectsNonGitDirectory(t *testing.T) {
	exitCode, result, _ := runJSON(
		t,
		t.TempDir(),
		"config",
		"validate",
		"--json",
	)

	if exitCode != 8 {
		t.Errorf("exit code = %d, want 8", exitCode)
	}
	if result.Kind != "not_git_project" {
		t.Errorf("kind = %q, want %q", result.Kind, "not_git_project")
	}
}

func TestHelpAndVersion(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains string
	}{
		{
			name:     "no arguments shows help",
			contains: "higurashi config validate",
		},
		{
			name:     "help command",
			args:     []string{"help"},
			contains: "higurashi config validate",
		},
		{
			name:     "version command",
			args:     []string{"version"},
			contains: "higurashi test",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := cli.Run(
				context.Background(),
				test.args,
				&stdout,
				&stderr,
				cli.Options{
					WorkingDirectory: t.TempDir(),
					Version:          "test",
				},
			)

			if exitCode != 0 {
				t.Errorf("exit code = %d, want 0", exitCode)
			}
			if !bytes.Contains(stdout.Bytes(), []byte(test.contains)) {
				t.Errorf(
					"stdout = %q, want it to contain %q",
					stdout.String(),
					test.contains,
				)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func runJSON(
	t *testing.T,
	workingDirectory string,
	args ...string,
) (int, jsonResult, string) {
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

	var result jsonResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf(
			"decode JSON result: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return exitCode, result, stderr.String()
}

func newGitRepository(t *testing.T) string {
	t.Helper()

	repository := t.TempDir()
	command := exec.Command("git", "init", "--quiet", repository)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	return repository
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("create parent directory for %s: %v", name, err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
