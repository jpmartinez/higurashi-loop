package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/verification"
)

func runVerification(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	jsonOutput := false
	var operation string
	for _, argument := range args {
		switch argument {
		case "--json":
			if jsonOutput {
				return writeUsageFailure(
					stdout,
					stderr,
					true,
					"verification.suggest",
					"--json may be supplied only once",
				)
			}
			jsonOutput = true
		case "-h", "--help":
			writeVerificationHelp(stdout)
			return exitSuccess
		default:
			if operation != "" {
				return writeUsageFailure(
					stdout,
					stderr,
					jsonOutput,
					"verification",
					"verification accepts exactly one operation",
				)
			}
			operation = argument
		}
	}
	if operation != "suggest" {
		return writeUsageFailure(
			stdout,
			stderr,
			jsonOutput,
			"verification",
			`verification requires the "suggest" operation`,
		)
	}

	root, err := project.ResolveRoot(
		ctx,
		options.WorkingDirectory,
		options.CommandRunner,
	)
	if err != nil {
		kind := "not_git_project"
		if errors.Is(err, project.ErrUnsafeProjectRoot) {
			kind = "unsafe_project_root"
		}
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"verification.suggest",
			kind,
			err.Error(),
			exitUnsafeProject,
		)
	}
	configuration, err := config.Load(root)
	if err != nil {
		kind := "configuration_invalid"
		if errors.Is(err, config.ErrMissing) {
			kind = "configuration_missing"
		}
		return writeFailure(
			stdout,
			stderr,
			jsonOutput,
			"verification.suggest",
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}

	report := verification.Suggest(root, configuration.Verification)
	kind := "current"
	if len(report.Suggestions) != 0 {
		kind = "suggestions"
	}
	envelope := result.Envelope{
		Command:                 "verification.suggest",
		OK:                      true,
		Kind:                    kind,
		ProjectRoot:             root,
		VerificationSuggestions: report.Suggestions,
		SuggestedVerification:   report.ConfigFragment,
		Warnings:                report.Warnings,
	}
	if jsonOutput {
		if err := result.WriteJSON(stdout, envelope); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
	} else {
		writeVerificationSuggestions(stdout, root, report)
	}
	for _, warning := range report.Warnings {
		fmt.Fprintf(stderr, "higurashi: warning: %s\n", warning)
	}
	return exitSuccess
}

func writeVerificationSuggestions(
	writer io.Writer,
	root string,
	report verification.Report,
) {
	fmt.Fprintln(writer, "Higurashi verification suggestions")
	fmt.Fprintf(writer, "Project root: %s\n", root)
	fmt.Fprintln(writer, "Configuration: .higurashi/config.json")
	if len(report.Suggestions) == 0 {
		fmt.Fprintln(writer, "No unconfigured commands were detected.")
		return
	}
	fmt.Fprintln(
		writer,
		"Suggestions are read-only and are not workflow authority until added to configuration.",
	)
	for _, suggestion := range report.Suggestions {
		fmt.Fprintf(writer, "\nSuggestion: %s\n", suggestion.ID)
		fmt.Fprintf(writer, "Field: %s\n", suggestion.ConfigField)
		fmt.Fprintf(writer, "Command: %s\n", suggestion.DisplayCommand)
		fmt.Fprintf(
			writer,
			"Timeout: %d seconds\n",
			suggestion.Command.TimeoutSeconds,
		)
		fmt.Fprintf(writer, "Source: %s\n", suggestion.Source)
		if suggestion.RequiresReview {
			fmt.Fprintf(writer, "Review required: %s\n", suggestion.Reason)
		}
	}
	fragment, err := json.MarshalIndent(report.ConfigFragment, "", "  ")
	if err == nil {
		fmt.Fprintln(writer, "\nSuggested `verification` value:")
		fmt.Fprintln(writer, string(fragment))
	}
}

func writeEnvelopeVerificationSuggestions(
	writer io.Writer,
	envelope result.Envelope,
) {
	suggestions, suggestionsOK := envelope.VerificationSuggestions.([]verification.Suggestion)
	suggested, suggestedOK := envelope.SuggestedVerification.(config.Verification)
	if !suggestionsOK || !suggestedOK || len(suggestions) == 0 {
		return
	}
	writeVerificationSuggestions(
		writer,
		envelope.ProjectRoot,
		verification.Report{
			Suggestions:    suggestions,
			ConfigFragment: suggested,
		},
	)
}

func writeVerificationHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi verification suggest [--json]

Reads known project manifests without executing commands or modifying files.
Returns exact argv arrays, their destination in .higurashi/config.json, source,
confidence, timeout, and any risk requiring review. Detected commands remain
untrusted suggestions until a user adds them to project configuration.`)
}
