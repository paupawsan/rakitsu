package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/paupawsan/rakitsu/internal/config"
)

// ============================================================
// Helpers
// ============================================================

func newFSTool(op string, allowedPaths []string) *Tool {
	return NewTool(&config.ToolDefinition{
		Name:         "test-fs",
		Description:  "test",
		Operation:    op,
		AllowedPaths: allowedPaths,
	})
}

// writeTemp creates a temp file with content and returns its path.
func writeTemp(t *testing.T, dir, content string) string {
	t.Helper()
	f, err := os.CreateTemp(dir, "fstool-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_, err = f.WriteString(content)
	f.Close()
	if err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	return f.Name()
}

// ============================================================
// isPathAllowed — traversal guard
// ============================================================

func TestIsPathAllowed_ExactDir(t *testing.T) {
	dir := t.TempDir()
	tool := newFSTool("read", []string{dir})
	if !tool.isPathAllowed(dir) {
		t.Errorf("exact dir %q should be allowed", dir)
	}
}

func TestIsPathAllowed_FileUnderDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "file.txt")
	tool := newFSTool("read", []string{dir})
	if !tool.isPathAllowed(file) {
		t.Errorf("file under allowed dir should be allowed")
	}
}

func TestIsPathAllowed_SiblingDir_Denied(t *testing.T) {
	parent := t.TempDir()
	allowed := filepath.Join(parent, "allowed")
	sibling := filepath.Join(parent, "secret")
	os.MkdirAll(allowed, 0755)
	os.MkdirAll(sibling, 0755)

	tool := newFSTool("read", []string{allowed})
	if tool.isPathAllowed(sibling) {
		t.Error("sibling directory should not be allowed")
	}
}

func TestIsPathAllowed_TraversalAttack_Denied(t *testing.T) {
	dir := t.TempDir()
	tool := newFSTool("read", []string{dir})

	// Attempt to escape via ../
	traversal := filepath.Join(dir, "..", "secret")
	if tool.isPathAllowed(traversal) {
		t.Error("path traversal via ../ should be denied")
	}
}

func TestIsPathAllowed_DefaultDot_AllowsCWD(t *testing.T) {
	tool := newFSTool("read", nil) // defaults to "."
	cwd, _ := os.Getwd()
	if !tool.isPathAllowed(cwd) {
		t.Error("current working directory should be allowed under default '.' policy")
	}
}

func TestIsPathAllowed_RootAllowedPath_AllowsSubpath(t *testing.T) {
	// Regression: realAllowed=="/" plus an unconditional separator append
	// produced "//", which no real path has a prefix of — every subpath
	// was incorrectly denied under an allowed_paths: ["/"] config.
	tool := newFSTool("read", []string{"/"})
	dir := t.TempDir() // some real absolute path guaranteed to exist
	if !tool.isPathAllowed(dir) {
		t.Errorf("path %q should be allowed under allowed_paths [\"/\"]", dir)
	}
}

// ============================================================
// Read operation
// ============================================================

func TestRead_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := writeTemp(t, dir, "hello world")
	tool := newFSTool("read", []string{dir})

	out, err := tool.Execute(context.Background(), map[string]interface{}{"path": file})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in output, got %q", out)
	}
}

func TestRead_MissingFile_Error(t *testing.T) {
	dir := t.TempDir()
	tool := newFSTool("read", []string{dir})

	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": filepath.Join(dir, "nonexistent.txt")})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRead_DirectoryPath_Error(t *testing.T) {
	dir := t.TempDir()
	tool := newFSTool("read", []string{dir})

	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": dir})
	if err == nil {
		t.Fatal("expected error when reading a directory")
	}
}

// ============================================================
// Write operation
// ============================================================

func TestWrite_NewFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.txt")
	tool := newFSTool("write", []string{dir})

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    dest,
		"content": "new content",
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !strings.Contains(out, "wrote") && !strings.Contains(out, "bytes") {
		t.Errorf("expected success message, got %q", out)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "new content" {
		t.Errorf("file content mismatch: %q", string(data))
	}
}

func TestWrite_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	file := writeTemp(t, dir, "old")
	tool := newFSTool("write", []string{dir})

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    file,
		"content": "new",
	})
	if err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "new" {
		t.Errorf("expected 'new', got %q", string(data))
	}
}

func TestWrite_AppendMode(t *testing.T) {
	dir := t.TempDir()
	file := writeTemp(t, dir, "first")
	tool := newFSTool("write", []string{dir})

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    file,
		"content": " second",
		"mode":    "append",
	})
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "first second" {
		t.Errorf("expected 'first second', got %q", string(data))
	}
}

func TestWrite_UnknownMode_Error(t *testing.T) {
	dir := t.TempDir()
	tool := newFSTool("write", []string{dir})

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    filepath.Join(dir, "f.txt"),
		"content": "x",
		"mode":    "truncate", // not valid
	})
	if err == nil {
		t.Fatal("expected error for unknown write mode")
	}
}

func TestWrite_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "a", "b", "c.txt")
	tool := newFSTool("write", []string{dir})

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    dest,
		"content": "deep",
	})
	if err != nil {
		t.Fatalf("write to nested path failed: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if string(data) != "deep" {
		t.Errorf("expected 'deep', got %q", string(data))
	}
}

// ============================================================
// List operation
// ============================================================

func TestList_Directory(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "x")
	writeTemp(t, dir, "y")
	tool := newFSTool("list", []string{dir})

	out, err := tool.Execute(context.Background(), map[string]interface{}{"path": dir})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if out == "" || strings.Contains(out, "No files") {
		t.Errorf("expected file listing, got %q", out)
	}
}

func TestList_PatternFilter(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "") // creates *.txt
	// create a .go file
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main"), 0644)

	tool := newFSTool("list", []string{dir})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    dir,
		"pattern": "*.go",
	})
	if err != nil {
		t.Fatalf("list with pattern failed: %v", err)
	}
	if !strings.Contains(out, "main.go") {
		t.Errorf("expected main.go in listing, got %q", out)
	}
}

func TestList_FilePath_Error(t *testing.T) {
	dir := t.TempDir()
	file := writeTemp(t, dir, "x")
	tool := newFSTool("list", []string{dir})

	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": file})
	if err == nil {
		t.Fatal("expected error when listing a file (not a dir)")
	}
}

// ============================================================
// Search operation
// ============================================================

func TestSearch_ContentMatch(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "needle in haystack")
	tool := newFSTool("search", []string{dir})

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    dir,
		"pattern": "needle",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if strings.Contains(out, "No files") {
		t.Errorf("expected match, got: %q", out)
	}
}

func TestSearch_NoMatch_EmptyResult(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "no match here")
	tool := newFSTool("search", []string{dir})

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    dir,
		"pattern": "xyzzy-not-found",
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.Contains(out, "No files") {
		t.Errorf("expected 'No files' message, got %q", out)
	}
}

// ============================================================
// Execute — path security via PathNotAllowedError
// ============================================================

func TestExecute_PathNotAllowed(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir() // separate temp dir — not in allowed paths
	tool := newFSTool("read", []string{allowed})

	file := writeTemp(t, secret, "secret")
	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": file})
	if err == nil {
		t.Fatal("expected error for path outside allowed paths")
	}
	var pe *PathNotAllowedError
	if !errors.As(err, &pe) {
		t.Errorf("expected *PathNotAllowedError, got %T: %v", err, err)
	}
}

func TestExecute_UnknownOperation(t *testing.T) {
	dir := t.TempDir()
	tool := newFSTool("upload", []string{dir}) // not a valid operation

	_, err := tool.Execute(context.Background(), map[string]interface{}{"path": dir})
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
}

func TestExecute_MissingPathArg_FallsBackToFirstAllowedPath(t *testing.T) {
	dir := t.TempDir()
	file := writeTemp(t, dir, "fallback content")
	tool := newFSTool("read", []string{file})

	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected the missing path to fall back to allowedPaths[0], got error: %v", err)
	}
	if out != "fallback content" {
		t.Errorf("want %q, got %q", "fallback content", out)
	}
}

func TestExecute_MissingPathArg_NoAllowedPathsIsError(t *testing.T) {
	// NewTool always defaults an empty AllowedPaths to ["."], so this
	// branch is unreachable through the public constructor today — built
	// directly to exercise Execute's own defensive check instead of
	// asserting on NewTool's default (that's covered separately by
	// TestIsPathAllowed_DefaultDot_AllowsCWD).
	tool := &Tool{operation: "read"}

	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error when 'path' is missing and no allowed paths are configured to fall back to")
	}
}

// ============================================================
// Stress — concurrent reads
// ============================================================

func TestStress_ConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	file := writeTemp(t, dir, "shared content")
	tool := newFSTool("read", []string{dir})

	const N = 50
	var wg sync.WaitGroup
	var successes atomic.Int64

	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := tool.Execute(context.Background(), map[string]interface{}{"path": file})
			if err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := successes.Load(); got != N {
		t.Errorf("expected %d successful reads, got %d", N, got)
	}
}

func TestStress_ConcurrentPathChecks(t *testing.T) {
	dir := t.TempDir()
	tool := newFSTool("read", []string{dir})

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				tool.isPathAllowed(dir)
			} else {
				tool.isPathAllowed("/etc/passwd")
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// working-dir resolution — paupawsan/rakitsu#28
// ============================================================

// TestWrite_FencedAllowedPath_ResolvesAgainstCwd locks down the #28 fix:
// with allowed_paths fencing a subdirectory and no explicit working_dir, a
// relative path that already names the fenced directory resolves against the
// process cwd — the old allowedPaths[0] default doubled it into
// workspace/workspace/… via writeFile's MkdirAll.
func TestWrite_FencedAllowedPath_ResolvesAgainstCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("workspace", 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tool := newFSTool("write", []string{"./workspace/"})
	if _, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    "./workspace/file.txt",
		"content": "hi",
	}); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := os.Stat("workspace/file.txt"); err != nil {
		t.Errorf("expected file at cwd-relative path: %v", err)
	}
	if _, err := os.Stat("workspace/workspace"); err == nil {
		t.Error("doubled workspace/workspace path was created")
	}
}

// TestWrite_FencedAllowedPath_BareRelativeDenied: without the old fallback a
// bare filename resolves outside the fence and must fail loudly instead of
// being silently relocated into it.
func TestWrite_FencedAllowedPath_BareRelativeDenied(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("workspace", 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tool := newFSTool("write", []string{"./workspace/"})
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    "file.txt",
		"content": "hi",
	})
	var pErr *PathNotAllowedError
	if !errors.As(err, &pErr) {
		t.Errorf("expected *PathNotAllowedError, got %T: %v", err, err)
	}
}

// TestWrite_ExplicitWorkingDir_StillJoins: an explicit working_dir keeps its
// meaning — relative paths join onto it.
func TestWrite_ExplicitWorkingDir_StillJoins(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("workspace", 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tool := NewTool(&config.ToolDefinition{
		Name:         "test-fs",
		Description:  "test",
		Operation:    "write",
		AllowedPaths: []string{"./workspace/"},
		WorkingDir:   "./workspace/",
	})
	if _, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    "file.txt",
		"content": "hi",
	}); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := os.Stat("workspace/file.txt"); err != nil {
		t.Errorf("expected file under working_dir: %v", err)
	}
}
