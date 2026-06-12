package helper

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「系统设置 → 分组与模型定价设置」中为该模型配置价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or configure the model price in System Settings → Group & Model Pricing.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 1.6

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, fmt.Sprintf("final group: %s", autoGroup))
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ApplySelectedGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, selectedGroup string) types.GroupRatioInfo {
	if relayInfo == nil {
		return types.GroupRatioInfo{
			GroupRatio:        1.0,
			GroupSpecialRatio: -1,
		}
	}
	selectedGroup = strings.TrimSpace(selectedGroup)
	if selectedGroup != "" {
		relayInfo.UsingGroup = selectedGroup
		if ctx != nil {
			common.SetContextKey(ctx, constant.ContextKeyUsingGroup, selectedGroup)
			if relayInfo.TokenGroup == "auto" || common.GetContextKeyString(ctx, constant.ContextKeyTokenGroup) == "auto" {
				common.SetContextKey(ctx, constant.ContextKeyAutoGroup, selectedGroup)
			}
		}
	}
	groupRatioInfo := HandleGroupRatio(ctx, relayInfo)
	applyDynamicBillingRatio(ctx, relayInfo, &groupRatioInfo)
	applyBillingMultiplier(ctx, relayInfo, &groupRatioInfo)
	relayInfo.PriceData.GroupRatioInfo = groupRatioInfo
	relayInfo.PriceData.BillingMultiplier = relayInfoBillingMultiplier(relayInfo)
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		snap.GroupRatio = groupRatioInfo.GroupRatio
		snap.EstimatedQuotaAfterGroup = billingexpr.QuotaRound(snap.EstimatedQuotaBeforeGroup * groupRatioInfo.GroupRatio)
		relayInfo.PriceData.QuotaToPreConsume = snap.EstimatedQuotaAfterGroup
		relayInfo.PriceData.QuotaBeforeGroup = snap.EstimatedQuotaBeforeGroup
	} else if relayInfo.PriceData.QuotaBeforeGroup > 0 {
		relayInfo.PriceData.QuotaToPreConsume = billingexpr.QuotaRound(relayInfo.PriceData.QuotaBeforeGroup * groupRatioInfo.GroupRatio)
	}
	return groupRatioInfo
}

func applyCurrentPlanBillingRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, groupRatioInfo *types.GroupRatioInfo) {
	if relayInfo == nil || groupRatioInfo == nil {
		return
	}
	applyDynamicBillingRatio(ctx, relayInfo, groupRatioInfo)
	applyBillingMultiplier(ctx, relayInfo, groupRatioInfo)
}

func relayInfoBillingMultiplier(relayInfo *relaycommon.RelayInfo) *types.BillingMultiplierSnapshot {
	if relayInfo == nil || relayInfo.PriceData.BillingMultiplier == nil {
		return nil
	}
	return relayInfo.PriceData.BillingMultiplier
}

func applyBillingMultiplier(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, groupRatioInfo *types.GroupRatioInfo) {
	if relayInfo == nil || groupRatioInfo == nil {
		return
	}
	baseRatio := groupRatioInfo.GroupRatio
	groupRatioInfo.BaseGroupRatio = baseRatio
	subscriptionPlanID := relayInfo.SubscriptionPlanId
	subscriptionPlanIDs := []int(nil)
	if subscriptionPlanID <= 0 {
		subscriptionPlanIDs = resolveBillingMultiplierSubscriptionPlanIDs(relayInfo.UserId)
		if len(subscriptionPlanIDs) > 0 {
			subscriptionPlanID = subscriptionPlanIDs[0]
		}
	} else {
		subscriptionPlanIDs = []int{subscriptionPlanID}
	}
	snapshot := model.EvaluateBillingMultiplier(model.BillingMultiplierContext{
		UserID:              relayInfo.UserId,
		UserGroup:           relayInfo.UserGroup,
		UsingGroup:          relayInfo.UsingGroup,
		ModelName:           relayInfo.OriginModelName,
		SubscriptionPlanID:  subscriptionPlanID,
		SubscriptionPlanIDs: subscriptionPlanIDs,
		BaseGroupRatio:      baseRatio,
	})
	groupRatioInfo.GroupRatio = snapshot.FinalGroupRatio
	if snapshot.Applied {
		relayInfo.PriceData.BillingMultiplier = &snapshot
	} else {
		relayInfo.PriceData.BillingMultiplier = nil
	}
}

func resolveBillingMultiplierSubscriptionPlanIDs(userID int) []int {
	if userID <= 0 {
		return nil
	}
	subscriptions, err := model.GetAllActiveUserSubscriptions(userID)
	if err != nil || len(subscriptions) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	planIDs := make([]int, 0, len(subscriptions))
	for _, summary := range subscriptions {
		if summary.Subscription != nil && summary.Subscription.PlanId > 0 {
			if _, ok := seen[summary.Subscription.PlanId]; ok {
				continue
			}
			seen[summary.Subscription.PlanId] = struct{}{}
			planIDs = append(planIDs, summary.Subscription.PlanId)
		}
	}
	return planIDs
}

func applyDynamicBillingRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, groupRatioInfo *types.GroupRatioInfo) {
	if relayInfo == nil || groupRatioInfo == nil {
		return
	}
	setting := scheduler_setting.GetSetting()
	mode := resolveDynamicBillingRatioMode(ctx, relayInfo, setting)
	if mode != scheduler_setting.BillingRatioModeDynamic {
		relayInfo.DynamicBilling = nil
		return
	}
	staticRatio := groupRatioInfo.GroupRatio
	snapshot := modelgatewaydynamicbilling.Apply(modelgatewaydynamicbilling.ApplyInput{
		RequestedModel:   relayInfo.OriginModelName,
		Group:            relayInfo.UsingGroup,
		StaticGroupRatio: staticRatio,
		Mode:             mode,
		Setting:          setting,
		Provider:         modelgatewaydynamicbilling.DefaultRatioProvider(),
	})
	relayInfo.DynamicBilling = &snapshot
	if !snapshot.Applied {
		return
	}
	groupRatioInfo.GroupRatio = snapshot.DynamicRatio
	groupRatioInfo.GroupSpecialRatio = -1
	groupRatioInfo.HasSpecialRatio = false
}

func resolveDynamicBillingRatioMode(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, setting scheduler_setting.SchedulerSetting) string {
	actualGroup := ""
	if relayInfo != nil {
		actualGroup = strings.TrimSpace(relayInfo.UsingGroup)
	}
	if groupCoveredByDynamicBillingPolicy(setting, actualGroup) {
		return scheduler_setting.BillingRatioModeDynamic
	}

	plan, hasPlan := modelgatewayintegration.GetSelectedPlan(ctx)
	if hasPlan && plan != nil {
		if requestedGroupHasDynamicBillingPolicy(setting, plan.RequestedGroup, actualGroup) {
			return scheduler_setting.BillingRatioModeDynamic
		}
		if strings.TrimSpace(plan.BillingRatioMode) == scheduler_setting.BillingRatioModeDynamic {
			return scheduler_setting.BillingRatioModeDynamic
		}
	}

	if relayInfo != nil && requestedGroupHasDynamicBillingPolicy(setting, relayInfo.TokenGroup, actualGroup) {
		return scheduler_setting.BillingRatioModeDynamic
	}
	if ctx != nil {
		if requestedGroupHasDynamicBillingPolicy(setting, common.GetContextKeyString(ctx, constant.ContextKeyTokenGroup), actualGroup) {
			return scheduler_setting.BillingRatioModeDynamic
		}
	}
	return scheduler_setting.BillingRatioModeStatic
}

func requestedGroupHasDynamicBillingPolicy(setting scheduler_setting.SchedulerSetting, requestedGroup string, actualGroup string) bool {
	requestedGroup = strings.TrimSpace(requestedGroup)
	if requestedGroup == "" {
		return false
	}
	policy, ok := setting.GroupPolicies[requestedGroup]
	if !ok || strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
		return false
	}
	actualGroup = strings.TrimSpace(actualGroup)
	if actualGroup == "" || actualGroup == requestedGroup || len(policy.CandidateGroups) == 0 {
		return true
	}
	return groupInList(policy.CandidateGroups, actualGroup)
}

func groupCoveredByDynamicBillingPolicy(setting scheduler_setting.SchedulerSetting, group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	for policyGroup, policy := range setting.GroupPolicies {
		if strings.TrimSpace(policy.BillingRatioMode) != scheduler_setting.BillingRatioModeDynamic {
			continue
		}
		if strings.TrimSpace(policyGroup) == group || groupInList(policy.CandidateGroups, group) {
			return true
		}
	}
	return false
}

func groupInList(groups []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, group := range groups {
		if strings.TrimSpace(group) == target {
			return true
		}
	}
	return false
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)

	groupRatioInfo := HandleGroupRatio(c, info)
	applyCurrentPlanBillingRatio(c, info, &groupRatioInfo)

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
	}

	var preConsumedQuota int
	var quotaBeforeGroup float64
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		quotaBeforeGroup = float64(preConsumedTokens) * modelRatio
		ratio := modelRatio * groupRatioInfo.GroupRatio
		preConsumedQuota = int(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		quotaBeforeGroup = modelPrice * common.QuotaPerUnit
		preConsumedQuota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
		QuotaBeforeGroup:     quotaBeforeGroup,
		BillingMultiplier:    relayInfoBillingMultiplier(info),
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次/按量计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)
	applyCurrentPlanBillingRatio(c, info, &groupRatioInfo)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int
	freeModel := false
	var fixedPriceMarginGuard *types.FixedPriceMarginGuardInfo

	if usePrice {
		fixedPriceMarginGuard = applyPerCallFixedPriceMarginGuard(c, info, modelPrice, &groupRatioInfo)
		quota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		quota = int(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:         freeModel,
		ModelPrice:        modelPrice,
		ModelRatio:        modelRatio,
		UsePrice:          usePrice,
		Quota:             quota,
		GroupRatioInfo:    groupRatioInfo,
		BillingMultiplier: relayInfoBillingMultiplier(info),
	}
	priceData.FixedPriceMarginGuard = fixedPriceMarginGuard
	return priceData, nil
}

func applyPerCallFixedPriceMarginGuard(c *gin.Context, info *relaycommon.RelayInfo, modelPrice float64, groupRatioInfo *types.GroupRatioInfo) *types.FixedPriceMarginGuardInfo {
	if info == nil || groupRatioInfo == nil || info.ChannelId <= 0 || modelPrice <= 0 || groupRatioInfo.GroupRatio <= 0 {
		return nil
	}
	upstreamModel := fixedPriceMarginGuardUpstreamModel(info)
	if upstreamModel == "" {
		return nil
	}
	requestContext := context.Background()
	if c != nil && c.Request != nil {
		requestContext = c.Request.Context()
	}
	profile := modelgatewaycost.ResolveDefaultProfile(requestContext, info.ChannelId, upstreamModel)
	guard := modelgatewaycost.EvaluateFixedPriceMarginGuard(profile, upstreamModel, modelPrice, groupRatioInfo.GroupRatio)
	if !guard.Applied {
		return nil
	}
	groupRatioInfo.GroupRatio = guard.ProtectedGroupRatio
	return &types.FixedPriceMarginGuardInfo{
		Applied:             true,
		OriginalGroupRatio:  guard.OriginalGroupRatio,
		ProtectedGroupRatio: guard.ProtectedGroupRatio,
		CostUSD:             guard.CostUSD,
		TargetMargin:        guard.TargetMargin,
		MinRevenueUSD:       guard.MinRevenueUSD,
		ProfileID:           guard.ProfileID,
		ProfileModel:        guard.ProfileModel,
		ProfileSource:       guard.ProfileSource,
		ProfileAccuracy:     guard.ProfileAccuracy,
	}
}

func fixedPriceMarginGuardUpstreamModel(info *relaycommon.RelayInfo) string {
	if info == nil {
		return ""
	}
	candidates := []string{
		info.UpstreamModelName,
		info.ResponseModelName,
		info.OriginModelName,
		info.LogModelName(),
	}
	if info.ChannelMeta != nil {
		candidates = append([]string{info.ChannelMeta.UpstreamModelName}, candidates...)
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return true
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	return ok && strings.TrimSpace(expr) != ""
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	exprStr, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
	}

	estimatedCompletionTokens := 0
	if meta.MaxTokens != 0 {
		estimatedCompletionTokens = meta.MaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", info.OriginModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota := billingexpr.QuotaRound(quotaBeforeGroup * groupRatioInfo.GroupRatio)

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
		QuotaBeforeGroup:  quotaBeforeGroup,
		BillingMultiplier: relayInfoBillingMultiplier(info),
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier))
	}

	info.PriceData = priceData
	return priceData, nil
}
