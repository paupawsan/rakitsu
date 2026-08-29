package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WriteJSON marshals v to JSON and writes it to path.
func WriteJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// WriteYAML marshals v to YAML and writes it to path.
func WriteYAML(path string, v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// WriteFile writes content to path with 0644 permissions.
func WriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// WriteExecutable writes a file with 0755 permissions (executable).
func WriteExecutable(path, content string) error {
	return os.WriteFile(path, []byte(content), 0755)
}

// EnsureDir creates the parent directory of path if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0755)
}
