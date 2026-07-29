package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
	"github.com/jpmartinez/higurashi-loop/internal/config"
)

func TestConfigRequirementsSetUnblocksInspection(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)
	requirementSource := filepath.ToSlash(filepath.Join(
		"requirements",
		"Product requirements.md",
	))
	writeFile(
		t,
		filepath.Join(repository, filepath.FromSlash(requirementSource)),
		"### WORK-123: Export audit records\n",
	)

	exitCode, result, stderr := runJSON(
		t,
		repository,
		"config",
		"requirements",
		"set",
		requirementSource,
		"--json",
	)
	if exitCode != 0 {
		t.Fatalf(
			"config requirements set exit code = %d\n%s",
			exitCode,
			stderr,
		)
	}
	if result.Kind != "updated" {
		t.Errorf("kind = %q, want updated", result.Kind)
	}
	configuration, err := config.Load(repository)
	if err != nil {
		t.Fatalf("load updated config: %v", err)
	}
	if len(configuration.WorkItems.RequirementSources) != 1 ||
		configuration.WorkItems.RequirementSources[0] != requirementSource {
		t.Errorf(
			"requirement sources = %v",
			configuration.WorkItems.RequirementSources,
		)
	}

	var stdout bytes.Buffer
	var inspectStderr bytes.Buffer
	exitCode = cli.Run(
		context.Background(),
		[]string{"inspect", "WORK-123", "--json"},
		&stdout,
		&inspectStderr,
		cli.Options{
			WorkingDirectory: repository,
			Version:          "test",
			CommandRunner: &doctorRunner{
				projectRoot: repository,
			},
		},
	)
	if exitCode != 0 {
		t.Fatalf(
			"inspect exit code = %d\nstdout:\n%s\nstderr:\n%s",
			exitCode,
			stdout.String(),
			inspectStderr.String(),
		)
	}
	var inspected jsonResult
	if err := json.Unmarshal(stdout.Bytes(), &inspected); err != nil {
		t.Fatalf("decode inspect result: %v", err)
	}
	if inspected.Kind != "ready" {
		t.Errorf("inspect kind = %q, want ready", inspected.Kind)
	}
	if inspected.RequirementSource != requirementSource {
		t.Errorf(
			"requirement source = %q, want %q",
			inspected.RequirementSource,
			requirementSource,
		)
	}
	if inspected.ArtifactPath == "" {
		t.Error("inspect did not return an artifact path")
	}
}

func TestConfigRequirementsSetRejectsMissingSourceWithoutMutation(t *testing.T) {
	repository := newGitRepository(t)
	configName := filepath.Join(repository, ".higurashi", "config.json")
	writeFile(
		t,
		configName,
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)
	before, err := os.ReadFile(configName)
	if err != nil {
		t.Fatalf("read config before command: %v", err)
	}

	exitCode, result, _ := runJSON(
		t,
		repository,
		"config",
		"requirements",
		"set",
		"missing.md",
		"--json",
	)

	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3", exitCode)
	}
	if result.Kind != "requirement_source_invalid" {
		t.Errorf(
			"kind = %q, want requirement_source_invalid",
			result.Kind,
		)
	}
	after, err := os.ReadFile(configName)
	if err != nil {
		t.Fatalf("read config after command: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("invalid requirement source changed configuration")
	}
}

func TestConfigRequirementsSetIsIdempotent(t *testing.T) {
	repository := newGitRepository(t)
	source := "requirements/product.md"
	writeFile(
		t,
		filepath.Join(repository, filepath.FromSlash(source)),
		"## WORK-123 Behavior\n",
	)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{
  "schemaVersion": 1,
  "workItems": {
    "requirementSources": ["requirements/product.md"]
  },
  "codegraph": {
    "mode": "preferred"
  }
}`,
	)

	exitCode, result, stderr := runJSON(
		t,
		repository,
		"config",
		"requirements",
		"set",
		source,
		"--json",
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d\n%s", exitCode, stderr)
	}
	if result.Kind != "current" {
		t.Errorf("kind = %q, want current", result.Kind)
	}
}
