package artifact

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicInterruptedCopyPreservesOriginal(t *testing.T) {
	directory := t.TempDir()
	name := filepath.Join(directory, "WORK-123.md")
	original := []byte("original durable artifact\n")
	if err := os.WriteFile(name, original, 0o640); err != nil {
		t.Fatalf("write original artifact: %v", err)
	}

	err := writeAtomic(
		name,
		&interruptedReader{content: []byte("partial replacement")},
		0o640,
	)

	if err == nil {
		t.Fatal("writeAtomic() error = nil, want interrupted write")
	}
	actual, readErr := os.ReadFile(name)
	if readErr != nil {
		t.Fatalf("read original artifact: %v", readErr)
	}
	if string(actual) != string(original) {
		t.Errorf("artifact = %q, want %q", actual, original)
	}
	temporary, globErr := filepath.Glob(
		filepath.Join(directory, ".WORK-123.md.tmp-*"),
	)
	if globErr != nil {
		t.Fatalf("glob temporary artifacts: %v", globErr)
	}
	if len(temporary) != 0 {
		t.Errorf("temporary artifacts remain: %v", temporary)
	}
}

type interruptedReader struct {
	content []byte
	read    bool
}

func (reader *interruptedReader) Read(buffer []byte) (int, error) {
	if reader.read {
		return 0, errors.New("simulated interruption")
	}
	reader.read = true
	count := copy(buffer, reader.content)
	if count < len(reader.content) {
		reader.content = reader.content[count:]
		reader.read = false
	}
	return count, nil
}

var _ io.Reader = (*interruptedReader)(nil)
