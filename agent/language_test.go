package agent

import (
	"strings"
	"testing"
)

func TestParseAgentLanguage(t *testing.T) {
	for in, want := range map[string]string{
		"es": "es", " ES ": "es", "Español": "es", "spanish": "es",
		"en": "en", "English": "en", "auto": "auto",
	} {
		if got, ok := ParseAgentLanguage(in); !ok || got != want {
			t.Errorf("ParseAgentLanguage(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "fr", "es-BO"} {
		if _, ok := ParseAgentLanguage(in); ok {
			t.Errorf("ParseAgentLanguage(%q) accepted an unknown language", in)
		}
	}
}

func TestRenderLanguageBlock(t *testing.T) {
	if got := renderLanguageBlock(AgentLanguageEnglish); got != "" {
		t.Fatalf("English must leave prompts unchanged, got %q", got)
	}
	for _, code := range []string{AgentLanguageSpanish, AgentLanguageAuto} {
		block := renderLanguageBlock(code)
		if !strings.Contains(block, languageHeader) || !strings.Contains(block, "Operator setting, not a target fact.") {
			t.Errorf("%s block lacks the code-owned header: %q", code, block)
		}
		if !strings.Contains(block, "enumerated value") {
			t.Errorf("%s block must keep schema enums untranslated: %q", code, block)
		}
	}
	if !strings.Contains(renderLanguageBlock(AgentLanguageSpanish), "answer in Spanish") {
		t.Error("Spanish block must ask for Spanish answers")
	}
}

func TestAgentLanguageIgnoresUnknown(t *testing.T) {
	defer SetAgentLanguage(AgentLanguage())
	SetAgentLanguage("en")
	SetAgentLanguage("klingon")
	if got := AgentLanguage(); got != AgentLanguageEnglish {
		t.Fatalf("unknown language changed the setting to %q", got)
	}
}

func TestPromptsCarryLanguageBlock(t *testing.T) {
	defer SetAgentLanguage(AgentLanguage())
	SetAgentLanguage(AgentLanguageSpanish)
	if !strings.Contains(chatSystem("assistant", t.TempDir(), t.TempDir()), languageHeader+"Spanish") {
		t.Error("chat prompt lacks the language block")
	}
	SetAgentLanguage(AgentLanguageEnglish)
	if strings.Contains(chatSystem("assistant", t.TempDir(), t.TempDir()), languageHeader) {
		t.Error("English chat prompt still carries a language block")
	}
}
