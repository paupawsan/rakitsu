// Package fs provides file system tool implementations.
// It supports read, write, list, and search operations with path restrictions.
package fs

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/paupawsan/rakitsu/internal/config"
)

// Tool implements the Tool interface for file system operations
type Tool struct {
	name         string
	description  string
	operation    string   // "read", "write", "list", "search"
	allowedPaths []string // Security: restrict file access
	workingDir   string   // resolve relative paths against this
	parameters   map[string]config.Parameter
}

// NewTool creates a new FS tool from configuration
func NewTool(def *config.ToolDefinition) *Tool {
	allowedPaths := def.AllowedPaths
	if len(allowedPaths) == 0 {
		// Default to current directory only
		allowedPaths = []string{"."}
	}

	workDir := def.WorkingDir
	if workDir == "" && len(allowedPaths) > 0 && allowedPaths[0] != "." {
		workDir = allowedPaths[0] // use first allowed path as default working dir
	}

	return &Tool{
		name:         def.Name,
		description:  def.Description,
		operation:    def.Operation,
		allowedPaths: allowedPaths,
		workingDir:   workDir,
		parameters:   def.Parameters,
	}
}

// GetName returns the tool's name
func (t *Tool) GetName() string {
	return t.name
}

// GetDescription returns the tool's description
func (t *Tool) GetDescription() string {
	return t.description
}

// GetParametersSchema returns the JSON Schema for parameters
func (t *Tool) GetParametersSchema() map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
		"required":   []string{},
	}
	
	props := schema["properties"].(map[string]interface{})
	var required []string
	
	for name, param := range t.parameters {
		prop := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Default != nil {
			prop["default"] = param.Default
		}
		if len(param.Enum) > 0 {
			prop["enum"] = param.Enum
		}
		props[name] = prop
		
		if param.Required {
			required = append(required, name)
		}
	}
	
	schema["required"] = required
	return schema
}

// Execute performs the file system operation
func (t *Tool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	// Get path argument — fall back to first allowed path if omitted
	pathVal, ok := args["path"]
	pathStr := ""
	if ok && pathVal != nil {
		pathStr = fmt.Sprintf("%v", pathVal)
	}
	if pathStr == "" || pathStr == "<nil>" {
		if len(t.allowedPaths) > 0 {
			pathStr = t.allowedPaths[0]
		} else {
			return "", fmt.Errorf("path argument required")
		}
	}
	path := pathStr

	// Resolve relative paths against working directory
	if !filepath.IsAbs(path) && t.workingDir != "" {
		path = filepath.Join(t.workingDir, path)
	}

	// Validate path is within allowed paths
	if !t.isPathAllowed(path) {
		return "", &PathNotAllowedError{
			Path:   path,
			Allowed: t.allowedPaths,
		}
	}
	
	// Execute based on operation type
	switch t.operation {
	case "read":
		return t.readFile(ctx, path)
	case "write":
		content, ok := args["content"].(string)
		if !ok {
			return "", fmt.Errorf("content argument required for write operation")
		}
		mode, _ := args["mode"].(string)
		return t.writeFile(ctx, path, content, mode)
	case "list":
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			pattern = "*"
		}
		return t.listDir(ctx, path, pattern)
	case "search":
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			// No content pattern — fall back to listing all files
			return t.listDir(ctx, path, "*")
		}
		filePattern, _ := args["file_pattern"].(string)
		if filePattern == "" {
			filePattern = "*"
		}
		return t.searchFiles(ctx, path, pattern, filePattern)
	default:
		return "", fmt.Errorf("unknown operation: %s", t.operation)
	}
}

// isPathAllowed checks if a path is within allowed directories.
// Resolves symlinks to prevent traversal via symlink chains.
func (t *Tool) isPathAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Resolve symlinks to get the real path.
	// Walk up the path until we find an existing ancestor to resolve.
	realPath := resolvePathWithSymlinks(absPath)

	for _, allowed := range t.allowedPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		realAllowed := strings.TrimRight(resolvePathWithSymlinks(absAllowed), string(filepath.Separator))
		if realPath == realAllowed || strings.HasPrefix(realPath, realAllowed+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// resolvePathWithSymlinks resolves symlinks for existing path components.
// For paths that don't fully exist yet (e.g., write targets), it resolves
// the deepest existing ancestor and appends the remaining components.
func resolvePathWithSymlinks(absPath string) string {
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return resolved
	}

	// Walk up until we find an existing directory
	dir := absPath
	var tail []string
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached root
		}
		tail = append([]string{filepath.Base(dir)}, tail...)
		dir = parent
		resolved, err = filepath.EvalSymlinks(dir)
		if err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...)
		}
	}
	return absPath // fallback to original
}

// readFile reads the contents of a file
func (t *Tool) readFile(ctx context.Context, path string) (string, error) {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot access file: %w", err)
	}
	
	// Check if it's a directory
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}
	
	// Read file contents
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	
	return string(content), nil
}

// writeFile writes content to a file
func (t *Tool) writeFile(ctx context.Context, path, content, mode string) (string, error) {
	// Default mode is "write" (overwrite)
	if mode == "" {
		mode = "write"
	}
	
	// Create parent directories if needed
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			return "", fmt.Errorf("failed to create directory: %w", mkErr)
		}
	}

	var err error
	switch mode {
	case "write":
		err = ioutil.WriteFile(path, []byte(content), 0644)
	case "append":
		var f *os.File
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "", fmt.Errorf("failed to open file: %w", err)
		}
		defer f.Close()
		_, err = f.WriteString(content)
	default:
		return "", fmt.Errorf("unknown write mode: %s", mode)
	}
	
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// listDir lists the contents of a directory
func (t *Tool) listDir(ctx context.Context, path, pattern string) (string, error) {
	// Check if directory exists
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot access directory: %w", err)
	}
	
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}
	
	// Read directory contents
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}
	
	// Filter by pattern and format output
	var lines []string
	for _, entry := range entries {
		matched, err := filepath.Match(pattern, entry.Name())
		if err != nil {
			continue
		}
		if matched {
			typeStr := "file"
			if entry.IsDir() {
				typeStr = "dir"
			}
			lines = append(lines, fmt.Sprintf("%s\t%s\t%d", typeStr, entry.Name(), entry.Size()))
		}
	}
	
	if len(lines) == 0 {
		return "No files found matching pattern: " + pattern, nil
	}
	
	return strings.Join(lines, "\n"), nil
}

// searchFiles searches for files matching a pattern
func (t *Tool) searchFiles(ctx context.Context, basePath, contentPattern, filePattern string) (string, error) {
	var results []string
	
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Check file pattern
		matched, err := filepath.Match(filePattern, info.Name())
		if err != nil || !matched {
			return nil
		}
		
		// Read file and search for content pattern
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return nil // Skip files we can't read
		}
		
		if strings.Contains(string(content), contentPattern) {
			results = append(results, path)
		}
		
		return nil
	})
	
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	
	if len(results) == 0 {
		return "No files found containing: " + contentPattern, nil
	}
	
	return strings.Join(results, "\n"), nil
}

// PathNotAllowedError is returned when a path is not in allowed paths
type PathNotAllowedError struct {
	Path    string
	Allowed []string
}

func (e *PathNotAllowedError) Error() string {
	return fmt.Sprintf("path '%s' is not in allowed paths: %v", e.Path, e.Allowed)
}
