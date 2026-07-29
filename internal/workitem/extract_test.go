package workitem

import (
	"bytes"
	"errors"
	"testing"
)

func TestExtractPreservesTargetMarkdownSectionBytes(t *testing.T) {
	content := []byte("## Parent\r\n\r\n### WORK-123: Target\r\n\r\nBody\r\n\r\n#### Evidence\r\n\r\nProof\r\n\r\n### WORK-124: Next\r\n")
	want := []byte("### WORK-123: Target\r\n\r\nBody\r\n\r\n#### Evidence\r\n\r\nProof\r\n\r\n")

	got, err := Extract(content, "WORK-123")

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Extract() = %q, want %q", got, want)
	}
}

func TestExtractIgnoresHeadingsInsideFences(t *testing.T) {
	content := []byte("```markdown\n### WORK-123: Example\n```\n\n### WORK-123: Real\n\nBody\n")

	got, err := Extract(content, "WORK-123")

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !bytes.Equal(got, []byte("### WORK-123: Real\n\nBody\n")) {
		t.Errorf("Extract() = %q", got)
	}
}

func TestExtractRejectsDuplicateWorkItem(t *testing.T) {
	content := []byte("## WORK-123 First\n\n## WORK-123 Second\n")

	_, err := Extract(content, "WORK-123")

	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Extract() error = %v, want ErrConflict", err)
	}
}
