package ratio_setting

import "testing"

func TestFormatMatchingModelNameCodexGeneratedMiniVariants(t *testing.T) {
	tests := map[string]string{
		"gpt-5.4-mini":                 "gpt-5.4",
		"gpt-5.4-mini-2026-03-05":      "gpt-5.4",
		"gpt-5.5-mini":                 "gpt-5.5",
		"gpt-5.5-mini-openai-compact":  "gpt-5.5",
		"gpt-5-mini":                   "gpt-5-mini",
		"gpt-5.4-nano":                 "gpt-5.4-nano",
		"gemini-2.5-flash-thinking-10": "gemini-2.5-flash-thinking-*",
	}

	for input, want := range tests {
		if got := FormatMatchingModelName(input); got != want {
			t.Fatalf("FormatMatchingModelName(%q) = %q, want %q", input, got, want)
		}
	}
}
