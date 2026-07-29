package protocol

import "embed"

// Files contains the runner-neutral canonical protocol sources.
//
//go:embed higurashi-deliver/SKILL.md higurashi-deliver/references/*.md
//go:embed higurashi-refine/SKILL.md
var Files embed.FS
