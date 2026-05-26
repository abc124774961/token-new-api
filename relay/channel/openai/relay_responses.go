package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}
	normalizeResponsesResponseModel(info, &responsesResponse)
	responseBody = normalizeOpenAIJSONBodyModel(info, responseBody)

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
		}
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	var sawCompleted bool
	var deliveredEventCount int
	var streamErr *types.NewAPIError
	var bufferedEvents []bufferedResponsesStreamEvent

	flushBufferedEvents := func() {
		for _, event := range bufferedEvents {
			sendResponsesStreamData(c, event.response, event.data)
			deliveredEventCount++
		}
		bufferedEvents = nil
	}

	sendOrBufferEvent := func(streamResponse dto.ResponsesStreamResponse, data string, force bool) {
		if deliveredEventCount > 0 || force {
			if len(bufferedEvents) > 0 {
				flushBufferedEvents()
			}
			sendResponsesStreamData(c, streamResponse, data)
			deliveredEventCount++
			return
		}
		bufferedEvents = append(bufferedEvents, bufferedResponsesStreamEvent{
			response: streamResponse,
			data:     data,
		})
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}

		switch streamResponse.Type {
		case "response.created", "response.in_progress":
			normalizeResponsesStreamResponseModel(info, &streamResponse)
			data = string(normalizeResponsesStreamJSONBodyModel(info, common.StringToByteSlice(data)))
			sendOrBufferEvent(streamResponse, data, false)
		case "response.completed":
			sawCompleted = true
			normalizeResponsesStreamResponseModel(info, &streamResponse)
			data = string(normalizeResponsesStreamJSONBodyModel(info, common.StringToByteSlice(data)))
			sendOrBufferEvent(streamResponse, data, true)
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
					}
				}
				if streamResponse.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
					c.Set("image_generation_call_size", streamResponse.Response.GetSize())
				}
			}
		case "response.output_text.delta":
			sendOrBufferEvent(streamResponse, data, true)
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			sendOrBufferEvent(streamResponse, data, true)
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
						if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
							webSearchTool.CallCount++
						}
					}
				}
			}
		case "response.error", "response.failed":
			streamErr = newAPIErrorFromResponsesStreamFailure(streamResponse)
			if deliveredEventCount > 0 {
				common.SetContextKey(c, appconstant.ContextKeyRelayStreamInterrupted, true)
				sendResponsesStreamData(c, streamResponse, data)
				deliveredEventCount++
			}
			sr.Stop(streamErr)
			return
		default:
			sendOrBufferEvent(streamResponse, data, true)
		}
	})
	if err := helper.InternalRelayAttemptError(c); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusGatewayTimeout)
	}

	if streamErr != nil {
		return nil, streamErr
	}

	if !sawCompleted && info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF {
		if deliveredEventCount == 0 {
			return nil, types.NewOpenAIError(fmt.Errorf("responses stream ended before any usable event was delivered"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		logger.LogWarn(c, "responses stream ended before response.completed; using delivered stream events for usage fallback")
	}

	if usage.CompletionTokens == 0 {
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	markRelayEmptyOutput(c, info, usage, responseTextBuilder.String())

	return usage, nil
}

type bufferedResponsesStreamEvent struct {
	response dto.ResponsesStreamResponse
	data     string
}

func newAPIErrorFromResponsesStreamFailure(streamResponse dto.ResponsesStreamResponse) *types.NewAPIError {
	if streamResponse.Response != nil {
		if oaiErr := streamResponse.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
			return types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
		}
	}
	return types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResponse.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
}
