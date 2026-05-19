package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	modelgatewayproxy "github.com/QuantumNous/new-api/pkg/modelgateway/proxy"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type ProxyContract struct {
	Name             string                `json:"name"`
	ProviderProfile  string                `json:"provider_profile"`
	ProxyMode        string                `json:"proxy_mode"`
	RequestModel     string                `json:"request_model"`
	UpstreamModel    string                `json:"upstream_model"`
	EndpointType     string                `json:"endpoint_type"`
	Stream           bool                  `json:"stream"`
	Request          map[string]any        `json:"request,omitempty"`
	UpstreamResponse map[string]any        `json:"upstream_response,omitempty"`
	UpstreamStream   []map[string]any      `json:"upstream_stream,omitempty"`
	Expected         ProxyContractExpected `json:"expected"`
}

type ProxyContractExpected struct {
	ProviderProfile   string         `json:"provider_profile"`
	ProxyMode         string         `json:"proxy_mode"`
	NativeResponses   bool           `json:"native_responses"`
	ResponsesViaChat  bool           `json:"responses_via_chat"`
	Reasoning         bool           `json:"reasoning"`
	WebSearch         bool           `json:"web_search"`
	UpstreamPath      string         `json:"upstream_path,omitempty"`
	UpstreamModel     string         `json:"upstream_model,omitempty"`
	DownstreamModel   string         `json:"downstream_model,omitempty"`
	OutputText        string         `json:"output_text,omitempty"`
	ReasoningText     string         `json:"reasoning_text,omitempty"`
	ToolName          string         `json:"tool_name,omitempty"`
	ToolArguments     string         `json:"tool_arguments,omitempty"`
	UpstreamToolTypes []string       `json:"upstream_tool_types,omitempty"`
	CustomToolTypes   []string       `json:"custom_tool_types,omitempty"`
	StreamItemTypes   []string       `json:"stream_item_types,omitempty"`
	ToolNames         []string       `json:"tool_names,omitempty"`
	ToolArgumentsList []string       `json:"tool_arguments_list,omitempty"`
	PromptCacheKey    string         `json:"prompt_cache_key,omitempty"`
	ReasoningEffort   string         `json:"reasoning_effort,omitempty"`
	ContentType       string         `json:"content_type,omitempty"`
	FileData          string         `json:"file_data,omitempty"`
	ImageURL          string         `json:"image_url,omitempty"`
	StreamEventType   string         `json:"stream_event_type,omitempty"`
	StreamEventCount  map[string]int `json:"stream_event_count,omitempty"`
	StreamErrorCode   string         `json:"stream_error_code,omitempty"`
	StreamErrorType   string         `json:"stream_error_type,omitempty"`
	StreamErrorText   string         `json:"stream_error_text,omitempty"`
}

func LoadProxyContract(path string) (*ProxyContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var contract ProxyContract
	if err := common.Unmarshal(data, &contract); err != nil {
		return nil, err
	}
	return &contract, nil
}

func ProxyContractPaths(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "testdata", "proxy", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	return paths
}

func RunProxyContract(t *testing.T, path string) {
	t.Helper()
	contract, err := LoadProxyContract(path)
	require.NoError(t, err)

	registry := provider.NewStandardProviderRegistry()
	profile, ok := registry.Get(contract.ProviderProfile)
	require.True(t, ok, "contract %s profile not registered", contract.Name)
	require.Equal(t, contract.Expected.ProviderProfile, profile.Name())
	require.Equal(t, contract.Expected.ProxyMode, contract.ProxyMode)

	capability := profile.Capabilities(nil, contract.RequestModel)
	require.Equal(t, contract.Expected.NativeResponses, capability.NativeResponses)
	require.Equal(t, contract.Expected.ResponsesViaChat, capability.ResponsesViaChat)
	require.Equal(t, contract.Expected.Reasoning, capability.Reasoning)
	require.Equal(t, contract.Expected.WebSearch, capability.WebSearch)
	require.Contains(t, capability.SupportedProxyModes, contract.ProxyMode)
	require.NotEmpty(t, capability.CapabilityFingerprint)
	runProxyConversionContract(t, contract)
}

func runProxyConversionContract(t *testing.T, contract *ProxyContract) {
	t.Helper()
	if len(contract.Request) == 0 && len(contract.UpstreamResponse) == 0 && len(contract.UpstreamStream) == 0 {
		return
	}
	engine := modelgatewayproxy.NewEngine(provider.NewStandardProviderRegistry())
	if len(contract.Request) > 0 {
		requestBody, err := common.Marshal(contract.Request)
		require.NoError(t, err)
		result, err := engine.ConvertRequest(modelgatewayproxy.ConvertRequestInput{
			ProviderProfile: contract.ProviderProfile,
			ProxyMode:       contract.ProxyMode,
			RequestModel:    contract.RequestModel,
			UpstreamModel:   contract.UpstreamModel,
			RequestBody:     requestBody,
		})
		require.NoError(t, err)
		require.Equal(t, contract.Expected.UpstreamPath, result.UpstreamPath)
		if contract.Expected.UpstreamModel != "" {
			require.Equal(t, contract.Expected.UpstreamModel, gjson.GetBytes(result.UpstreamBody, "model").String())
		}
		if contract.ProxyMode == provider.ProxyModeResponsesViaChat {
			require.True(t, gjson.GetBytes(result.UpstreamBody, "messages").IsArray())
		}
		if len(contract.Expected.UpstreamToolTypes) > 0 {
			require.ElementsMatch(t, contract.Expected.UpstreamToolTypes, upstreamToolTypes(result.UpstreamBody))
		}
		if len(contract.Expected.CustomToolTypes) > 0 {
			require.ElementsMatch(t, contract.Expected.CustomToolTypes, upstreamCustomToolTypes(result.UpstreamBody))
		}
		if contract.Expected.PromptCacheKey != "" {
			require.Equal(t, contract.Expected.PromptCacheKey, gjson.GetBytes(result.UpstreamBody, "prompt_cache_key").String())
		}
		if contract.Expected.ReasoningEffort != "" {
			require.Equal(t, contract.Expected.ReasoningEffort, gjson.GetBytes(result.UpstreamBody, "reasoning_effort").String())
		}
		if contract.Expected.ContentType != "" {
			require.Equal(t, contract.Expected.ContentType, gjson.GetBytes(result.UpstreamBody, "messages.1.content.1.type").String())
		}
		if contract.Expected.FileData != "" {
			require.Equal(t, contract.Expected.FileData, gjson.GetBytes(result.UpstreamBody, "messages.1.content.1.file.file_data").String())
		}
		if contract.Expected.ImageURL != "" {
			require.Equal(t, contract.Expected.ImageURL, gjson.GetBytes(result.UpstreamBody, "messages.1.content.2.image_url").String())
		}
	}
	if len(contract.UpstreamResponse) > 0 {
		responseBody, err := common.Marshal(contract.UpstreamResponse)
		require.NoError(t, err)
		result, err := engine.ConvertResponse(modelgatewayproxy.ConvertResponseInput{
			ProviderProfile: contract.ProviderProfile,
			ProxyMode:       contract.ProxyMode,
			RequestModel:    contract.RequestModel,
			UpstreamModel:   contract.UpstreamModel,
			ResponseBody:    responseBody,
		})
		require.NoError(t, err)
		if contract.Expected.DownstreamModel != "" {
			require.Equal(t, contract.Expected.DownstreamModel, gjson.GetBytes(result.DownstreamBody, "model").String())
		}
		if contract.Expected.OutputText != "" {
			require.Equal(t, contract.Expected.OutputText, gjson.GetBytes(result.DownstreamBody, "output.0.content.0.text").String())
		}
		if contract.Expected.ToolName != "" {
			require.Equal(t, contract.Expected.ToolName, gjson.GetBytes(result.DownstreamBody, "output.0.name").String())
		}
	}
	if len(contract.UpstreamStream) > 0 {
		events := make([]string, 0, len(contract.UpstreamStream))
		for _, event := range contract.UpstreamStream {
			body, err := common.Marshal(event)
			require.NoError(t, err)
			events = append(events, string(body))
		}
		result, err := engine.ConvertStream(modelgatewayproxy.ConvertStreamInput{
			ProviderProfile: contract.ProviderProfile,
			ProxyMode:       contract.ProxyMode,
			RequestModel:    contract.RequestModel,
			UpstreamModel:   contract.UpstreamModel,
			UpstreamEvents:  events,
		})
		require.NoError(t, err)
		require.NotEmpty(t, result.DownstreamEvents)
		if contract.Expected.OutputText != "" {
			require.Equal(t, contract.Expected.OutputText, result.OutputText)
		}
		if contract.Expected.ReasoningText != "" {
			require.Equal(t, contract.Expected.ReasoningText, result.ReasoningText)
		}
		if contract.Expected.ToolName != "" {
			require.Equal(t, contract.Expected.ToolName, result.ToolName)
		}
		if contract.Expected.ToolArguments != "" {
			require.JSONEq(t, contract.Expected.ToolArguments, result.ToolArguments)
		}
		if len(contract.Expected.ToolNames) > 0 {
			require.ElementsMatch(t, contract.Expected.ToolNames, streamToolNames(result.DownstreamEvents))
		}
		if len(contract.Expected.StreamItemTypes) > 0 {
			require.ElementsMatch(t, contract.Expected.StreamItemTypes, streamItemTypes(result.DownstreamEvents))
		}
		if len(contract.Expected.ToolArgumentsList) > 0 {
			require.ElementsMatch(t, contract.Expected.ToolArgumentsList, streamToolArguments(result.DownstreamEvents))
		}
		for eventType, expectedCount := range contract.Expected.StreamEventCount {
			require.Equal(t, expectedCount, countStreamEventsByType(result.DownstreamEvents, eventType), "event type %s", eventType)
		}
		if contract.Expected.StreamEventType != "" {
			event, ok := findStreamEventByType(result.DownstreamEvents, contract.Expected.StreamEventType)
			require.True(t, ok, "expected stream event type %s", contract.Expected.StreamEventType)
			if contract.Expected.StreamErrorCode != "" {
				require.Equal(t, contract.Expected.StreamErrorCode, gjson.Get(event, "response.error.code").String())
			}
			if contract.Expected.StreamErrorType != "" {
				require.Equal(t, contract.Expected.StreamErrorType, gjson.Get(event, "response.error.type").String())
			}
			if contract.Expected.StreamErrorText != "" {
				require.Contains(t, gjson.Get(event, "response.error.message").String(), contract.Expected.StreamErrorText)
			}
		}
	}
}

func upstreamToolTypes(body []byte) []string {
	values := gjson.GetBytes(body, "tools.#.type").Array()
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value.String() != "" {
			out = append(out, value.String())
		}
	}
	return out
}

func upstreamCustomToolTypes(body []byte) []string {
	values := gjson.GetBytes(body, "tools.#.custom.type").Array()
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value.String() != "" {
			out = append(out, value.String())
		}
	}
	return out
}

func findStreamEventByType(events []string, eventType string) (string, bool) {
	for _, event := range events {
		if gjson.Get(event, "type").String() == eventType {
			return event, true
		}
	}
	return "", false
}

func countStreamEventsByType(events []string, eventType string) int {
	count := 0
	for _, event := range events {
		if gjson.Get(event, "type").String() == eventType {
			count++
		}
	}
	return count
}

func streamToolNames(events []string) []string {
	names := make([]string, 0)
	for _, event := range events {
		if gjson.Get(event, "type").String() != "response.output_item.done" {
			continue
		}
		if gjson.Get(event, "item.type").String() != "function_call" {
			continue
		}
		if name := gjson.Get(event, "item.name").String(); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func streamItemTypes(events []string) []string {
	types := make([]string, 0)
	for _, event := range events {
		if gjson.Get(event, "type").String() != "response.output_item.done" {
			continue
		}
		if itemType := gjson.Get(event, "item.type").String(); itemType != "" {
			types = append(types, itemType)
		}
	}
	return types
}

func streamToolArguments(events []string) []string {
	args := make([]string, 0)
	for _, event := range events {
		if gjson.Get(event, "type").String() != "response.output_item.done" {
			continue
		}
		if gjson.Get(event, "item.type").String() != "function_call" {
			continue
		}
		if raw := gjson.Get(event, "item.arguments").Raw; raw != "" {
			args = append(args, raw)
		}
	}
	return args
}
