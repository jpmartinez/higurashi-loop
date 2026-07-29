package project_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/project"
)

func TestResolveContainedPathRejectsLexicalEscape(t *testing.T) {
	root := t.TempDir()

	_, err := project.ResolveContainedPath(root, "../outside")

	if !errors.Is(err, project.ErrPathEscapesRoot) {
		t.Fatalf(
			"ResolveContainedPath() error = %v, want ErrPathEscapesRoot",
			err,
		)
	}
}

func TestResolveContainedPathRejectsSymlinkAncestorEscape(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(root, "artifacts")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	_, err := project.ResolveContainedPath(
		root,
		"artifacts/WORK-123.md",
	)

	if !errors.Is(err, project.ErrPathEscapesRoot) {
		t.Fatalf(
			"ResolveContainedPath() error = %v, want ErrPathEscapesRoot",
			err,
		)
	}
}

func TestValidateMutationPathRejectsGitMetadata(t *testing.T) {
	root := t.TempDir()
	gitDirectory := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDirectory, 0o755); err != nil {
		t.Fatalf("create Git metadata directory: %v", err)
	}
	target := filepath.Join(gitDirectory, "config")
	if err := os.WriteFile(target, []byte("[core]\n"), 0o600); err != nil {
		t.Fatalf("write Git config: %v", err)
	}

	err := project.ValidateMutationPath(root, target)

	if !errors.Is(err, project.ErrUnsafeMutationPath) {
		t.Fatalf(
			"ValidateMutationPath() error = %v, want ErrUnsafeMutationPath",
			err,
		)
	}
}
