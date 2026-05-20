package scheduler

import (
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const defaultRuntimeQueueNodeID = "local"

type RuntimeQueueSnapshotSyncerOptions struct {
	Store       RuntimeSyncStore
	NodeID      string
	MinInterval time.Duration
	Now         func() time.Time
}

type RuntimeQueueSnapshotSyncer struct {
	store       RuntimeSyncStore
	nodeID      string
	minInterval time.Duration
	now         func() time.Time
	mu          sync.Mutex
	lastWrite   time.Time
}

func NewRuntimeQueueSnapshotSyncer(options RuntimeQueueSnapshotSyncerOptions) *RuntimeQueueSnapshotSyncer {
	nodeID := options.NodeID
	if nodeID == "" {
		nodeID = defaultRuntimeQueueNodeID
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &RuntimeQueueSnapshotSyncer{
		store:       options.Store,
		nodeID:      nodeID,
		minInterval: options.MinInterval,
		now:         now,
	}
}

func (s *RuntimeQueueSnapshotSyncer) SyncAndAggregate(local core.RuntimeQueueSnapshot) core.RuntimeQueueSnapshot {
	if s == nil || s.store == nil {
		return local
	}
	if s.shouldWrite(local) {
		s.store.PutQueueSnapshot(s.nodeID, local)
	}
	snapshots := s.store.ListQueueSnapshots()
	if len(snapshots) == 0 {
		return local
	}
	snapshots = replaceRuntimeQueueSnapshotForNode(snapshots, s.nodeID, local)
	return AggregateRuntimeQueueSnapshots(snapshots)
}

func (s *RuntimeQueueSnapshotSyncer) shouldWrite(local core.RuntimeQueueSnapshot) bool {
	if s == nil || !hasRuntimeQueueSnapshotData(local) {
		return false
	}
	if s.minInterval <= 0 {
		return true
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastWrite.IsZero() && now.Sub(s.lastWrite) < s.minInterval {
		return false
	}
	s.lastWrite = now
	return true
}

func replaceRuntimeQueueSnapshotForNode(snapshots []core.RuntimeQueueSnapshot, nodeID string, local core.RuntimeQueueSnapshot) []core.RuntimeQueueSnapshot {
	nodeID = sanitizeRuntimeKeyPart(nodeID)
	if nodeID == "_" {
		nodeID = defaultRuntimeQueueNodeID
	}
	result := make([]core.RuntimeQueueSnapshot, 0, len(snapshots)+1)
	for _, snapshot := range snapshots {
		if sanitizeRuntimeKeyPart(snapshot.NodeID) == nodeID {
			continue
		}
		result = append(result, snapshot)
	}
	if hasRuntimeQueueSnapshotData(local) {
		local.NodeID = nodeID
		if local.UpdatedAt == 0 {
			local.UpdatedAt = time.Now().Unix()
		}
		if local.Summary.UpdatedAt == 0 {
			local.Summary.UpdatedAt = local.UpdatedAt
		}
		result = append(result, local)
	}
	return result
}

func SyncAndAggregateQueueSnapshot(syncStore RuntimeSyncStore, nodeID string, local core.RuntimeQueueSnapshot) core.RuntimeQueueSnapshot {
	if nodeID == "" {
		nodeID = defaultRuntimeQueueNodeID
	}
	if syncStore == nil {
		return local
	}
	if hasRuntimeQueueSnapshotData(local) {
		syncStore.PutQueueSnapshot(nodeID, local)
	}
	snapshots := syncStore.ListQueueSnapshots()
	if len(snapshots) == 0 {
		return local
	}
	return AggregateRuntimeQueueSnapshots(snapshots)
}

func AggregateRuntimeQueueSnapshots(snapshots []core.RuntimeQueueSnapshot) core.RuntimeQueueSnapshot {
	if len(snapshots) == 0 {
		return core.RuntimeQueueSnapshot{}
	}
	out := core.RuntimeQueueSnapshot{}
	channelMap := map[int]*core.RuntimeQueueChannelSnapshot{}
	runtimeKeyMap := map[core.RuntimeKey]*core.RuntimeQueueKeySnapshot{}
	groupMap := map[string]*core.RuntimeQueueGroupSnapshot{}
	reasonMap := map[string]*core.RuntimeQueueReasonCount{}
	for _, snapshot := range snapshots {
		node := runtimeQueueNodeSnapshot(snapshot)
		if node.NodeID != "" && hasRuntimeQueueNodeSnapshotData(node) {
			out.Nodes = append(out.Nodes, node)
		}
		if snapshot.UpdatedAt > out.UpdatedAt {
			out.UpdatedAt = snapshot.UpdatedAt
		}
		if snapshot.Summary.UpdatedAt > out.Summary.UpdatedAt {
			out.Summary.UpdatedAt = snapshot.Summary.UpdatedAt
		}
		for _, channel := range snapshot.Channels {
			target := channelMap[channel.ChannelID]
			if target == nil {
				copyChannel := core.RuntimeQueueChannelSnapshot{ChannelID: channel.ChannelID}
				target = &copyChannel
				channelMap[channel.ChannelID] = target
			}
			mergeRuntimeQueueChannel(target, channel)
			for _, group := range channel.Groups {
				mergeRuntimeQueueGroup(groupMap, group)
			}
		}
		for _, item := range snapshot.RuntimeKeys {
			key := item.RuntimeKey
			if key.ChannelID <= 0 {
				key.ChannelID = item.ChannelID
			}
			target := runtimeKeyMap[key]
			if target == nil {
				copyItem := item
				copyItem.RuntimeKey = key
				target = &copyItem
				runtimeKeyMap[key] = target
				continue
			}
			target.QueueDepth += item.QueueDepth
			target.QueuedRequests += item.QueuedRequests
			target.WaitingRequests += item.WaitingRequests
			target.HighPriorityDepth += item.HighPriorityDepth
			target.NormalDepth += item.NormalDepth
		}
		if len(snapshot.Channels) == 0 {
			for _, group := range snapshot.Groups {
				mergeRuntimeQueueGroup(groupMap, group)
			}
		}
		for _, reason := range snapshot.RejectReasons {
			key := reason.Reason
			if reason.ChannelID > 0 || reason.Group != "" {
				key = key + "|" + reason.Group
			}
			target := reasonMap[key]
			if target == nil {
				copyReason := reason
				target = &copyReason
				reasonMap[key] = target
				continue
			}
			target.Count += reason.Count
		}
		out.Cooldowns = append(out.Cooldowns, snapshot.Cooldowns...)
	}
	for _, channel := range channelMap {
		out.Channels = append(out.Channels, *channel)
		addRuntimeQueueChannelSummary(&out.Summary, *channel)
	}
	for _, item := range runtimeKeyMap {
		out.RuntimeKeys = append(out.RuntimeKeys, *item)
	}
	for _, group := range groupMap {
		out.Groups = append(out.Groups, *group)
	}
	for _, reason := range reasonMap {
		out.RejectReasons = append(out.RejectReasons, *reason)
	}
	out.Summary.QueueChannels = len(out.Channels)
	out.Summary.QueueGroups = len(out.Groups)
	out.Summary.QueueNodes = len(out.Nodes)
	sortAggregatedRuntimeQueueSnapshot(&out)
	if out.Summary.UpdatedAt == 0 {
		out.Summary.UpdatedAt = out.UpdatedAt
	}
	return out
}

func runtimeQueueNodeSnapshot(snapshot core.RuntimeQueueSnapshot) core.RuntimeQueueNodeSnapshot {
	nodeID := sanitizeRuntimeKeyPart(snapshot.NodeID)
	if nodeID == "_" {
		nodeID = defaultRuntimeQueueNodeID
	}
	node := core.RuntimeQueueNodeSnapshot{
		NodeID:        nodeID,
		UpdatedAt:     snapshot.UpdatedAt,
		Summary:       snapshot.Summary,
		Channels:      append([]core.RuntimeQueueChannelSnapshot(nil), snapshot.Channels...),
		RuntimeKeys:   append([]core.RuntimeQueueKeySnapshot(nil), snapshot.RuntimeKeys...),
		Groups:        append([]core.RuntimeQueueGroupSnapshot(nil), snapshot.Groups...),
		RejectReasons: append([]core.RuntimeQueueReasonCount(nil), snapshot.RejectReasons...),
		Cooldowns:     append([]core.RuntimeQueueCooldownHint(nil), snapshot.Cooldowns...),
	}
	if node.UpdatedAt == 0 {
		node.UpdatedAt = node.Summary.UpdatedAt
	}
	if node.Summary.UpdatedAt == 0 {
		node.Summary.UpdatedAt = node.UpdatedAt
	}
	if node.Summary.TotalQueued == 0 && len(node.Channels) > 0 {
		node.Summary = core.RuntimeQueueSummary{UpdatedAt: node.Summary.UpdatedAt}
		for _, channel := range node.Channels {
			addRuntimeQueueChannelSummary(&node.Summary, channel)
		}
		node.Summary.QueueGroups = len(node.Groups)
	}
	return node
}

func hasRuntimeQueueNodeSnapshotData(node core.RuntimeQueueNodeSnapshot) bool {
	return len(node.Channels) > 0 ||
		len(node.RuntimeKeys) > 0 ||
		len(node.Groups) > 0 ||
		len(node.RejectReasons) > 0 ||
		node.Summary.TotalQueued > 0
}

func mergeRuntimeQueueChannel(target *core.RuntimeQueueChannelSnapshot, source core.RuntimeQueueChannelSnapshot) {
	if target == nil {
		return
	}
	target.QueueDepth += source.QueueDepth
	target.QueuedRequests += source.QueuedRequests
	target.WaitingRequests += source.WaitingRequests
	target.HighPriorityDepth += source.HighPriorityDepth
	target.NormalDepth += source.NormalDepth
	target.QueueCapacity += source.QueueCapacity
	target.HighPriorityCapacity += source.HighPriorityCapacity
	target.NormalCapacity += source.NormalCapacity
	if source.MaxQueueDepth > target.MaxQueueDepth {
		target.MaxQueueDepth = source.MaxQueueDepth
	}
	target.Groups = mergeRuntimeQueueGroupSlices(target.Groups, source.Groups)
}

func mergeRuntimeQueueGroup(groupMap map[string]*core.RuntimeQueueGroupSnapshot, source core.RuntimeQueueGroupSnapshot) {
	key := source.Group
	target := groupMap[key]
	if target == nil {
		copyGroup := source
		target = &copyGroup
		groupMap[key] = target
		return
	}
	target.QueueDepth += source.QueueDepth
	target.QueuedRequests += source.QueuedRequests
	target.WaitingRequests += source.WaitingRequests
	target.HighPriorityDepth += source.HighPriorityDepth
	target.NormalDepth += source.NormalDepth
}

func mergeRuntimeQueueGroupSlices(left []core.RuntimeQueueGroupSnapshot, right []core.RuntimeQueueGroupSnapshot) []core.RuntimeQueueGroupSnapshot {
	groupMap := map[string]*core.RuntimeQueueGroupSnapshot{}
	for _, group := range left {
		mergeRuntimeQueueGroup(groupMap, group)
	}
	for _, group := range right {
		mergeRuntimeQueueGroup(groupMap, group)
	}
	out := make([]core.RuntimeQueueGroupSnapshot, 0, len(groupMap))
	for _, group := range groupMap {
		out = append(out, *group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].QueueDepth != out[j].QueueDepth {
			return out[i].QueueDepth > out[j].QueueDepth
		}
		return out[i].Group < out[j].Group
	})
	return out
}

func addRuntimeQueueChannelSummary(summary *core.RuntimeQueueSummary, channel core.RuntimeQueueChannelSnapshot) {
	if summary == nil {
		return
	}
	summary.TotalQueued += channel.QueueDepth
	summary.TotalDepth += channel.QueueDepth
	summary.Waiting += channel.WaitingRequests
	summary.QueuedRequests += channel.QueuedRequests
	summary.WaitingRequests += channel.WaitingRequests
	summary.TotalCapacity += channel.QueueCapacity
	summary.QueueCapacity += channel.QueueCapacity
	summary.HighPriorityDepth += channel.HighPriorityDepth
	summary.NormalDepth += channel.NormalDepth
	summary.HighPriorityCapacity += channel.HighPriorityCapacity
	summary.NormalCapacity += channel.NormalCapacity
	if channel.QueueDepth > summary.MaxQueueDepth {
		summary.MaxQueueDepth = channel.QueueDepth
	}
}

func sortAggregatedRuntimeQueueSnapshot(snapshot *core.RuntimeQueueSnapshot) {
	if snapshot == nil {
		return
	}
	sort.SliceStable(snapshot.Channels, func(i, j int) bool {
		if snapshot.Channels[i].QueueDepth != snapshot.Channels[j].QueueDepth {
			return snapshot.Channels[i].QueueDepth > snapshot.Channels[j].QueueDepth
		}
		return snapshot.Channels[i].ChannelID < snapshot.Channels[j].ChannelID
	})
	sort.SliceStable(snapshot.RuntimeKeys, func(i, j int) bool {
		if snapshot.RuntimeKeys[i].QueueDepth != snapshot.RuntimeKeys[j].QueueDepth {
			return snapshot.RuntimeKeys[i].QueueDepth > snapshot.RuntimeKeys[j].QueueDepth
		}
		if snapshot.RuntimeKeys[i].ChannelID != snapshot.RuntimeKeys[j].ChannelID {
			return snapshot.RuntimeKeys[i].ChannelID < snapshot.RuntimeKeys[j].ChannelID
		}
		return snapshot.RuntimeKeys[i].RequestedModel < snapshot.RuntimeKeys[j].RequestedModel
	})
	sort.SliceStable(snapshot.Groups, func(i, j int) bool {
		if snapshot.Groups[i].QueueDepth != snapshot.Groups[j].QueueDepth {
			return snapshot.Groups[i].QueueDepth > snapshot.Groups[j].QueueDepth
		}
		return snapshot.Groups[i].Group < snapshot.Groups[j].Group
	})
	sort.SliceStable(snapshot.RejectReasons, func(i, j int) bool {
		if snapshot.RejectReasons[i].Count != snapshot.RejectReasons[j].Count {
			return snapshot.RejectReasons[i].Count > snapshot.RejectReasons[j].Count
		}
		return snapshot.RejectReasons[i].Reason < snapshot.RejectReasons[j].Reason
	})
	sort.SliceStable(snapshot.Nodes, func(i, j int) bool {
		if snapshot.Nodes[i].Summary.TotalQueued != snapshot.Nodes[j].Summary.TotalQueued {
			return snapshot.Nodes[i].Summary.TotalQueued > snapshot.Nodes[j].Summary.TotalQueued
		}
		if snapshot.Nodes[i].UpdatedAt != snapshot.Nodes[j].UpdatedAt {
			return snapshot.Nodes[i].UpdatedAt > snapshot.Nodes[j].UpdatedAt
		}
		return snapshot.Nodes[i].NodeID < snapshot.Nodes[j].NodeID
	})
}

func hasRuntimeQueueSnapshotData(snapshot core.RuntimeQueueSnapshot) bool {
	return len(snapshot.Channels) > 0 || len(snapshot.RuntimeKeys) > 0 || len(snapshot.Groups) > 0 || snapshot.Summary.TotalQueued > 0
}
