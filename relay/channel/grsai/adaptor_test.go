package grsai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

func TestGetRequestURLUsesStandardImageGenerationPath(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    appconstant.ChannelTypeGrsai,
			ChannelBaseUrl: "https://grsaiapi.com",
		},
		RelayMode:      relayconstant.RelayModeImagesEdits,
		RequestURLPath: "/v1/images/edits",
	}

	got, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}
	want := "https://grsaiapi.com/v1/images/generations"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestConvertImageRequestMapsImagesToGrsaiImageArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    appconstant.ChannelTypeGrsai,
			ChannelBaseUrl: "https://grsaiapi.com",
		},
		RelayMode:      relayconstant.RelayModeImagesGenerations,
		RequestURLPath: "/v1/images/generations",
	}

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:          ModelGPTImage2,
		Prompt:         "draw",
		Images:         []byte(`["https://example.com/a.png"]`),
		Size:           "1024x1024",
		ResponseFormat: "url",
	})
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	payload, ok := converted.(map[string]any)
	if !ok {
		t.Fatalf("converted request type = %T, want map[string]any", converted)
	}
	images, ok := payload["image"].([]string)
	if !ok {
		t.Fatalf("payload image type = %T, want []string", payload["image"])
	}
	if len(images) != 1 || images[0] != "https://example.com/a.png" {
		t.Fatalf("payload image = %#v", images)
	}
	if payload["size"] != "1024x1024" {
		t.Fatalf("payload size = %#v", payload["size"])
	}
	if info.RequestURLPath != standardImagePath {
		t.Fatalf("RequestURLPath = %q, want %q", info.RequestURLPath, standardImagePath)
	}
}

func TestConvertImageRequestSupportsLegacyPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    appconstant.ChannelTypeGrsai,
			ChannelBaseUrl: "https://grsaiapi.com",
		},
		RelayMode:      relayconstant.RelayModeImagesGenerations,
		RequestURLPath: "/v1/images/generations",
	}

	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  ModelGPTImage2,
		Prompt: "draw",
		Image:  []byte(`"https://example.com/a.png"`),
		Size:   "16:9",
		Extra: map[string]json.RawMessage{
			"grsai_legacy": json.RawMessage(`true`),
		},
	})
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}

	payload := converted.(map[string]any)
	images, ok := payload["images"].([]string)
	if !ok || len(images) != 1 || images[0] != "https://example.com/a.png" {
		t.Fatalf("legacy payload images = %#v", payload["images"])
	}
	if payload["aspectRatio"] != "16:9" {
		t.Fatalf("legacy payload aspectRatio = %#v", payload["aspectRatio"])
	}
	if info.RequestURLPath != legacyImagePath {
		t.Fatalf("RequestURLPath = %q, want %q", info.RequestURLPath, legacyImagePath)
	}
}

func TestGrsaiImageHandlerPassesThroughStandardResponseAndUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"created":1777689832,"data":[{"url":"https://example.com/a.png"}],"usage":{"total_tokens":10,"input_tokens":3,"output_tokens":7,"input_tokens_details":{}}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	usage, err := grsaiImageHandler(c, resp, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: appconstant.ChannelTypeGrsai,
		},
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Now(),
	})
	if err != nil {
		t.Fatalf("grsaiImageHandler returned error: %v", err)
	}
	if usage.PromptTokens != 3 || usage.CompletionTokens != 7 || usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v", usage)
	}
	if !strings.Contains(recorder.Body.String(), "https://example.com/a.png") {
		t.Fatalf("response body = %s", recorder.Body.String())
	}
}

func TestGrsaiImageHandlerConvertsLegacyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"id":"14-5f3cf761","status":"succeeded","results":[{"url":"https://example.com/a.png"}]}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	if _, err := grsaiImageHandler(c, resp, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: appconstant.ChannelTypeGrsai,
		},
		RelayMode: relayconstant.RelayModeImagesGenerations,
		StartTime: time.Unix(1777689832, 0),
	}); err != nil {
		t.Fatalf("grsaiImageHandler returned error: %v", err)
	}

	var imageResp dto.ImageResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &imageResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(imageResp.Data) != 1 || imageResp.Data[0].Url != "https://example.com/a.png" {
		t.Fatalf("image response = %+v", imageResp)
	}
	if imageResp.Created != 1777689832 {
		t.Fatalf("created = %d", imageResp.Created)
	}
}
