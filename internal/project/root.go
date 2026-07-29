package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	rootResolutionTimeout = 5 * time.Second
	rootOutputLimit       = 4 << 10
)

var (
	ErrNotGitProject     = errors.New("not a Git project")
	ErrUnsafeProjectRoot = errors.New("unsafe project root")
	ErrOutputLimit       = errors.New("command output exceeded limit")
)

// CommandRunner is an internal seam for local command execution.
type CommandRunner interface {
	Output(context.Context, int, string, ...string) ([]byte, error)
}

// ExecRunner executes local commands without a shell.
type ExecRunner struct{}

// Output runs a command and returns its standard output.
func (ExecRunner) Output(
	ctx context.Context,
	maxBytes int,
	name string,
	args ...string,
) ([]byte, error) {
	if maxBytes < 1 {
		return nil, errors.New("command output limit must be positive")
	}

	stdout := newLimitedBuffer(maxBytes)
	stderr := newLimitedBuffer(maxBytes)
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()

	if stdout.truncated || stderr.truncated {
		return nil, fmt.Errorf("%w: %d bytes", ErrOutputLimit, maxBytes)
	}
	if err != nil {
		diagnostic := strings.TrimSpace(stderr.String())
		if diagnostic != "" {
			return nil, fmt.Errorf("%w: %s", err, diagnostic)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// ResolveRoot resolves and validates the Git project containing workingDirectory.
func ResolveRoot(
	ctx context.Context,
	workingDirectory string,
	runner CommandRunner,
) (string, error) {
	if runner == nil {
		return "", fmt.Errorf("resolve project root: missing command runner")
	}

	workingDirectory, err := canonicalPath(workingDirectory)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}

	commandContext, cancel := context.WithTimeout(ctx, rootResolutionTimeout)
	defer cancel()

	output, err := runner.Output(
		commandContext,
		rootOutputLimit,
		"git",
		"-C",
		workingDirectory,
		"rev-parse",
		"--show-toplevel",
	)
	if err != nil {
		if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%w: Git root lookup timed out", ErrNotGitProject)
		}
		return "", fmt.Errorf("%w: %v", ErrNotGitProject, err)
	}

	rootText := strings.TrimSpace(string(output))
	if rootText == "" {
		return "", fmt.Errorf("%w: Git returned an empty root", ErrNotGitProject)
	}
	root, err := canonicalPath(rootText)
	if err != nil {
		return "", fmt.Errorf("%w: resolve Git root: %v", ErrUnsafeProjectRoot, err)
	}

	relative, err := filepath.Rel(root, workingDirectory)
	if err != nil || escapesBase(relative) {
		return "", fmt.Errorf(
			"%w: working directory is outside the resolved Git root",
			ErrUnsafeProjectRoot,
		)
	}

	home, err := os.UserHomeDir()
	if err == nil {
		canonicalHome, homeErr := canonicalPath(home)
		if homeErr == nil && root == canonicalHome {
			return "", fmt.Errorf(
				"%w: refusing to use the home directory as a project root",
				ErrUnsafeProjectRoot,
			)
		}
	}

	return root, nil
}

func canonicalPath(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func escapesBase(relative string) bool {
	return relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = buffer.buffer.Write(data)
	}
	if originalLength > remaining {
		buffer.truncated = true
	}
	return originalLength, nil
}

func (buffer *limitedBuffer) Bytes() []byte {
	return buffer.buffer.Bytes()
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}
