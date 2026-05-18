package controller

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type channelGroupSummaryResponse struct {
	Summary channelGroupSummaryOverview `json:"summary"`
	Groups  []channelGroupSummaryItem   `json:"groups"`
}

type channelGroupSummaryOverview struct {
	TotalGroups          int            `json:"total_groups"`
	TotalChannels        int            `json:"total_channels"`
	EnabledChannels      int            `json:"enabled_channels"`
	DisabledChannels     int            `json:"disabled_channels"`
	AutoDisabledChannels int            `json:"auto_disabled_channels"`
	TotalModels          int            `json:"total_models"`
	EnabledModels        int            `json:"enabled_models"`
	CapabilityCounts     map[string]int `json:"capability_counts"`
}

type channelGroupSummaryItem struct {
	Group                string         `json:"group"`
	TotalChannels        int            `json:"total_channels"`
	EnabledChannels      int            `json:"enabled_channels"`
	DisabledChannels     int            `json:"disabled_channels"`
	AutoDisabledChannels int            `json:"auto_disabled_channels"`
	TotalModels          int            `json:"total_models"`
	EnabledModels        int            `json:"enabled_models"`
	TypeCounts           map[int]int    `json:"type_counts"`
	Capabilities         []string       `json:"capabilities"`
	EndpointTypes        []string       `json:"endpoint_types"`
	CodexSupportedTools  []string       `json:"codex_supported_tools"`
	SampleModels         []string       `json:"sample_models"`
	CapabilityCounts     map[string]int `json:"capability_counts"`
}

type channelGroupSummaryAccumulator struct {
	item               channelGroupSummaryItem
	allModels          map[string]struct{}
	enabledModels      map[string]struct{}
	capabilities       map[string]struct{}
	endpointTypes      map[string]struct{}
	codexSupportedTool map[string]struct{}
}

func GetChannelGroupSummary(c *gin.Context) {
	channels := make([]*model.Channel, 0)
	if err := model.DB.Omit("key").Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	abilities := make([]model.Ability, 0)
	if err := model.DB.Find(&abilities).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	response := buildChannelGroupSummary(channels, abilities)
	common.ApiSuccess(c, response)
}

func buildChannelGroupSummary(channels []*model.Channel, abilities []model.Ability) channelGroupSummaryResponse {
	accumulators := map[string]*channelGroupSummaryAccumulator{}
	channelByID := make(map[int]*model.Channel, len(channels))
	overview := channelGroupSummaryOverview{
		TotalChannels:    len(channels),
		CapabilityCounts: map[string]int{},
	}
	globalModels := map[string]struct{}{}
	globalEnabledModels := map[string]struct{}{}

	getAccumulator := func(group string) *channelGroupSummaryAccumulator {
		group = normalizeChannelGroupName(group)
		acc := accumulators[group]
		if acc != nil {
			return acc
		}
		acc = &channelGroupSummaryAccumulator{
			item: channelGroupSummaryItem{
				Group:            group,
				TypeCounts:       map[int]int{},
				CapabilityCounts: map[string]int{},
			},
			allModels:          map[string]struct{}{},
			enabledModels:      map[string]struct{}{},
			capabilities:       map[string]struct{}{},
			endpointTypes:      map[string]struct{}{},
			codexSupportedTool: map[string]struct{}{},
		}
		accumulators[group] = acc
		return acc
	}

	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelByID[channel.Id] = channel
		if channel.Status == common.ChannelStatusEnabled {
			overview.EnabledChannels++
		} else {
			overview.DisabledChannels++
			if channel.Status == common.ChannelStatusAutoDisabled {
				overview.AutoDisabledChannels++
			}
		}

		for _, group := range channelSummaryGroups(channel) {
			acc := getAccumulator(group)
			acc.item.TotalChannels++
			acc.item.TypeCounts[channel.Type]++
			if channel.Status == common.ChannelStatusEnabled {
				acc.item.EnabledChannels++
			} else {
				acc.item.DisabledChannels++
				if channel.Status == common.ChannelStatusAutoDisabled {
					acc.item.AutoDisabledChannels++
				}
			}
		}
	}

	for _, ability := range abilities {
		group := normalizeChannelGroupName(ability.Group)
		acc := getAccumulator(group)
		modelName := strings.TrimSpace(ability.Model)
		if modelName != "" {
			acc.allModels[modelName] = struct{}{}
			globalModels[modelName] = struct{}{}
		}
		if !ability.Enabled {
			continue
		}
		if modelName != "" {
			acc.enabledModels[modelName] = struct{}{}
			globalEnabledModels[modelName] = struct{}{}
		}

		channel := channelByID[ability.ChannelId]
		channelType := 0
		settings := dto.ChannelOtherSettings{}
		if channel != nil {
			channelType = channel.Type
			settings = channel.GetOtherSettings()
		}

		endpointTypes := ability.GetSupportedEndpointTypes(channelType, settings)
		for _, endpointType := range endpointTypes {
			acc.endpointTypes[string(endpointType)] = struct{}{}
		}

		for _, tool := range getAbilityCodexSupportedTools(ability, channelType, settings) {
			acc.codexSupportedTool[tool] = struct{}{}
		}

		for _, capability := range buildAbilityCapabilityKeys(ability, endpointTypes, channelType, settings) {
			acc.capabilities[capability] = struct{}{}
			acc.item.CapabilityCounts[capability]++
		}
	}

	groups := make([]channelGroupSummaryItem, 0, len(accumulators))
	for _, acc := range accumulators {
		acc.item.TotalModels = len(acc.allModels)
		acc.item.EnabledModels = len(acc.enabledModels)
		acc.item.Capabilities = sortCapabilityKeys(setToSortedSlice(acc.capabilities))
		acc.item.EndpointTypes = setToSortedSlice(acc.endpointTypes)
		acc.item.CodexSupportedTools = setToSortedSlice(acc.codexSupportedTool)
		acc.item.SampleModels = firstN(setToSortedSlice(acc.enabledModels), 12)
		groups = append(groups, acc.item)
		for _, capability := range acc.item.Capabilities {
			overview.CapabilityCounts[capability]++
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Group < groups[j].Group
	})

	overview.TotalGroups = len(groups)
	overview.TotalModels = len(globalModels)
	overview.EnabledModels = len(globalEnabledModels)
	return channelGroupSummaryResponse{
		Summary: overview,
		Groups:  groups,
	}
}

func channelSummaryGroups(channel *model.Channel) []string {
	if channel == nil {
		return []string{"default"}
	}
	groups := channel.GetGroups()
	if len(groups) == 0 {
		return []string{"default"}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		group = normalizeChannelGroupName(group)
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return result
}

func normalizeChannelGroupName(group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return "default"
	}
	return group
}

func getAbilityCodexSupportedTools(ability model.Ability, channelType int, settings dto.ChannelOtherSettings) []string {
	tools := make([]string, 0)
	if strings.TrimSpace(ability.CodexSupportedTools) != "" {
		if err := common.UnmarshalJsonStr(ability.CodexSupportedTools, &tools); err != nil {
			tools = tools[:0]
		}
	}
	seen := make(map[string]struct{}, len(tools)+1)
	result := make([]string, 0, len(tools)+1)
	for _, tool := range tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		result = append(result, tool)
	}
	if ability.SupportsCodexImageGenerationTool(channelType, settings) {
		if _, ok := seen[dto.BuildInToolImageGeneration]; !ok {
			result = append(result, dto.BuildInToolImageGeneration)
		}
	}
	sort.Strings(result)
	return result
}

func buildAbilityCapabilityKeys(ability model.Ability, endpointTypes []constant.EndpointType, channelType int, settings dto.ChannelOtherSettings) []string {
	keys := map[string]struct{}{}
	if containsEndpointType(endpointTypes, constant.EndpointTypeOpenAIResponse) {
		keys["responses"] = struct{}{}
	}
	if containsEndpointType(endpointTypes, constant.EndpointTypeOpenAIResponseCompact) {
		keys["compact"] = struct{}{}
	}
	if common.IsImageGenerationModel(ability.Model) ||
		containsEndpointType(endpointTypes, constant.EndpointTypeImageGeneration) ||
		containsEndpointType(endpointTypes, constant.EndpointTypeImageEdit) {
		keys["image_api"] = struct{}{}
	}
	for _, tool := range getAbilityCodexSupportedTools(ability, channelType, settings) {
		keys["codex_tool:"+tool] = struct{}{}
	}
	if len(keys) == 0 {
		keys["chat"] = struct{}{}
	}
	return sortCapabilityKeys(setToSortedSlice(keys))
}

func containsEndpointType(endpointTypes []constant.EndpointType, target constant.EndpointType) bool {
	for _, endpointType := range endpointTypes {
		if endpointType == target {
			return true
		}
	}
	return false
}

func setToSortedSlice(set map[string]struct{}) []string {
	items := make([]string, 0, len(set))
	for item := range set {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

func firstN(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	return items[:n]
}

func sortCapabilityKeys(keys []string) []string {
	order := map[string]int{
		"chat":                        10,
		"responses":                   20,
		"compact":                     30,
		"image_api":                   40,
		"codex_tool:image_generation": 50,
	}
	sort.SliceStable(keys, func(i, j int) bool {
		leftOrder, leftOK := order[keys[i]]
		rightOrder, rightOK := order[keys[j]]
		if leftOK && rightOK {
			if leftOrder == rightOrder {
				return keys[i] < keys[j]
			}
			return leftOrder < rightOrder
		}
		if leftOK {
			return true
		}
		if rightOK {
			return false
		}
		return keys[i] < keys[j]
	})
	return keys
}
