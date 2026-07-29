package project_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/project"
)

func TestResolveRootRejectsHomeDirectory(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home directory: %v", err)
	}

	_, err = project.ResolveRoot(
		context.Background(),
		home,
		staticRunner{output: []byte(home + "\n")},
	)

	if !errors.Is(err, project.ErrUnsafeProjectRoot) {
		t.Fatalf("ResolveRoot error = %v, want ErrUnsafeProjectRoot", err)
	}
}

func TestExecRunnerBoundsCapturedOutput(t *testing.T) {
	_, err := (project.ExecRunner{}).Output(
		context.Background(),
		32,
		os.Args[0],
		"-test.run=TestCommandOutputHelper",
		"--",
		"large-output",
	)

	if !errors.Is(err, project.ErrOutputLimit) {
		t.Fatalf("Output error = %v, want ErrOutputLimit", err)
	}
}

func TestCommandOutputHelper(_ *testing.T) {
	for _, argument := range os.Args {
		if argument == "large-output" {
			fmt.Print(strings.Repeat("x", 1024))
			return
		}
	}
}

type staticRunner struct {
	output []byte
	err    error
}

func (runner staticRunner) Output(
	_ context.Context,
	_ int,
	_ string,
	_ ...string,
) ([]byte, error) {
	return runner.output, runner.err
}
