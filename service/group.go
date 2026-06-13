package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const AutoGroupName = "auto"

func NormalizeTokenGroup(group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return AutoGroupName
	}
	return group
}

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	groupName = NormalizeTokenGroup(groupName)
	if groupName == AutoGroupName || groupName == userGroup {
		return true
	}
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

func GroupRatioConfiguredOrImplicit(groupName string) bool {
	groupName = NormalizeTokenGroup(groupName)
	if groupName == AutoGroupName {
		return true
	}
	return ratio_setting.ContainsGroupRatio(groupName)
}

func EffectiveRoutingGroups(groupName string) []string {
	groupName = NormalizeTokenGroup(groupName)
	groups := []string{groupName}
	for _, fallback := range RoutingFallbackGroups(groupName) {
		if fallback != "" && fallback != groupName {
			groups = append(groups, fallback)
		}
	}
	return groups
}

func RoutingFallbackGroups(groupName string) []string {
	groupName = NormalizeTokenGroup(groupName)
	normalizedGroupName := strings.ToLower(groupName)
	switch {
	case groupName == AutoGroupName:
		return nil
	case normalizedGroupName != "codex-plus" && (strings.HasPrefix(normalizedGroupName, "codex-plus") || strings.HasPrefix(normalizedGroupName, "code-plus")):
		return []string{"codex-plus"}
	case strings.HasPrefix(normalizedGroupName, "codex-pro"):
		if normalizedGroupName != "codex-pro" {
			return []string{"codex-pro"}
		}
	}
	return nil
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
