package types

type BillingMultiplierRuleSnapshot struct {
	ID              int     `json:"id"`
	Name            string  `json:"name,omitempty"`
	ScopeType       string  `json:"scope_type,omitempty"`
	ScopeValue      string  `json:"scope_value,omitempty"`
	ScopeID         int     `json:"scope_id,omitempty"`
	ScopeKey        string  `json:"scope_key,omitempty"`
	ScopeName       string  `json:"scope_name,omitempty"`
	UsingGroup      string  `json:"using_group,omitempty"`
	GroupMultiplier bool    `json:"group_multiplier,omitempty"`
	Mode            string  `json:"mode"`
	Multiplier      float64 `json:"multiplier"`
	Priority        int     `json:"priority"`
	Description     string  `json:"description,omitempty"`
}

type BillingMultiplierSnapshot struct {
	Applied         bool                            `json:"applied"`
	BaseGroupRatio  float64                         `json:"base_group_ratio"`
	FinalGroupRatio float64                         `json:"final_group_ratio"`
	Multiplier      float64                         `json:"multiplier"`
	Rules           []BillingMultiplierRuleSnapshot `json:"rules,omitempty"`
}
