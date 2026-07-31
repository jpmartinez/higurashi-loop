package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const (
	SchemaVersion                      = 1
	DefaultArtifactDirectory           = "docs/higurashi"
	DefaultManagedRequirementDirectory = "docs/higurashi/requirements"
	maxConfigBytes                     = 1 << 20
)

var ErrMissing = errors.New("configuration missing")

// Config is the normalized project configuration.
type Config struct {
	SchemaVersion int          `json:"schemaVersion"`
	WorkItems     WorkItems    `json:"workItems"`
	Artifacts     Artifacts    `json:"artifacts"`
	Loop          Loop         `json:"loop"`
	CodeGraph     CodeGraph    `json:"codegraph"`
	Context       Context      `json:"context"`
	Verification  Verification `json:"verification"`
	Runners       Runners      `json:"runners"`
}

type WorkItems struct {
	IDPattern          string   `json:"idPattern"`
	RequirementSources []string `json:"requirementSources"`
}

type Artifacts struct {
	Directory string `json:"directory"`
}

type Loop struct {
	MaxApplyBatchesPerRun          int  `json:"maxApplyBatchesPerRun"`
	MaxRepairAttempts              int  `json:"maxRepairAttempts"`
	RequireProgressAfterEveryBatch bool `json:"requireProgressAfterEveryBatch"`
}

type CodeGraph struct {
	Mode                       string `json:"mode"`
	AutoInitializeProjectIndex bool   `json:"autoInitializeProjectIndex"`
}

type Context struct {
	InstructionFiles   []string `json:"instructionFiles"`
	AuthoritativeFiles []string `json:"authoritativeFiles"`
}

type Verification struct {
	NormalizationCommands   []Command `json:"normalizationCommands"`
	RequiredCommands        []Command `json:"requiredCommands"`
	CandidateFormatCommands []Command `json:"candidateFormatCommands"`
}

type Command struct {
	Argv           []string `json:"argv"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

type Runners struct {
	OpenCode   Runner `json:"opencode"`
	ClaudeCode Runner `json:"claudeCode"`
	Pi         Runner `json:"pi"`
}

type Runner struct {
	Enabled bool             `json:"enabled"`
	Models  ModelAssignments `json:"models"`
}

// ModelAssignments selects the model used by each durable workflow role.
// "inherit" delegates selection to the runner's active/default model.
type ModelAssignments struct {
	Orchestrator   string `json:"orchestrator"`
	Refine         string `json:"refine"`
	Plan           string `json:"plan"`
	Apply          string `json:"apply"`
	VerifyContract string `json:"verifyContract"`
	VerifyRisk     string `json:"verifyRisk"`
}

// Defaults returns the conservative schema version 1 configuration.
func Defaults() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		WorkItems: WorkItems{
			IDPattern:          `^[A-Z][A-Z0-9]*(-[A-Z0-9][A-Z0-9._-]*)$`,
			RequirementSources: []string{DefaultManagedRequirementDirectory},
		},
		Artifacts: Artifacts{
			Directory: DefaultArtifactDirectory,
		},
		Loop: Loop{
			MaxApplyBatchesPerRun:          8,
			MaxRepairAttempts:              1,
			RequireProgressAfterEveryBatch: true,
		},
		CodeGraph: CodeGraph{
			Mode:                       "required",
			AutoInitializeProjectIndex: true,
		},
		Context: Context{
			InstructionFiles:   []string{"AGENTS.md", "CLAUDE.md"},
			AuthoritativeFiles: []string{},
		},
		Verification: Verification{
			NormalizationCommands:   []Command{},
			RequiredCommands:        []Command{},
			CandidateFormatCommands: []Command{},
		},
		Runners: Runners{
			OpenCode: Runner{
				Enabled: true,
				Models:  InheritedModels(),
			},
			ClaudeCode: Runner{
				Enabled: true,
				Models:  InheritedModels(),
			},
			Pi: Runner{
				Enabled: true,
				Models:  InheritedModels(),
			},
		},
	}
}

// InheritedModels returns assignments that preserve runner-native selection.
func InheritedModels() ModelAssignments {
	return ModelAssignments{
		Orchestrator:   "inherit",
		Refine:         "inherit",
		Plan:           "inherit",
		Apply:          "inherit",
		VerifyContract: "inherit",
		VerifyRisk:     "inherit",
	}
}

// Runner returns the named runner configuration.
func (configuration Config) Runner(name string) (Runner, bool) {
	switch name {
	case "opencode":
		return configuration.Runners.OpenCode, true
	case "claude-code":
		return configuration.Runners.ClaudeCode, true
	case "pi":
		return configuration.Runners.Pi, true
	default:
		return Runner{}, false
	}
}

// Encode validates and deterministically serializes a complete configuration.
func Encode(configuration Config) ([]byte, error) {
	if err := Validate(configuration); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(configuration); err != nil {
		return nil, fmt.Errorf("encode configuration: %w", err)
	}
	return output.Bytes(), nil
}

// Load reads, strictly decodes, normalizes, and validates project configuration.
func Load(projectRoot string) (Config, error) {
	name := filepath.Join(projectRoot, ".higurashi", "config.json")
	file, err := os.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("%w: %s", ErrMissing, name)
		}
		return Config{}, fmt.Errorf("open configuration: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return Config{}, fmt.Errorf("read configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return Config{}, fmt.Errorf(
			"configuration exceeds %d bytes",
			maxConfigBytes,
		)
	}

	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if err := rejectNull(document, ""); err != nil {
		return Config{}, err
	}
	if _, ok := document["schemaVersion"]; !ok {
		return Config{}, errors.New("schemaVersion is required")
	}

	configuration := Defaults()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&configuration); err != nil {
		return Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if err := requireEndOfJSON(decoder); err != nil {
		return Config{}, err
	}

	normalize(&configuration)
	if err := Validate(configuration); err != nil {
		return Config{}, err
	}
	return configuration, nil
}

func requireEndOfJSON(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode configuration: multiple JSON values")
		}
		return fmt.Errorf("decode configuration: %w", err)
	}
	return nil
}

func rejectNull(value any, location string) error {
	if value == nil {
		if location == "" {
			return errors.New("configuration must be a JSON object")
		}
		return fmt.Errorf("%s must not be null", location)
	}

	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childLocation := key
			if location != "" {
				childLocation = location + "." + key
			}
			if err := rejectNull(typed[key], childLocation); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := rejectNull(
				child,
				fmt.Sprintf("%s[%d]", location, index),
			); err != nil {
				return err
			}
		}
	}
	return nil
}
