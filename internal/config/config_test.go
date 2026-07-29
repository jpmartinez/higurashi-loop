package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/config"
)

func TestLoadDefaultsMissingModelAssignmentsToInherit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".higurashi"), 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".higurashi", "config.json"),
		[]byte(`{"schemaVersion":1}`),
		0o600,
	); err != nil {
		t.Fatalf("write configuration: %v", err)
	}

	configuration, err := config.Load(root)
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if configuration.Runners.OpenCode.Models != config.InheritedModels() {
		t.Errorf(
			"OpenCode models = %+v, want inherited assignments",
			configuration.Runners.OpenCode.Models,
		)
	}
	if configuration.Runners.ClaudeCode.Models != config.InheritedModels() {
		t.Errorf(
			"Claude Code models = %+v, want inherited assignments",
			configuration.Runners.ClaudeCode.Models,
		)
	}
}

func TestValidateRejectsUnsafeModelFrontmatterValues(t *testing.T) {
	configuration := config.Defaults()
	configuration.Runners.OpenCode.Models.Apply = "openai/model\npermission: allow"

	if err := config.Validate(configuration); err == nil {
		t.Fatal("Validate accepted a model ID containing a YAML injection")
	}
}

func TestWithModelVariantSetsAndClearsEffort(t *testing.T) {
	reference, err := config.WithModelVariant("openai/coding-model", "max")
	if err != nil {
		t.Fatalf("set model variant: %v", err)
	}
	if reference != "openai/coding-model#max" {
		t.Errorf("reference = %q, want max variant", reference)
	}
	reference, err = config.WithModelVariant(reference, "inherit")
	if err != nil {
		t.Fatalf("clear model variant: %v", err)
	}
	if reference != "openai/coding-model" {
		t.Errorf("reference = %q, want base model", reference)
	}
	if _, err := config.WithModelVariant("inherit", "max"); err == nil {
		t.Fatal("WithModelVariant accepted effort without an explicit model")
	}
}
