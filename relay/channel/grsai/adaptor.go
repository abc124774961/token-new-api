package grsai

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	standardImagePath = "/v1/images/generations"
	legacyImagePath   = "/v1/api/generate"
	legacyResultPath  = "/v1/api/result"

	legacyAPICtxKey = "grsai_legacy_image_api"
)

type Adaptor struct {
	openaiAdaptor openai.Adaptor
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.openaiAdaptor.Init(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("grsai adaptor: relay info is nil")
	}
	if info.ChannelBaseUrl == "" {
		info.ChannelBaseUrl = appconstant.ChannelBaseURLs[appconstant.ChannelTypeGrsai]
	}
	if info.RelayMode == relayconstant.RelayModeImagesGenerations ||
		info.RelayMode == relayconstant.RelayModeImagesEdits {
		requestPath := standardImagePath
		if info.RequestURLPath == legacyImagePath {
			requestPath = legacyImagePath
		}
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestPath, info.ChannelType), nil
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	if req.Get("Content-Type") == "" || strings.HasPrefix(req.Get("Content-Type"), "multipart/form-data") {
		req.Set("Content-Type", "application/json")
	}
	if req.Get("Accept") == "" {
		req.Set("Accept", "application/json")
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return a.openaiAdaptor.ConvertOpenAIRequest(c, info, request)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, request)
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return a.openaiAdaptor.ConvertGeminiRequest(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return a.openaiAdaptor.ConvertRerankRequest(c, relayMode, request)
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return a.openaiAdaptor.ConvertAudioRequest(c, info, request)
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return a.openaiAdaptor.ConvertOpenAIResponsesRequest(c, info, request)
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil {
		return nil, errors.New("grsai adaptor: relay info is nil")
	}
	if info.RelayMode != relayconstant.RelayModeImagesGenerations &&
		info.RelayMode != relayconstant.RelayModeImagesEdits {
		return nil, fmt.Errorf("grsai adaptor: unsupported image relay mode %d", info.RelayMode)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, errors.New("grsai adaptor: prompt is required")
	}

	modelName := strings.TrimSpace(info.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(request.Model)
	}
	if modelName == "" {
		modelName = ModelGPTImage2
	}
	info.UpstreamModelName = modelName

	images, err := collectImageInputs(c, request)
	if err != nil {
		return nil, err
	}

	useLegacyAPI := shouldUseLegacyImageAPI(request)
	if useLegacyAPI {
		info.RequestURLPath = legacyImagePath
		if c != nil {
			c.Set(legacyAPICtxKey, true)
		}
		return buildLegacyImagePayload(request, modelName, images), nil
	}

	info.RequestURLPath = standardImagePath
	return buildStandardImagePayload(request, modelName, images), nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info != nil && (info.RelayMode == relayconstant.RelayModeImagesGenerations ||
		info.RelayMode == relayconstant.RelayModeImagesEdits) {
		return grsaiImageHandler(c, resp, info)
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		return a.openaiAdaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func buildStandardImagePayload(request dto.ImageRequest, modelName string, images []string) map[string]any {
	payload := baseImagePayload(request, modelName)
	if len(images) > 0 {
		payload["image"] = images
	}
	return payload
}

func buildLegacyImagePayload(request dto.ImageRequest, modelName string, images []string) map[string]any {
	payload := map[string]any{
		"model":     modelName,
		"prompt":    request.Prompt,
		"replyType": "json",
	}
	if len(images) > 0 {
		payload["images"] = images
	}
	if aspectRatio := grsaiAspectRatio(request); aspectRatio != "" {
		payload["aspectRatio"] = aspectRatio
	}
	mergeExtraFields(payload, request)
	if replyType, ok := stringFromRaw(request.Extra["replyType"]); ok && replyType != "" {
		payload["replyType"] = replyType
	}
	return payload
}

func baseImagePayload(request dto.ImageRequest, modelName string) map[string]any {
	payload := map[string]any{
		"model":  modelName,
		"prompt": request.Prompt,
	}
	if request.Size != "" {
		payload["size"] = request.Size
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = normalizeResponseFormat(request.ResponseFormat)
	}
	if request.N != nil && *request.N > 0 {
		payload["n"] = int(*request.N)
	}
	mergeExtraFields(payload, request)
	return payload
}

func mergeExtraFields(payload map[string]any, request dto.ImageRequest) {
	if len(request.ExtraFields) > 0 {
		var extra map[string]any
		if err := common.Unmarshal(request.ExtraFields, &extra); err == nil {
			for key, val := range extra {
				payload[key] = val
			}
		}
	}
	for key, raw := range request.Extra {
		if shouldSkipExtraKey(key) {
			continue
		}
		var val any
		if err := common.Unmarshal(raw, &val); err != nil {
			continue
		}
		payload[key] = val
	}
}

func shouldSkipExtraKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "image", "images", "grsai_legacy", "grsai_legacy_api", "grsai_reply_type":
		return true
	default:
		return false
	}
}

func normalizeResponseFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "url":
		return "url"
	case "b64_json", "base64":
		return "b64_json"
	default:
		return format
	}
}

func grsaiAspectRatio(request dto.ImageRequest) string {
	for _, key := range []string{"aspectRatio", "aspect_ratio"} {
		if value, ok := stringFromRaw(request.Extra[key]); ok && value != "" {
			return value
		}
	}
	return request.Size
}

func shouldUseLegacyImageAPI(request dto.ImageRequest) bool {
	for _, key := range []string{"grsai_legacy", "grsai_legacy_api"} {
		if raw, ok := request.Extra[key]; ok {
			var enabled bool
			if err := common.Unmarshal(raw, &enabled); err == nil {
				return enabled
			}
		}
	}
	if _, ok := request.Extra["replyType"]; ok {
		return true
	}
	return false
}

func collectImageInputs(c *gin.Context, request dto.ImageRequest) ([]string, error) {
	var images []string
	for _, raw := range []struct {
		name string
		data []byte
	}{
		{name: "image", data: request.Image},
		{name: "images", data: request.Images},
	} {
		values, err := imageStringsFromRaw(raw.data)
		if err != nil {
			return nil, fmt.Errorf("grsai adaptor: invalid %s field: %w", raw.name, err)
		}
		images = append(images, values...)
	}

	formImages, err := imageStringsFromMultipart(c)
	if err != nil {
		return nil, err
	}
	images = append(images, formImages...)
	return dedupeStrings(images), nil
}

func imageStringsFromRaw(raw []byte) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		return nil, nil
	}
	var values []string
	if err := common.Unmarshal(raw, &values); err == nil {
		return nonEmptyStrings(values), nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		if strings.TrimSpace(value) == "" {
			return nil, nil
		}
		return []string{strings.TrimSpace(value)}, nil
	}
	var objects []map[string]any
	if err := common.Unmarshal(raw, &objects); err == nil {
		for _, object := range objects {
			if value := imageStringFromObject(object); value != "" {
				values = append(values, value)
			}
		}
		return nonEmptyStrings(values), nil
	}
	return nil, errors.New("expected string or array of strings")
}

func imageStringFromObject(object map[string]any) string {
	for _, key := range []string{"url", "image_url", "b64_json", "base64"} {
		if value, ok := object[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	if imageURL, ok := object["image_url"].(map[string]any); ok {
		if value, ok := imageURL["url"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func imageStringsFromMultipart(c *gin.Context) ([]string, error) {
	if c == nil || c.Request == nil || !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		return nil, nil
	}
	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return nil, fmt.Errorf("grsai adaptor: failed to parse multipart form: %w", err)
		}
		mf = c.Request.MultipartForm
	}
	if mf == nil {
		return nil, nil
	}

	var images []string
	for _, field := range []string{"image", "image[]", "images", "input_reference"} {
		images = append(images, nonEmptyStrings(mf.Value[field])...)
		fileImages, err := imageStringsFromFiles(mf.File[field])
		if err != nil {
			return nil, err
		}
		images = append(images, fileImages...)
	}
	return images, nil
}

func imageStringsFromFiles(files []*multipart.FileHeader) ([]string, error) {
	var images []string
	for _, fileHeader := range files {
		if fileHeader == nil {
			continue
		}
		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("grsai adaptor: failed to open image file %s: %w", fileHeader.Filename, err)
		}
		fileBytes, readErr := io.ReadAll(file)
		_ = file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("grsai adaptor: failed to read image file %s: %w", fileHeader.Filename, readErr)
		}
		if len(fileBytes) == 0 {
			continue
		}
		images = append(images, base64.StdEncoding.EncodeToString(fileBytes))
	}
	return images, nil
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range nonEmptyStrings(values) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func stringFromRaw(raw []byte) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return strings.TrimSpace(value), true
}

type legacyImageResponse struct {
	ID       string              `json:"id"`
	Status   string              `json:"status"`
	Progress int                 `json:"progress,omitempty"`
	Results  []legacyImageResult `json:"results,omitempty"`
	Error    string              `json:"error,omitempty"`
}

type legacyImageResult struct {
	URL   string `json:"url,omitempty"`
	Error string `json:"error,omitempty"`
}

func grsaiImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("grsai adaptor: invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := service.ReadAllWithJSONKeepAlive(c, resp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if parsedSSE := lastSSEPayload(responseBody); len(parsedSSE) > 0 {
		responseBody = parsedSSE
	}

	var legacy legacyImageResponse
	if err := common.Unmarshal(responseBody, &legacy); err == nil && looksLikeLegacyResponse(legacy) {
		if shouldPollLegacyResponse(legacy) {
			polled, pollErr := pollLegacyResult(c, info, legacy.ID)
			if pollErr != nil {
				return nil, types.NewOpenAIError(pollErr, types.ErrorCodeBadResponse, http.StatusGatewayTimeout)
			}
			legacy = *polled
		}
		return writeLegacyImageResponse(c, resp, info, legacy)
	}

	var usageResp dto.SimpleResponse
	if err := common.Unmarshal(responseBody, &usageResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesWithJSONKeepAliveGracefully(c, resp, responseBody)
	return normalizeUsage(&usageResp.Usage), nil
}

func looksLikeLegacyResponse(response legacyImageResponse) bool {
	return response.ID != "" && response.Status != ""
}

func shouldPollLegacyResponse(response legacyImageResponse) bool {
	switch strings.ToLower(strings.TrimSpace(response.Status)) {
	case "running", "queued", "pending", "processing":
		return response.ID != ""
	default:
		return false
	}
}

func writeLegacyImageResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, legacy legacyImageResponse) (*dto.Usage, *types.NewAPIError) {
	switch strings.ToLower(strings.TrimSpace(legacy.Status)) {
	case "succeeded", "success", "completed":
		imageResponse := dto.ImageResponse{
			Created: common.GetTimestamp(),
			Data:    make([]dto.ImageData, 0, len(legacy.Results)),
		}
		if info != nil && !info.StartTime.IsZero() {
			imageResponse.Created = info.StartTime.Unix()
		}
		for _, result := range legacy.Results {
			if result.URL != "" {
				imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: result.URL})
			}
		}
		if len(imageResponse.Data) == 0 {
			return nil, types.NewOpenAIError(errors.New("grsai adaptor: no image returned"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseBytes, err := common.Marshal(imageResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.Writer.Header().Set("Content-Type", "application/json")
		c.Writer.WriteHeader(http.StatusOK)
		_, _ = c.Writer.Write(responseBytes)
		return &dto.Usage{}, nil
	case "failed", "failure", "violation":
		message := legacy.Error
		for _, result := range legacy.Results {
			if result.Error != "" {
				message = result.Error
				break
			}
		}
		if message == "" {
			message = fmt.Sprintf("grsai image task %s", legacy.Status)
		}
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: message,
			Type:    "grsai_image_error",
			Code:    legacy.Status,
		}, http.StatusBadGateway)
	default:
		return nil, types.NewOpenAIError(fmt.Errorf("grsai adaptor: image task not completed, status=%s", legacy.Status), types.ErrorCodeBadResponse, http.StatusGatewayTimeout)
	}
}

func normalizeUsage(usage *dto.Usage) *dto.Usage {
	if usage == nil {
		return &dto.Usage{}
	}
	if usage.InputTokens > 0 {
		usage.PromptTokens += usage.InputTokens
	}
	if usage.OutputTokens > 0 {
		usage.CompletionTokens += usage.OutputTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails.ImageTokens += usage.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.TextTokens += usage.InputTokensDetails.TextTokens
	}
	return usage
}

func pollLegacyResult(c *gin.Context, info *relaycommon.RelayInfo, taskID string) (*legacyImageResponse, error) {
	if info == nil {
		return nil, errors.New("grsai adaptor: relay info is nil")
	}
	if taskID == "" {
		return nil, errors.New("grsai adaptor: task id is empty")
	}
	proxyURL := common.GetContextKeyString(c, appconstant.ContextKeyChannelAccountProxyURL)
	if proxyURL == "" {
		proxyURL = info.ChannelSetting.Proxy
	}
	client, err := service.GetHttpClientWithProxy(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("grsai adaptor: create http client failed: %w", err)
	}
	requestURL := relaycommon.GetFullRequestURL(info.ChannelBaseUrl, legacyResultPath, info.ChannelType)
	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return nil, fmt.Errorf("grsai adaptor: invalid result url: %w", err)
	}
	query := parsedURL.Query()
	query.Set("id", taskID)
	parsedURL.RawQuery = query.Encode()

	var lastResponse *legacyImageResponse
	for attempt := 0; attempt < 30; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(2 * time.Second):
			case <-c.Request.Context().Done():
				return nil, c.Request.Context().Err()
			}
		}
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+info.ApiKey)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("grsai adaptor: result query failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var parsed legacyImageResponse
		if err := common.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("grsai adaptor: parse result response failed: %w", err)
		}
		lastResponse = &parsed
		if !shouldPollLegacyResponse(parsed) {
			return &parsed, nil
		}
	}
	if lastResponse != nil {
		return lastResponse, nil
	}
	return nil, errors.New("grsai adaptor: result query timed out")
}

func lastSSEPayload(body []byte) []byte {
	text := strings.TrimSpace(string(body))
	if !strings.Contains(text, "\ndata:") && !strings.HasPrefix(text, "data:") {
		return nil
	}
	var last string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		last = data
	}
	if last == "" {
		return nil
	}
	return []byte(last)
}
