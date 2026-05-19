package proxy

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
)

const (
	pathResponses      = "/v1/responses"
	pathChatCompletion = "/v1/chat/completions"
)

func NewEngine(registry provider.ProviderRegistry) *Engine {
	if registry == nil {
		registry = provider.NewStandardProviderRegistry()
	}
	return &Engine{registry: registry}
}

func (e *Engine) ConvertRequest(input ConvertRequestInput) (*ConvertRequestResult, error) {
	profile, err := e.profile(input.ProviderProfile)
	if err != nil {
		return nil, err
	}
	proxyMode := input.ProxyMode
	if proxyMode == "" {
		proxyMode = profile.ProxyMode(nil, input.RequestModel)
	}
	switch proxyMode {
	case provider.ProxyModeNativeResponses:
		var req dto.OpenAIResponsesRequest
		if err := common.Unmarshal(input.RequestBody, &req); err != nil {
			return nil, err
		}
		req.Model = upstreamOrRequestModel(input.UpstreamModel, req.Model, input.RequestModel)
		body, err := common.Marshal(req)
		if err != nil {
			return nil, err
		}
		return &ConvertRequestResult{
			ProviderProfile: profile.Name(),
			ProxyMode:       proxyMode,
			UpstreamPath:    pathResponses,
			UpstreamBody:    body,
			UpstreamRequest: &req,
		}, nil
	case provider.ProxyModeResponsesViaChat:
		var req dto.OpenAIResponsesRequest
		if err := common.Unmarshal(input.RequestBody, &req); err != nil {
			return nil, err
		}
		chatReq, err := NewResponsesViaChatConverter().ConvertRequest(&req, upstreamOrRequestModel(input.UpstreamModel, req.Model, input.RequestModel))
		if err != nil {
			return nil, err
		}
		body, err := common.Marshal(chatReq)
		if err != nil {
			return nil, err
		}
		return &ConvertRequestResult{
			ProviderProfile: profile.Name(),
			ProxyMode:       proxyMode,
			UpstreamPath:    pathChatCompletion,
			UpstreamBody:    body,
			UpstreamRequest: chatReq,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy mode %q", proxyMode)
	}
}

func (e *Engine) ConvertResponse(input ConvertResponseInput) (*ConvertResponseResult, error) {
	profile, err := e.profile(input.ProviderProfile)
	if err != nil {
		return nil, err
	}
	proxyMode := input.ProxyMode
	if proxyMode == "" {
		proxyMode = profile.ProxyMode(nil, input.RequestModel)
	}
	switch proxyMode {
	case provider.ProxyModeNativeResponses:
		var resp dto.OpenAIResponsesResponse
		if err := common.Unmarshal(input.ResponseBody, &resp); err != nil {
			return nil, err
		}
		body, err := common.Marshal(resp)
		if err != nil {
			return nil, err
		}
		return &ConvertResponseResult{
			ProviderProfile:  profile.Name(),
			ProxyMode:        proxyMode,
			DownstreamBody:   body,
			DownstreamResult: &resp,
		}, nil
	case provider.ProxyModeResponsesViaChat:
		var resp dto.OpenAITextResponse
		if err := common.Unmarshal(input.ResponseBody, &resp); err != nil {
			return nil, err
		}
		responsesResp, err := NewResponsesViaChatConverter().ConvertResponse(&resp, upstreamOrRequestModel(input.RequestModel, resp.Model, input.UpstreamModel))
		if err != nil {
			return nil, err
		}
		body, err := common.Marshal(responsesResp)
		if err != nil {
			return nil, err
		}
		return &ConvertResponseResult{
			ProviderProfile:  profile.Name(),
			ProxyMode:        proxyMode,
			DownstreamBody:   body,
			DownstreamResult: responsesResp,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy mode %q", proxyMode)
	}
}

func (e *Engine) ConvertStream(input ConvertStreamInput) (*ConvertStreamResult, error) {
	profile, err := e.profile(input.ProviderProfile)
	if err != nil {
		return nil, err
	}
	proxyMode := input.ProxyMode
	if proxyMode == "" {
		proxyMode = profile.ProxyMode(nil, input.RequestModel)
	}
	switch proxyMode {
	case provider.ProxyModeNativeResponses:
		return &ConvertStreamResult{
			ProviderProfile:  profile.Name(),
			ProxyMode:        proxyMode,
			DownstreamEvents: append([]string(nil), input.UpstreamEvents...),
		}, nil
	case provider.ProxyModeResponsesViaChat:
		result, err := NewResponsesViaChatConverter().ConvertStream(input.UpstreamEvents, upstreamOrRequestModel(input.RequestModel, input.UpstreamModel))
		if err != nil {
			return nil, err
		}
		result.ProviderProfile = profile.Name()
		result.ProxyMode = proxyMode
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported proxy mode %q", proxyMode)
	}
}

func (e *Engine) NewStreamConverter(input ConvertStreamInput) (StreamConverter, error) {
	profile, err := e.profile(input.ProviderProfile)
	if err != nil {
		return nil, err
	}
	proxyMode := input.ProxyMode
	if proxyMode == "" {
		proxyMode = profile.ProxyMode(nil, input.RequestModel)
	}
	switch proxyMode {
	case provider.ProxyModeNativeResponses:
		return newNativeResponsesStreamConverter(profile.Name(), proxyMode), nil
	case provider.ProxyModeResponsesViaChat:
		converter := NewResponsesViaChatConverter().NewStreamConverter(upstreamOrRequestModel(input.RequestModel, input.UpstreamModel))
		converter.providerProfile = profile.Name()
		converter.proxyMode = proxyMode
		return converter, nil
	default:
		return nil, fmt.Errorf("unsupported proxy mode %q", proxyMode)
	}
}

func (e *Engine) profile(name string) (provider.ProviderProfile, error) {
	if e == nil || e.registry == nil {
		return nil, fmt.Errorf("proxy engine registry is nil")
	}
	profile, ok := e.registry.Get(name)
	if !ok {
		return nil, fmt.Errorf("provider profile %q is not registered", name)
	}
	return profile, nil
}

func upstreamOrRequestModel(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ ProxyEngine = (*Engine)(nil)

type nativeResponsesStreamConverter struct {
	providerProfile string
	proxyMode       string
}

func newNativeResponsesStreamConverter(providerProfile string, proxyMode string) *nativeResponsesStreamConverter {
	return &nativeResponsesStreamConverter{
		providerProfile: providerProfile,
		proxyMode:       proxyMode,
	}
}

func (c *nativeResponsesStreamConverter) Accept(upstreamEvent string) (*ConvertStreamResult, error) {
	return &ConvertStreamResult{
		ProviderProfile:  c.providerProfile,
		ProxyMode:        c.proxyMode,
		DownstreamEvents: []string{upstreamEvent},
	}, nil
}

func (c *nativeResponsesStreamConverter) Finish() (*ConvertStreamResult, error) {
	return &ConvertStreamResult{
		ProviderProfile: c.providerProfile,
		ProxyMode:       c.proxyMode,
	}, nil
}

var _ StreamConverter = (*nativeResponsesStreamConverter)(nil)
