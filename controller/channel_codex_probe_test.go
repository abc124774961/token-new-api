package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCollectCodexResponsesImageToolProbeModelsExcludesImageModelsAndDedupes(t *testing.T) {
	models := []string{
		"gpt-image-1",
		"gpt-5.5",
		"gpt-5.5",
		"gpt-5.2",
		"custom-model",
	}

	require.Equal(t, []string{"gpt-5.5", "gpt-5.2"}, collectCodexResponsesImageToolProbeModels(models))
}

func TestProbeCodexResponsesImageToolRequiresImageGenerationCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/responses", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_probe",
			"status":"completed",
			"model":"gpt-5.5",
			"output":[{"type":"image_generation_call","status":"completed","result":"abc"}]
		}`))
	}))
	defer server.Close()

	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		BaseURL: &server.URL,
	}

	ok, message := probeCodexResponsesImageTool(channel, []string{"gpt-5.5", "gpt-image-1"}, []string{"gpt-image-1"})
	require.True(t, ok)
	require.Contains(t, message, "gpt-5.5")
}

func TestProbeCodexResponsesImageToolRejectsTextOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_probe",
			"status":"completed",
			"model":"gpt-5.5",
			"output":[{"type":"message","content":[{"type":"output_text","text":"<svg></svg>"}]}]
		}`))
	}))
	defer server.Close()

	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		BaseURL: &server.URL,
	}

	ok, message := probeCodexResponsesImageTool(channel, []string{"gpt-5.5", "gpt-image-1"}, []string{"gpt-image-1"})
	require.False(t, ok)
	require.Contains(t, message, "未通过")
}

func TestProbeCodexResponsesImageToolAcceptsGeneratingImageCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_probe",
			"status":"in_progress",
			"model":"gpt-5.5",
			"output":[{"type":"image_generation_call","status":"generating"}]
		}`))
	}))
	defer server.Close()

	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		BaseURL: &server.URL,
	}

	ok, message := probeCodexResponsesImageTool(channel, []string{"gpt-5.5", "gpt-image-1"}, []string{"gpt-image-1"})
	require.True(t, ok)
	require.Contains(t, message, "gpt-5.5")
}

func TestProbeCodexResponsesImageToolRejectsFailedImageCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_probe",
			"status":"completed",
			"model":"gpt-5.5",
			"output":[{"type":"image_generation_call","status":"failed"}]
		}`))
	}))
	defer server.Close()

	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "test-key",
		BaseURL: &server.URL,
	}

	ok, message := probeCodexResponsesImageTool(channel, []string{"gpt-5.5", "gpt-image-1"}, []string{"gpt-image-1"})
	require.False(t, ok)
	require.Contains(t, message, "未通过")
}

func TestProbeUnsavedChannelCodexImageGenerationToolUsesSmokeTest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{
				"data":[{"id":"gpt-5.5"},{"id":"gpt-image-1"}]
			}`))
		case "/v1/responses":
			_, _ = w.Write([]byte(`{
				"id":"resp_probe",
				"status":"completed",
				"model":"gpt-5.5",
				"output":[{"type":"message","content":[{"type":"output_text","text":"<svg></svg>"}]}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	body, err := common.Marshal(map[string]any{
		"base_url": server.URL,
		"type":     constant.ChannelTypeOpenAI,
		"key":      "test-key",
		"settings": `{"codex_compatibility_mode":true}`,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/codex/image_generation_tool/probe", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ProbeUnsavedChannelCodexImageGenerationTool(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Supported bool     `json:"supported"`
			Models    []string `json:"models"`
			Message   string   `json:"message"`
			Tools     []string `json:"tools"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.False(t, payload.Data.Supported)
	require.Equal(t, []string{"gpt-image-1"}, payload.Data.Models)
	require.Empty(t, payload.Data.Tools)
	require.Contains(t, payload.Data.Message, "未通过")
}

func TestProbeUnsavedChannelCodexImageGenerationToolReturnsToolNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5"},{"id":"gpt-image-1"}]}`))
		case "/v1/responses":
			_, _ = w.Write([]byte(`{
				"id":"resp_probe",
				"status":"in_progress",
				"model":"gpt-5.5",
				"output":[{"type":"image_generation_call","status":"generating"}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	body, err := common.Marshal(map[string]any{
		"base_url": server.URL,
		"type":     constant.ChannelTypeOpenAI,
		"key":      "test-key",
		"settings": `{"codex_compatibility_mode":true}`,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/codex/image_generation_tool/probe", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ProbeUnsavedChannelCodexImageGenerationTool(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Supported bool     `json:"supported"`
			Tools     []string `json:"tools"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.True(t, payload.Data.Supported)
	require.Equal(t, []string{dto.BuildInToolImageGeneration}, payload.Data.Tools)
}
