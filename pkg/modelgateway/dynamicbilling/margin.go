package dynamicbilling

func SanitizeTargetGrossMargin(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value >= 0.95 {
		return 0.95
	}
	return value
}

func RevenueMultiplierForGrossMargin(targetGrossMargin float64) float64 {
	targetGrossMargin = SanitizeTargetGrossMargin(targetGrossMargin)
	return 1 / (1 - targetGrossMargin)
}

func RequiredRevenueForGrossMargin(costUSD float64, targetGrossMargin float64) float64 {
	if costUSD <= 0 {
		return 0
	}
	return costUSD * RevenueMultiplierForGrossMargin(targetGrossMargin)
}
