package result

import (
	"encoding/json"
	"io"
)

const SchemaVersion = 1

// Envelope is the stable machine-readable result shared by CLI commands.
type Envelope struct {
	SchemaVersion           int      `json:"schemaVersion"`
	Command                 string   `json:"command"`
	OK                      bool     `json:"ok"`
	Kind                    string   `json:"kind"`
	Message                 string   `json:"message,omitempty"`
	ProjectRoot             string   `json:"projectRoot,omitempty"`
	Config                  any      `json:"config,omitempty"`
	WorkItemID              string   `json:"workItemId,omitempty"`
	RequirementSource       string   `json:"requirementSource,omitempty"`
	RequirementSources      []string `json:"requirementSources,omitempty"`
	SourceKind              string   `json:"sourceKind,omitempty"`
	SourceHash              string   `json:"sourceHash,omitempty"`
	ArtifactPath            string   `json:"artifactPath,omitempty"`
	ArtifactStatus          string   `json:"artifactStatus,omitempty"`
	ArtifactHash            string   `json:"artifactHash,omitempty"`
	BlockedFrom             string   `json:"blockedFrom,omitempty"`
	CompletionNote          string   `json:"completionNote,omitempty"`
	RepairRound             *int     `json:"repairRound,omitempty"`
	HandoffPath             string   `json:"handoffPath,omitempty"`
	HandoffValidation       string   `json:"handoffValidation,omitempty"`
	BlockerCount            int      `json:"blockerCount,omitempty"`
	Blockers                any      `json:"blockers,omitempty"`
	AuthorizationRequired   *bool    `json:"authorizationRequired,omitempty"`
	NextCommand             string   `json:"nextCommand,omitempty"`
	CandidateStrategy       string   `json:"candidateStrategy,omitempty"`
	Progress                any      `json:"progress,omitempty"`
	NextTask                any      `json:"nextTask,omitempty"`
	Loop                    any      `json:"loop,omitempty"`
	Adapter                 string   `json:"adapter,omitempty"`
	Adapters                []string `json:"adapters,omitempty"`
	Runner                  string   `json:"runner,omitempty"`
	Models                  any      `json:"models,omitempty"`
	VerificationSuggestions any      `json:"verificationSuggestions,omitempty"`
	SuggestedVerification   any      `json:"suggestedVerification,omitempty"`
	Files                   any      `json:"files,omitempty"`
	ChangedPaths            []string `json:"changedPaths,omitempty"`
	FollowUpRequirements    []string `json:"followUpRequirements,omitempty"`
	Conflicts               []string `json:"conflicts,omitempty"`
	StalePaths              []string `json:"stalePaths,omitempty"`
	NextCommands            []string `json:"nextCommands,omitempty"`
	Warnings                []string `json:"warnings,omitempty"`
	CodeGraph               any      `json:"codegraph,omitempty"`
}

// WriteJSON writes one indented JSON result followed by a newline.
func WriteJSON(writer io.Writer, envelope Envelope) error {
	envelope.SchemaVersion = SchemaVersion
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(envelope)
}
