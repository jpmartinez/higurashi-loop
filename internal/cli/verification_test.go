package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
	"github.com/jpmartinez/higurashi-loop/internal/result"
)

func TestVerificationSuggestReportsConfigFragmentWithoutWriting(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	writeFile(t, filepath.Join(repository, "bun.lock"), "")
	writeFile(t, filepath.Join(repository, "package.json"), `{
  "scripts": {
    "lint": "biome lint .",
    "supabase:test": "supabase db reset --local && supabase test db"
  }
}`)
	configName := filepath.Join(repository, ".higurashi", "config.json")
	before, err := os.ReadFile(configName)
	if err != nil {
		t.Fatalf("read config before suggestion: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		[]string{"verification", "suggest", "--json"},
		&stdout,
		&stderr,
		cli.Options{WorkingDirectory: repository, Version: "test"},
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d\nstderr:\n%s", exitCode, stderr.String())
	}
	var envelope result.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout.String())
	}
	if envelope.Kind != "suggestions" ||
		envelope.VerificationSuggestions == nil ||
		envelope.SuggestedVerification == nil {
		t.Errorf("envelope = %+v, want structured suggestions", envelope)
	}
	after, err := os.ReadFile(configName)
	if err != nil {
		t.Fatalf("read config after suggestion: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("verification suggest changed configuration")
	}
}

func TestInitIncludesVerificationSuggestionCommand(t *testing.T) {
	repository := newGitRepository(t)
	writeFile(t, filepath.Join(repository, "bun.lock"), "")
	writeFile(t, filepath.Join(repository, "package.json"), `{
  "scripts": {"test": "bun test"}
}`)

	exitCode, envelope, stderr := runInitJSON(
		t,
		repository,
		"init",
		"--runner",
		"opencode",
		"--json",
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d\nstderr:\n%s", exitCode, stderr)
	}
	if !slices.Contains(
		envelope.NextCommands,
		"higurashi verification suggest",
	) {
		t.Errorf("nextCommands = %v, want verification suggestion command", envelope.NextCommands)
	}
	if envelope.VerificationSuggestions == nil ||
		envelope.SuggestedVerification == nil {
		t.Errorf("init envelope = %+v, want suggestions", envelope)
	}
}

func TestVerificationSuggestRejectsUnknownOperation(t *testing.T) {
	repository := newGitRepository(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := cli.Run(
		context.Background(),
		[]string{"verification", "invent", "--json"},
		&stdout,
		&stderr,
		cli.Options{WorkingDirectory: repository, Version: "test"},
	)

	if exitCode != 2 || !strings.Contains(stdout.String(), "invalid_usage") {
		t.Errorf(
			"result = code:%d stdout:%s stderr:%s",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}
