package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const ArtifactVersion = 1

type Artifact struct {
	Version int      `json:"version"`
	Kind    string   `json:"kind"`
	Records []Record `json:"records"`
}

type Record struct {
	CreatedAt             int64                  `json:"created_at,omitempty"`
	RequestID             string                 `json:"request_id,omitempty"`
	AttemptIndex          int                    `json:"attempt_index,omitempty"`
	UserID                int                    `json:"user_id,omitempty"`
	TokenID               int                    `json:"token_id,omitempty"`
	RequestedGroup        string                 `json:"requested_group,omitempty"`
	SelectedGroup         string                 `json:"selected_group,omitempty"`
	ActualGroup           string                 `json:"actual_group,omitempty"`
	RequestedModel        string                 `json:"requested_model,omitempty"`
	ChannelID             int                    `json:"channel_id,omitempty"`
	ChannelName           string                 `json:"channel_name,omitempty"`
	ActualChannelID       int                    `json:"actual_channel_id,omitempty"`
	ActualChannelName     string                 `json:"actual_channel_name,omitempty"`
	EndpointType          string                 `json:"endpoint_type,omitempty"`
	PolicyMode            string                 `json:"policy_mode,omitempty"`
	AutoMode              string                 `json:"auto_mode,omitempty"`
	Strategy              string                 `json:"strategy,omitempty"`
	Shadow                bool                   `json:"shadow,omitempty"`
	SmartHandled          bool                   `json:"smart_handled,omitempty"`
	FallbackUsed          bool                   `json:"fallback_used,omitempty"`
	Success               bool                   `json:"success,omitempty"`
	StatusCode            int                    `json:"status_code,omitempty"`
	ErrorCode             string                 `json:"error_code,omitempty"`
	ErrorType             string                 `json:"error_type,omitempty"`
	DurationMs            int64                  `json:"duration_ms,omitempty"`
	TTFTMs                int64                  `json:"ttft_ms,omitempty"`
	StreamInterrupted     bool                   `json:"stream_interrupted,omitempty"`
	ScoreTotal            float64                `json:"score_total,omitempty"`
	ScoreBreakdown        map[string]float64     `json:"score_breakdown,omitempty"`
	CandidateGroups       []string               `json:"candidate_groups,omitempty"`
	CandidateExplanations []CandidateExplanation `json:"candidate_explanations,omitempty"`
	SelectedReason        string                 `json:"selected_reason,omitempty"`
	RequestMeta           RequestMeta            `json:"request_meta,omitempty"`
}

type RequestMeta struct {
	OriginalModelName     string                 `json:"original_model_name,omitempty"`
	UserUsingGroup        string                 `json:"user_using_group,omitempty"`
	PromptTokens          int                    `json:"prompt_tokens,omitempty"`
	PreConsumedQuota      int                    `json:"pre_consumed_quota,omitempty"`
	CandidateExplanations []CandidateExplanation `json:"candidate_explanations,omitempty"`
}

type CandidateExplanation struct {
	ChannelID           int                `json:"channel_id,omitempty"`
	ChannelName         string             `json:"channel_name,omitempty"`
	Group               string             `json:"group,omitempty"`
	UpstreamModel       string             `json:"upstream_model,omitempty"`
	ProviderProfile     string             `json:"provider_profile,omitempty"`
	ProxyMode           string             `json:"proxy_mode,omitempty"`
	RuntimeKey          RuntimeKey         `json:"runtime_key,omitempty"`
	Available           bool               `json:"available,omitempty"`
	RejectReason        string             `json:"reject_reason,omitempty"`
	ChannelStatus       int                `json:"channel_status,omitempty"`
	StatusReason        string             `json:"status_reason,omitempty"`
	BalanceInsufficient bool               `json:"balance_insufficient,omitempty"`
	ScoreTotal          float64            `json:"score_total,omitempty"`
	ScoreBreakdown      map[string]float64 `json:"score_breakdown,omitempty"`
	StickyMatched       bool               `json:"sticky_matched,omitempty"`
	Selected            bool               `json:"selected,omitempty"`
}

type RuntimeKey struct {
	RequestedModel        string `json:"requested_model,omitempty"`
	UpstreamModel         string `json:"upstream_model,omitempty"`
	ChannelID             int    `json:"channel_id,omitempty"`
	Group                 string `json:"group,omitempty"`
	EndpointType          string `json:"endpoint_type,omitempty"`
	CapabilityFingerprint string `json:"capability_fingerprint,omitempty"`
}

type ExportOptions struct {
	StableIDs bool
}

type RecordRepository interface {
	FindByRequestID(requestID string) ([]model.ModelExecutionRecord, error)
}

type GormRecordRepository struct {
	db *gorm.DB
}

func NewGormRecordRepository(db *gorm.DB) *GormRecordRepository {
	return &GormRecordRepository{db: db}
}

func (r *GormRecordRepository) FindByRequestID(requestID string) ([]model.ModelExecutionRecord, error) {
	requestID = strings.TrimSpace(requestID)
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("replay record repository has no db")
	}
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	var records []model.ModelExecutionRecord
	err := r.db.
		Where("request_id = ?", requestID).
		Order("created_at ASC").
		Order("attempt_index ASC").
		Order("id ASC").
		Find(&records).Error
	return records, err
}

type ArtifactExporter struct {
	repository RecordRepository
	options    ExportOptions
}

func NewArtifactExporter(repository RecordRepository, options ExportOptions) *ArtifactExporter {
	return &ArtifactExporter{
		repository: repository,
		options:    options,
	}
}

func (e *ArtifactExporter) ExportByRequestID(requestID string) (*Artifact, error) {
	if e == nil || e.repository == nil {
		return nil, fmt.Errorf("replay artifact exporter has no repository")
	}
	records, err := e.repository.FindByRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no model execution records found for request_id %q", strings.TrimSpace(requestID))
	}
	return ExportArtifact(records, e.options)
}

func (e *ArtifactExporter) WriteGoldenByRequestID(requestID string, path string) (*Artifact, error) {
	artifact, err := e.ExportByRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if err := WriteArtifact(path, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func ExportArtifact(records []model.ModelExecutionRecord, options ExportOptions) (*Artifact, error) {
	artifact := &Artifact{
		Version: ArtifactVersion,
		Kind:    "modelgateway_replay",
		Records: make([]Record, 0, len(records)),
	}
	for idx, record := range records {
		replayRecord, err := SanitizeModelExecutionRecord(record, idx, options)
		if err != nil {
			return nil, err
		}
		artifact.Records = append(artifact.Records, replayRecord)
	}
	return artifact, nil
}

func MarshalArtifact(artifact *Artifact) ([]byte, error) {
	if artifact == nil {
		return nil, fmt.Errorf("replay artifact is nil")
	}
	if err := artifact.Validate(); err != nil {
		return nil, err
	}
	data, err := common.Marshal(artifact)
	if err != nil {
		return nil, err
	}
	return formatJSON(data)
}

func WriteArtifact(path string, artifact *Artifact) error {
	data, err := MarshalArtifact(artifact)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func LoadArtifact(path string) (*Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact Artifact
	if err := common.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}
	if err := artifact.Validate(); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func (a *Artifact) Validate() error {
	if a == nil {
		return fmt.Errorf("replay artifact is nil")
	}
	if a.Version != ArtifactVersion {
		return fmt.Errorf("unsupported replay artifact version %d", a.Version)
	}
	if strings.TrimSpace(a.Kind) != "modelgateway_replay" {
		return fmt.Errorf("unsupported replay artifact kind %q", a.Kind)
	}
	for idx, record := range a.Records {
		if record.RequestID != "" {
			return fmt.Errorf("record %d contains request_id", idx)
		}
		if record.UserID != 0 || record.TokenID != 0 {
			return fmt.Errorf("record %d contains user_id or token_id", idx)
		}
		if record.RequestMeta.PromptTokens != 0 || record.RequestMeta.PreConsumedQuota != 0 {
			return fmt.Errorf("record %d contains token or quota request metadata", idx)
		}
		if strings.Contains(record.ChannelName, "sk-") || strings.Contains(record.ActualChannelName, "sk-") {
			return fmt.Errorf("record %d contains a secret-like channel name", idx)
		}
	}
	return nil
}

func SanitizeModelExecutionRecord(record model.ModelExecutionRecord, index int, options ExportOptions) (Record, error) {
	scoreBreakdown, err := parseFloatMap(record.ScoreBreakdown)
	if err != nil {
		return Record{}, fmt.Errorf("parse score_breakdown: %w", err)
	}
	candidateGroups, err := parseStringSlice(record.CandidateGroups)
	if err != nil {
		return Record{}, fmt.Errorf("parse candidate_groups: %w", err)
	}
	requestMeta, err := parseRequestMeta(record.RequestMeta)
	if err != nil {
		return Record{}, fmt.Errorf("parse request_meta: %w", err)
	}
	idMap := sanitizedIDMap(record, requestMeta.CandidateExplanations, index, options)
	candidateExplanations := sanitizeCandidateExplanations(requestMeta.CandidateExplanations, index, options, idMap)
	requestMeta.PromptTokens = 0
	requestMeta.PreConsumedQuota = 0
	requestMeta.CandidateExplanations = nil

	out := Record{
		CreatedAt:             0,
		AttemptIndex:          record.AttemptIndex,
		RequestedGroup:        sanitizeLabel(record.RequestedGroup),
		SelectedGroup:         sanitizeLabel(record.SelectedGroup),
		ActualGroup:           sanitizeLabel(record.ActualGroup),
		RequestedModel:        sanitizeLabel(firstNonEmpty(record.RequestedModel, requestMeta.OriginalModelName)),
		ChannelID:             sanitizeIDWithMap(record.ChannelId, idMap),
		ChannelName:           sanitizeName(record.ChannelName, "channel", sanitizeIDWithMap(record.ChannelId, idMap)),
		ActualChannelID:       sanitizeIDWithMap(record.ActualChannelId, idMap),
		ActualChannelName:     sanitizeName(record.ActualChannelName, "actual_channel", sanitizeIDWithMap(record.ActualChannelId, idMap)),
		EndpointType:          sanitizeLabel(record.EndpointType),
		PolicyMode:            sanitizeLabel(record.PolicyMode),
		AutoMode:              sanitizeLabel(record.AutoMode),
		Strategy:              sanitizeLabel(record.Strategy),
		Shadow:                record.Shadow,
		SmartHandled:          record.SmartHandled,
		FallbackUsed:          record.FallbackUsed,
		Success:               record.Success,
		StatusCode:            record.StatusCode,
		ErrorCode:             sanitizeLabel(record.ErrorCode),
		ErrorType:             sanitizeLabel(record.ErrorType),
		DurationMs:            record.DurationMs,
		TTFTMs:                record.TTFTMs,
		StreamInterrupted:     record.StreamInterrupted,
		ScoreTotal:            record.ScoreTotal,
		ScoreBreakdown:        scoreBreakdown,
		CandidateGroups:       sanitizeGroups(candidateGroups),
		CandidateExplanations: candidateExplanations,
		SelectedReason:        sanitizeLabel(record.SelectedReason),
		RequestMeta: RequestMeta{
			OriginalModelName: sanitizeLabel(requestMeta.OriginalModelName),
			UserUsingGroup:    sanitizeLabel(requestMeta.UserUsingGroup),
		},
	}
	if out.RequestedModel == "" {
		out.RequestedModel = "unknown-model"
	}
	if out.SelectedGroup == "" {
		out.SelectedGroup = out.RequestedGroup
	}
	return out, nil
}

func sanitizeCandidateExplanations(candidates []CandidateExplanation, recordIndex int, options ExportOptions, idMap map[int]int) []CandidateExplanation {
	if len(candidates) == 0 {
		return nil
	}
	out := make([]CandidateExplanation, 0, len(candidates))
	for idx, candidate := range candidates {
		channelID := sanitizeIDWithMap(candidate.ChannelID, idMap)
		if channelID == 0 && candidate.RuntimeKey.ChannelID > 0 {
			channelID = sanitizeIDWithMap(candidate.RuntimeKey.ChannelID, idMap)
		}
		if channelID == 0 {
			channelID = sanitizedID(candidate.ChannelID, recordIndex, idx+3, options.StableIDs)
		}
		sanitized := CandidateExplanation{
			ChannelID:           channelID,
			ChannelName:         sanitizeName(candidate.ChannelName, "candidate_channel", channelID),
			Group:               sanitizeLabel(candidate.Group),
			UpstreamModel:       sanitizeLabel(candidate.UpstreamModel),
			ProviderProfile:     sanitizeLabel(candidate.ProviderProfile),
			ProxyMode:           sanitizeLabel(candidate.ProxyMode),
			RuntimeKey:          sanitizeRuntimeKey(candidate.RuntimeKey, recordIndex, idx+3, options, idMap),
			Available:           candidate.Available,
			RejectReason:        sanitizeLabel(candidate.RejectReason),
			ChannelStatus:       candidate.ChannelStatus,
			StatusReason:        sanitizeLabel(candidate.StatusReason),
			BalanceInsufficient: candidate.BalanceInsufficient,
			ScoreTotal:          candidate.ScoreTotal,
			ScoreBreakdown:      copyFloatMap(candidate.ScoreBreakdown),
			StickyMatched:       candidate.StickyMatched,
			Selected:            candidate.Selected,
		}
		if sanitized.ChannelID == 0 {
			sanitized.ChannelID = sanitized.RuntimeKey.ChannelID
		}
		if sanitized.Group == "" {
			sanitized.Group = sanitized.RuntimeKey.Group
		}
		if sanitized.UpstreamModel == "" {
			sanitized.UpstreamModel = sanitized.RuntimeKey.UpstreamModel
		}
		out = append(out, sanitized)
	}
	return out
}

func sanitizeRuntimeKey(key RuntimeKey, recordIndex int, offset int, options ExportOptions, idMap map[int]int) RuntimeKey {
	return RuntimeKey{
		RequestedModel:        sanitizeLabel(key.RequestedModel),
		UpstreamModel:         sanitizeLabel(key.UpstreamModel),
		ChannelID:             firstPositiveInt(sanitizeIDWithMap(key.ChannelID, idMap), sanitizedID(key.ChannelID, recordIndex, offset, options.StableIDs)),
		Group:                 sanitizeLabel(key.Group),
		EndpointType:          sanitizeLabel(key.EndpointType),
		CapabilityFingerprint: sanitizeLabel(key.CapabilityFingerprint),
	}
}

func formatJSON(data []byte) ([]byte, error) {
	var v any
	if err := common.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return common.MarshalIndent(v, "", "  ")
}

func parseFloatMap(raw string) (map[string]float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out map[string]float64
	if err := common.UnmarshalJsonStr(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func parseStringSlice(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var out []string
	if err := common.UnmarshalJsonStr(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func parseRequestMeta(raw string) (RequestMeta, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RequestMeta{}, nil
	}
	var out RequestMeta
	if err := common.UnmarshalJsonStr(raw, &out); err != nil {
		return RequestMeta{}, err
	}
	return out, nil
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[sanitizeLabel(key)] = value
	}
	return out
}

func sanitizeGroups(groups []string) []string {
	out := make([]string, 0, len(groups))
	seen := map[string]struct{}{}
	for _, group := range groups {
		group = sanitizeLabel(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		out = append(out, group)
	}
	return out
}

func sanitizeLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return common.MaskSensitiveInfo(value)
}

func sanitizeName(value string, prefix string, id int) string {
	value = sanitizeLabel(value)
	if value == "" || value == "***" || strings.Contains(value, "sk-") || strings.Contains(strings.ToLower(value), "api_key") {
		if id <= 0 {
			return prefix
		}
		return fmt.Sprintf("%s_%d", prefix, id)
	}
	return value
}

func sanitizedID(id int, index int, offset int, stable bool) int {
	if id <= 0 {
		return 0
	}
	if stable {
		return index*10 + offset
	}
	return id
}

func sanitizedIDMap(record model.ModelExecutionRecord, candidates []CandidateExplanation, index int, options ExportOptions) map[int]int {
	ids := make([]int, 0, len(candidates)+2)
	if record.ChannelId > 0 {
		ids = append(ids, record.ChannelId)
	}
	if record.ActualChannelId > 0 && record.ActualChannelId != record.ChannelId {
		ids = append(ids, record.ActualChannelId)
	}
	for _, candidate := range candidates {
		if candidate.ChannelID > 0 && !containsInt(ids, candidate.ChannelID) {
			ids = append(ids, candidate.ChannelID)
		}
		if candidate.RuntimeKey.ChannelID > 0 && !containsInt(ids, candidate.RuntimeKey.ChannelID) {
			ids = append(ids, candidate.RuntimeKey.ChannelID)
		}
	}
	idMap := make(map[int]int, len(ids))
	for idx, id := range ids {
		idMap[id] = sanitizedID(id, index, idx+1, options.StableIDs)
	}
	return idMap
}

func sanitizeIDWithMap(id int, idMap map[int]int) int {
	if id <= 0 {
		return 0
	}
	if mapped, ok := idMap[id]; ok {
		return mapped
	}
	return id
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
