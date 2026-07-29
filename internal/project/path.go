package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathEscapesRoot    = errors.New("path escapes project root")
	ErrUnsafeMutationPath = errors.New("unsafe mutation path")
)

// ResolveContainedPath converts a project-relative path to an absolute path
// after verifying that neither its lexical form nor an existing symlinked
// ancestor escapes the project root.
func ResolveContainedPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) ||
		filepath.VolumeName(relative) != "" {
		return "", fmt.Errorf(
			"%w: path must be project-relative: %s",
			ErrPathEscapesRoot,
			relative,
		)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root symlinks: %w", err)
	}

	candidate, err := filepath.Abs(
		filepath.Join(absoluteRoot, filepath.FromSlash(relative)),
	)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	if !isWithin(absoluteRoot, candidate) {
		return "", fmt.Errorf("%w: %s", ErrPathEscapesRoot, relative)
	}

	existing := candidate
	for {
		_, statErr := os.Lstat(existing)
		if statErr == nil {
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect project path %s: %w", relative, statErr)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("find existing ancestor for %s", relative)
		}
		existing = parent
	}

	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("resolve project path symlinks: %w", err)
	}
	if !isWithin(resolvedRoot, resolvedExisting) {
		return "", fmt.Errorf(
			"%w through symlink: %s",
			ErrPathEscapesRoot,
			relative,
		)
	}
	return candidate, nil
}

func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// ValidateMutationPath rejects resolved targets in Git's project metadata.
func ValidateMutationPath(root, target string) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return fmt.Errorf("resolve project root symlinks: %w", err)
	}
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve mutation target: %w", err)
	}

	existing := absoluteTarget
	for {
		_, statErr := os.Lstat(existing)
		if statErr == nil {
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect mutation target: %w", statErr)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return errors.New("find existing mutation-target ancestor")
		}
		existing = parent
	}
	resolvedExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("resolve mutation target symlinks: %w", err)
	}
	remainder, err := filepath.Rel(existing, absoluteTarget)
	if err != nil {
		return fmt.Errorf("resolve mutation target remainder: %w", err)
	}
	resolvedTarget := filepath.Join(resolvedExisting, remainder)
	if !isWithin(resolvedRoot, resolvedTarget) {
		return fmt.Errorf(
			"%w: target escapes project root",
			ErrUnsafeMutationPath,
		)
	}

	gitMetadata := filepath.Join(resolvedRoot, ".git")
	if isWithin(gitMetadata, resolvedTarget) {
		return fmt.Errorf(
			"%w: refusing to mutate Git metadata",
			ErrUnsafeMutationPath,
		)
	}
	return nil
}
