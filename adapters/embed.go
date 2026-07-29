package adapters

import "embed"

// Files contains runner-specific templates. Canonical workflow behavior remains
// in protocol; adapters only express runner-native invocation and permissions.
//
//go:embed opencode/commands/*.tmpl opencode/agents/*.tmpl
//go:embed claude-code/.claude-plugin/*.tmpl claude-code/skills/*/*.tmpl
//go:embed claude-code/agents/*.tmpl claude-code/*.tmpl
var Files embed.FS
