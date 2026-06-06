package controller

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"

	"github.com/gin-gonic/gin"
)

const (
	userGroupDynamicBillingCacheTTL        = 30 * time.Second
	userGroupDynamicBillingPartialCacheTTL = 5 * time.Second
)

type userGroupResponseItem struct {
	Ratio          any                              `json:"ratio"`
	Desc           string                           `json:"desc"`
	DynamicBilling *userGroupDynamicBillingResponse `json:"dynamic_billing,omitempty"`
}

type userGroupDynamicBillingResponse struct {
	PolicyGroup    string  `json:"policy_group,omitempty"`
	Status         string  `json:"status,omitempty"`
	CurrentRatio   float64 `json:"current_ratio,omitempty"`
	AverageRatio7d float64 `json:"average_ratio_7d,omitempty"`
	MinRatio7d     float64 `json:"min_ratio_7d,omitempty"`
	MaxRatio7d     float64 `json:"max_ratio_7d,omitempty"`
	MinPricePerM7d float64 `json:"min_price_per_m_7d,omitempty"`
	MaxPricePerM7d float64 `json:"max_price_per_m_7d,omitempty"`
}

type userGroupDynamicBillingCacheEntry struct {
	expiresAt time.Time
	values    map[string]userGroupDynamicBillingResponse
	partial   bool
}

var userGroupDynamicBillingCache struct {
	mu         sync.RWMutex
	entry      userGroupDynamicBillingCacheEntry
	refreshing bool
	generation uint64
}

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]userGroupResponseItem)
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	includeDynamicBilling := parseUserGroupDynamicBillingQuery(c.Query("include_dynamic_billing"))
	dynamicBillingByGroup := map[string]userGroupDynamicBillingResponse{}
	if includeDynamicBilling {
		dynamicBillingByGroup = getCachedUserGroupDynamicBillingDisplay()
	}
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			item := userGroupResponseItem{
				Ratio: service.GetUserGroupRatio(userGroup, groupName),
				Desc:  desc,
			}
			if dynamicBilling, ok := dynamicBillingByGroup[groupName]; ok {
				dynamicBillingCopy := dynamicBilling
				item.DynamicBilling = &dynamicBillingCopy
			}
			usableGroups[groupName] = item
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		item := userGroupResponseItem{
			Ratio: "自动",
			Desc:  setting.GetUsableGroupDescription("auto"),
		}
		if dynamicBilling, ok := dynamicBillingByGroup["auto"]; ok {
			dynamicBillingCopy := dynamicBilling
			item.DynamicBilling = &dynamicBillingCopy
		}
		usableGroups["auto"] = item
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}

func parseUserGroupDynamicBillingQuery(raw string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	return err == nil && parsed
}

func getCachedUserGroupDynamicBillingDisplay() map[string]userGroupDynamicBillingResponse {
	settingValue := scheduler_setting.GetSetting()
	if !settingValue.DynamicBillingEnabled || len(settingValue.GroupPolicies) == 0 {
		clearUserGroupDynamicBillingDisplayCache()
		return map[string]userGroupDynamicBillingResponse{}
	}

	now := time.Now()
	userGroupDynamicBillingCache.mu.Lock()
	if userGroupDynamicBillingCache.entry.values != nil {
		cached := cloneUserGroupDynamicBillingDisplayMap(userGroupDynamicBillingCache.entry.values)
		if now.Before(userGroupDynamicBillingCache.entry.expiresAt) && !userGroupDynamicBillingCache.entry.partial {
			userGroupDynamicBillingCache.mu.Unlock()
			return cached
		}
		if !userGroupDynamicBillingCache.refreshing {
			userGroupDynamicBillingCache.refreshing = true
			generation := userGroupDynamicBillingCache.generation
			go refreshUserGroupDynamicBillingDisplayCache(generation)
		}
		userGroupDynamicBillingCache.mu.Unlock()
		return cached
	}

	if !userGroupDynamicBillingCache.refreshing {
		userGroupDynamicBillingCache.refreshing = true
		generation := userGroupDynamicBillingCache.generation
		go refreshUserGroupDynamicBillingDisplayCache(generation)
	}
	generation := userGroupDynamicBillingCache.generation
	userGroupDynamicBillingCache.mu.Unlock()

	values := buildUserGroupDynamicBillingCurrentDisplayMap(now.Unix())
	storeUserGroupDynamicBillingPartialDisplay(values, generation)
	return values
}

func refreshUserGroupDynamicBillingDisplayCache(generation uint64) {
	settingValue := scheduler_setting.GetSetting()
	if !settingValue.DynamicBillingEnabled || len(settingValue.GroupPolicies) == 0 {
		userGroupDynamicBillingCache.mu.Lock()
		if generation == userGroupDynamicBillingCache.generation {
			userGroupDynamicBillingCache.entry = userGroupDynamicBillingCacheEntry{}
			userGroupDynamicBillingCache.refreshing = false
		}
		userGroupDynamicBillingCache.mu.Unlock()
		return
	}

	now := time.Now()
	values := buildUserGroupDynamicBillingFullDisplayMap(now.Unix())
	ttl := time.Duration(settingValue.DynamicBillingRefreshSeconds) * time.Second
	if ttl <= 0 {
		ttl = userGroupDynamicBillingCacheTTL
	}
	if ttl > userGroupDynamicBillingCacheTTL {
		ttl = userGroupDynamicBillingCacheTTL
	}

	userGroupDynamicBillingCache.mu.Lock()
	if generation != userGroupDynamicBillingCache.generation {
		userGroupDynamicBillingCache.mu.Unlock()
		return
	}
	userGroupDynamicBillingCache.entry = userGroupDynamicBillingCacheEntry{
		expiresAt: now.Add(ttl),
		values:    cloneUserGroupDynamicBillingDisplayMap(values),
		partial:   false,
	}
	userGroupDynamicBillingCache.refreshing = false
	userGroupDynamicBillingCache.mu.Unlock()
}

func buildUserGroupDynamicBillingCurrentDisplayMap(now int64) map[string]userGroupDynamicBillingResponse {
	currentOverview := buildModelGatewayDynamicBillingOverviewForDisplay(now, 0)
	return buildUserGroupDynamicBillingDisplayMap(
		currentOverview,
		ModelGatewayDynamicBillingOverview{Enabled: currentOverview.Enabled},
	)
}

func buildUserGroupDynamicBillingFullDisplayMap(now int64) map[string]userGroupDynamicBillingResponse {
	return buildUserGroupDynamicBillingDisplayMap(
		buildModelGatewayDynamicBillingOverviewForDisplay(now, 0),
		buildModelGatewayDynamicBillingOverviewForDisplay(now, 7*24*60),
	)
}

func storeUserGroupDynamicBillingPartialDisplay(values map[string]userGroupDynamicBillingResponse, generation uint64) {
	userGroupDynamicBillingCache.mu.Lock()
	defer userGroupDynamicBillingCache.mu.Unlock()
	if generation != userGroupDynamicBillingCache.generation {
		return
	}
	if userGroupDynamicBillingCache.entry.values != nil &&
		!userGroupDynamicBillingCache.entry.partial &&
		time.Now().Before(userGroupDynamicBillingCache.entry.expiresAt) {
		return
	}
	userGroupDynamicBillingCache.entry = userGroupDynamicBillingCacheEntry{
		expiresAt: time.Now().Add(userGroupDynamicBillingPartialCacheTTL),
		values:    cloneUserGroupDynamicBillingDisplayMap(values),
		partial:   true,
	}
}

func buildUserGroupDynamicBillingDisplayMap(currentOverview ModelGatewayDynamicBillingOverview, overview7d ModelGatewayDynamicBillingOverview) map[string]userGroupDynamicBillingResponse {
	if !currentOverview.Enabled {
		return map[string]userGroupDynamicBillingResponse{}
	}

	overview7dByGroup := make(map[string]ModelGatewayDynamicBillingGroupOverview, len(overview7d.Groups))
	for _, item := range overview7d.Groups {
		policyGroup := strings.TrimSpace(item.PolicyGroup)
		if policyGroup == "" {
			continue
		}
		overview7dByGroup[policyGroup] = item
	}

	result := make(map[string]userGroupDynamicBillingResponse)
	for _, item := range currentOverview.Groups {
		policyGroup := strings.TrimSpace(item.PolicyGroup)
		if policyGroup == "" {
			continue
		}
		if strings.TrimSpace(item.Status) == "global_disabled" {
			continue
		}
		recentItem := overview7dByGroup[policyGroup]
		response := userGroupDynamicBillingResponse{
			PolicyGroup:    policyGroup,
			Status:         firstNonEmptyTrimmed(item.Status, recentItem.Status),
			CurrentRatio:   firstPositiveDynamicBillingValue(item.CurrentRatio, item.BlendedRatio, item.AverageRatio, item.MaxRatio, item.MinRatio),
			AverageRatio7d: firstPositiveDynamicBillingValue(recentItem.BlendedRatio, recentItem.AverageRatio, recentItem.CurrentRatio, recentItem.MaxRatio, recentItem.MinRatio),
			MinRatio7d:     firstPositiveDynamicBillingValue(recentItem.MinRatio, recentItem.BlendedRatio, recentItem.AverageRatio, recentItem.CurrentRatio, recentItem.MaxRatio),
			MaxRatio7d:     firstPositiveDynamicBillingValue(recentItem.MaxRatio, recentItem.BlendedRatio, recentItem.AverageRatio, recentItem.CurrentRatio, recentItem.MinRatio),
			MinPricePerM7d: firstPositiveDynamicBillingValue(recentItem.MinPricePerM, recentItem.AveragePricePerM, recentItem.BlendedPricePerM, recentItem.CurrentPricePerM),
			MaxPricePerM7d: firstPositiveDynamicBillingValue(recentItem.MaxPricePerM, recentItem.AveragePricePerM, recentItem.BlendedPricePerM, recentItem.CurrentPricePerM),
		}
		if response.CurrentRatio <= 0 && response.AverageRatio7d <= 0 && response.MinRatio7d <= 0 && response.MaxRatio7d <= 0 && response.MinPricePerM7d <= 0 && response.MaxPricePerM7d <= 0 {
			continue
		}
		if response.MaxRatio7d > 0 && response.MinRatio7d <= 0 {
			response.MinRatio7d = response.MaxRatio7d
		}
		if response.MinRatio7d > 0 && response.MaxRatio7d <= 0 {
			response.MaxRatio7d = response.MinRatio7d
		}
		if response.MinRatio7d > 0 && response.MaxRatio7d > 0 && response.MinRatio7d > response.MaxRatio7d {
			response.MinRatio7d, response.MaxRatio7d = response.MaxRatio7d, response.MinRatio7d
		}
		if response.MaxPricePerM7d > 0 && response.MinPricePerM7d <= 0 {
			response.MinPricePerM7d = response.MaxPricePerM7d
		}
		if response.MinPricePerM7d > 0 && response.MaxPricePerM7d <= 0 {
			response.MaxPricePerM7d = response.MinPricePerM7d
		}
		result[policyGroup] = response
	}
	return result
}

func firstPositiveDynamicBillingValue(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func cloneUserGroupDynamicBillingDisplayMap(values map[string]userGroupDynamicBillingResponse) map[string]userGroupDynamicBillingResponse {
	if len(values) == 0 {
		return map[string]userGroupDynamicBillingResponse{}
	}
	cloned := make(map[string]userGroupDynamicBillingResponse, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func clearUserGroupDynamicBillingDisplayCache() {
	userGroupDynamicBillingCache.mu.Lock()
	defer userGroupDynamicBillingCache.mu.Unlock()
	userGroupDynamicBillingCache.entry = userGroupDynamicBillingCacheEntry{}
	userGroupDynamicBillingCache.refreshing = false
	userGroupDynamicBillingCache.generation++
}

func resetUserGroupDynamicBillingDisplayCache() {
	clearUserGroupDynamicBillingDisplayCache()
}
