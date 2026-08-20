package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContentBlock_TextBlockOmitsSource(t *testing.T) {
	// Regression: `omitempty` has no effect on a plain struct field. Source
	// must be *BlockSource so a text-only block doesn't always serialize a
	// spurious "source":{}.
	msg := NewTextMessage("user", "hello")
	data, err := json.Marshal(msg.Content[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"source"`) {
		t.Errorf("text-only block should omit source, got %s", data)
	}
}

func TestContentBlock_ImageBlockIncludesSource(t *testing.T) {
	block := ContentBlock{
		Type:     ContentTypeImage,
		MIMEType: "image/png",
		Source:   &BlockSource{Kind: SourceKindBase64, Base64: "aGVsbG8="},
	}
	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"base64":"aGVsbG8="`) {
		t.Errorf("image block should serialize its source, got %s", data)
	}
}
