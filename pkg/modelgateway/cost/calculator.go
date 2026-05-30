package cost

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"
)

const (
	SourceManual      = "manual"
	SourceAutoSynced  = "auto_synced"
	SourceSystemRatio = "system_ratio"
	SourceMissing     = "missing"
	SourcePending     = "pending"

	AccuracyPrecise = "precise"
	AccuracyMissing = "missing"
	AccuracyPending = "pending"

	claudeCacheCreation1hMultiplier = 6 / 3.75
)

type UsageNormalizer interface {
	Normalize(log model.Log) UsageSnapshot
}

type DefaultUsageNormalizer struct{}

type UsageSnapshot struct {
	RequestID            string
	ChannelID            int
	UpstreamModel        string
	PromptTokens         int
	CompletionTokens     int
	CacheReadTokens      int
	CacheWriteTokens     int
	CacheWriteTokens5m   int
	CacheWriteTokens1h   int
	ImageInputTokens     int
	ImageOutputTokens    int
	AudioInputTokens     int
	AudioOutputTokens    int
	UsageSemantic        string
	WebSearchCallCount   int
	FileSearchCallCount  int
	ImageGenerationCalls int
	ToolCallCount        int
	AdditionalToolCounts map[string]int
	UnrecognizedUsage    map[string]interface{}
}

type BreakdownComponent struct {
	Tokens          int     `json:"tokens,omitempty"`
	Count           int     `json:"count,omitempty"`
	PricePerMillion float64 `json:"price_per_million,omitempty"`
	UnitPrice       float64 `json:"unit_price,omitempty"`
	Amount          float64 `json:"amount"`
}

type Breakdown struct {
	Currency             string                        `json:"currency"`
	UsageSemantic        string                        `json:"usage_semantic,omitempty"`
	CostCoefficient      float64                       `json:"cost_coefficient,omitempty"`
	FeeMultiplier        float64                       `json:"fee_multiplier,omitempty"`
	TokenMultiplier      float64                       `json:"token_multiplier,omitempty"`
	InputMultiplier      float64                       `json:"input_multiplier,omitempty"`
	OutputMultiplier     float64                       `json:"output_multiplier,omitempty"`
	CacheReadMultiplier  float64                       `json:"cache_read_multiplier,omitempty"`
	CacheWriteMultiplier float64                       `json:"cache_write_multiplier,omitempty"`
	RechargeMultiplier   float64                       `json:"recharge_multiplier,omitempty"`
	Input                BreakdownComponent            `json:"input,omitempty"`
	Output               BreakdownComponent            `json:"output,omitempty"`
	CacheRead            BreakdownComponent            `json:"cache_read,omitempty"`
	CacheWrite           BreakdownComponent            `json:"cache_write,omitempty"`
	CacheWrite5m         BreakdownComponent            `json:"cache_write_5m,omitempty"`
	CacheWrite1h         BreakdownComponent            `json:"cache_write_1h,omitempty"`
	ImageInput           BreakdownComponent            `json:"image_input,omitempty"`
	ImageOutput          BreakdownComponent            `json:"image_output,omitempty"`
	AudioInput           BreakdownComponent            `json:"audio_input,omitempty"`
	AudioOutput          BreakdownComponent            `json:"audio_output,omitempty"`
	Request              BreakdownComponent            `json:"request,omitempty"`
	Tools                map[string]BreakdownComponent `json:"tools,omitempty"`
	Unrecognized         map[string]interface{}        `json:"unrecognized_usage,omitempty"`
	BaseInputTokens      int                           `json:"base_input_tokens,omitempty"`
	BaseOutputTokens     int                           `json:"base_output_tokens,omitempty"`
}

type Result struct {
	RequestID     string
	ChannelID     int
	UpstreamModel string
	Total         float64
	Breakdown     Breakdown
	Source        string
	Accuracy      string
}

func MultiplierFromBreakdownJSON(raw string) (float64, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	var breakdown Breakdown
	if err := common.UnmarshalJsonStr(raw, &breakdown); err != nil {
		return 0, false
	}
	return MultiplierFromBreakdown(breakdown)
}

func MultiplierFromBreakdown(breakdown Breakdown) (float64, bool) {
	candidates := []float64{
		breakdown.TokenMultiplier,
		breakdown.InputMultiplier,
		breakdown.OutputMultiplier,
		breakdown.CacheReadMultiplier,
		breakdown.CacheWriteMultiplier,
	}
	for _, value := range candidates {
		if value > 0 && value <= 100 {
			return value, true
		}
	}
	return 0, false
}

type SystemRatioQuote struct {
	Model                      string  `json:"model"`
	PricingModel               string  `json:"pricing_model,omitempty"`
	PriceSource                string  `json:"price_source"`
	Currency                   string  `json:"currency"`
	PricingMode                string  `json:"pricing_mode"`
	CostCoefficient            float64 `json:"cost_coefficient"`
	FeeMultiplier              float64 `json:"fee_multiplier"`
	BaseCostMultiplier         float64 `json:"base_cost_multiplier"`
	TokenMultiplier            float64 `json:"token_multiplier"`
	ActualTokenMultiplier      float64 `json:"actual_token_multiplier"`
	BaseInputPerMillion        float64 `json:"base_input_per_million"`
	BaseOutputPerMillion       float64 `json:"base_output_per_million"`
	BaseCacheReadPerMillion    float64 `json:"base_cache_read_per_million"`
	BaseCacheWritePerMillion   float64 `json:"base_cache_write_per_million"`
	BaseCacheWrite1hPerMillion float64 `json:"base_cache_write_1h_per_million"`
	BaseImageInputPerMillion   float64 `json:"base_image_input_per_million"`
	BaseAudioInputPerMillion   float64 `json:"base_audio_input_per_million"`
	BaseAudioOutputPerMillion  float64 `json:"base_audio_output_per_million"`
	InputCostMultiplier        float64 `json:"input_cost_multiplier"`
	OutputCostMultiplier       float64 `json:"output_cost_multiplier"`
	CacheReadMultiplier        float64 `json:"cache_read_multiplier"`
	CacheWriteMultiplier       float64 `json:"cache_write_multiplier"`
	RechargeMultiplier         float64 `json:"recharge_multiplier"`
	InputPerMillion            float64 `json:"input_per_million"`
	OutputPerMillion           float64 `json:"output_per_million"`
	CacheReadPerMillion        float64 `json:"cache_read_per_million"`
	CacheWritePerMillion       float64 `json:"cache_write_per_million"`
	CacheWrite1hPerMillion     float64 `json:"cache_write_1h_per_million"`
	ImageInputPerMillion       float64 `json:"image_input_per_million"`
	AudioInputPerMillion       float64 `json:"audio_input_per_million"`
	AudioOutputPerMillion      float64 `json:"audio_output_per_million"`
	RequestPrice               float64 `json:"request_price"`
	Accuracy                   string  `json:"accuracy"`
}

func Calculate(usage UsageSnapshot, profile *model.ModelGatewayChannelCostProfile) Result {
	result := Result{
		RequestID:     strings.TrimSpace(usage.RequestID),
		ChannelID:     usage.ChannelID,
		UpstreamModel: strings.TrimSpace(usage.UpstreamModel),
		Breakdown: Breakdown{
			Currency:      "USD",
			UsageSemantic: strings.TrimSpace(usage.UsageSemantic),
		},
		Source:   SourceMissing,
		Accuracy: AccuracyMissing,
	}
	result.Breakdown.Unrecognized = cleanUnrecognizedUsage(usage.UnrecognizedUsage)
	if profile == nil {
		return result
	}
	derivedProfile := DeriveSystemRatioProfile(usage.UpstreamModel, *profile)
	profile = &derivedProfile

	result.Source = normalizeSource(profile.Source)
	result.Accuracy = normalizeAccuracy(profile.Accuracy)
	if strings.TrimSpace(profile.Currency) != "" {
		result.Breakdown.Currency = strings.TrimSpace(profile.Currency)
	}
	costCoefficient := normalizedCostCoefficient(*profile)
	feeMultiplier := normalizedTokenMultiplier(*profile)
	tokenMultiplier := normalizedActualTokenMultiplier(*profile)
	if !strings.EqualFold(strings.TrimSpace(profile.PricingMode), "request") {
		result.Breakdown.CostCoefficient = costCoefficient
		result.Breakdown.FeeMultiplier = feeMultiplier
		result.Breakdown.TokenMultiplier = tokenMultiplier
		result.Breakdown.InputMultiplier = tokenMultiplier
		result.Breakdown.OutputMultiplier = tokenMultiplier
		result.Breakdown.CacheReadMultiplier = tokenMultiplier
		result.Breakdown.CacheWriteMultiplier = tokenMultiplier
	}
	result.Breakdown.RechargeMultiplier = normalizedPositive(profile.RechargeMultiplier)

	isAnthropic := strings.EqualFold(strings.TrimSpace(usage.UsageSemantic), "anthropic")
	cacheWrite5m := nonNegative(usage.CacheWriteTokens5m)
	cacheWrite1h := nonNegative(usage.CacheWriteTokens1h)
	cacheWriteTotal := nonNegative(usage.CacheWriteTokens)
	if cacheWriteTotal == 0 {
		cacheWriteTotal = cacheWrite5m + cacheWrite1h
	}
	chargedSplitCacheWrite := 0
	if profile.CacheWrite5mPerMillion > 0 {
		chargedSplitCacheWrite += cacheWrite5m
	}
	if profile.CacheWrite1hPerMillion > 0 {
		chargedSplitCacheWrite += cacheWrite1h
	}
	cacheWriteGeneric := 0
	if profile.CacheWritePerMillion > 0 {
		cacheWriteGeneric = nonNegative(cacheWriteTotal - chargedSplitCacheWrite)
	}

	baseInput := nonNegative(usage.PromptTokens)
	if !isAnthropic {
		if profile.CacheReadPerMillion > 0 {
			baseInput -= nonNegative(usage.CacheReadTokens)
		}
		if profile.CacheWritePerMillion > 0 {
			baseInput -= cacheWriteGeneric
		}
		if profile.CacheWrite5mPerMillion > 0 {
			baseInput -= cacheWrite5m
		}
		if profile.CacheWrite1hPerMillion > 0 {
			baseInput -= cacheWrite1h
		}
		if profile.ImageInputPerMillion > 0 {
			baseInput -= nonNegative(usage.ImageInputTokens)
		}
		if profile.AudioInputPerMillion > 0 {
			baseInput -= nonNegative(usage.AudioInputTokens)
		}
	}
	baseInput = nonNegative(baseInput)

	baseOutput := nonNegative(usage.CompletionTokens)
	if !isAnthropic {
		if profile.ImageOutputPerMillion > 0 {
			baseOutput -= nonNegative(usage.ImageOutputTokens)
		}
		if profile.AudioOutputPerMillion > 0 {
			baseOutput -= nonNegative(usage.AudioOutputTokens)
		}
	}
	baseOutput = nonNegative(baseOutput)

	result.Breakdown.BaseInputTokens = baseInput
	result.Breakdown.BaseOutputTokens = baseOutput
	result.Breakdown.Input = tokenComponent(baseInput, profile.InputPerMillion)
	result.Breakdown.Output = tokenComponent(baseOutput, profile.OutputPerMillion)
	result.Breakdown.CacheRead = tokenComponent(usage.CacheReadTokens, profile.CacheReadPerMillion)
	result.Breakdown.CacheWrite = tokenComponent(cacheWriteGeneric, profile.CacheWritePerMillion)
	result.Breakdown.CacheWrite5m = tokenComponent(cacheWrite5m, profile.CacheWrite5mPerMillion)
	result.Breakdown.CacheWrite1h = tokenComponent(cacheWrite1h, profile.CacheWrite1hPerMillion)
	result.Breakdown.ImageInput = tokenComponent(usage.ImageInputTokens, profile.ImageInputPerMillion)
	result.Breakdown.ImageOutput = tokenComponent(usage.ImageOutputTokens, profile.ImageOutputPerMillion)
	result.Breakdown.AudioInput = tokenComponent(usage.AudioInputTokens, profile.AudioInputPerMillion)
	result.Breakdown.AudioOutput = tokenComponent(usage.AudioOutputTokens, profile.AudioOutputPerMillion)
	result.Breakdown.Request = countComponent(1, profile.RequestPrice)
	result.Breakdown.Tools = toolComponents(usage, profile.ToolPricesJSON)
	result.Total = breakdownTotal(result.Breakdown)
	return result
}

func DeriveSystemRatioProfile(modelName string, profile model.ModelGatewayChannelCostProfile) model.ModelGatewayChannelCostProfile {
	if strings.TrimSpace(profile.Source) != SourceSystemRatio {
		return profile
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == "*" {
		modelName = strings.TrimSpace(profile.UpstreamModel)
	}
	costCoefficient := normalizedCostCoefficient(profile)
	feeMultiplier := normalizedTokenMultiplier(profile)
	rechargeMultiplier := normalizedPositive(profile.RechargeMultiplier)
	configuredRequestPrice := profile.RequestPrice
	resetSystemRatioDerivedPrices(&profile)
	profile.CostCoefficient = costCoefficient
	profile.TokenMultiplier = feeMultiplier
	profile.InputCostMultiplier = feeMultiplier
	profile.OutputCostMultiplier = feeMultiplier
	profile.CacheReadMultiplier = feeMultiplier
	profile.CacheWriteMultiplier = feeMultiplier
	profile.RequestCostMultiplier = 1
	if systemRequestPrice, ok := ratio_setting.GetModelPrice(modelName, false); ok && systemRequestPrice >= 0 {
		requestPrice := configuredRequestPrice
		if requestPrice <= 0 {
			requestPrice = systemRequestPrice
		}
		profile.RequestPrice = roundCostPrice(costMultiplier(requestPrice, rechargeMultiplier))
		if strings.TrimSpace(profile.Currency) == "" {
			profile.Currency = "USD"
		}
		profile.PricingMode = "request"
		profile.Accuracy = "estimated"
		return profile
	}
	unitPrice := inferredSystemInputPricePerMillion(modelName)
	if unitPrice <= 0 {
		profile.Accuracy = AccuracyMissing
		return profile
	}
	actualMultiplier := costMultiplier(costCoefficient*feeMultiplier, rechargeMultiplier)
	profile.InputPerMillion = roundCostPrice(unitPrice * actualMultiplier)
	profile.OutputPerMillion = roundCostPrice(unitPrice * systemCompletionRatio(modelName) * actualMultiplier)
	profile.CacheReadPerMillion = roundCostPrice(unitPrice * systemCacheReadRatio(modelName) * actualMultiplier)
	cacheWrite := roundCostPrice(unitPrice * systemCacheWriteRatio(modelName) * actualMultiplier)
	profile.CacheWritePerMillion = cacheWrite
	profile.CacheWrite5mPerMillion = cacheWrite
	profile.CacheWrite1hPerMillion = roundCostPrice(cacheWrite * claudeCacheCreation1hMultiplier)
	profile.ImageInputPerMillion = roundCostPrice(unitPrice * systemImageRatio(modelName) * actualMultiplier)
	profile.AudioInputPerMillion = roundCostPrice(unitPrice * systemAudioInputRatio(modelName) * actualMultiplier)
	profile.AudioOutputPerMillion = roundCostPrice(unitPrice * systemAudioInputRatio(modelName) * systemAudioOutputRatio(modelName) * actualMultiplier)
	if strings.TrimSpace(profile.Currency) == "" {
		profile.Currency = "USD"
	}
	if strings.TrimSpace(profile.PricingMode) == "" {
		profile.PricingMode = "token"
	}
	profile.Accuracy = "estimated"
	return profile
}

func QuoteSystemRatioProfile(modelName string, profile model.ModelGatewayChannelCostProfile) SystemRatioQuote {
	derived := DeriveSystemRatioProfile(modelName, profile)
	costCoefficient := normalizedCostCoefficient(derived)
	feeMultiplier := normalizedTokenMultiplier(derived)
	rechargeMultiplier := normalizedPositive(derived.RechargeMultiplier)
	baseCostMultiplier := costCoefficient
	actualMultiplier := costMultiplier(costCoefficient*feeMultiplier, rechargeMultiplier)
	unitPrice := inferredSystemInputPricePerMillion(modelName)
	baseInputPerMillion := roundCostPrice(unitPrice * baseCostMultiplier)
	baseOutputPerMillion := roundCostPrice(unitPrice * systemCompletionRatio(modelName) * baseCostMultiplier)
	baseCacheReadPerMillion := roundCostPrice(unitPrice * systemCacheReadRatio(modelName) * baseCostMultiplier)
	baseCacheWrite := roundCostPrice(unitPrice * systemCacheWriteRatio(modelName) * baseCostMultiplier)
	baseImageInputPerMillion := roundCostPrice(unitPrice * systemImageRatio(modelName) * baseCostMultiplier)
	baseAudioInputPerMillion := roundCostPrice(unitPrice * systemAudioInputRatio(modelName) * baseCostMultiplier)
	baseAudioOutputPerMillion := roundCostPrice(unitPrice * systemAudioInputRatio(modelName) * systemAudioOutputRatio(modelName) * baseCostMultiplier)
	requestBase := profile.RequestPrice
	actualRequestPrice := derived.RequestPrice
	if normalizeSource(profile.Source) == SourceSystemRatio {
		systemRequestPrice, ok := ratio_setting.GetModelPrice(modelName, false)
		if ok && systemRequestPrice >= 0 && requestBase <= 0 {
			requestBase = systemRequestPrice
		}
		actualRequestPrice = roundCostPrice(costMultiplier(requestBase, rechargeMultiplier))
	}
	if actualRequestPrice <= 0 {
		actualRequestPrice = roundCostPrice(requestBase)
	}
	return SystemRatioQuote{
		Model:                      strings.TrimSpace(modelName),
		PricingModel:               strings.TrimSpace(modelName),
		PriceSource:                inferSystemPriceSource(modelName),
		Currency:                   firstString(strings.TrimSpace(derived.Currency), "USD"),
		PricingMode:                firstString(strings.TrimSpace(derived.PricingMode), "token"),
		CostCoefficient:            costCoefficient,
		FeeMultiplier:              feeMultiplier,
		BaseCostMultiplier:         baseCostMultiplier,
		TokenMultiplier:            feeMultiplier,
		ActualTokenMultiplier:      actualMultiplier,
		BaseInputPerMillion:        baseInputPerMillion,
		BaseOutputPerMillion:       baseOutputPerMillion,
		BaseCacheReadPerMillion:    baseCacheReadPerMillion,
		BaseCacheWritePerMillion:   baseCacheWrite,
		BaseCacheWrite1hPerMillion: roundCostPrice(baseCacheWrite * claudeCacheCreation1hMultiplier),
		BaseImageInputPerMillion:   baseImageInputPerMillion,
		BaseAudioInputPerMillion:   baseAudioInputPerMillion,
		BaseAudioOutputPerMillion:  baseAudioOutputPerMillion,
		InputCostMultiplier:        normalizedPositive(derived.InputCostMultiplier),
		OutputCostMultiplier:       normalizedPositive(derived.OutputCostMultiplier),
		CacheReadMultiplier:        normalizedPositive(derived.CacheReadMultiplier),
		CacheWriteMultiplier:       normalizedPositive(derived.CacheWriteMultiplier),
		RechargeMultiplier:         rechargeMultiplier,
		InputPerMillion:            derived.InputPerMillion,
		OutputPerMillion:           derived.OutputPerMillion,
		CacheReadPerMillion:        derived.CacheReadPerMillion,
		CacheWritePerMillion:       derived.CacheWritePerMillion,
		CacheWrite1hPerMillion:     derived.CacheWrite1hPerMillion,
		ImageInputPerMillion:       derived.ImageInputPerMillion,
		AudioInputPerMillion:       derived.AudioInputPerMillion,
		AudioOutputPerMillion:      derived.AudioOutputPerMillion,
		RequestPrice:               actualRequestPrice,
		Accuracy:                   normalizeAccuracy(derived.Accuracy),
	}
}

func costMultiplier(tokenMultiplier float64, rechargeMultiplier float64) float64 {
	if rechargeMultiplier <= 0 {
		rechargeMultiplier = 1
	}
	return tokenMultiplier / rechargeMultiplier
}

func UsageSnapshotFromLog(log model.Log) UsageSnapshot {
	return DefaultUsageNormalizer{}.Normalize(log)
}

func (DefaultUsageNormalizer) Normalize(log model.Log) UsageSnapshot {
	other := map[string]interface{}{}
	if strings.TrimSpace(log.Other) != "" {
		_ = common.UnmarshalJsonStr(log.Other, &other)
	}
	unrecognized := unrecognizedUsage(other)
	upstreamModel := firstString(
		mapString(other, "upstream_model_name"),
		mapString(other, "upstream_response_model_name"),
		log.ModelName,
	)
	cacheWriteTokens := mapInt(other, "cache_write_tokens")
	if cacheWriteTokens == 0 {
		cacheWriteTokens = mapInt(other, "cache_creation_tokens")
	}
	imageInputTokens := firstPositiveInt(
		mapInt(other, "image_input_tokens"),
		mapInt(other, "image_input"),
		mapInt(other, "image_tokens"),
		mapInt(other, "image_output"),
	)
	imageOutputTokens := firstPositiveInt(
		mapInt(other, "image_output_tokens"),
	)
	audioInputTokens := firstPositiveInt(
		mapInt(other, "audio_input_token_count"),
		mapInt(other, "audio_input"),
	)
	audioOutputTokens := firstPositiveInt(
		mapInt(other, "audio_output_token_count"),
		mapInt(other, "audio_output"),
	)
	return UsageSnapshot{
		RequestID:            log.RequestId,
		ChannelID:            log.ChannelId,
		UpstreamModel:        upstreamModel,
		PromptTokens:         log.PromptTokens,
		CompletionTokens:     log.CompletionTokens,
		CacheReadTokens:      mapInt(other, "cache_tokens"),
		CacheWriteTokens:     cacheWriteTokens,
		CacheWriteTokens5m:   mapInt(other, "cache_creation_tokens_5m"),
		CacheWriteTokens1h:   mapInt(other, "cache_creation_tokens_1h"),
		ImageInputTokens:     imageInputTokens,
		ImageOutputTokens:    imageOutputTokens,
		AudioInputTokens:     audioInputTokens,
		AudioOutputTokens:    audioOutputTokens,
		UsageSemantic:        mapString(other, "usage_semantic"),
		WebSearchCallCount:   mapInt(other, "web_search_call_count"),
		FileSearchCallCount:  mapInt(other, "file_search_call_count"),
		ImageGenerationCalls: mapInt(other, "image_generation_call_count"),
		ToolCallCount:        mapInt(other, "tool_call_count"),
		AdditionalToolCounts: additionalToolCounts(other),
		UnrecognizedUsage:    unrecognized,
	}
}

func (r Result) Summary(now int64) model.ModelGatewayRequestCostSummary {
	breakdown, err := common.Marshal(r.Breakdown)
	if err != nil {
		breakdown = []byte("{}")
	}
	return model.ModelGatewayRequestCostSummary{
		RequestId:         r.RequestID,
		ChannelID:         r.ChannelID,
		UpstreamModel:     r.UpstreamModel,
		UpstreamCostTotal: r.Total,
		BreakdownJSON:     string(breakdown),
		CostSource:        r.Source,
		CostAccuracy:      r.Accuracy,
		CalculatedAt:      now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func tokenComponent(tokens int, pricePerMillion float64) BreakdownComponent {
	tokens = nonNegative(tokens)
	if tokens <= 0 || pricePerMillion <= 0 {
		return BreakdownComponent{Tokens: tokens, PricePerMillion: pricePerMillion}
	}
	amount, _ := decimal.NewFromInt(int64(tokens)).
		Mul(decimal.NewFromFloat(pricePerMillion)).
		Div(decimal.NewFromInt(1_000_000)).
		Float64()
	return BreakdownComponent{Tokens: tokens, PricePerMillion: pricePerMillion, Amount: amount}
}

func countComponent(count int, unitPrice float64) BreakdownComponent {
	count = nonNegative(count)
	if count <= 0 || unitPrice <= 0 {
		return BreakdownComponent{Count: count, UnitPrice: unitPrice}
	}
	amount, _ := decimal.NewFromInt(int64(count)).Mul(decimal.NewFromFloat(unitPrice)).Float64()
	return BreakdownComponent{Count: count, UnitPrice: unitPrice, Amount: amount}
}

func toolComponents(usage UsageSnapshot, raw string) map[string]BreakdownComponent {
	prices := map[string]float64{}
	if strings.TrimSpace(raw) != "" {
		_ = common.UnmarshalJsonStr(raw, &prices)
	}
	if len(prices) == 0 {
		return nil
	}
	counts := map[string]int{
		"web_search":       usage.WebSearchCallCount,
		"file_search":      usage.FileSearchCallCount,
		"image_generation": usage.ImageGenerationCalls,
		"tool_call":        usage.ToolCallCount,
	}
	for name, count := range usage.AdditionalToolCounts {
		counts[name] = count
	}
	components := make(map[string]BreakdownComponent)
	for name, price := range prices {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		count := nonNegative(counts[name])
		component := countComponent(count, price)
		if component.Amount <= 0 && component.Count <= 0 && component.UnitPrice <= 0 {
			continue
		}
		components[name] = component
	}
	if len(components) == 0 {
		return nil
	}
	return components
}

func breakdownTotal(b Breakdown) float64 {
	total := decimal.Zero
	components := []BreakdownComponent{
		b.Input,
		b.Output,
		b.CacheRead,
		b.CacheWrite,
		b.CacheWrite5m,
		b.CacheWrite1h,
		b.ImageInput,
		b.ImageOutput,
		b.AudioInput,
		b.AudioOutput,
		b.Request,
	}
	for _, component := range components {
		total = total.Add(decimal.NewFromFloat(component.Amount))
	}
	for _, component := range b.Tools {
		total = total.Add(decimal.NewFromFloat(component.Amount))
	}
	value, _ := total.Float64()
	return value
}

func normalizeSource(source string) string {
	switch strings.TrimSpace(source) {
	case SourceAutoSynced:
		return SourceAutoSynced
	case SourceSystemRatio:
		return SourceSystemRatio
	case SourceMissing:
		return SourceMissing
	case "":
		return SourceManual
	default:
		return SourceManual
	}
}

func inferredSystemInputPricePerMillion(modelName string) float64 {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == "*" {
		return 0
	}
	if ratio, ok, _ := ratio_setting.GetModelRatio(modelName); ok && ratio >= 0 {
		return ratio * 2
	}
	return 0
}

func resetSystemRatioDerivedPrices(profile *model.ModelGatewayChannelCostProfile) {
	if profile == nil {
		return
	}
	profile.InputPerMillion = 0
	profile.OutputPerMillion = 0
	profile.CacheReadPerMillion = 0
	profile.CacheWritePerMillion = 0
	profile.CacheWrite5mPerMillion = 0
	profile.CacheWrite1hPerMillion = 0
	profile.ImageInputPerMillion = 0
	profile.ImageOutputPerMillion = 0
	profile.AudioInputPerMillion = 0
	profile.AudioOutputPerMillion = 0
	profile.RequestPrice = 0
	profile.ToolPricesJSON = ""
}

func inferSystemPriceSource(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || modelName == "*" {
		return "missing"
	}
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return "model_price"
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return "model_ratio"
	}
	return "missing"
}

func systemCompletionRatio(modelName string) float64 {
	ratio := ratio_setting.GetCompletionRatio(modelName)
	if ratio <= 0 {
		return 1
	}
	return ratio
}

func systemCacheReadRatio(modelName string) float64 {
	ratio, _ := ratio_setting.GetCacheRatio(modelName)
	if ratio <= 0 {
		return 1
	}
	return ratio
}

func systemCacheWriteRatio(modelName string) float64 {
	ratio, ok := ratio_setting.GetCreateCacheRatio(modelName)
	if !ok {
		return 0
	}
	if ratio <= 0 {
		return 1
	}
	return ratio
}

func systemImageRatio(modelName string) float64 {
	ratio, _ := ratio_setting.GetImageRatio(modelName)
	if ratio <= 0 {
		return 1
	}
	return ratio
}

func systemAudioInputRatio(modelName string) float64 {
	ratio := ratio_setting.GetAudioRatio(modelName)
	if ratio <= 0 {
		return 1
	}
	return ratio
}

func systemAudioOutputRatio(modelName string) float64 {
	ratio := ratio_setting.GetAudioCompletionRatio(modelName)
	if ratio <= 0 {
		return 1
	}
	return ratio
}

func normalizedPositive(value float64) float64 {
	if value <= 0 {
		return 1
	}
	return value
}

func normalizedCostCoefficient(profile model.ModelGatewayChannelCostProfile) float64 {
	if profile.CostCoefficient > 0 {
		return profile.CostCoefficient
	}
	return 1
}

func normalizedTokenMultiplier(profile model.ModelGatewayChannelCostProfile) float64 {
	if profile.TokenMultiplier > 0 {
		return profile.TokenMultiplier
	}
	if profile.InputCostMultiplier > 0 {
		return profile.InputCostMultiplier
	}
	return 1
}

func normalizedActualTokenMultiplier(profile model.ModelGatewayChannelCostProfile) float64 {
	return costMultiplier(
		normalizedCostCoefficient(profile)*normalizedTokenMultiplier(profile),
		normalizedPositive(profile.RechargeMultiplier),
	)
}

func roundCostPrice(value float64) float64 {
	if value <= 0 {
		return 0
	}
	rounded, _ := decimal.NewFromFloat(value).Round(8).Float64()
	return rounded
}

func normalizeAccuracy(accuracy string) string {
	switch strings.TrimSpace(accuracy) {
	case "estimated":
		return "estimated"
	case AccuracyMissing:
		return AccuracyMissing
	case AccuracyPending:
		return AccuracyPending
	case "", AccuracyPrecise:
		return AccuracyPrecise
	default:
		return strings.TrimSpace(accuracy)
	}
}

func nonNegative(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func firstString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func mapString(values map[string]interface{}, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func mapInt(values map[string]interface{}, key string) int {
	value, ok := values[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case string:
		parsed, err := decimal.NewFromString(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return int(parsed.IntPart())
	default:
		return 0
	}
}

func additionalToolCounts(values map[string]interface{}) map[string]int {
	raw, ok := values["tool_counts"]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := common.Marshal(raw)
	if err != nil {
		return nil
	}
	counts := map[string]int{}
	if err := common.Unmarshal(encoded, &counts); err != nil {
		return nil
	}
	cleaned := make(map[string]int, len(counts))
	for name, count := range counts {
		name = strings.TrimSpace(name)
		if name == "" || count <= 0 {
			continue
		}
		cleaned[name] = count
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func unrecognizedUsage(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	known := map[string]struct{}{
		"upstream_model_name":          {},
		"upstream_response_model_name": {},
		"cache_tokens":                 {},
		"cache_write_tokens":           {},
		"cache_creation_tokens":        {},
		"cache_creation_tokens_5m":     {},
		"cache_creation_tokens_1h":     {},
		"image_input_tokens":           {},
		"image_input":                  {},
		"image_tokens":                 {},
		"image_output":                 {},
		"image_output_tokens":          {},
		"audio_input_token_count":      {},
		"audio_input":                  {},
		"audio_output_token_count":     {},
		"audio_output":                 {},
		"usage_semantic":               {},
		"web_search_call_count":        {},
		"file_search_call_count":       {},
		"image_generation_call_count":  {},
		"tool_call_count":              {},
		"tool_counts":                  {},
		"group_ratio":                  {},
		"model_ratio":                  {},
		"completion_ratio":             {},
	}
	unrecognized := make(map[string]interface{})
	for key, value := range values {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if _, ok := known[trimmed]; ok {
			continue
		}
		if !looksLikeUsageKey(trimmed) {
			continue
		}
		unrecognized[trimmed] = value
	}
	if len(unrecognized) == 0 {
		return nil
	}
	return unrecognized
}

func looksLikeUsageKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lower, "token") ||
		strings.Contains(lower, "usage") ||
		strings.Contains(lower, "cache") ||
		strings.Contains(lower, "image") ||
		strings.Contains(lower, "audio") ||
		strings.Contains(lower, "tool") ||
		strings.Contains(lower, "search")
}

func cleanUnrecognizedUsage(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	cleaned := make(map[string]interface{}, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		cleaned[key] = value
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}
