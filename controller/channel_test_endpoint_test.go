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

func TestResolveChannelTestEndpointUsesChatWhenStoredCapabilityDeniesResponses(t *testing.T) {
	t.Parallel()

	index := 0
	denied := false
	allowed := true
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI,
		Key:  `{"access_token":"access-token","refresh_token":"refresh-token","account_id":"account-id"}`,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					ResponsesWrite:       &denied,
					ChatCompletionsWrite: &allowed,
				},
			},
		},
	}

	got := resolveChannelTestEndpoint(channel, "gpt-4o", "", channelTestOptions{CredentialIndex: &index})

	if got != string(constant.EndpointTypeOpenAI) {
		t.Fatalf("resolveChannelTestEndpoint() = %q, want %q", got, string(constant.EndpointTypeOpenAI))
	}
}

func TestResolveChannelTestEndpointUsesResponsesWhenProxyBridgeAllowed(t *testing.T) {
	t.Parallel()

	index := 0
	denied := false
	allowed := true
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI,
		Key:  `{"access_token":"access-token","refresh_token":"refresh-token","account_id":"account-id"}`,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					ResponsesWrite:       &denied,
					ChatCompletionsWrite: &allowed,
				},
			},
		},
	}

	got := resolveChannelTestEndpoint(channel, "gpt-4o", "", channelTestOptions{
		CredentialIndex:  &index,
		AllowProxyBridge: true,
	})

	if got != string(constant.EndpointTypeOpenAIResponse) {
		t.Fatalf("resolveChannelTestEndpoint() = %q, want %q", got, string(constant.EndpointTypeOpenAIResponse))
	}
}
