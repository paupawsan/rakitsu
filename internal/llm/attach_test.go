package llm

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestPNG writes a tiny valid PNG to path.
func writeTestPNG(t *testing.T, path string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write PNG fixture: %v", err)
	}
	return buf.Bytes()
}

// writeTestJPEG writes a tiny valid JPEG to path.
func writeTestJPEG(t *testing.T, path string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{0, 255, 0, 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode JPEG: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write JPEG fixture: %v", err)
	}
	return buf.Bytes()
}

func TestLoadImageAttachment_ValidPNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	want := writeTestPNG(t, path)

	block, err := LoadImageAttachment(path)
	if err != nil {
		t.Fatalf("LoadImageAttachment() error = %v", err)
	}
	if block.Type != ContentTypeImage {
		t.Errorf("Type = %q, want %q", block.Type, ContentTypeImage)
	}
	if block.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want image/png", block.MIMEType)
	}
	if block.Source.Kind != SourceKindBase64 {
		t.Errorf("Source.Kind = %q, want %q", block.Source.Kind, SourceKindBase64)
	}
	got, err := base64.StdEncoding.DecodeString(block.Source.Base64)
	if err != nil {
		t.Fatalf("decode Source.Base64: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("decoded bytes do not match original file")
	}
}

func TestLoadImageAttachment_ValidJPEG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	writeTestJPEG(t, path)

	block, err := LoadImageAttachment(path)
	if err != nil {
		t.Fatalf("LoadImageAttachment() error = %v", err)
	}
	if block.MIMEType != "image/jpeg" {
		t.Errorf("MIMEType = %q, want image/jpeg", block.MIMEType)
	}
}

func TestLoadImageAttachment_OversizedFile(t *testing.T) {
	orig := MaxAttachmentBytes
	MaxAttachmentBytes = 100
	defer func() { MaxAttachmentBytes = orig }()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	// Content past the 512-byte sniff window doesn't matter — the size
	// check runs before any read, so any 200-byte file trips it.
	if err := os.WriteFile(path, bytes.Repeat([]byte{0}, 200), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadImageAttachment(path)
	if err == nil {
		t.Fatal("LoadImageAttachment() error = nil, want a size-limit error")
	}
	// Regression: MaxAttachmentBytes/(1024*1024) used integer division,
	// which truncated any sub-1MB override to a misleading "0 MB" in the
	// message. The error must carry the real limit unambiguously.
	if strings.Contains(err.Error(), "0 MB attachment size limit") {
		t.Errorf("error message shows a misleading truncated limit: %v", err)
	}
	if !strings.Contains(err.Error(), "100 bytes") {
		t.Errorf("error message should state the exact byte limit, got: %v", err)
	}
}

func TestLoadImageAttachment_UnsupportedMIME(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\n%moredata"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadImageAttachment(path)
	if err == nil {
		t.Fatal("LoadImageAttachment() error = nil, want an unsupported-content-type error")
	}
}

func TestLoadImageAttachment_NonexistentPath(t *testing.T) {
	_, err := LoadImageAttachment("/nonexistent/path/does-not-exist.png")
	if err == nil {
		t.Fatal("LoadImageAttachment() error = nil, want a stat error")
	}
}

func TestLoadImageAttachment_Directory(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadImageAttachment(dir)
	if err == nil {
		t.Fatal("LoadImageAttachment() error = nil, want a directory rejection")
	}
}
