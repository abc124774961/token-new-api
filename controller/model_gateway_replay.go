package controller

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/replay"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	modelGatewayReplayBatchDefaultHours = 24
	modelGatewayReplayBatchDefaultLimit = 20
	modelGatewayReplayBatchMaxLimit     = 200
)

type ModelGatewayReplayBatchExport struct {
	Kind      string                            `json:"kind"`
	Filters   ModelGatewayReplayBatchFilters    `json:"filters"`
	Manifest  ModelGatewayReplayBatchManifest   `json:"manifest"`
	Artifacts []ModelGatewayReplayBatchArtifact `json:"artifacts"`
}

type ModelGatewayReplayBatchFilters struct {
	Hours      int      `json:"hours"`
	StartTime  int64    `json:"start_time"`
	EndTime    int64    `json:"end_time"`
	Limit      int      `json:"limit"`
	Model      string   `json:"model,omitempty"`
	Group      string   `json:"group,omitempty"`
	ChannelID  int      `json:"channel_id,omitempty"`
	ErrorType  string   `json:"error_type,omitempty"`
	RequestIDs []string `json:"request_ids,omitempty"`
	Success    *bool    `json:"success,omitempty"`
	StableIDs  bool     `json:"stable_ids"`
}

type ModelGatewayReplayBatchManifest struct {
	GeneratedAt   int64                                 `json:"generated_at"`
	ArtifactCount int                                   `json:"artifact_count"`
	RecordCount   int                                   `json:"record_count"`
	FailedCount   int                                   `json:"failed_count"`
	Items         []ModelGatewayReplayBatchManifestItem `json:"items"`
}

type ModelGatewayReplayBatchManifestItem struct {
	RequestID   string `json:"request_id"`
	Filename    string `json:"filename"`
	RecordCount int    `json:"record_count"`
	Error       string `json:"error,omitempty"`
}

type ModelGatewayReplayBatchArtifact struct {
	RequestID string           `json:"request_id"`
	Filename  string           `json:"filename"`
	Artifact  *replay.Artifact `json:"artifact,omitempty"`
}

func ExportModelGatewayReplay(c *gin.Context) {
	requestID := strings.TrimSpace(c.Query("request_id"))
	if requestID == "" {
		common.ApiErrorMsg(c, "request_id is required")
		return
	}

	exporter := replay.NewArtifactExporter(
		replay.NewGormRecordRepository(model.DB),
		replay.ExportOptions{StableIDs: c.Query("stable_ids") == "true"},
	)
	artifact, err := exporter.ExportByRequestID(requestID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if c.Query("download") == "true" {
		data, err := replay.MarshalArtifact(artifact)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filename := fmt.Sprintf("modelgateway-replay-%s.json", safeReplayFilenamePart(requestID))
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Data(http.StatusOK, "application/json; charset=utf-8", append(data, '\n'))
		return
	}

	common.ApiSuccess(c, gin.H{
		"request_id": requestID,
		"count":      len(artifact.Records),
		"artifact":   artifact,
	})
}

func ExportModelGatewayReplayBatch(c *gin.Context) {
	options, err := parseModelGatewayReplayBatchOptions(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if model.DB == nil {
		common.ApiErrorMsg(c, "database is not initialized")
		return
	}

	requestIDs, err := findModelGatewayReplayBatchRequestIDs(options)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	exporter := replay.NewArtifactExporter(
		replay.NewGormRecordRepository(model.DB),
		replay.ExportOptions{StableIDs: options.StableIDs},
	)
	result := ModelGatewayReplayBatchExport{
		Kind:    "modelgateway_replay_batch",
		Filters: options,
		Manifest: ModelGatewayReplayBatchManifest{
			GeneratedAt: common.GetTimestamp(),
			Items:       make([]ModelGatewayReplayBatchManifestItem, 0, len(requestIDs)),
		},
		Artifacts: make([]ModelGatewayReplayBatchArtifact, 0, len(requestIDs)),
	}
	for _, requestID := range requestIDs {
		filename := fmt.Sprintf("modelgateway-replay-%s.json", safeReplayFilenamePart(requestID))
		item := ModelGatewayReplayBatchManifestItem{
			RequestID: requestID,
			Filename:  filename,
		}
		artifact, err := exporter.ExportByRequestID(requestID)
		if err != nil {
			item.Error = err.Error()
			result.Manifest.FailedCount++
			result.Manifest.Items = append(result.Manifest.Items, item)
			continue
		}
		item.RecordCount = len(artifact.Records)
		result.Manifest.RecordCount += len(artifact.Records)
		result.Manifest.ArtifactCount++
		result.Manifest.Items = append(result.Manifest.Items, item)
		result.Artifacts = append(result.Artifacts, ModelGatewayReplayBatchArtifact{
			RequestID: requestID,
			Filename:  filename,
			Artifact:  artifact,
		})
	}
	recordModelGatewayReplayExportAudit(c, options, result.Manifest)

	if c.Query("download") == "true" {
		data, err := common.Marshal(result)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filename := fmt.Sprintf("modelgateway-replay-batch-%d.json", result.Manifest.GeneratedAt)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		c.Data(http.StatusOK, "application/json; charset=utf-8", append(data, '\n'))
		return
	}

	common.ApiSuccess(c, result)
}

func safeReplayFilenamePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := b.String()
	if strings.Trim(out, "_") == "" {
		return "unknown"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

func parseModelGatewayReplayBatchOptions(c *gin.Context) (ModelGatewayReplayBatchFilters, error) {
	hours := normalizeReplayBatchInt(c.Query("hours"), modelGatewayReplayBatchDefaultHours, 1, 24*30)
	limit := normalizeReplayBatchInt(c.Query("limit"), modelGatewayReplayBatchDefaultLimit, 1, modelGatewayReplayBatchMaxLimit)
	endTime := common.GetTimestamp()
	startTime := endTime - int64(hours*3600)
	if raw := strings.TrimSpace(c.Query("start_time")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return ModelGatewayReplayBatchFilters{}, fmt.Errorf("invalid start_time")
		}
		startTime = value
	}
	if raw := strings.TrimSpace(c.Query("end_time")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return ModelGatewayReplayBatchFilters{}, fmt.Errorf("invalid end_time")
		}
		endTime = value
	}
	if startTime > endTime {
		return ModelGatewayReplayBatchFilters{}, fmt.Errorf("start_time cannot be greater than end_time")
	}
	options := ModelGatewayReplayBatchFilters{
		Hours:      hours,
		StartTime:  startTime,
		EndTime:    endTime,
		Limit:      limit,
		Model:      strings.TrimSpace(c.Query("model")),
		Group:      strings.TrimSpace(c.Query("group")),
		ErrorType:  strings.TrimSpace(c.Query("error_type")),
		RequestIDs: splitReplayBatchRequestIDs(c.Query("request_ids")),
		StableIDs:  c.Query("stable_ids") == "true",
	}
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return ModelGatewayReplayBatchFilters{}, fmt.Errorf("invalid channel_id")
		}
		options.ChannelID = value
	}
	if raw := strings.TrimSpace(c.Query("success")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return ModelGatewayReplayBatchFilters{}, fmt.Errorf("invalid success")
		}
		options.Success = &value
	}
	return options, nil
}

func findModelGatewayReplayBatchRequestIDs(options ModelGatewayReplayBatchFilters) ([]string, error) {
	if len(options.RequestIDs) > 0 {
		if len(options.RequestIDs) > options.Limit {
			return options.RequestIDs[:options.Limit], nil
		}
		return options.RequestIDs, nil
	}
	query := model.DB.Model(&model.ModelExecutionRecord{}).
		Where("request_id <> ''").
		Where("created_at >= ? AND created_at <= ?", options.StartTime, options.EndTime)
	if options.Model != "" {
		query = query.Where("requested_model = ?", options.Model)
	}
	if options.Group != "" {
		query = query.Where("(requested_group = ? OR selected_group = ? OR actual_group = ?)", options.Group, options.Group, options.Group)
	}
	if options.ChannelID > 0 {
		query = query.Where("(channel_id = ? OR actual_channel_id = ?)", options.ChannelID, options.ChannelID)
	}
	if options.ErrorType != "" {
		query = query.Where("error_type = ?", options.ErrorType)
	}
	if options.Success != nil {
		query = withModelGatewayReplayAttemptRecords(query)
		query = query.Where("success = ?", *options.Success)
	}
	var rows []struct {
		RequestID string `gorm:"column:request_id"`
		Latest    int64  `gorm:"column:latest"`
	}
	if err := query.
		Select("request_id, MAX(created_at) AS latest").
		Group("request_id").
		Order("latest DESC").
		Limit(options.Limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	requestIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		requestID := strings.TrimSpace(row.RequestID)
		if requestID != "" {
			requestIDs = append(requestIDs, requestID)
		}
	}
	return requestIDs, nil
}

func withModelGatewayReplayAttemptRecords(query *gorm.DB) *gorm.DB {
	return query.Where(
		"success = ? OR stream_interrupted = ? OR status_code <> ? OR error_code <> ? OR error_type <> ? OR duration_ms <> ? OR ttft_ms <> ?",
		true,
		true,
		0,
		"",
		"",
		0,
		0,
	)
}

func splitReplayBatchRequestIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		requestID := strings.TrimSpace(part)
		if requestID == "" {
			continue
		}
		if len(requestID) > 64 {
			requestID = requestID[:64]
		}
		if _, ok := seen[requestID]; ok {
			continue
		}
		seen[requestID] = struct{}{}
		out = append(out, requestID)
	}
	sort.Strings(out)
	return out
}

func normalizeReplayBatchInt(raw string, fallback int, minValue int, maxValue int) int {
	value := fallback
	if strings.TrimSpace(raw) != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			value = parsed
		}
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func recordModelGatewayReplayExportAudit(c *gin.Context, options ModelGatewayReplayBatchFilters, manifest ModelGatewayReplayBatchManifest) {
	if model.LOG_DB == nil {
		return
	}
	adminInfo := map[string]interface{}{
		"kind":           "modelgateway_replay_batch_export",
		"filters":        options,
		"artifact_count": manifest.ArtifactCount,
		"record_count":   manifest.RecordCount,
		"failed_count":   manifest.FailedCount,
		"request_hash":   modelGatewayReplayRequestHash(options.RequestIDs),
		"download":       c.Query("download") == "true",
		"stable_ids":     options.StableIDs,
		"client_ip":      c.ClientIP(),
	}
	model.RecordLogWithAdminInfo(c.GetInt("id"), model.LogTypeManage, "导出智能模型网关 replay 批量样本", adminInfo)
}

func modelGatewayReplayRequestHash(requestIDs []string) string {
	if len(requestIDs) == 0 {
		return ""
	}
	data, err := common.Marshal(requestIDs)
	if err != nil {
		return ""
	}
	sum := common.Sha256Raw(data)
	return hex.EncodeToString(sum)
}
