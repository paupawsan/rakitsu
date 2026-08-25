package chat

import (
	"strings"
	"testing"
)

func TestParseSlashCommand_NotACommand(t *testing.T) {
	cases := []string{"hello", "", "  ", "not a /command"}
	for _, input := range cases {
		if cmd := ParseSlashCommand(input); cmd != nil {
			t.Errorf("ParseSlashCommand(%q) = %+v, want nil", input, cmd)
		}
	}
}

func TestParseSlashCommand_Simple(t *testing.T) {
	cmd := ParseSlashCommand("/help")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Name != "help" {
		t.Errorf("Name = %q, want help", cmd.Name)
	}
	if cmd.Args != "" {
		t.Errorf("Args = %q, want empty", cmd.Args)
	}
}

func TestParseSlashCommand_WithArgs(t *testing.T) {
	cmd := ParseSlashCommand("/model gpt-4o")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Name != "model" {
		t.Errorf("Name = %q, want model", cmd.Name)
	}
	if cmd.Args != "gpt-4o" {
		t.Errorf("Args = %q, want gpt-4o", cmd.Args)
	}
}

func TestParseSlashCommand_CaseInsensitive(t *testing.T) {
	cmd := ParseSlashCommand("/HELP")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Name != "help" {
		t.Errorf("Name = %q, want help (lowercase)", cmd.Name)
	}
}

func TestParseSlashCommand_Whitespace(t *testing.T) {
	cmd := ParseSlashCommand("  /exit  ")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Name != "exit" {
		t.Errorf("Name = %q, want exit", cmd.Name)
	}
}

func TestParseSlashCommand_MultiWordArgs(t *testing.T) {
	cmd := ParseSlashCommand("/model some long model name")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}
	if cmd.Args != "some long model name" {
		t.Errorf("Args = %q, want 'some long model name'", cmd.Args)
	}
}

func TestHelpText_MentionsUsageNotHistory(t *testing.T) {
	h := HelpText()
	if !strings.Contains(h, "/usage") {
		t.Errorf("HelpText() missing /usage")
	}
	if strings.Contains(h, "/history") {
		t.Errorf("HelpText() still mentions /history, want it removed")
	}
}

func TestHelpText_MentionsContext(t *testing.T) {
	h := HelpText()
	if !strings.Contains(h, "/context") {
		t.Errorf("HelpText() missing /context")
	}
}
