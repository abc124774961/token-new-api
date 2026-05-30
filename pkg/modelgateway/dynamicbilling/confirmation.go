package dynamicbilling

import (
	"errors"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"gorm.io/gorm"
)

const manualConfirmationOptionKey = "model_gateway.dynamic_billing.manual_confirmations"

var (
	errDynamicBillingConfirmMissingGroup    = errors.New("缺少动态倍率分组")
	errDynamicBillingConfirmMissingBaseline = errors.New("未找到可确认的动态倍率结果")
	errDynamicBillingConfirmInvalidRatio    = errors.New("动态倍率结果无效")
)

type ManualConfirmation struct {
	Group                string  `json:"group"`
	ConfirmedRatio       float64 `json:"confirmed_ratio"`
	TargetRatio          float64 `json:"target_ratio,omitempty"`
	EffectiveRatio       float64 `json:"effective_ratio,omitempty"`
	BaselineCalculatedAt int64   `json:"baseline_calculated_at,omitempty"`
	WindowStart          int64   `json:"window_start,omitempty"`
	WindowEnd            int64   `json:"window_end,omitempty"`
	ConfirmedAt          int64   `json:"confirmed_at"`
	ConfirmedByID        int     `json:"confirmed_by_id,omitempty"`
	ConfirmedBy          string  `json:"confirmed_by,omitempty"`
}

type manualConfirmationEnvelope struct {
	Confirmations map[string]ManualConfirmation `json:"confirmations"`
}

func ConfirmManualRatio(db *gorm.DB, group string, operatorID int, operatorName string) (ManualConfirmation, RatioBaseline, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return ManualConfirmation{}, RatioBaseline{}, errDynamicBillingConfirmMissingGroup
	}
	baseline, ok := findManualConfirmationBaseline(db, group)
	if !ok {
		return ManualConfirmation{}, RatioBaseline{}, errDynamicBillingConfirmMissingBaseline
	}
	ratio := baseline.EffectiveRatio
	if ratio <= 0 {
		ratio = baseline.Ratio
	}
	if ratio <= 0 {
		return ManualConfirmation{}, RatioBaseline{}, errDynamicBillingConfirmInvalidRatio
	}
	confirmation := ManualConfirmation{
		Group:                strings.TrimSpace(baseline.Group),
		ConfirmedRatio:       ratio,
		TargetRatio:          baseline.TargetRatio,
		EffectiveRatio:       baseline.EffectiveRatio,
		BaselineCalculatedAt: baseline.CalculatedAt,
		WindowStart:          baseline.WindowStart,
		WindowEnd:            baseline.WindowEnd,
		ConfirmedAt:          common.GetTimestamp(),
		ConfirmedByID:        operatorID,
		ConfirmedBy:          strings.TrimSpace(operatorName),
	}
	confirmations := loadManualConfirmations(db)
	confirmations[groupCacheKey(confirmation.Group)] = confirmation
	if err := saveManualConfirmations(db, confirmations); err != nil {
		return ManualConfirmation{}, RatioBaseline{}, err
	}
	return confirmation, baseline, nil
}

func findManualConfirmationBaseline(db *gorm.DB, group string) (RatioBaseline, bool) {
	groupKey := groupCacheKey(group)
	if groupKey == "" {
		return RatioBaseline{}, false
	}
	for _, baseline := range DefaultBaselineSnapshots() {
		if groupCacheKey(baseline.Group) == groupKey {
			return baseline, true
		}
	}
	for key, baseline := range loadPersistedBaselines(db) {
		if key == groupKey || groupCacheKey(baseline.Group) == groupKey {
			return baseline, true
		}
	}
	return RatioBaseline{}, false
}

func loadManualConfirmations(db *gorm.DB) map[string]ManualConfirmation {
	raw := optionValueFromMemory(manualConfirmationOptionKey)
	if strings.TrimSpace(raw) == "" && db != nil {
		option := model.Option{}
		if err := db.Where(&model.Option{Key: manualConfirmationOptionKey}).First(&option).Error; err == nil {
			raw = option.Value
		}
	}
	return parseManualConfirmations(raw)
}

func parseManualConfirmations(raw string) map[string]ManualConfirmation {
	result := map[string]ManualConfirmation{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return result
	}
	envelope := manualConfirmationEnvelope{}
	if err := common.UnmarshalJsonStr(raw, &envelope); err == nil && len(envelope.Confirmations) > 0 {
		for key, confirmation := range envelope.Confirmations {
			confirmation = normalizeManualConfirmation(key, confirmation)
			if confirmation.Group != "" && confirmation.ConfirmedRatio > 0 {
				result[groupCacheKey(confirmation.Group)] = confirmation
			}
		}
		return result
	}
	legacy := map[string]ManualConfirmation{}
	if err := common.UnmarshalJsonStr(raw, &legacy); err != nil {
		return result
	}
	for key, confirmation := range legacy {
		confirmation = normalizeManualConfirmation(key, confirmation)
		if confirmation.Group != "" && confirmation.ConfirmedRatio > 0 {
			result[groupCacheKey(confirmation.Group)] = confirmation
		}
	}
	return result
}

func saveManualConfirmations(db *gorm.DB, confirmations map[string]ManualConfirmation) error {
	normalized := make(map[string]ManualConfirmation, len(confirmations))
	for key, confirmation := range confirmations {
		confirmation = normalizeManualConfirmation(key, confirmation)
		if confirmation.Group == "" || confirmation.ConfirmedRatio <= 0 {
			continue
		}
		normalized[groupCacheKey(confirmation.Group)] = confirmation
	}
	payload, err := common.Marshal(manualConfirmationEnvelope{Confirmations: normalized})
	if err != nil {
		return err
	}
	value := string(payload)
	if db != nil {
		option := model.Option{Key: manualConfirmationOptionKey}
		if err := db.FirstOrCreate(&option, model.Option{Key: manualConfirmationOptionKey}).Error; err != nil {
			return err
		}
		option.Value = value
		if err := db.Save(&option).Error; err != nil {
			return err
		}
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMap[manualConfirmationOptionKey] = value
	common.OptionMapRWMutex.Unlock()
	return nil
}

func normalizeManualConfirmation(key string, confirmation ManualConfirmation) ManualConfirmation {
	if strings.TrimSpace(confirmation.Group) == "" {
		confirmation.Group = strings.TrimSpace(key)
	}
	confirmation.Group = strings.TrimSpace(confirmation.Group)
	confirmation.ConfirmedBy = strings.TrimSpace(confirmation.ConfirmedBy)
	if confirmation.ConfirmedRatio <= 0 && confirmation.EffectiveRatio > 0 {
		confirmation.ConfirmedRatio = confirmation.EffectiveRatio
	}
	return confirmation
}

func optionValueFromMemory(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if common.OptionMap == nil {
		return ""
	}
	return common.OptionMap[key]
}

func manualConfirmationAllowsRatio(confirmation ManualConfirmation, group string, ratio float64, maxStepChange float64) bool {
	if groupCacheKey(confirmation.Group) != groupCacheKey(group) || confirmation.ConfirmedRatio <= 0 || ratio <= 0 {
		return false
	}
	if maxStepChange <= 0 {
		maxStepChange = scheduler_setting.DefaultSetting().DynamicBillingMaxStepChange
	}
	return ratioOrZero(math.Abs(ratio-confirmation.ConfirmedRatio), confirmation.ConfirmedRatio) <= maxStepChange
}
