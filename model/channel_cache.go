package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func weightedRandomSelect(targetChannels []*Channel) (*Channel, error) {
	if len(targetChannels) == 0 {
		return nil, errors.New("channel not found")
	}
	if len(targetChannels) == 1 {
		return targetChannels[0], nil
	}

	var sumWeight = 0
	for _, channel := range targetChannels {
		sumWeight += channel.GetWeight()
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	totalWeight := sumWeight * smoothingFactor
	randomWeight := rand.Intn(totalWeight)

	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}

	return nil, errors.New("channel not found")
}

func getCachedCandidateChannels(group string, model string, excludedChannelIDs map[int]struct{}) []int {
	channels := group2model2channels[group][model]
	if hasUnexcludedChannel(channels, excludedChannelIDs) {
		return channels
	}

	normalizedModel := ratio_setting.FormatMatchingModelName(model)
	if normalizedModel != "" && normalizedModel != model {
		if normalizedChannels := group2model2channels[group][normalizedModel]; len(normalizedChannels) > 0 {
			return normalizedChannels
		}
	}
	return channels
}

func hasUnexcludedChannel(channels []int, excludedChannelIDs map[int]struct{}) bool {
	if len(channels) == 0 {
		return false
	}
	if len(excludedChannelIDs) == 0 {
		return true
	}
	for _, channelID := range channels {
		if _, excluded := excludedChannelIDs[channelID]; !excluded {
			return true
		}
	}
	return false
}

func GetRandomSatisfiedChannel(group string, model string, retry int, excludedChannelIDs map[int]struct{}) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry, excludedChannelIDs)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channels := getCachedCandidateChannels(group, model, excludedChannelIDs)
	if len(channels) == 0 {
		return nil, nil
	}

	if len(excludedChannelIDs) > 0 {
		priorityBuckets := make(map[int64][]*Channel)
		for _, channelId := range channels {
			if _, excluded := excludedChannelIDs[channelId]; excluded {
				continue
			}
			channel, ok := channelsIDM[channelId]
			if !ok {
				return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
			}
			priorityBuckets[channel.GetPriority()] = append(priorityBuckets[channel.GetPriority()], channel)
		}

		if len(priorityBuckets) == 0 {
			return nil, nil
		}

		var priorities []int
		for priority := range priorityBuckets {
			priorities = append(priorities, int(priority))
		}
		sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

		return weightedRandomSelect(priorityBuckets[int64(priorities[0])])
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	return weightedRandomSelect(targetChannels)
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	channel, ok := channelsIDM[id]
	if !ok {
		return
	}
	channel.Status = status
	if status != common.ChannelStatusEnabled {
		removeChannelFromCacheIndex(id)
		return
	}
	addChannelToCacheIndex(channel)
}

func removeChannelFromCacheIndex(id int) {
	for group, model2channels := range group2model2channels {
		for model, channels := range model2channels {
			filtered := channels[:0]
			for _, channelId := range channels {
				if channelId != id {
					filtered = append(filtered, channelId)
				}
			}
			group2model2channels[group][model] = filtered
		}
	}
}

func addChannelToCacheIndex(channel *Channel) {
	if channel == nil {
		return
	}
	for _, group := range channel.GetGroups() {
		if group == "" {
			continue
		}
		if group2model2channels[group] == nil {
			group2model2channels[group] = make(map[string][]int)
		}
		for _, modelName := range channel.GetModels() {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			channels := group2model2channels[group][modelName]
			exists := false
			for _, channelId := range channels {
				if channelId == channel.Id {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			channels = append(channels, channel.Id)
			sort.Slice(channels, func(i, j int) bool {
				left, leftOk := channelsIDM[channels[i]]
				right, rightOk := channelsIDM[channels[j]]
				if !leftOk || !rightOk {
					return leftOk
				}
				return left.GetPriority() > right.GetPriority()
			})
			group2model2channels[group][modelName] = channels
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}
	channelsIDM[channel.Id] = channel
}
