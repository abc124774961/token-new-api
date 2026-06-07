package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"
)

const upstreamResponseSnippetLimit = 512
const upstreamBalanceInsufficientClientMessage = "上游渠道暂不可用，请稍后重试"

func MidjourneyErrorWrapper(code int, desc string) *dto.MidjourneyResponse {
	return &dto.MidjourneyResponse{
		Code:        code,
		Description: desc,
	}
}

func MidjourneyErrorWithStatusCodeWrapper(code int, desc string, statusCode int) *dto.MidjourneyResponseWithStatusCode {
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   *MidjourneyErrorWrapper(code, desc),
	}
}

//// OpenAIErrorWrapper wraps an error into an OpenAIErrorWithStatusCode
//func OpenAIErrorWrapper(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	text := err.Error()
//	lowerText := strings.ToLower(text)
//	if !strings.HasPrefix(lowerText, "get file base64 from url") && !strings.HasPrefix(lowerText, "mime type is not supported") {
//		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
//			common.SysLog(fmt.Sprintf("error: %s", text))
//			text = "请求上游地址失败"
//		}
//	}
//	openAIError := dto.OpenAIError{
//		Message: text,
//		Type:    "new_api_error",
//		Code:    code,
//	}
//	return &dto.OpenAIErrorWithStatusCode{
//		Error:      openAIError,
//		StatusCode: statusCode,
//	}
//}
//
//func OpenAIErrorWrapperLocal(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	openaiErr := OpenAIErrorWrapper(err, code, statusCode)
//	openaiErr.LocalError = true
//	return openaiErr
//}

func ClaudeErrorWrapper(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if !strings.HasPrefix(lowerText, "get file base64 from url") {
		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
			common.SysLog(fmt.Sprintf("error: %s", text))
			text = "请求上游地址失败"
		}
	}
	claudeError := types.ClaudeError{
		Message: text,
		Type:    "new_api_error",
	}
	return &dto.ClaudeErrorWithStatusCode{
		Error:      claudeError,
		StatusCode: statusCode,
	}
}

func ClaudeErrorWrapperLocal(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	claudeErr := ClaudeErrorWrapper(err, code, statusCode)
	claudeErr.LocalError = true
	return claudeErr
}

func UpstreamOpenAIError(openAIError types.OpenAIError, statusCode int) *types.NewAPIError {
	newApiErr := types.WithOpenAIError(openAIError, statusCode)
	if IsBalanceInsufficientError(newApiErr) {
		return NewUpstreamBalanceInsufficientError(statusCode, nil)
	}
	return newApiErr
}

func NewUpstreamBalanceInsufficientError(upstreamStatusCode int, metadata json.RawMessage) *types.NewAPIError {
	newApiErr := types.NewErrorWithStatusCode(
		errors.New(upstreamBalanceInsufficientClientMessage),
		types.ErrorCodeUpstreamUnavailable,
		http.StatusServiceUnavailable,
	)
	if len(metadata) > 0 {
		newApiErr.Metadata = metadata
	} else if upstreamStatusCode > 0 {
		newApiErr.Metadata = buildUpstreamStatusMetadata(upstreamStatusCode)
	}
	return newApiErr
}

func SanitizeClientRelayError(newApiErr *types.NewAPIError) *types.NewAPIError {
	if newApiErr == nil {
		return nil
	}
	if !IsBalanceInsufficientError(newApiErr) || types.IsSkipRetryError(newApiErr) {
		return newApiErr
	}
	return NewUpstreamBalanceInsufficientError(newApiErr.StatusCode, newApiErr.Metadata)
}

func IsClientContextLimitError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	switch err.StatusCode {
	case 0, http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
	default:
		return false
	}
	openAIError := err.ToOpenAIError()
	label := strings.Join([]string{
		string(err.GetErrorCode()),
		string(err.GetErrorType()),
		fmt.Sprint(openAIError.Code),
		openAIError.Type,
		openAIError.Message,
		err.Error(),
	}, " ")
	return IsClientContextLimitMessage(label)
}

func IsClientContextLimitMessage(message string) bool {
	label := strings.ToLower(strings.TrimSpace(message))
	if label == "" {
		return false
	}
	for _, keyword := range []string{
		"context_too_large",
		"context_length_exceeded",
		"context_length_error",
		"context length exceeded",
		"maximum context length",
		"max context length",
		"exceeds context length",
		"exceeded context length",
		"exceeds the context window",
		"exceeded the context window",
		"context window exceeded",
		"prompt is too long",
		"prompt too long",
		"input is too long",
		"input too long",
	} {
		if strings.Contains(label, keyword) {
			return true
		}
	}
	contextLimit := strings.Contains(label, "context") &&
		(strings.Contains(label, "length") || strings.Contains(label, "window")) &&
		(strings.Contains(label, "exceed") ||
			strings.Contains(label, "too large") ||
			strings.Contains(label, "too long") ||
			strings.Contains(label, "over limit") ||
			strings.Contains(label, "limit exceeded"))
	if contextLimit {
		return true
	}
	return strings.Contains(label, "上下文") &&
		(strings.Contains(label, "超过") ||
			strings.Contains(label, "超出") ||
			strings.Contains(label, "超限") ||
			strings.Contains(label, "过长") ||
			strings.Contains(label, "太长"))
}

func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (newApiErr *types.NewAPIError) {
	newApiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	CloseResponseBodyGracefully(resp)
	hideUpstreamBalanceError := !showBodyWhenFail
	upstreamMetadata := buildUpstreamResponseMetadata(resp, responseBody, false)
	attachUpstreamMetadata := func(e *types.NewAPIError) *types.NewAPIError {
		if e != nil && len(upstreamMetadata) > 0 {
			e.Metadata = upstreamMetadata
		}
		return e
	}
	buildBalanceInsufficientError := func() *types.NewAPIError {
		return NewUpstreamBalanceInsufficientError(resp.StatusCode, buildUpstreamResponseMetadata(resp, responseBody, true))
	}
	var errResponse dto.GeneralErrorResponse
	buildErrWithBody := func(message string) error {
		if message == "" {
			return fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody))
		}
		return fmt.Errorf("bad response status code %d, message: %s, body: %s", resp.StatusCode, message, string(responseBody))
	}

	err = common.Unmarshal(responseBody, &errResponse)
	if err != nil {
		if hideUpstreamBalanceError && IsBalanceInsufficientMessage(string(responseBody)) {
			return buildBalanceInsufficientError()
		}
		if showBodyWhenFail {
			newApiErr.Err = buildErrWithBody("")
		} else {
			logger.LogError(ctx, fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody)))
			newApiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		return attachUpstreamMetadata(newApiErr)
	}

	if common.GetJsonType(errResponse.Error) == "object" {
		// General format error (OpenAI, Anthropic, Gemini, etc.)
		oaiError := errResponse.TryToOpenAIError()
		if oaiError != nil {
			newApiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
			if hideUpstreamBalanceError && IsBalanceInsufficientError(newApiErr) {
				return buildBalanceInsufficientError()
			}
			if showBodyWhenFail {
				newApiErr.Err = buildErrWithBody(newApiErr.Error())
			}
			return attachUpstreamMetadata(newApiErr)
		}
	}
	newApiErr = types.NewOpenAIError(errors.New(errResponse.ToMessage()), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	if hideUpstreamBalanceError && IsBalanceInsufficientError(newApiErr) {
		return buildBalanceInsufficientError()
	}
	if showBodyWhenFail {
		newApiErr.Err = buildErrWithBody(newApiErr.Error())
	}
	return attachUpstreamMetadata(newApiErr)
}

func buildUpstreamStatusMetadata(statusCode int) json.RawMessage {
	if statusCode <= 0 {
		return nil
	}
	raw, err := common.Marshal(map[string]any{
		"status_code": statusCode,
	})
	if err != nil {
		return nil
	}
	return raw
}

func buildUpstreamResponseMetadata(resp *http.Response, responseBody []byte, omitBodySnippet bool) json.RawMessage {
	metadata := map[string]any{}
	if resp != nil {
		metadata["status_code"] = resp.StatusCode
		if statusText := strings.TrimSpace(resp.Status); statusText != "" {
			metadata["status_text"] = statusText
		}
		if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
			metadata["content_type"] = contentType
		}
		if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
			metadata["retry_after"] = retryAfter
			if retryAfterSeconds, ok := parseRetryAfterSeconds(retryAfter); ok {
				metadata["retry_after_seconds"] = retryAfterSeconds
			}
		}
		attachRateLimitHeader(metadata, resp.Header, "X-RateLimit-Limit", "rate_limit_limit")
		attachRateLimitHeader(metadata, resp.Header, "X-RateLimit-Remaining", "rate_limit_remaining")
		attachRateLimitHeader(metadata, resp.Header, "X-RateLimit-Reset", "rate_limit_reset")
		attachRateLimitHeader(metadata, resp.Header, "x-ratelimit-limit-requests", "rate_limit_requests")
		attachRateLimitHeader(metadata, resp.Header, "x-ratelimit-remaining-requests", "rate_limit_remaining_requests")
		attachRateLimitHeader(metadata, resp.Header, "x-ratelimit-reset-requests", "rate_limit_reset_requests")
		attachRateLimitHeader(metadata, resp.Header, "x-ratelimit-limit-tokens", "rate_limit_tokens")
		attachRateLimitHeader(metadata, resp.Header, "x-ratelimit-remaining-tokens", "rate_limit_remaining_tokens")
		attachRateLimitHeader(metadata, resp.Header, "x-ratelimit-reset-tokens", "rate_limit_reset_tokens")
	}
	if len(responseBody) > 0 {
		bodySnippet := responseBody
		truncated := false
		if len(bodySnippet) > upstreamResponseSnippetLimit {
			bodySnippet = bodySnippet[:upstreamResponseSnippetLimit]
			truncated = true
		}
		metadata["body_length"] = len(responseBody)
		if !omitBodySnippet {
			metadata["body_snippet"] = strings.TrimSpace(string(bodySnippet))
			if truncated {
				metadata["body_truncated"] = true
			}
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	raw, err := common.Marshal(metadata)
	if err != nil {
		return nil
	}
	return raw
}

func attachRateLimitHeader(metadata map[string]any, headers http.Header, headerName string, metadataKey string) {
	if metadata == nil || headers == nil {
		return
	}
	if value := strings.TrimSpace(headers.Get(headerName)); value != "" {
		metadata[metadataKey] = value
	}
}

func parseRetryAfterSeconds(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return seconds, true
	}
	if t, err := http.ParseTime(value); err == nil {
		seconds := int64(time.Until(t).Seconds())
		if seconds < 0 {
			return 0, true
		}
		return seconds, true
	}
	return 0, false
}

func ResetStatusCode(newApiErr *types.NewAPIError, statusCodeMappingStr string) {
	if newApiErr == nil {
		return
	}
	if statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	statusCodeMapping := make(map[string]any)
	err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping)
	if err != nil {
		return
	}
	if newApiErr.StatusCode == http.StatusOK {
		return
	}
	codeStr := strconv.Itoa(newApiErr.StatusCode)
	if value, ok := statusCodeMapping[codeStr]; ok {
		intCode, ok := parseStatusCodeMappingValue(value)
		if !ok {
			return
		}
		newApiErr.StatusCode = intCode
	}
}

func parseStatusCodeMappingValue(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		statusCode, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return statusCode, true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case json.Number:
		statusCode, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return statusCode, true
	default:
		return 0, false
	}
}

func TaskErrorWrapperLocal(err error, code string, statusCode int) *dto.TaskError {
	openaiErr := TaskErrorWrapper(err, code, statusCode)
	openaiErr.LocalError = true
	return openaiErr
}

func TaskErrorWrapper(err error, code string, statusCode int) *dto.TaskError {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
		common.SysLog(fmt.Sprintf("error: %s", text))
		//text = "请求上游地址失败"
		text = common.MaskSensitiveInfo(text)
	}
	//避免暴露内部错误
	taskError := &dto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      err,
	}

	return taskError
}

// TaskErrorFromAPIError 将 PreConsumeBilling 返回的 NewAPIError 转换为 TaskError。
func TaskErrorFromAPIError(apiErr *types.NewAPIError) *dto.TaskError {
	if apiErr == nil {
		return nil
	}
	return &dto.TaskError{
		Code:       string(apiErr.GetErrorCode()),
		Message:    apiErr.Err.Error(),
		StatusCode: apiErr.StatusCode,
		Error:      apiErr.Err,
	}
}
