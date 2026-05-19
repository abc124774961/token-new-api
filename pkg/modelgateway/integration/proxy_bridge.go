package integration

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	modelgatewayproxy "github.com/QuantumNous/new-api/pkg/modelgateway/proxy"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

type ProxyBridge struct {
	engine modelgatewayproxy.ProxyEngine
}

type ProxyBridgeDecision struct {
	Enabled         bool
	Reason          string
	Plan            *core.DispatchPlan
	ProviderProfile string
	ProxyMode       string
}

func NewProxyBridge(engine modelgatewayproxy.ProxyEngine) *ProxyBridge {
	if engine == nil {
		engine = modelgatewayproxy.NewEngine(provider.NewStandardProviderRegistry())
	}
	return &ProxyBridge{engine: engine}
}

func (b *ProxyBridge) Resolve(c *gin.Context, info *relaycommon.RelayInfo) ProxyBridgeDecision {
	plan, ok := GetSelectedPlan(c)
	if !ok || plan == nil {
		return ProxyBridgeDecision{Reason: "no_smart_dispatch_plan"}
	}
	if info == nil {
		return ProxyBridgeDecision{Plan: plan, Reason: "nil_relay_info"}
	}
	if info.RelayMode != relayconstant.RelayModeResponses {
		return ProxyBridgeDecision{Plan: plan, Reason: "unsupported_relay_mode"}
	}
	if plan.ProxyMode == "" || plan.ProxyMode == provider.ProxyModeNative || plan.ProxyMode == provider.ProxyModeNativeResponses {
		return ProxyBridgeDecision{Plan: plan, ProviderProfile: plan.ProviderProfile, ProxyMode: plan.ProxyMode, Reason: "native_or_empty_proxy_mode"}
	}
	if plan.ProviderProfile == "" {
		return ProxyBridgeDecision{Plan: plan, ProxyMode: plan.ProxyMode, Reason: "missing_provider_profile"}
	}
	if plan.ProxyMode != provider.ProxyModeResponsesViaChat {
		return ProxyBridgeDecision{Plan: plan, ProviderProfile: plan.ProviderProfile, ProxyMode: plan.ProxyMode, Reason: "unsupported_proxy_mode"}
	}
	return ProxyBridgeDecision{
		Enabled:         true,
		Reason:          "enabled",
		Plan:            plan,
		ProviderProfile: plan.ProviderProfile,
		ProxyMode:       plan.ProxyMode,
	}
}

func (b *ProxyBridge) ConvertResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) (*modelgatewayproxy.ConvertRequestResult, bool, error) {
	decision := b.Resolve(c, info)
	if !decision.Enabled {
		return nil, false, nil
	}
	if request == nil {
		return nil, true, fmt.Errorf("responses request is nil")
	}
	body, err := common.Marshal(request)
	if err != nil {
		return nil, true, err
	}
	result, err := b.engine.ConvertRequest(modelgatewayproxy.ConvertRequestInput{
		ProviderProfile: decision.ProviderProfile,
		ProxyMode:       decision.ProxyMode,
		RequestModel:    request.Model,
		UpstreamModel:   upstreamModelName(info, request.Model),
		RequestBody:     body,
	})
	return result, true, err
}

func (b *ProxyBridge) ConvertChatResponse(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte) (*modelgatewayproxy.ConvertResponseResult, bool, error) {
	decision := b.Resolve(c, info)
	if !decision.Enabled {
		return nil, false, nil
	}
	result, err := b.engine.ConvertResponse(modelgatewayproxy.ConvertResponseInput{
		ProviderProfile: decision.ProviderProfile,
		ProxyMode:       decision.ProxyMode,
		RequestModel:    requestModelName(info),
		UpstreamModel:   upstreamModelName(info, ""),
		ResponseBody:    responseBody,
	})
	return result, true, err
}

func (b *ProxyBridge) ConvertChatStream(c *gin.Context, info *relaycommon.RelayInfo, events []string) (*modelgatewayproxy.ConvertStreamResult, bool, error) {
	decision := b.Resolve(c, info)
	if !decision.Enabled {
		return nil, false, nil
	}
	result, err := b.engine.ConvertStream(modelgatewayproxy.ConvertStreamInput{
		ProviderProfile: decision.ProviderProfile,
		ProxyMode:       decision.ProxyMode,
		RequestModel:    requestModelName(info),
		UpstreamModel:   upstreamModelName(info, ""),
		UpstreamEvents:  events,
	})
	return result, true, err
}

func (b *ProxyBridge) NewChatStreamConverter(c *gin.Context, info *relaycommon.RelayInfo) (modelgatewayproxy.StreamConverter, bool, error) {
	decision := b.Resolve(c, info)
	if !decision.Enabled {
		return nil, false, nil
	}
	converter, err := b.engine.NewStreamConverter(modelgatewayproxy.ConvertStreamInput{
		ProviderProfile: decision.ProviderProfile,
		ProxyMode:       decision.ProxyMode,
		RequestModel:    requestModelName(info),
		UpstreamModel:   upstreamModelName(info, ""),
	})
	return converter, true, err
}

func requestModelName(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	if info.RequestModelName != "" {
		return info.RequestModelName
	}
	return info.OriginModelName
}

func upstreamModelName(info *relaycommon.RelayInfo, fallback string) string {
	if info == nil {
		return fallback
	}
	if info.ChannelMeta != nil && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	if info.OriginModelName != "" {
		return info.OriginModelName
	}
	return fallback
}
