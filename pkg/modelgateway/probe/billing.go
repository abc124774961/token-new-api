package probe

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type ProbeBillingRecorder struct{}

func NewProbeBillingRecorder() *ProbeBillingRecorder {
	return &ProbeBillingRecorder{}
}

func (r *ProbeBillingRecorder) RecordSuccess(c *gin.Context, result ProbeRunResult, priceData types.PriceData, usage *dto.Usage) (int, error) {
	if r == nil {
		return 0, nil
	}
	if !result.Success || result.Channel == nil || result.RelayInfo == nil {
		return 0, nil
	}
	usage = normalizeProbeUsage(usage, 1)
	quota, tieredResult := settleProbeQuota(result.RelayInfo, priceData, usage)
	if quota < 0 {
		quota = 0
	}
	rootID, err := rootUserID()
	if err != nil {
		return quota, err
	}
	if quota > 0 {
		if err := model.DecreaseUserQuota(rootID, quota, false); err != nil {
			return quota, err
		}
		model.UpdateUserUsedQuotaAndRequestCount(rootID, quota)
		model.UpdateChannelUsedQuota(result.Channel.Id, quota)
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(rootID, 0)
	}
	other := r.logOther(c, result, priceData, usage, tieredResult)
	model.RecordConsumeLog(c, rootID, model.RecordConsumeLogParams{
		ChannelId:        result.Channel.Id,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        result.RelayInfo.LogModelName(),
		TokenName:        TokenName,
		Quota:            quota,
		Content:          ConsumeLogContent,
		TokenId:          0,
		UseTimeSeconds:   int(result.Duration / time.Second),
		IsStream:         false,
		Group:            result.RelayInfo.UsingGroup,
		Other:            other,
	})
	return quota, nil
}

func (r *ProbeBillingRecorder) logOther(c *gin.Context, result ProbeRunResult, priceData types.PriceData, usage *dto.Usage, tieredResult *billingexpr.TieredResult) map[string]interface{} {
	info := result.RelayInfo
	groupRatio := priceData.GroupRatioInfo.GroupRatio
	other := service.GenerateTextOtherInfo(c, info, priceData.ModelRatio, groupRatio, priceData.CompletionRatio,
		usage.PromptTokensDetails.CachedTokens, priceData.CacheRatio, priceData.ModelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		service.InjectTieredBillingInfo(other, info, tieredResult)
	}
	other["is_health_probe"] = true
	other["billing_source"] = BillingSource
	other["probe_id"] = result.ProbeID
	other["probe_reason"] = result.Reason
	other["runtime_key"] = runtimeKeyLogValue(result.RuntimeKey)
	other["channel_id"] = result.Channel.Id
	other["upstream_model"] = upstreamModelName(result.RelayInfo, result.Model)
	other["latency_ms"] = result.Duration.Milliseconds()
	other["ttft_ms"] = result.TTFT.Milliseconds()
	other["usage_semantic"] = usageSemantic(info, usage)
	other["source"] = "health_probe"
	return other
}

func rootUserID() (int, error) {
	root := model.GetRootUser()
	if root == nil || root.Id <= 0 {
		return 0, fmt.Errorf("root user not found")
	}
	return root.Id, nil
}

func runtimeKeyLogValue(key any) map[string]interface{} {
	bytes, err := common.Marshal(key)
	if err != nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := common.Unmarshal(bytes, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func upstreamModelName(info *relaycommon.RelayInfo, fallback string) string {
	if info != nil && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	return fallback
}

func usageSemantic(info *relaycommon.RelayInfo, usage *dto.Usage) string {
	if usage != nil && usage.UsageSemantic != "" {
		return usage.UsageSemantic
	}
	if info != nil && info.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		return "anthropic"
	}
	return "openai"
}
