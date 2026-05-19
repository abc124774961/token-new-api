package proxy

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

type ResponsesViaChatConverter struct{}

func NewResponsesViaChatConverter() *ResponsesViaChatConverter {
	return &ResponsesViaChatConverter{}
}

func (c *ResponsesViaChatConverter) ConvertRequest(req *dto.OpenAIResponsesRequest, upstreamModel string) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("responses request is nil")
	}
	model := upstreamModel
	if model == "" {
		model = req.Model
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	messages, err := responsesInputToMessages(req)
	if err != nil {
		return nil, err
	}
	out := &dto.GeneralOpenAIRequest{
		Model:            model,
		Messages:         messages,
		Stream:           req.Stream,
		StreamOptions:    req.StreamOptions,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		User:             req.User,
		Store:            req.Store,
		Metadata:         req.Metadata,
		PromptCacheKey:   rawMessageString(req.PromptCacheKey),
		ReasoningEffort:  reasoningEffort(req.Reasoning),
		ParallelTooCalls: rawMessageBoolPtr(req.ParallelToolCalls),
	}
	if req.MaxOutputTokens != nil {
		out.MaxCompletionTokens = req.MaxOutputTokens
	}
	if len(req.Tools) > 0 {
		tools, err := responsesToolsToChatTools(req.Tools)
		if err != nil {
			return nil, err
		}
		out.Tools = tools
	}
	if len(req.ToolChoice) > 0 {
		out.ToolChoice = rawMessageAny(req.ToolChoice)
	}
	return out, nil
}

func (c *ResponsesViaChatConverter) ConvertResponse(resp *dto.OpenAITextResponse, requestModel string) (*dto.OpenAIResponsesResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("chat response is nil")
	}
	model := requestModel
	if model == "" {
		model = resp.Model
	}
	createdAt := int(time.Now().Unix())
	switch v := resp.Created.(type) {
	case int:
		createdAt = v
	case int64:
		createdAt = int(v)
	case float64:
		createdAt = int(v)
	}
	output := make([]dto.ResponsesOutput, 0, len(resp.Choices))
	for _, choice := range resp.Choices {
		if len(choice.Message.ParseToolCalls()) > 0 {
			for _, toolCall := range choice.Message.ParseToolCalls() {
				if strings.TrimSpace(toolCall.Function.Name) == "" {
					continue
				}
				arguments, _ := common.Marshal(rawArgumentsObject(toolCall.Function.Arguments))
				callID := firstNonEmpty(toolCall.ID, "call_"+common.GetRandomString(8))
				output = append(output, dto.ResponsesOutput{
					Type:      "function_call",
					ID:        callID,
					Status:    "completed",
					CallId:    callID,
					Name:      toolCall.Function.Name,
					Arguments: arguments,
				})
			}
			continue
		}
		text := choice.Message.StringContent()
		if text == "" {
			text = choice.Message.GetReasoningContent()
		}
		output = append(output, dto.ResponsesOutput{
			Type:   "message",
			ID:     "msg_" + common.GetRandomString(8),
			Status: "completed",
			Role:   "assistant",
			Content: []dto.ResponsesOutputContent{
				{
					Type: "output_text",
					Text: text,
				},
			},
		})
	}
	if len(output) == 0 {
		output = append(output, dto.ResponsesOutput{
			Type:   "message",
			ID:     "msg_" + common.GetRandomString(8),
			Status: "completed",
			Role:   "assistant",
		})
	}
	status, _ := common.Marshal("completed")
	out := &dto.OpenAIResponsesResponse{
		ID:        firstNonEmpty(resp.Id, "resp_"+common.GetRandomString(12)),
		Object:    "response",
		CreatedAt: createdAt,
		Status:    status,
		Model:     model,
		Output:    output,
	}
	if resp.Usage.TotalTokens != 0 || resp.Usage.PromptTokens != 0 || resp.Usage.CompletionTokens != 0 {
		usage := resp.Usage
		out.Usage = &usage
	}
	return out, nil
}

func (c *ResponsesViaChatConverter) ConvertStream(events []string, requestModel string) (*ConvertStreamResult, error) {
	result := &ConvertStreamResult{
		DownstreamEvents: make([]string, 0, len(events)+2),
	}
	stream := c.NewStreamConverter(requestModel)
	for _, raw := range events {
		partial, err := stream.Accept(raw)
		if err != nil {
			return nil, err
		}
		mergeStreamResult(result, partial)
	}
	final, err := stream.Finish()
	if err != nil {
		return nil, err
	}
	mergeStreamResult(result, final)
	return result, nil
}

func (c *ResponsesViaChatConverter) NewStreamConverter(requestModel string) *ResponsesViaChatStreamConverter {
	return &ResponsesViaChatStreamConverter{
		requestModel:  requestModel,
		responseID:    "resp_" + common.GetRandomString(12),
		outputItemID:  "msg_" + common.GetRandomString(8),
		createdAt:     int(time.Now().Unix()),
		model:         requestModel,
		toolItemIDs:   map[int]string{},
		toolCallIDs:   map[int]string{},
		toolNames:     map[int]string{},
		toolArgs:      map[int]string{},
		toolItemAdded: map[int]bool{},
	}
}

type ResponsesViaChatStreamConverter struct {
	providerProfile string
	proxyMode       string
	requestModel    string
	responseID      string
	outputItemID    string
	createdAt       int
	model           string
	sentCreated     bool
	sentOutputItem  bool
	sentFinish      bool
	toolItemIDs     map[int]string
	toolCallIDs     map[int]string
	toolNames       map[int]string
	toolArgs        map[int]string
	toolItemAdded   map[int]bool
	outputText      strings.Builder
	reasoningText   strings.Builder
	usage           *dto.Usage
	failed          bool
}

func (c *ResponsesViaChatStreamConverter) Accept(raw string) (*ConvertStreamResult, error) {
	result := c.newResult()
	event := parseUpstreamChatSSEEvent(raw)
	if event.data == "" && !isUpstreamErrorEvent(event.name) {
		return result, nil
	}
	if event.data == "[DONE]" {
		return result, nil
	}
	if c.failed {
		return result, nil
	}
	if upstreamErr, ok := c.upstreamStreamError(event); ok {
		if err := c.appendFailure(result, upstreamErr); err != nil {
			return nil, err
		}
		c.copyStateToResult(result)
		return result, nil
	}
	var chunk dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal([]byte(event.data), &chunk); err != nil {
		return nil, err
	}
	c.captureChunkMeta(&chunk)
	if err := c.appendCreatedIfNeeded(result); err != nil {
		return nil, err
	}
	for _, choice := range chunk.Choices {
		if err := c.acceptChoice(result, choice); err != nil {
			return nil, err
		}
	}
	c.copyStateToResult(result)
	return result, nil
}

func (c *ResponsesViaChatStreamConverter) Finish() (*ConvertStreamResult, error) {
	result := c.newResult()
	if c.failed {
		c.copyStateToResult(result)
		return result, nil
	}
	if c.sentCreated && !c.sentFinish {
		finishReason := "stop"
		if len(c.toolItemIDs) > 0 && c.outputText.Len() == 0 {
			finishReason = "tool_calls"
		}
		if err := appendFinishEvents(result, &finishReason, c.outputItemID, c.sentOutputItem, c.toolItemIDs, c.toolCallIDs, c.toolNames, c.toolArgs); err != nil {
			return nil, err
		}
		c.sentFinish = true
	}
	completed := dto.ResponsesStreamResponse{
		Type: "response.completed",
		Response: &dto.OpenAIResponsesResponse{
			ID:        c.responseID,
			Object:    "response",
			CreatedAt: c.createdAt,
			Status:    rawStatus("completed"),
			Model:     c.model,
			Usage:     c.usage,
		},
	}
	if err := appendResponsesStreamEvent(result, completed); err != nil {
		return nil, err
	}
	c.copyStateToResult(result)
	return result, nil
}

func (c *ResponsesViaChatStreamConverter) upstreamStreamError(event upstreamChatSSEEvent) (types.OpenAIError, bool) {
	var payload map[string]any
	if event.data != "" {
		if err := common.Unmarshal([]byte(event.data), &payload); err != nil {
			if isUpstreamErrorEvent(event.name) {
				return normalizeUpstreamChatError(event.data, "upstream chat stream error"), true
			}
			return types.OpenAIError{}, false
		}
		c.captureChunkMetaMap(payload)
		if rawErr, ok := payload["error"]; ok && rawErr != nil {
			return normalizeUpstreamChatError(rawErr, "upstream chat stream error"), true
		}
		if looksLikeUpstreamErrorEnvelope(payload) {
			return normalizeUpstreamChatError(payload, "upstream chat stream error"), true
		}
		if isUpstreamErrorEvent(event.name) {
			return normalizeUpstreamChatError(payload, "upstream chat stream error"), true
		}
		return types.OpenAIError{}, false
	}
	if isUpstreamErrorEvent(event.name) {
		return normalizeUpstreamChatError(nil, "upstream chat stream error"), true
	}
	return types.OpenAIError{}, false
}

func (c *ResponsesViaChatStreamConverter) captureChunkMeta(chunk *dto.ChatCompletionsStreamResponse) {
	if chunk == nil {
		return
	}
	if chunk.Id != "" {
		c.responseID = chunk.Id
	}
	if chunk.Created != 0 {
		c.createdAt = int(chunk.Created)
	}
	if chunk.Model != "" {
		c.model = firstNonEmpty(c.requestModel, chunk.Model, c.model)
	}
	if chunk.Usage != nil {
		c.usage = chunk.Usage
	}
}

func (c *ResponsesViaChatStreamConverter) captureChunkMetaMap(payload map[string]any) {
	if len(payload) == 0 {
		return
	}
	if id := stringValue(payload["id"]); id != "" {
		c.responseID = id
	}
	if model := stringValue(payload["model"]); model != "" {
		c.model = firstNonEmpty(c.requestModel, model, c.model)
	}
	switch created := payload["created"].(type) {
	case int:
		if created != 0 {
			c.createdAt = created
		}
	case int64:
		if created != 0 {
			c.createdAt = int(created)
		}
	case float64:
		if created != 0 {
			c.createdAt = int(created)
		}
	}
}

func (c *ResponsesViaChatStreamConverter) appendCreatedIfNeeded(result *ConvertStreamResult) error {
	if c.sentCreated {
		return nil
	}
	created := dto.ResponsesStreamResponse{
		Type: "response.created",
		Response: &dto.OpenAIResponsesResponse{
			ID:        c.responseID,
			Object:    "response",
			CreatedAt: c.createdAt,
			Model:     c.model,
		},
	}
	if err := appendResponsesStreamEvent(result, created); err != nil {
		return err
	}
	c.sentCreated = true
	return nil
}

func (c *ResponsesViaChatStreamConverter) appendFailure(result *ConvertStreamResult, upstreamErr types.OpenAIError) error {
	if c.failed {
		return nil
	}
	event := dto.ResponsesStreamResponse{
		Type: "response.failed",
		Response: &dto.OpenAIResponsesResponse{
			ID:        c.responseID,
			Object:    "response",
			CreatedAt: c.createdAt,
			Status:    rawStatus("failed"),
			Model:     c.model,
			Error:     upstreamErr,
			Usage:     c.usage,
		},
	}
	if err := appendResponsesStreamEvent(result, event); err != nil {
		return err
	}
	c.failed = true
	c.sentFinish = true
	return nil
}

func (c *ResponsesViaChatStreamConverter) acceptChoice(result *ConvertStreamResult, choice dto.ChatCompletionsStreamResponseChoice) error {
	if delta := choice.Delta.GetReasoningContent(); delta != "" {
		c.reasoningText.WriteString(delta)
		event := dto.ResponsesStreamResponse{
			Type:  "response.reasoning_summary_text.delta",
			Delta: delta,
		}
		if err := appendResponsesStreamEvent(result, event); err != nil {
			return err
		}
	}
	if text := choice.Delta.GetContentString(); text != "" {
		if err := c.appendTextDelta(result, text); err != nil {
			return err
		}
	}
	for _, toolCall := range choice.Delta.ToolCalls {
		if err := c.appendToolCallDelta(result, toolCall); err != nil {
			return err
		}
	}
	if choice.FinishReason != nil && *choice.FinishReason != "" && !c.sentFinish {
		if err := appendFinishEvents(result, choice.FinishReason, c.outputItemID, c.sentOutputItem, c.toolItemIDs, c.toolCallIDs, c.toolNames, c.toolArgs); err != nil {
			return err
		}
		c.sentFinish = true
	}
	return nil
}

func (c *ResponsesViaChatStreamConverter) appendTextDelta(result *ConvertStreamResult, text string) error {
	if !c.sentOutputItem {
		item := dto.ResponsesStreamResponse{
			Type:        "response.output_item.added",
			OutputIndex: intPtr(0),
			Item: &dto.ResponsesOutput{
				Type:   "message",
				ID:     c.outputItemID,
				Status: "in_progress",
				Role:   "assistant",
			},
		}
		if err := appendResponsesStreamEvent(result, item); err != nil {
			return err
		}
		c.sentOutputItem = true
	}
	c.outputText.WriteString(text)
	event := dto.ResponsesStreamResponse{
		Type:         "response.output_text.delta",
		OutputIndex:  intPtr(0),
		ContentIndex: intPtr(0),
		ItemID:       c.outputItemID,
		Delta:        text,
	}
	return appendResponsesStreamEvent(result, event)
}

func (c *ResponsesViaChatStreamConverter) appendToolCallDelta(result *ConvertStreamResult, toolCall dto.ToolCallResponse) error {
	index := 0
	if toolCall.Index != nil {
		index = *toolCall.Index
	}
	itemID := c.toolItemIDs[index]
	if itemID == "" {
		itemID = firstNonEmpty(toolCall.ID, "fc_"+common.GetRandomString(8))
		c.toolItemIDs[index] = itemID
	}
	callID := c.toolCallIDs[index]
	if callID == "" {
		callID = firstNonEmpty(toolCall.ID, itemID)
		c.toolCallIDs[index] = callID
	}
	if toolCall.Function.Name != "" {
		c.toolNames[index] = toolCall.Function.Name
	}
	if toolCall.Function.Arguments != "" {
		c.toolArgs[index] += toolCall.Function.Arguments
	}
	if !c.toolItemAdded[index] {
		item := dto.ResponsesStreamResponse{
			Type:        "response.output_item.added",
			OutputIndex: intPtr(index),
			Item: &dto.ResponsesOutput{
				Type:   "function_call",
				ID:     itemID,
				Status: "in_progress",
				CallId: callID,
				Name:   c.toolNames[index],
			},
		}
		if err := appendResponsesStreamEvent(result, item); err != nil {
			return err
		}
		c.toolItemAdded[index] = true
	}
	if toolCall.Function.Arguments == "" {
		return nil
	}
	event := dto.ResponsesStreamResponse{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: intPtr(index),
		ItemID:      itemID,
		Delta:       toolCall.Function.Arguments,
	}
	return appendResponsesStreamEvent(result, event)
}

func (c *ResponsesViaChatStreamConverter) newResult() *ConvertStreamResult {
	return &ConvertStreamResult{
		ProviderProfile:  c.providerProfile,
		ProxyMode:        c.proxyMode,
		DownstreamEvents: make([]string, 0, 4),
	}
}

func (c *ResponsesViaChatStreamConverter) copyStateToResult(result *ConvertStreamResult) {
	if c == nil || result == nil {
		return
	}
	result.OutputText = c.outputText.String()
	result.ReasoningText = c.reasoningText.String()
	result.Usage = c.usage
	for _, name := range c.toolNames {
		if result.ToolName == "" && name != "" {
			result.ToolName = name
		}
	}
	for _, args := range c.toolArgs {
		result.ToolArguments += args
	}
}

func mergeStreamResult(dst *ConvertStreamResult, src *ConvertStreamResult) {
	if dst == nil || src == nil {
		return
	}
	dst.DownstreamEvents = append(dst.DownstreamEvents, src.DownstreamEvents...)
	if src.ProviderProfile != "" {
		dst.ProviderProfile = src.ProviderProfile
	}
	if src.ProxyMode != "" {
		dst.ProxyMode = src.ProxyMode
	}
	if src.OutputText != "" {
		dst.OutputText = src.OutputText
	}
	if src.ReasoningText != "" {
		dst.ReasoningText = src.ReasoningText
	}
	if src.ToolName != "" {
		dst.ToolName = src.ToolName
	}
	if src.ToolArguments != "" {
		dst.ToolArguments = src.ToolArguments
	}
	if src.Usage != nil {
		dst.Usage = src.Usage
	}
}

func responsesInputToMessages(req *dto.OpenAIResponsesRequest) ([]dto.Message, error) {
	messages := make([]dto.Message, 0)
	if len(req.Instructions) > 0 {
		var instructions string
		if err := common.Unmarshal(req.Instructions, &instructions); err == nil && strings.TrimSpace(instructions) != "" {
			messages = append(messages, dto.Message{Role: "system", Content: instructions})
		}
	}
	if len(req.Input) == 0 {
		return messages, nil
	}
	switch common.GetJsonType(req.Input) {
	case "string":
		var text string
		if err := common.Unmarshal(req.Input, &text); err != nil {
			return nil, err
		}
		messages = append(messages, dto.Message{Role: "user", Content: text})
	case "array":
		var items []map[string]any
		if err := common.Unmarshal(req.Input, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			converted, err := responsesInputItemToMessage(item)
			if err != nil {
				return nil, err
			}
			messages = append(messages, converted...)
		}
	default:
		return nil, fmt.Errorf("unsupported responses input type %s", common.GetJsonType(req.Input))
	}
	return messages, nil
}

func responsesInputItemToMessage(item map[string]any) ([]dto.Message, error) {
	itemType := stringValue(item["type"])
	if itemType == "function_call_output" {
		return []dto.Message{{
			Role:       "tool",
			Content:    item["output"],
			ToolCallId: stringValue(item["call_id"]),
		}}, nil
	}
	if itemType == "function_call" {
		arguments, _ := common.Marshal(rawArgumentsObject(stringValue(item["arguments"])))
		toolCall := dto.ToolCallRequest{
			ID:   stringValue(item["call_id"]),
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      stringValue(item["name"]),
				Arguments: string(arguments),
			},
		}
		toolCalls, _ := common.Marshal([]dto.ToolCallRequest{toolCall})
		return []dto.Message{{
			Role:      "assistant",
			Content:   "",
			ToolCalls: toolCalls,
		}}, nil
	}
	role := firstNonEmpty(stringValue(item["role"]), "user")
	content, ok := item["content"]
	if !ok {
		if text := stringValue(item["text"]); text != "" {
			content = text
		}
	}
	switch typed := content.(type) {
	case string:
		return []dto.Message{{Role: role, Content: typed}}, nil
	case []any:
		parts := make([]map[string]any, 0, len(typed))
		for _, raw := range typed {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			parts = append(parts, responsesContentPartToChatPart(part))
		}
		return []dto.Message{{Role: role, Content: parts}}, nil
	default:
		if content == nil {
			return []dto.Message{{Role: role, Content: ""}}, nil
		}
		return []dto.Message{{Role: role, Content: content}}, nil
	}
}

func responsesContentPartToChatPart(part map[string]any) map[string]any {
	switch stringValue(part["type"]) {
	case "input_text", "output_text":
		return map[string]any{
			"type": dto.ContentTypeText,
			"text": stringValue(part["text"]),
		}
	case "input_image":
		return map[string]any{
			"type":      dto.ContentTypeImageURL,
			"image_url": part["image_url"],
		}
	case "input_file":
		return map[string]any{
			"type": "file",
			"file": map[string]any{
				"file_data": firstNonEmpty(stringValue(part["file_url"]), stringValue(part["file_id"])),
			},
		}
	default:
		return part
	}
}

func responsesToolsToChatTools(raw []byte) ([]dto.ToolCallRequest, error) {
	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil, err
	}
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, tool := range tools {
		switch stringValue(tool["type"]) {
		case "function":
			out = append(out, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        stringValue(tool["name"]),
					Description: stringValue(tool["description"]),
					Parameters:  tool["parameters"],
				},
			})
		default:
			body, _ := common.Marshal(tool)
			out = append(out, dto.ToolCallRequest{
				Type:   stringValue(tool["type"]),
				Custom: body,
			})
		}
	}
	return out, nil
}

func reasoningEffort(reasoning *dto.Reasoning) string {
	if reasoning == nil {
		return ""
	}
	return strings.TrimSpace(reasoning.Effort)
}

func rawMessageString(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := common.Unmarshal(raw, &s); err == nil {
		return s
	}
	return common.JsonRawMessageToString(raw)
}

func rawMessageBoolPtr(raw []byte) *bool {
	if len(raw) == 0 {
		return nil
	}
	var b bool
	if err := common.Unmarshal(raw, &b); err != nil {
		return nil
	}
	return &b
}

func rawMessageAny(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := common.Unmarshal(raw, &v); err != nil {
		return common.JsonRawMessageToString(raw)
	}
	return v
}

func appendResponsesStreamEvent(result *ConvertStreamResult, event dto.ResponsesStreamResponse) error {
	body, err := common.Marshal(event)
	if err != nil {
		return err
	}
	result.DownstreamEvents = append(result.DownstreamEvents, string(body))
	return nil
}

func appendFinishEvents(result *ConvertStreamResult, finishReason *string, outputItemID string, sentOutputItem bool, toolItemIDs map[int]string, toolCallIDs map[int]string, toolNames map[int]string, toolArgs map[int]string) error {
	if finishReason == nil {
		return nil
	}
	if sentOutputItem {
		item := dto.ResponsesStreamResponse{
			Type:        dto.ResponsesOutputTypeItemDone,
			OutputIndex: intPtr(0),
			Item: &dto.ResponsesOutput{
				Type:   "message",
				ID:     outputItemID,
				Status: "completed",
				Role:   "assistant",
			},
		}
		if err := appendResponsesStreamEvent(result, item); err != nil {
			return err
		}
	}
	for index, itemID := range toolItemIDs {
		if args := toolArgs[index]; args != "" {
			event := dto.ResponsesStreamResponse{
				Type:        "response.function_call_arguments.done",
				OutputIndex: intPtr(index),
				ItemID:      itemID,
				Delta:       args,
			}
			if err := appendResponsesStreamEvent(result, event); err != nil {
				return err
			}
		}
		body, _ := common.Marshal(rawArgumentsObject(toolArgs[index]))
		item := dto.ResponsesStreamResponse{
			Type:        dto.ResponsesOutputTypeItemDone,
			OutputIndex: intPtr(index),
			Item: &dto.ResponsesOutput{
				Type:      "function_call",
				ID:        itemID,
				Status:    "completed",
				CallId:    firstNonEmpty(toolCallIDs[index], itemID),
				Name:      toolNames[index],
				Arguments: body,
			},
		}
		if err := appendResponsesStreamEvent(result, item); err != nil {
			return err
		}
	}
	return nil
}

func intPtr(value int) *int {
	return &value
}

func rawStatus(status string) []byte {
	body, _ := common.Marshal(status)
	return body
}

type upstreamChatSSEEvent struct {
	name string
	data string
}

func parseUpstreamChatSSEEvent(raw string) upstreamChatSSEEvent {
	raw = strings.TrimSpace(raw)
	event := upstreamChatSSEEvent{data: raw}
	if raw == "" {
		return event
	}
	lines := strings.Split(raw, "\n")
	dataLines := make([]string, 0, len(lines))
	sawSSEField := false
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "event:"):
			event.name = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			sawSSEField = true
		case strings.HasPrefix(trimmed, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			sawSSEField = true
		}
	}
	if sawSSEField {
		event.data = strings.TrimSpace(strings.Join(dataLines, "\n"))
	}
	return event
}

func isUpstreamErrorEvent(eventName string) bool {
	return strings.EqualFold(strings.TrimSpace(eventName), "error")
}

func looksLikeUpstreamErrorEnvelope(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	if _, hasChoices := payload["choices"]; hasChoices {
		return false
	}
	if stringValue(payload["error_msg"]) != "" || stringValue(payload["error_message"]) != "" {
		return true
	}
	if stringValue(payload["message"]) != "" && (stringValue(payload["type"]) != "" || stringValue(payload["code"]) != "") {
		return true
	}
	return false
}

func normalizeUpstreamChatError(raw any, fallbackMessage string) types.OpenAIError {
	openaiErr := dto.GetOpenAIError(raw)
	if openaiErr == nil {
		openaiErr = &types.OpenAIError{}
	}
	if payload, ok := raw.(map[string]any); ok {
		applyUpstreamErrorAliases(openaiErr, payload)
	}
	normalized := *openaiErr
	if strings.TrimSpace(normalized.Message) == "" {
		normalized.Message = upstreamErrorMessage(raw, fallbackMessage)
	}
	if strings.TrimSpace(normalized.Type) == "" {
		normalized.Type = "upstream_error"
	}
	if normalized.Code == nil {
		normalized.Code = "upstream_error"
	}
	return normalized
}

func applyUpstreamErrorAliases(openaiErr *types.OpenAIError, payload map[string]any) {
	if openaiErr == nil || len(payload) == 0 {
		return
	}
	if openaiErr.Message == "" {
		openaiErr.Message = firstNonEmpty(
			stringValue(payload["message"]),
			stringValue(payload["error_msg"]),
			stringValue(payload["error_message"]),
		)
	}
	if openaiErr.Type == "" {
		openaiErr.Type = firstNonEmpty(
			stringValue(payload["type"]),
			stringValue(payload["error_type"]),
		)
	}
	if openaiErr.Code == nil {
		if code := firstNonEmpty(stringValue(payload["code"]), stringValue(payload["error_code"])); code != "" {
			openaiErr.Code = code
		}
	}
}

func upstreamErrorMessage(raw any, fallbackMessage string) string {
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	case map[string]any:
		if message := stringValue(value["message"]); message != "" {
			return message
		}
		if message := stringValue(value["error_msg"]); message != "" {
			return message
		}
		if message := stringValue(value["error_message"]); message != "" {
			return message
		}
	}
	return fallbackMessage
}

func rawArgumentsObject(arguments string) any {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return map[string]any{}
	}
	var v any
	if err := common.Unmarshal([]byte(arguments), &v); err == nil {
		return v
	}
	return arguments
}

func stringValue(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ ResponsesToChatConverter = (*ResponsesViaChatConverter)(nil)
