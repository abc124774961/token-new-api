package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func TestNormalizeChannelTestEndpointUsesResponsesWireAPI(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		OtherSettings: `{"wire_api":"responses"}`,
	}

	got := normalizeChannelTestEndpoint(channel, "gpt-4o", "")
	want := string(constant.EndpointTypeOpenAIResponse)
	if got != want {
		t.Fatalf("normalizeChannelTestEndpoint() = %q, want %q", got, want)
	}
}

func TestNormalizeChannelTestEndpointKeepsCompactPriority(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		OtherSettings: `{"wire_api":"responses"}`,
	}

	got := normalizeChannelTestEndpoint(channel, ratio_setting.WithCompactModelSuffix("gpt-4o"), "")
	want := string(constant.EndpointTypeOpenAIResponseCompact)
	if got != want {
		t.Fatalf("normalizeChannelTestEndpoint() = %q, want %q", got, want)
	}
}
