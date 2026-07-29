package workitem_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

func TestFindSearchesRequirementDirectoriesRecursively(t *testing.T) {
	root := t.TempDir()
	writeRequirement(t, root, "requirements/ignored.md", `# Examples

~~~
## WORK-123 — This fenced example is not a requirement
~~~
`)
	writeRequirement(t, root, "requirements/product/inspection.md", `# Inspection

### WORK-123 — Deterministic artifact inspection
`)

	match, err := workitem.Find(root, []string{"requirements"}, "WORK-123")
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if match.Source != "requirements/product/inspection.md" {
		t.Errorf(
			"match.Source = %q, want %q",
			match.Source,
			"requirements/product/inspection.md",
		)
	}
}

func TestFindRejectsAmbiguousRequirementHeadings(t *testing.T) {
	root := t.TempDir()
	writeRequirement(t, root, "requirements.md", `# Requirements

## WORK-123 — First definition
## WORK-123 — Second definition
`)

	_, err := workitem.Find(root, []string{"requirements.md"}, "WORK-123")

	if !errors.Is(err, workitem.ErrConflict) {
		t.Fatalf("Find() error = %v, want ErrConflict", err)
	}
}

func TestFindReportsMissingConfiguredSource(t *testing.T) {
	root := t.TempDir()

	_, err := workitem.Find(root, []string{"missing.md"}, "WORK-123")

	if !errors.Is(err, workitem.ErrMissingSource) {
		t.Fatalf("Find() error = %v, want ErrMissingSource", err)
	}
}

func writeRequirement(t *testing.T, root, relative, content string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("create requirement directory: %v", err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatalf("write requirement: %v", err)
	}
}
