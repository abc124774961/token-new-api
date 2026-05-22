package integration

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
)

const defaultRuntimeSnapshotSeedLimit = 5000

type runtimeSnapshotSeedAttemptMeta struct {
	ClientAborted      bool   `json:"client_aborted,omitempty"`
	ConcurrencyLimited bool   `json:"concurrency_limited,omitempty"`
	EmptyOutput        bool   `json:"empty_output,omitempty"`
	ExperienceIssue    string `json:"experience_issue,omitempty"`
}

func SeedRuntimeSnapshotsFromExecutionRecords(ctx context.Context, store core.RuntimeSnapshotStore, limit int) (int, error) {
	if store == nil || model.DB == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = defaultRuntimeSnapshotSeedLimit
	}
	records := make([]model.ModelExecutionRecord, 0)
	err := model.DB.WithContext(ctx).
		Where("channel_id > ?", 0).
		Where("requested_model <> ?", "").
		Where("(duration_ms > ? OR ttft_ms > ? OR status_code > ? OR success = ? OR stream_interrupted = ?)", 0, 0, 0, true, true).
		Order("id DESC").
		Limit(limit).
		Find(&records).Error
	if err != nil || len(records) == 0 {
		return 0, err
	}

	channels, err := runtimeSnapshotSeedChannels(ctx, records)
	if err != nil {
		return 0, err
	}
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	registry := provider.NewStandardProviderRegistry()
	seeded := 0
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		channel, ok := channels[record.ChannelId]
		if !ok {
			continue
		}
		attempt, ok := runtimeSnapshotSeedAttempt(record, channel, registry)
		if !ok {
			continue
		}
		monitor.Report(ctx, attempt)
		seeded++
	}
	return seeded, nil
}

func runtimeSnapshotSeedChannels(ctx context.Context, records []model.ModelExecutionRecord) (map[int]*model.Channel, error) {
	ids := make([]int, 0)
	seen := make(map[int]struct{})
	for _, record := range records {
		if record.ChannelId <= 0 {
			continue
		}
		if _, ok := seen[record.ChannelId]; ok {
			continue
		}
		seen[record.ChannelId] = struct{}{}
		ids = append(ids, record.ChannelId)
	}
	if len(ids) == 0 {
		return map[int]*model.Channel{}, nil
	}
	rows := make([]model.Channel, 0, len(ids))
	if err := model.DB.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	channels := make(map[int]*model.Channel, len(rows))
	for i := range rows {
		channel := rows[i]
		channels[channel.Id] = &channel
	}
	return channels, nil
}

func runtimeSnapshotSeedAttempt(record model.ModelExecutionRecord, channel *model.Channel, registry provider.ProviderRegistry) (core.AttemptResult, bool) {
	meta := runtimeSnapshotSeedMeta(record.RequestMeta)
	if meta.ClientAborted || meta.ConcurrencyLimited {
		return core.AttemptResult{}, false
	}
	key := runtimeSnapshotSeedKey(record, channel, registry)
	if key.ChannelID <= 0 || key.RequestedModel == "" {
		return core.AttemptResult{}, false
	}
	success := record.Success && !record.StreamInterrupted
	return core.AttemptResult{
		Key:                key,
		RequestID:          record.RequestId,
		AttemptIndex:       record.AttemptIndex,
		ChannelID:          channel.Id,
		ChannelName:        channel.Name,
		RequestedGroup:     record.RequestedGroup,
		SelectedGroup:      runtimeSnapshotSeedGroup(record, channel),
		ModelName:          key.RequestedModel,
		EndpointType:       key.EndpointType,
		Success:            success,
		StatusCode:         record.StatusCode,
		ErrorCode:          record.ErrorCode,
		ErrorType:          record.ErrorType,
		ObservedAt:         time.Unix(record.CreatedAt, 0),
		Duration:           time.Duration(record.DurationMs) * time.Millisecond,
		TTFT:               time.Duration(record.TTFTMs) * time.Millisecond,
		StreamInterrupted:  record.StreamInterrupted,
		ClientAborted:      meta.ClientAborted,
		ConcurrencyLimited: meta.ConcurrencyLimited,
		EmptyOutput:        meta.EmptyOutput,
		ExperienceIssue:    meta.ExperienceIssue,
	}, true
}

func runtimeSnapshotSeedMeta(raw string) runtimeSnapshotSeedAttemptMeta {
	meta := runtimeSnapshotSeedAttemptMeta{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return meta
	}
	_ = common.UnmarshalJsonStr(raw, &meta)
	return meta
}

func runtimeSnapshotSeedKey(record model.ModelExecutionRecord, channel *model.Channel, registry provider.ProviderRegistry) core.RuntimeKey {
	if channel == nil {
		return core.RuntimeKey{}
	}
	requestedModel := strings.TrimSpace(record.RequestedModel)
	endpointType := constant.EndpointType(strings.TrimSpace(record.EndpointType))
	if endpointType == "" {
		endpointType = constant.EndpointTypeOpenAI
	}
	profile := provider.ProviderProfile(nil)
	if registry != nil {
		profile = registry.Best(channel, requestedModel)
	}
	if profile == nil {
		profile = provider.NewStandardOpenAICompatibleProfile()
	}
	capability := profile.Capabilities(channel, requestedModel)
	proxyMode := profile.ProxyMode(channel, requestedModel)
	fingerprint := runtimeSnapshotSeedAppendCapabilityPart(capability.CapabilityFingerprint, profile.Name())
	fingerprint = runtimeSnapshotSeedAppendCapabilityPart(fingerprint, proxyMode)
	return core.RuntimeKey{
		RequestedModel:        requestedModel,
		UpstreamModel:         channel.ResolveMappedModelName(requestedModel),
		ChannelID:             channel.Id,
		Group:                 runtimeSnapshotSeedGroup(record, channel),
		EndpointType:          endpointType,
		CapabilityFingerprint: fingerprint,
	}
}

func runtimeSnapshotSeedGroup(record model.ModelExecutionRecord, channel *model.Channel) string {
	if group := strings.TrimSpace(record.SelectedGroup); group != "" {
		return group
	}
	if group := strings.TrimSpace(record.RequestedGroup); group != "" {
		return group
	}
	if channel != nil {
		return strings.TrimSpace(channel.Group)
	}
	return ""
}

func runtimeSnapshotSeedAppendCapabilityPart(fingerprint string, part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return fingerprint
	}
	if fingerprint == "" {
		return part
	}
	parts := strings.Split(fingerprint, "|")
	for _, existing := range parts {
		if existing == part {
			return fingerprint
		}
	}
	parts = append(parts, part)
	return strings.Join(parts, "|")
}
