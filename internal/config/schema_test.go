package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemasAreValidJSON(t *testing.T) {
	tests := []string{
		"claude-mcp.schema.json",
		"claude-plugin.schema.json",
		"config.schema.json",
		"doctor.schema.json",
		"inspection.schema.json",
	}

	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			name := filepath.Join("..", "..", "schemas", test)
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}

			var schema any
			if err := json.Unmarshal(data, &schema); err != nil {
				t.Fatalf("decode %s: %v", name, err)
			}
		})
	}
}
