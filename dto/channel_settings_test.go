package dto

import "testing"

func TestChannelOtherSettingsUsesResponsesWireAPI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		wireAPI string
		want    bool
	}{
		{name: "empty", wireAPI: "", want: false},
		{name: "responses", wireAPI: "responses", want: true},
		{name: "trimmed", wireAPI: " /responses/ ", want: true},
		{name: "nested responses", wireAPI: "/backend-api/codex/responses", want: true},
		{name: "other path", wireAPI: "/chat/completions", want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := (&ChannelOtherSettings{WireAPI: tc.wireAPI}).UsesResponsesWireAPI()
			if got != tc.want {
				t.Fatalf("UsesResponsesWireAPI(%q) = %v, want %v", tc.wireAPI, got, tc.want)
			}
		})
	}
}

func TestChannelOtherSettingsUsesCodexCompatibilityMode(t *testing.T) {
	t.Parallel()

	if (&ChannelOtherSettings{}).UsesCodexCompatibilityMode() {
		t.Fatal("empty settings should not enable Codex compatibility mode")
	}
	if !(&ChannelOtherSettings{CodexCompatibilityMode: true}).UsesCodexCompatibilityMode() {
		t.Fatal("expected Codex compatibility mode to be enabled")
	}
}
