package codegraph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jpmartinez/higurashi-loop/internal/project"
)

const (
	commandTimeout = 10 * time.Second
	outputLimit    = 64 << 10
)

// Diagnostic is the stable CodeGraph portion of doctor output.
type Diagnostic struct {
	Mode               string   `json:"mode"`
	CLIAvailable       bool     `json:"cliAvailable"`
	IndexPresent       bool     `json:"indexPresent"`
	StatusHealthy      bool     `json:"statusHealthy"`
	RootMatches        bool     `json:"rootMatches"`
	Synchronization    string   `json:"synchronization"`
	WatcherState       string   `json:"watcherState"`
	Version            string   `json:"version,omitempty"`
	IndexState         string   `json:"indexState,omitempty"`
	PendingChanges     int      `json:"pendingChanges"`
	ReindexRecommended bool     `json:"reindexRecommended"`
	Problem            string   `json:"problem,omitempty"`
	Warnings           []string `json:"-"`
}

// Diagnose checks the local CodeGraph CLI and project index without mutation.
func Diagnose(
	ctx context.Context,
	projectRoot string,
	mode string,
	runner project.CommandRunner,
) Diagnostic {
	diagnostic := Diagnostic{
		Mode:            mode,
		Synchronization: "unknown",
		WatcherState:    "not_reported",
	}

	versionOutput, err := run(
		ctx,
		runner,
		"codegraph",
		"version",
	)
	if err != nil {
		diagnostic.Problem = commandProblem("CodeGraph version check", err)
		return diagnostic
	}
	diagnostic.CLIAvailable = true
	diagnostic.Version = normalizeVersion(string(versionOutput))

	indexPath := filepath.Join(projectRoot, ".codegraph")
	info, err := os.Lstat(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			diagnostic.Problem = "project-local CodeGraph index is missing"
		} else {
			diagnostic.Problem = fmt.Sprintf(
				"inspect project-local CodeGraph index: %v",
				err,
			)
		}
		return diagnostic
	}
	if info.Mode()&os.ModeSymlink != 0 {
		diagnostic.Problem = "project-local .codegraph index must not be a symlink"
		return diagnostic
	}
	if !info.IsDir() {
		diagnostic.Problem = "project-local .codegraph path is not a directory"
		return diagnostic
	}
	diagnostic.IndexPresent = true

	statusOutput, err := run(
		ctx,
		runner,
		"codegraph",
		"status",
		"--json",
		projectRoot,
	)
	if err != nil {
		diagnostic.Problem = commandProblem("CodeGraph status check", err)
		return diagnostic
	}

	var status statusResult
	if err := json.Unmarshal(statusOutput, &status); err != nil {
		diagnostic.Problem = fmt.Sprintf(
			"decode CodeGraph status JSON: %v",
			err,
		)
		return diagnostic
	}

	if status.Version != "" {
		diagnostic.Version = status.Version
	}
	diagnostic.IndexState = status.Index.State
	diagnostic.PendingChanges = status.PendingChanges.Added +
		status.PendingChanges.Modified +
		status.PendingChanges.Removed
	diagnostic.ReindexRecommended = status.Index.ReindexRecommended
	diagnostic.RootMatches = samePath(projectRoot, status.ProjectPath)

	mismatch := hasJSONValue(status.WorktreeMismatch)
	diagnostic.StatusHealthy = status.Initialized &&
		status.Index.State == "complete" &&
		!mismatch &&
		diagnostic.PendingChanges == 0

	if diagnostic.PendingChanges == 0 {
		diagnostic.Synchronization = "current"
	} else {
		diagnostic.Synchronization = "stale"
	}

	switch {
	case !status.Initialized:
		diagnostic.Problem = "CodeGraph status reports an uninitialized index"
	case !diagnostic.RootMatches:
		diagnostic.Problem = "CodeGraph status resolved a different project root"
	case mismatch:
		diagnostic.Problem = "CodeGraph status reports a worktree mismatch"
	case status.Index.State != "complete":
		diagnostic.Problem = fmt.Sprintf(
			"CodeGraph index state is %q, want %q",
			status.Index.State,
			"complete",
		)
	case diagnostic.PendingChanges != 0:
		diagnostic.Problem = fmt.Sprintf(
			"CodeGraph index has %d pending file changes",
			diagnostic.PendingChanges,
		)
	}

	if diagnostic.ReindexRecommended {
		diagnostic.Warnings = append(
			diagnostic.Warnings,
			"CodeGraph recommends rebuilding this index with the current engine",
		)
	}
	return diagnostic
}

// Healthy reports whether all required deterministic checks passed.
func (diagnostic Diagnostic) Healthy() bool {
	return diagnostic.CLIAvailable &&
		diagnostic.IndexPresent &&
		diagnostic.StatusHealthy &&
		diagnostic.RootMatches
}

// Unavailable reports whether the CLI or project-local index is absent.
func (diagnostic Diagnostic) Unavailable() bool {
	return !diagnostic.CLIAvailable || !diagnostic.IndexPresent
}

func run(
	ctx context.Context,
	runner project.CommandRunner,
	name string,
	args ...string,
) ([]byte, error) {
	commandContext, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	output, err := runner.Output(
		commandContext,
		outputLimit,
		name,
		args...,
	)
	if err != nil {
		if errors.Is(commandContext.Err(), context.DeadlineExceeded) ||
			errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}
	return output, nil
}

func commandProblem(check string, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return check + " timed out"
	case errors.Is(err, project.ErrOutputLimit):
		return check + " exceeded the bounded output limit"
	default:
		return fmt.Sprintf("%s failed: %v", check, err)
	}
}

func normalizeVersion(output string) string {
	version := strings.TrimSpace(output)
	version = strings.TrimPrefix(version, "CodeGraph ")
	version = strings.TrimPrefix(version, "v")
	return version
}

func samePath(expected, actual string) bool {
	if actual == "" {
		return false
	}
	expectedInfo, expectedErr := os.Stat(expected)
	actualInfo, actualErr := os.Stat(actual)
	if expectedErr == nil && actualErr == nil {
		return os.SameFile(expectedInfo, actualInfo)
	}
	expected = canonicalPath(expected)
	actual = canonicalPath(actual)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(expected, actual)
	}
	return expected == actual
}

func canonicalPath(name string) string {
	absolute, err := filepath.Abs(name)
	if err == nil {
		name = absolute
	}
	canonical, err := filepath.EvalSymlinks(name)
	if err == nil {
		name = canonical
	}
	return filepath.Clean(name)
}

func hasJSONValue(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	return len(value) != 0 && !bytes.Equal(value, []byte("null"))
}

type statusResult struct {
	Initialized      bool            `json:"initialized"`
	Version          string          `json:"version"`
	ProjectPath      string          `json:"projectPath"`
	WorktreeMismatch json.RawMessage `json:"worktreeMismatch"`
	PendingChanges   struct {
		Added    int `json:"added"`
		Modified int `json:"modified"`
		Removed  int `json:"removed"`
	} `json:"pendingChanges"`
	Index struct {
		ReindexRecommended bool   `json:"reindexRecommended"`
		State              string `json:"state"`
	} `json:"index"`
}
