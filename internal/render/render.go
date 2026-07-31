package render

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/jpmartinez/higurashi-loop/adapters"
	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/protocol"
)

const (
	AntiBypassRule    = "Do not alter control documents, generated ownership markers, or existing tests to bypass required behavior; report a blocker instead."
	markerPlaceholder = "{{HIGURASHI_GENERATED}}"
)

var (
	ErrUnknownAdapter        = errors.New("unknown adapter")
	ErrConflict              = errors.New("generated file conflict")
	ErrMissingAntiBypassRule = errors.New("agent template is missing the anti-bypass rule")
)

type File struct {
	Path       string
	TemplateID string
	SourceHash string
	Source     []byte
	Content    []byte
}

type Bundle struct {
	Adapter          string
	GeneratorVersion string
	Files            []File
}

type FileStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type Report struct {
	Adapter      string       `json:"adapter"`
	Files        []FileStatus `json:"files"`
	ChangedPaths []string     `json:"changedPaths,omitempty"`
	Conflicts    []string     `json:"conflicts,omitempty"`
	Stale        []string     `json:"stale,omitempty"`
}

type sourceSpec struct {
	filesystem  fs.FS
	templateID  string
	sourceName  string
	destination string
	skillName   string
	modelRole   string
}

var canonicalSources = []sourceSpec{
	{
		filesystem:  protocol.Files,
		templateID:  "protocol/higurashi-deliver/SKILL.md",
		sourceName:  "higurashi-deliver/SKILL.md",
		destination: "SKILL.md",
		skillName:   "higurashi-deliver",
	},
	{
		filesystem:  protocol.Files,
		templateID:  "protocol/higurashi-deliver/references/artifact-contract.md",
		sourceName:  "higurashi-deliver/references/artifact-contract.md",
		destination: "references/artifact-contract.md",
		skillName:   "higurashi-deliver",
	},
	{
		filesystem:  protocol.Files,
		templateID:  "protocol/higurashi-deliver/references/reviewer-contract.md",
		sourceName:  "higurashi-deliver/references/reviewer-contract.md",
		destination: "references/reviewer-contract.md",
		skillName:   "higurashi-deliver",
	},
	{
		filesystem:  protocol.Files,
		templateID:  "protocol/higurashi-refine/SKILL.md",
		sourceName:  "higurashi-refine/SKILL.md",
		destination: "SKILL.md",
		skillName:   "higurashi-refine",
	},
}

var opencodeSources = []sourceSpec{
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/commands/higurashi-deliver.md.tmpl",
		sourceName:  "opencode/commands/higurashi-deliver.md.tmpl",
		destination: ".opencode/commands/higurashi-deliver.md",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/commands/higurashi-refine.md.tmpl",
		sourceName:  "opencode/commands/higurashi-refine.md.tmpl",
		destination: ".opencode/commands/higurashi-refine.md",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/agents/higurashi-orchestrator.md.tmpl",
		sourceName:  "opencode/agents/higurashi-orchestrator.md.tmpl",
		destination: ".opencode/agents/higurashi-orchestrator.md",
		modelRole:   "orchestrator",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/agents/higurashi-refine.md.tmpl",
		sourceName:  "opencode/agents/higurashi-refine.md.tmpl",
		destination: ".opencode/agents/higurashi-refine.md",
		modelRole:   "refine",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/agents/higurashi-plan.md.tmpl",
		sourceName:  "opencode/agents/higurashi-plan.md.tmpl",
		destination: ".opencode/agents/higurashi-plan.md",
		modelRole:   "plan",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/agents/higurashi-apply.md.tmpl",
		sourceName:  "opencode/agents/higurashi-apply.md.tmpl",
		destination: ".opencode/agents/higurashi-apply.md",
		modelRole:   "apply",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/agents/higurashi-verify-contract.md.tmpl",
		sourceName:  "opencode/agents/higurashi-verify-contract.md.tmpl",
		destination: ".opencode/agents/higurashi-verify-contract.md",
		modelRole:   "verify-contract",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/opencode/agents/higurashi-verify-risk.md.tmpl",
		sourceName:  "opencode/agents/higurashi-verify-risk.md.tmpl",
		destination: ".opencode/agents/higurashi-verify-risk.md",
		modelRole:   "verify-risk",
	},
}

var claudeCodeSources = []sourceSpec{
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/claude-code/.claude-plugin/plugin.json.tmpl",
		sourceName:  "claude-code/.claude-plugin/plugin.json.tmpl",
		destination: ".claude-plugin/plugin.json",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/claude-code/.mcp.json.tmpl",
		sourceName:  "claude-code/.mcp.json.tmpl",
		destination: ".mcp.json",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/claude-code/skills/deliver/SKILL.md.tmpl",
		sourceName:  "claude-code/skills/deliver/SKILL.md.tmpl",
		destination: "skills/deliver/SKILL.md",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/claude-code/skills/refine/SKILL.md.tmpl",
		sourceName:  "claude-code/skills/refine/SKILL.md.tmpl",
		destination: "skills/refine/SKILL.md",
	},
}

var claudeCodeAgentSources = []sourceSpec{
	{
		filesystem: adapters.Files,
		templateID: "adapters/claude-code/agents/higurashi-refine.md.tmpl",
		sourceName: "claude-code/agents/higurashi-refine.md.tmpl",
	},
	{
		filesystem: adapters.Files,
		templateID: "adapters/claude-code/agents/higurashi-plan.md.tmpl",
		sourceName: "claude-code/agents/higurashi-plan.md.tmpl",
	},
	{
		filesystem: adapters.Files,
		templateID: "adapters/claude-code/agents/higurashi-apply.md.tmpl",
		sourceName: "claude-code/agents/higurashi-apply.md.tmpl",
	},
	{
		filesystem: adapters.Files,
		templateID: "adapters/claude-code/agents/higurashi-verify-contract.md.tmpl",
		sourceName: "claude-code/agents/higurashi-verify-contract.md.tmpl",
	},
	{
		filesystem: adapters.Files,
		templateID: "adapters/claude-code/agents/higurashi-verify-risk.md.tmpl",
		sourceName: "claude-code/agents/higurashi-verify-risk.md.tmpl",
	},
}

var piSources = []sourceSpec{
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/pi/prompts/higurashi-deliver.md.tmpl",
		sourceName:  "pi/prompts/higurashi-deliver.md.tmpl",
		destination: ".pi/prompts/higurashi-deliver.md",
	},
	{
		filesystem:  adapters.Files,
		templateID:  "adapters/pi/prompts/higurashi-refine.md.tmpl",
		sourceName:  "pi/prompts/higurashi-refine.md.tmpl",
		destination: ".pi/prompts/higurashi-refine.md",
	},
}

// Build renders the canonical protocol for one supported runner destination.
func Build(adapter, generatorVersion string) (Bundle, error) {
	return BuildConfigured(
		adapter,
		generatorVersion,
		config.InheritedModels(),
	)
}

// BuildConfigured renders the protocol with runner-specific model selection.
func BuildConfigured(
	adapter, generatorVersion string,
	models config.ModelAssignments,
) (Bundle, error) {
	_, err := adapterSkillDirectory(adapter, "higurashi-deliver")
	if err != nil {
		return Bundle{}, err
	}
	if err := config.ValidateModelAssignments(models); err != nil {
		return Bundle{}, fmt.Errorf("models: %w", err)
	}
	if generatorVersion == "" {
		return Bundle{}, errors.New("generator version must not be empty")
	}
	if len(generatorVersion) > 128 {
		return Bundle{}, errors.New("generator version exceeds 128 characters")
	}
	for _, character := range generatorVersion {
		if !(character >= 'a' && character <= 'z') &&
			!(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') &&
			!strings.ContainsRune("._+-", character) {
			return Bundle{}, errors.New(
				"generator version contains an unsafe character",
			)
		}
	}

	bundle := Bundle{
		Adapter:          adapter,
		GeneratorVersion: generatorVersion,
	}
	sources := append([]sourceSpec(nil), canonicalSources...)
	switch adapter {
	case "opencode":
		sources = append(sources, opencodeSources...)
	case "claude-code":
		sources = append(sources, claudeCodeSources...)
		for _, agent := range claudeCodeAgentSources {
			pluginAgent := agent
			pluginAgent.destination = path.Join(
				"agents",
				path.Base(agent.sourceName),
			)
			pluginAgent.destination = strings.TrimSuffix(
				pluginAgent.destination,
				".tmpl",
			)
			sources = append(sources, pluginAgent)

			standaloneAgent := agent
			standaloneAgent.destination = path.Join(
				".claude/agents",
				path.Base(agent.sourceName),
			)
			standaloneAgent.destination = strings.TrimSuffix(
				standaloneAgent.destination,
				".tmpl",
			)
			sources = append(sources, standaloneAgent)
		}
	case "pi":
		sources = append(sources, piSources...)
	}
	for _, specification := range sources {
		source, err := fs.ReadFile(
			specification.filesystem,
			specification.sourceName,
		)
		if err != nil {
			return Bundle{}, fmt.Errorf(
				"read embedded template %s: %w",
				specification.templateID,
				err,
			)
		}
		sourceHash := hashBytes(source)
		renderedSource := source
		if adapter == "opencode" && specification.modelRole != "" {
			renderedSource, err = addOpenCodeModel(
				source,
				modelForRole(models, specification.modelRole),
			)
			if err != nil {
				return Bundle{}, fmt.Errorf(
					"%s: %w",
					specification.templateID,
					err,
				)
			}
		}
		content := addGeneratedHeader(
			renderedSource,
			specification.templateID,
			sourceHash,
			generatorVersion,
		)
		destination := specification.destination
		if specification.skillName != "" {
			base, err := adapterSkillDirectory(
				adapter,
				specification.skillName,
			)
			if err != nil {
				return Bundle{}, err
			}
			destination = path.Join(base, destination)
		}
		bundle.Files = append(bundle.Files, File{
			Path:       destination,
			TemplateID: specification.templateID,
			SourceHash: sourceHash,
			Source:     append([]byte(nil), source...),
			Content:    content,
		})
	}
	sort.Slice(bundle.Files, func(left, right int) bool {
		return bundle.Files[left].Path < bundle.Files[right].Path
	})
	if err := ValidateBundle(bundle); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func modelForRole(models config.ModelAssignments, role string) string {
	switch role {
	case "orchestrator":
		return models.Orchestrator
	case "refine":
		return models.Refine
	case "plan":
		return models.Plan
	case "apply":
		return models.Apply
	case "verify-contract":
		return models.VerifyContract
	case "verify-risk":
		return models.VerifyRisk
	default:
		return "inherit"
	}
}

func addOpenCodeModel(source []byte, reference string) ([]byte, error) {
	if reference == "inherit" {
		return append([]byte(nil), source...), nil
	}
	if !bytes.HasPrefix(source, []byte("---\n")) {
		return nil, errors.New("OpenCode agent template has no YAML frontmatter")
	}
	model, variant := config.SplitModelReference(reference)
	frontmatter := []byte("---\nmodel: " + model + "\n")
	if variant != "" {
		frontmatter = append(
			frontmatter,
			[]byte("variant: "+variant+"\n")...,
		)
	}
	content := make([]byte, 0, len(source)+len(frontmatter))
	content = append(content, frontmatter...)
	content = append(content, source[len("---\n"):]...)
	return content, nil
}

// ValidateBundle enforces policy that applies to every generated runner file.
func ValidateBundle(bundle Bundle) error {
	for _, file := range bundle.Files {
		if isAgentPath(file.Path) {
			if err := ValidateAgentTemplate(file.Content); err != nil {
				return fmt.Errorf("%s: %w", file.Path, err)
			}
		}
	}
	return nil
}

// ValidateAgentTemplate enforces the minimum anti-bypass policy inherited by
// every runner-specific subagent template.
func ValidateAgentTemplate(content []byte) error {
	if !bytes.Contains(content, []byte(AntiBypassRule)) {
		return ErrMissingAntiBypassRule
	}
	return nil
}

func isAgentPath(name string) bool {
	cleaned := "/" + strings.Trim(path.Clean(name), "/") + "/"
	return strings.Contains(cleaned, "/agents/")
}

func adapterSkillDirectory(adapter, skillName string) (string, error) {
	switch adapter {
	case "opencode":
		return path.Join(".agents/skills", skillName), nil
	case "claude-code":
		return path.Join(".claude/skills", skillName), nil
	case "pi":
		return path.Join(".pi/skills", skillName), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownAdapter, adapter)
	}
}

func addGeneratedHeader(
	source []byte,
	templateID string,
	sourceHash string,
	generatorVersion string,
) []byte {
	marker := fmt.Sprintf(
		"Higurashi-Generated: generator=higurashi version=%s template=%s source-sha256=%s",
		generatorVersion,
		templateID,
		sourceHash,
	)
	if bytes.Contains(source, []byte(markerPlaceholder)) {
		return bytes.ReplaceAll(
			source,
			[]byte(markerPlaceholder),
			[]byte(marker),
		)
	}
	if bytes.HasPrefix(source, []byte("---\n")) {
		content := make([]byte, 0, len(source)+len(marker)+3)
		content = append(content, []byte("---\n# ")...)
		content = append(content, marker...)
		content = append(content, '\n')
		content = append(content, source[len("---\n"):]...)
		return content
	}
	content := make([]byte, 0, len(source)+len(marker)+10)
	content = append(content, []byte("<!-- ")...)
	content = append(content, marker...)
	content = append(content, []byte(" -->\n")...)
	content = append(content, source...)
	return content
}

func hashBytes(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}
