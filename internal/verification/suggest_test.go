package verification_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/verification"
)

func TestSuggestPackageScriptsReturnsExactProjectOwnedCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bun.lock"), "")
	writeFile(t, filepath.Join(root, "package.json"), `{
  "scripts": {
    "build": "vite build",
    "format": "biome format --write .",
    "format:check": "biome format .",
    "lint": "biome lint .",
    "supabase:test": "supabase db reset --local && supabase test db",
    "test": "bun test",
    "typecheck": "tsc --noEmit"
  }
}`)

	report := verification.Suggest(root, config.Verification{})

	if len(report.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", report.Warnings)
	}
	var ids []string
	for _, suggestion := range report.Suggestions {
		ids = append(ids, suggestion.ID)
	}
	wantIDs := []string{
		"package-json-normalization-format",
		"package-json-required-build",
		"package-json-required-lint",
		"package-json-required-supabase-test",
		"package-json-required-test",
		"package-json-required-typecheck",
		"package-json-candidate-format-check",
	}
	if !slices.Equal(ids, wantIDs) {
		t.Fatalf("suggestion IDs = %v, want %v", ids, wantIDs)
	}

	supabase := report.Suggestions[3]
	if !slices.Equal(
		supabase.Command.Argv,
		[]string{"bun", "run", "supabase:test"},
	) {
		t.Errorf("supabase argv = %v", supabase.Command.Argv)
	}
	if supabase.ConfigField != "verification.requiredCommands" ||
		!supabase.RequiresReview ||
		supabase.Reason == "" {
		t.Errorf("supabase suggestion = %+v, want reviewed required command", supabase)
	}
	if len(report.ConfigFragment.RequiredCommands) != 5 ||
		len(report.ConfigFragment.NormalizationCommands) != 1 ||
		len(report.ConfigFragment.CandidateFormatCommands) != 1 {
		t.Errorf("config fragment = %+v", report.ConfigFragment)
	}
}

func TestSuggestOmitsCommandsAlreadyConfiguredByArgv(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bun.lock"), "")
	writeFile(t, filepath.Join(root, "package.json"), `{
  "scripts": {
    "lint": "biome lint .",
    "test": "bun test"
  }
}`)
	existing := config.Verification{
		RequiredCommands: []config.Command{{
			Argv:           []string{"bun", "run", "lint"},
			TimeoutSeconds: 30,
		}},
	}

	report := verification.Suggest(root, existing)

	if len(report.Suggestions) != 1 ||
		report.Suggestions[0].ID != "package-json-required-test" {
		t.Fatalf("suggestions = %+v, want only test", report.Suggestions)
	}
}

func TestSuggestGoProjectUsesPortableGoCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.test/project\n\ngo 1.25\n")

	report := verification.Suggest(root, config.Verification{})

	if len(report.Suggestions) != 2 {
		t.Fatalf("suggestions = %+v, want go test and vet", report.Suggestions)
	}
	if !slices.Equal(
		report.Suggestions[0].Command.Argv,
		[]string{"go", "test", "./..."},
	) || !slices.Equal(
		report.Suggestions[1].Command.Argv,
		[]string{"go", "vet", "./..."},
	) {
		t.Errorf("Go suggestions = %+v", report.Suggestions)
	}
}

func TestSuggestMalformedPackageManifestWarnsWithoutInventingCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{")

	report := verification.Suggest(root, config.Verification{})

	if len(report.Suggestions) != 0 || len(report.Warnings) != 1 {
		t.Fatalf("report = %+v, want one warning and no suggestions", report)
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", name, err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
