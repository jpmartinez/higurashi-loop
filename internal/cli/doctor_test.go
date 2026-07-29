package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
)

func TestDoctorRequiredModeRejectsMissingCodeGraphIndex(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	runner := &doctorRunner{projectRoot: repository}

	exitCode, result, _ := runDoctorJSON(t, repository, runner)

	if exitCode != 6 {
		t.Errorf("exit code = %d, want 6", exitCode)
	}
	if result.OK {
		t.Error("ok = true, want false")
	}
	if result.Kind != "codegraph_unavailable" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"codegraph_unavailable",
		)
	}
	if !result.CodeGraph.CLIAvailable {
		t.Error("codegraph.cliAvailable = false, want true")
	}
	if result.CodeGraph.IndexPresent {
		t.Error("codegraph.indexPresent = true, want false")
	}
}

func TestDefaultInitProducesDoctorHealthyRequirementCatalog(t *testing.T) {
	repository := newGitRepository(t)
	createCodeGraphIndex(t, repository)
	runner := &doctorRunner{
		projectRoot:  repository,
		statusOutput: healthyCodeGraphStatus(repository),
	}
	var initStdout bytes.Buffer
	var initStderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		[]string{"init", "--runner", "opencode", "--json"},
		&initStdout,
		&initStderr,
		cli.Options{
			WorkingDirectory: repository,
			Version:          "test",
			CommandRunner:    runner,
		},
	)
	if exitCode != 0 {
		t.Fatalf(
			"init exit code = %d\nstdout:\n%s\nstderr:\n%s",
			exitCode,
			initStdout.String(),
			initStderr.String(),
		)
	}

	exitCode, result, stderr := runDoctorJSON(t, repository, runner)

	if exitCode != 0 {
		t.Fatalf(
			"doctor exit code = %d\nresult: %+v\nstderr:\n%s",
			exitCode,
			result,
			stderr,
		)
	}
	if result.Kind != "healthy" {
		t.Errorf("doctor kind = %q, want healthy", result.Kind)
	}
}

func TestDoctorRejectsMissingRequirementSourceBeforeReportingHealthy(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{
  "schemaVersion": 1,
  "workItems": {
    "requirementSources": ["docs/requirements.md"]
  },
  "codegraph": {
    "mode": "preferred"
  }
}`,
	)

	exitCode, result, _ := runDoctorJSON(
		t,
		repository,
		&doctorRunner{projectRoot: repository},
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
	if !strings.Contains(result.Message, "docs/requirements.md") {
		t.Errorf(
			"message = %q, want missing requirement path",
			result.Message,
		)
	}
}

func TestDoctorRejectsSymlinkedCodeGraphIndex(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	externalIndex := t.TempDir()
	if err := os.Symlink(
		externalIndex,
		filepath.Join(repository, ".codegraph"),
	); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	runner := &doctorRunner{
		projectRoot:  repository,
		statusOutput: healthyCodeGraphStatus(repository),
	}

	exitCode, result, _ := runDoctorJSON(t, repository, runner)

	if exitCode != 6 {
		t.Errorf("exit code = %d, want 6", exitCode)
	}
	if result.Kind != "codegraph_unavailable" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"codegraph_unavailable",
		)
	}
	if result.CodeGraph.IndexPresent {
		t.Error("codegraph.indexPresent = true, want false")
	}
	if !strings.Contains(result.Message, "symlink") {
		t.Errorf("message = %q, want symlink diagnostic", result.Message)
	}
}

func TestDoctorPreferredModeWarnsForMissingCodeGraphIndex(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	runner := &doctorRunner{projectRoot: repository}

	exitCode, result, _ := runDoctorJSON(t, repository, runner)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !result.OK {
		t.Error("ok = false, want true")
	}
	if result.Kind != "degraded" {
		t.Errorf("kind = %q, want %q", result.Kind, "degraded")
	}
	if len(result.Warnings) == 0 {
		t.Error("warnings is empty")
	}
}

func TestDoctorReportsHealthyCodeGraph(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	createCodeGraphIndex(t, repository)
	runner := &doctorRunner{
		projectRoot:  repository,
		statusOutput: healthyCodeGraphStatus(repository),
	}

	exitCode, result, stderr := runDoctorJSON(t, repository, runner)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !result.OK {
		t.Error("ok = false, want true")
	}
	if result.Kind != "healthy" {
		t.Errorf("kind = %q, want %q", result.Kind, "healthy")
	}
	if !result.CodeGraph.CLIAvailable {
		t.Error("codegraph.cliAvailable = false, want true")
	}
	if !result.CodeGraph.IndexPresent {
		t.Error("codegraph.indexPresent = false, want true")
	}
	if !result.CodeGraph.StatusHealthy {
		t.Error("codegraph.statusHealthy = false, want true")
	}
	if !result.CodeGraph.RootMatches {
		t.Error("codegraph.rootMatches = false, want true")
	}
	if result.CodeGraph.Synchronization != "current" {
		t.Errorf(
			"codegraph.synchronization = %q, want %q",
			result.CodeGraph.Synchronization,
			"current",
		)
	}
	if result.CodeGraph.WatcherState != "not_reported" {
		t.Errorf(
			"codegraph.watcherState = %q, want %q",
			result.CodeGraph.WatcherState,
			"not_reported",
		)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestDoctorRejectsMismatchedCodeGraphRoot(t *testing.T) {
	repository := newGitRepository(t)
	otherRepository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	createCodeGraphIndex(t, repository)
	runner := &doctorRunner{
		projectRoot:  repository,
		statusOutput: healthyCodeGraphStatus(otherRepository),
	}

	exitCode, result, _ := runDoctorJSON(t, repository, runner)

	if exitCode != 6 {
		t.Errorf("exit code = %d, want 6", exitCode)
	}
	if result.Kind != "codegraph_unhealthy" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"codegraph_unhealthy",
		)
	}
	if result.CodeGraph.RootMatches {
		t.Error("codegraph.rootMatches = true, want false")
	}
}

func TestDoctorRejectsPendingCodeGraphChanges(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	createCodeGraphIndex(t, repository)
	status := strings.Replace(
		healthyCodeGraphStatus(repository),
		`"added": 0`,
		`"added": 1`,
		1,
	)
	runner := &doctorRunner{
		projectRoot:  repository,
		statusOutput: status,
	}

	exitCode, result, _ := runDoctorJSON(t, repository, runner)

	if exitCode != 6 {
		t.Errorf("exit code = %d, want 6", exitCode)
	}
	if result.Kind != "codegraph_unhealthy" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"codegraph_unhealthy",
		)
	}
	if result.CodeGraph.Synchronization != "stale" {
		t.Errorf(
			"codegraph.synchronization = %q, want %q",
			result.CodeGraph.Synchronization,
			"stale",
		)
	}
	if result.CodeGraph.PendingChanges != 1 {
		t.Errorf(
			"codegraph.pendingChanges = %d, want 1",
			result.CodeGraph.PendingChanges,
		)
	}
}

func TestDoctorTreatsReindexRecommendationAsWarning(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	createCodeGraphIndex(t, repository)
	status := strings.Replace(
		healthyCodeGraphStatus(repository),
		`"reindexRecommended": false`,
		`"reindexRecommended": true`,
		1,
	)
	runner := &doctorRunner{
		projectRoot:  repository,
		statusOutput: status,
	}

	exitCode, result, stderr := runDoctorJSON(t, repository, runner)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if result.Kind != "healthy" {
		t.Errorf("kind = %q, want %q", result.Kind, "healthy")
	}
	if !result.CodeGraph.ReindexRecommended {
		t.Error("codegraph.reindexRecommended = false, want true")
	}
	if len(result.Warnings) != 1 {
		t.Errorf("len(warnings) = %d, want 1", len(result.Warnings))
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("stderr = %q, want warning", stderr)
	}
}

func TestDoctorReportsMissingCodeGraphCLI(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	runner := &doctorRunner{
		projectRoot:  repository,
		versionError: errors.New("executable file not found"),
	}

	exitCode, result, _ := runDoctorJSON(t, repository, runner)

	if exitCode != 6 {
		t.Errorf("exit code = %d, want 6", exitCode)
	}
	if result.Kind != "codegraph_unavailable" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"codegraph_unavailable",
		)
	}
	if result.CodeGraph.CLIAvailable {
		t.Error("codegraph.cliAvailable = true, want false")
	}
}

func TestDoctorReportsCodeGraphTimeoutSafely(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "required")
	createCodeGraphIndex(t, repository)
	runner := &doctorRunner{
		projectRoot: repository,
		statusError: context.DeadlineExceeded,
	}

	exitCode, result, _ := runDoctorJSON(t, repository, runner)

	if exitCode != 6 {
		t.Errorf("exit code = %d, want 6", exitCode)
	}
	if result.Kind != "codegraph_unhealthy" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"codegraph_unhealthy",
		)
	}
	if !strings.Contains(result.Message, "timed out") {
		t.Errorf("message = %q, want a bounded timeout diagnostic", result.Message)
	}
}

func TestDoctorRejectsNonGitDirectory(t *testing.T) {
	exitCode, result, _ := runJSON(
		t,
		t.TempDir(),
		"doctor",
		"--json",
	)

	if exitCode != 8 {
		t.Errorf("exit code = %d, want 8", exitCode)
	}
	if result.Kind != "not_git_project" {
		t.Errorf("kind = %q, want %q", result.Kind, "not_git_project")
	}
}

type doctorRunner struct {
	projectRoot  string
	versionError error
	statusOutput string
	statusError  error
}

func (runner *doctorRunner) Output(
	_ context.Context,
	_ int,
	name string,
	args ...string,
) ([]byte, error) {
	switch name {
	case "git":
		return []byte(runner.projectRoot + "\n"), nil
	case "codegraph":
		if len(args) == 1 && args[0] == "version" {
			if runner.versionError != nil {
				return nil, runner.versionError
			}
			return []byte("CodeGraph v1.5.0\n"), nil
		}
		if len(args) == 3 &&
			args[0] == "status" &&
			args[1] == "--json" {
			if runner.statusError != nil {
				return nil, runner.statusError
			}
			return []byte(runner.statusOutput), nil
		}
	}
	return nil, errors.New("unexpected command")
}

func runDoctorJSON(
	t *testing.T,
	workingDirectory string,
	runner *doctorRunner,
) (int, jsonResult, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		[]string{"doctor", "--json"},
		&stdout,
		&stderr,
		cli.Options{
			WorkingDirectory: workingDirectory,
			Version:          "test",
			CommandRunner:    runner,
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

func writeMinimalConfig(t *testing.T, repository, mode string) {
	t.Helper()
	writeFile(
		t,
		filepath.Join(repository, ".higurashi", "config.json"),
		`{
  "schemaVersion": 1,
  "workItems": {
    "requirementSources": ["docs/requirements.md"]
  },
  "codegraph": {
    "mode": "`+mode+`"
  }
}`,
	)
	writeFile(
		t,
		filepath.Join(repository, "docs", "requirements.md"),
		"# Requirements\n",
	)
}

func createCodeGraphIndex(t *testing.T, repository string) {
	t.Helper()
	if err := os.Mkdir(
		filepath.Join(repository, ".codegraph"),
		0o700,
	); err != nil {
		t.Fatalf("create CodeGraph index: %v", err)
	}
}

func healthyCodeGraphStatus(repository string) string {
	return `{
  "initialized": true,
  "version": "1.5.0",
  "projectPath": "` + filepath.ToSlash(repository) + `",
  "indexPath": "` + filepath.ToSlash(
		filepath.Join(repository, ".codegraph"),
	) + `",
  "pendingChanges": {
    "added": 0,
    "modified": 0,
    "removed": 0
  },
  "worktreeMismatch": null,
  "index": {
    "reindexRecommended": false,
    "state": "complete"
  }
}`
}
