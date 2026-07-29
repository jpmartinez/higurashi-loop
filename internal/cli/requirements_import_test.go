package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
	"github.com/jpmartinez/higurashi-loop/internal/config"
)

func TestRequirementsImportFromFileCreatesDurableSnapshot(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)
	source := "mvp-definitions/Product requirements.md"
	sourceContent := []byte(`# Product requirements

### WORK-123: Export audit records

Export records from the authoritative audit log.

#### Evidence

The export identifies its source.

### WORK-124: Another requirement

Do something else.
`)
	writeFile(
		t,
		filepath.Join(repository, filepath.FromSlash(source)),
		string(sourceContent),
	)

	exitCode, result, stderr := runJSON(
		t,
		repository,
		"requirements",
		"import",
		"WORK-123",
		"--from",
		source,
		"--json",
	)

	if exitCode != 0 {
		t.Fatalf("import exit code = %d\n%s", exitCode, stderr)
	}
	if result.Kind != "imported" {
		t.Errorf("kind = %q, want imported", result.Kind)
	}
	if result.SourceKind != "file" {
		t.Errorf("source kind = %q, want file", result.SourceKind)
	}
	wantSourceHash := fmt.Sprintf("%x", sha256.Sum256(sourceContent))
	if result.SourceHash != wantSourceHash {
		t.Errorf("source hash = %q, want %q", result.SourceHash, wantSourceHash)
	}
	wantPath := "docs/higurashi/requirements/WORK-123.md"
	if result.RequirementSource != wantPath {
		t.Errorf(
			"requirement source = %q, want %q",
			result.RequirementSource,
			wantPath,
		)
	}
	imported, err := os.ReadFile(filepath.Join(
		repository,
		filepath.FromSlash(wantPath),
	))
	if err != nil {
		t.Fatalf("read imported requirement: %v", err)
	}
	if !bytes.Contains(
		imported,
		[]byte("Source: \"mvp-definitions/Product requirements.md\""),
	) {
		t.Errorf("imported metadata does not record source:\n%s", imported)
	}
	wantSection := []byte(`### WORK-123: Export audit records

Export records from the authoritative audit log.

#### Evidence

The export identifies its source.

`)
	if !bytes.HasSuffix(imported, wantSection) {
		t.Errorf(
			"imported requirement does not preserve exact section\nwant suffix:\n%q\ngot:\n%q",
			wantSection,
			imported,
		)
	}
	if bytes.Contains(imported, []byte("WORK-124")) {
		t.Error("import included the next sibling requirement")
	}

	configuration, err := config.Load(repository)
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	if !slices.Equal(
		configuration.WorkItems.RequirementSources,
		[]string{"docs/higurashi/requirements"},
	) {
		t.Errorf(
			"requirement sources = %v",
			configuration.WorkItems.RequirementSources,
		)
	}

	exitCode, inspected, inspectStderr := runJSON(
		t,
		repository,
		"inspect",
		"WORK-123",
		"--json",
	)
	if exitCode != 0 {
		t.Fatalf(
			"inspect after import exit code = %d\n%s",
			exitCode,
			inspectStderr,
		)
	}
	if inspected.Kind != "ready" {
		t.Errorf("inspect kind = %q, want ready", inspected.Kind)
	}
	if inspected.RequirementSource != wantPath {
		t.Errorf(
			"inspect source = %q, want %q",
			inspected.RequirementSource,
			wantPath,
		)
	}
}

func TestRequirementsImportFromStdinPreservesInlineBytes(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)
	inline := []byte(
		"Export only entries from the authoritative audit log.\r\n" +
			"Fail when the log is unavailable.",
	)

	exitCode, result, stderr := runRequirementsJSON(
		t,
		repository,
		inline,
		"requirements",
		"import",
		"WORK-123",
		"--stdin",
		"--json",
	)

	if exitCode != 0 {
		t.Fatalf("import exit code = %d\n%s", exitCode, stderr)
	}
	if result.Kind != "imported" {
		t.Errorf("kind = %q, want imported", result.Kind)
	}
	if result.SourceKind != "inline" {
		t.Errorf("source kind = %q, want inline", result.SourceKind)
	}
	imported, err := os.ReadFile(filepath.Join(
		repository,
		"docs",
		"higurashi",
		"requirements",
		"WORK-123.md",
	))
	if err != nil {
		t.Fatalf("read imported requirement: %v", err)
	}
	if !bytes.HasSuffix(imported, inline) {
		t.Errorf(
			"inline bytes were not preserved\nwant suffix: %q\ngot: %q",
			inline,
			imported,
		)
	}
	if !bytes.Contains(imported, []byte("# WORK-123\n\n")) {
		t.Error("headingless inline requirement did not receive an ID heading")
	}
}

func TestRequirementsImportRejectsConflictingSnapshot(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)
	first := []byte("First authoritative requirement.")
	exitCode, _, stderr := runRequirementsJSON(
		t,
		repository,
		first,
		"requirements",
		"import",
		"WORK-123",
		"--stdin",
		"--json",
	)
	if exitCode != 0 {
		t.Fatalf("first import exit code = %d\n%s", exitCode, stderr)
	}
	name := filepath.Join(
		repository,
		"docs",
		"higurashi",
		"requirements",
		"WORK-123.md",
	)
	before, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read first snapshot: %v", err)
	}

	exitCode, result, _ := runRequirementsJSON(
		t,
		repository,
		[]byte("Different requirement."),
		"requirements",
		"import",
		"WORK-123",
		"--stdin",
		"--json",
	)

	if exitCode != 7 {
		t.Errorf("conflicting import exit code = %d, want 7", exitCode)
	}
	if result.Kind != "requirement_conflict" {
		t.Errorf("kind = %q, want requirement_conflict", result.Kind)
	}
	after, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read snapshot after conflict: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("conflicting import overwrote the existing snapshot")
	}
}

func TestRequirementsImportIsIdempotent(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)
	inline := []byte("Stable requirement.")
	for index, wantKind := range []string{"imported", "current"} {
		exitCode, result, stderr := runRequirementsJSON(
			t,
			repository,
			inline,
			"requirements",
			"import",
			"WORK-123",
			"--stdin",
			"--json",
		)
		if exitCode != 0 {
			t.Fatalf("import %d exit code = %d\n%s", index+1, exitCode, stderr)
		}
		if result.Kind != wantKind {
			t.Errorf(
				"import %d kind = %q, want %q",
				index+1,
				result.Kind,
				wantKind,
			)
		}
	}
}

func TestRequirementsImportRequiresOneInputMode(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)

	exitCode, result, _ := runRequirementsJSON(
		t,
		repository,
		[]byte("content"),
		"requirements",
		"import",
		"WORK-123",
		"--stdin",
		"--from",
		"requirements.md",
		"--json",
	)

	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2", exitCode)
	}
	if result.Kind != "invalid_usage" {
		t.Errorf("kind = %q, want invalid_usage", result.Kind)
	}
}

func runRequirementsJSON(
	t *testing.T,
	workingDirectory string,
	input []byte,
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
			Input:            bytes.NewReader(input),
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

func TestRequirementsImportInlineRejectsEmptyInput(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{"schemaVersion":1,"codegraph":{"mode":"preferred"}}`,
	)

	exitCode, result, _ := runRequirementsJSON(
		t,
		repository,
		[]byte(" \r\n\t"),
		"requirements",
		"import",
		"WORK-123",
		"--stdin",
		"--json",
	)

	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3", exitCode)
	}
	if result.Kind != "requirement_invalid" {
		t.Errorf("kind = %q, want requirement_invalid", result.Kind)
	}
	if !strings.Contains(result.Message, "empty") {
		t.Errorf("message = %q, want empty input diagnostic", result.Message)
	}
}
