package model

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	CodexApplicationEnvironmentDefaultSeedCount = 100
	CodexApplicationEnvironmentSystemSource     = "system_seed"
)

type CodexApplicationEnvironment struct {
	Id           int    `json:"id"`
	Name         string `json:"name" gorm:"type:varchar(128);not null;uniqueIndex"`
	Platform     string `json:"platform" gorm:"type:varchar(64);index"`
	AppVersion   string `json:"app_version" gorm:"type:varchar(64)"`
	UserAgent    string `json:"user_agent" gorm:"type:varchar(255)"`
	Originator   string `json:"originator" gorm:"type:varchar(128)"`
	SessionID    string `json:"session_id" gorm:"type:varchar(128)"`
	WindowID     string `json:"window_id" gorm:"type:varchar(128)"`
	BetaFeatures string `json:"beta_features" gorm:"type:varchar(255)"`
	TurnMetadata string `json:"turn_metadata" gorm:"type:text"`
	HeadersJSON  string `json:"headers_json" gorm:"type:text"`
	Enabled      bool   `json:"enabled" gorm:"default:true;index"`
	Source       string `json:"source" gorm:"type:varchar(64);index"`
	Remark       string `json:"remark" gorm:"type:varchar(255)"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
}

func (CodexApplicationEnvironment) TableName() string {
	return "codex_application_environments"
}

func (env *CodexApplicationEnvironment) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if env.CreatedAt == 0 {
		env.CreatedAt = now
	}
	env.UpdatedAt = now
	env.normalize()
	return nil
}

func (env *CodexApplicationEnvironment) BeforeUpdate(tx *gorm.DB) error {
	env.UpdatedAt = common.GetTimestamp()
	env.normalize()
	return nil
}

func (env *CodexApplicationEnvironment) normalize() {
	env.Name = strings.TrimSpace(env.Name)
	env.Platform = strings.TrimSpace(env.Platform)
	env.AppVersion = strings.TrimSpace(env.AppVersion)
	env.UserAgent = strings.TrimSpace(env.UserAgent)
	env.Originator = strings.TrimSpace(env.Originator)
	env.SessionID = strings.TrimSpace(env.SessionID)
	env.WindowID = strings.TrimSpace(env.WindowID)
	env.BetaFeatures = strings.TrimSpace(env.BetaFeatures)
	env.TurnMetadata = strings.TrimSpace(env.TurnMetadata)
	env.HeadersJSON = strings.TrimSpace(env.HeadersJSON)
	env.Source = strings.TrimSpace(env.Source)
	env.Remark = strings.TrimSpace(env.Remark)
	if env.Source == "" {
		env.Source = "custom"
	}
}

type CodexApplicationEnvironmentResponse struct {
	Id           int               `json:"id"`
	Name         string            `json:"name"`
	Platform     string            `json:"platform,omitempty"`
	AppVersion   string            `json:"app_version,omitempty"`
	UserAgent    string            `json:"user_agent,omitempty"`
	Originator   string            `json:"originator,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	WindowID     string            `json:"window_id,omitempty"`
	BetaFeatures string            `json:"beta_features,omitempty"`
	TurnMetadata string            `json:"turn_metadata,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Enabled      bool              `json:"enabled"`
	Source       string            `json:"source,omitempty"`
	Remark       string            `json:"remark,omitempty"`
	CreatedAt    int64             `json:"created_at,omitempty"`
	UpdatedAt    int64             `json:"updated_at,omitempty"`
}

var codexApplicationEnvironmentCache sync.Map // map[int]CodexApplicationEnvironment

func (env *CodexApplicationEnvironment) ToResponse() CodexApplicationEnvironmentResponse {
	if env == nil {
		return CodexApplicationEnvironmentResponse{}
	}
	return CodexApplicationEnvironmentResponse{
		Id:           env.Id,
		Name:         env.Name,
		Platform:     env.Platform,
		AppVersion:   env.AppVersion,
		UserAgent:    env.UserAgent,
		Originator:   env.Originator,
		SessionID:    env.SessionID,
		WindowID:     env.WindowID,
		BetaFeatures: env.BetaFeatures,
		TurnMetadata: env.TurnMetadata,
		Headers:      env.BuildHeaders(),
		Enabled:      env.Enabled,
		Source:       env.Source,
		Remark:       env.Remark,
		CreatedAt:    env.CreatedAt,
		UpdatedAt:    env.UpdatedAt,
	}
}

func (env *CodexApplicationEnvironment) BuildHeaders() map[string]string {
	if env == nil {
		return nil
	}
	headers := make(map[string]string)
	add := func(key, value string) {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return
		}
		headers[key] = value
	}
	add("User-Agent", env.UserAgent)
	add("originator", env.Originator)
	add("session_id", env.SessionID)
	add("x-codex-window-id", env.WindowID)
	add("x-codex-beta-features", env.BetaFeatures)
	turnMetadata := strings.TrimSpace(env.TurnMetadata)
	if turnMetadata == "" {
		metadata := map[string]any{}
		if env.Name != "" {
			metadata["environment_name"] = env.Name
		}
		if env.Platform != "" {
			metadata["platform"] = env.Platform
		}
		if env.AppVersion != "" {
			metadata["app_version"] = env.AppVersion
		}
		if env.UserAgent != "" {
			metadata["user_agent"] = env.UserAgent
		}
		if env.Originator != "" {
			metadata["originator"] = env.Originator
		}
		if env.SessionID != "" {
			metadata["session_id"] = env.SessionID
		}
		if env.WindowID != "" {
			metadata["window_id"] = env.WindowID
		}
		if env.BetaFeatures != "" {
			metadata["beta_features"] = env.BetaFeatures
		}
		if env.Source != "" {
			metadata["source"] = env.Source
		}
		if len(metadata) > 0 {
			if metadataBytes, err := common.Marshal(metadata); err == nil {
				turnMetadata = string(metadataBytes)
			}
		}
	}
	add("x-codex-turn-metadata", turnMetadata)
	if extra := parseCodexApplicationEnvironmentHeaderJSON(env.HeadersJSON); len(extra) > 0 {
		for key, value := range extra {
			add(key, value)
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func parseCodexApplicationEnvironmentHeaderJSON(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload map[string]any
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return nil
	}
	headers := make(map[string]string, len(payload))
	for key, value := range payload {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		valueStr := strings.TrimSpace(fmt.Sprint(value))
		if valueStr == "" || valueStr == "<nil>" {
			continue
		}
		headers[key] = valueStr
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func cacheCodexApplicationEnvironment(env *CodexApplicationEnvironment) {
	if env == nil || env.Id <= 0 {
		return
	}
	codexApplicationEnvironmentCache.Store(env.Id, *env)
}

func clearCodexApplicationEnvironmentCache(id int) {
	if id <= 0 {
		return
	}
	codexApplicationEnvironmentCache.Delete(id)
}

func GetCodexApplicationEnvironmentByID(id int) (*CodexApplicationEnvironment, error) {
	if id <= 0 {
		return nil, nil
	}
	if cached, ok := codexApplicationEnvironmentCache.Load(id); ok {
		if env, ok := cached.(CodexApplicationEnvironment); ok {
			cloned := env
			return &cloned, nil
		}
		codexApplicationEnvironmentCache.Delete(id)
	}
	var env CodexApplicationEnvironment
	if err := DB.First(&env, id).Error; err != nil {
		return nil, err
	}
	cacheCodexApplicationEnvironment(&env)
	cloned := env
	return &cloned, nil
}

func ListCodexApplicationEnvironments(includeDisabled bool) ([]CodexApplicationEnvironment, error) {
	query := DB.Model(&CodexApplicationEnvironment{})
	if !includeDisabled {
		query = query.Where("enabled = ?", true)
	}
	envs := make([]CodexApplicationEnvironment, 0, CodexApplicationEnvironmentDefaultSeedCount)
	if err := query.Order("id ASC").Find(&envs).Error; err != nil {
		return nil, err
	}
	for i := range envs {
		cacheCodexApplicationEnvironment(&envs[i])
	}
	return envs, nil
}

func SelectCodexApplicationEnvironmentForIndex(index int) (*CodexApplicationEnvironment, error) {
	envs, err := ListCodexApplicationEnvironments(false)
	if err != nil {
		return nil, err
	}
	if len(envs) == 0 {
		return nil, nil
	}
	if index < 0 {
		index = 0
	}
	env := envs[index%len(envs)]
	return &env, nil
}

func AssignCodexApplicationEnvironments(channel *Channel, credentialIndexes []int) error {
	if channel == nil || len(credentialIndexes) == 0 {
		return nil
	}
	envs, err := ListCodexApplicationEnvironments(false)
	if err != nil {
		return err
	}
	if len(envs) == 0 {
		return nil
	}
	if channel.ChannelInfo.MultiKeyCodexEnvironmentIDs == nil {
		channel.ChannelInfo.MultiKeyCodexEnvironmentIDs = make(map[int]int)
	}
	for _, credentialIndex := range credentialIndexes {
		if credentialIndex < 0 {
			continue
		}
		env := envs[credentialIndex%len(envs)]
		channel.ChannelInfo.MultiKeyCodexEnvironmentIDs[credentialIndex] = env.Id
	}
	return nil
}

func CleanupCodexApplicationEnvironmentIDs(info *ChannelInfo, maxSize int) {
	if info == nil || info.MultiKeyCodexEnvironmentIDs == nil {
		return
	}
	for idx := range info.MultiKeyCodexEnvironmentIDs {
		if idx < 0 || idx >= maxSize {
			delete(info.MultiKeyCodexEnvironmentIDs, idx)
		}
	}
	if len(info.MultiKeyCodexEnvironmentIDs) == 0 {
		info.MultiKeyCodexEnvironmentIDs = nil
	}
}

func SeedDefaultCodexApplicationEnvironments() error {
	var count int64
	if err := DB.Model(&CodexApplicationEnvironment{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	envs := buildDefaultCodexApplicationEnvironments()
	if len(envs) == 0 {
		return nil
	}
	if err := DB.Create(&envs).Error; err != nil {
		return err
	}
	for i := range envs {
		cacheCodexApplicationEnvironment(&envs[i])
	}
	return nil
}

func buildDefaultCodexApplicationEnvironments() []CodexApplicationEnvironment {
	platforms := []string{"macOS", "Windows", "Linux", "iOS", "Android"}
	appVersions := []string{"0.133.0", "0.134.1", "0.135.0-alpha.1", "0.135.0", "0.136.0"}
	userAgents := []string{
		"Codex Desktop/0.133.0",
		"Codex Desktop/0.134.1",
		"Codex Desktop/0.135.0",
		"Codex CLI/0.44.0",
		"Codex CLI/0.45.0",
	}
	originators := []string{"Codex CLI", "Codex Desktop", "codex_cli_rs", "codex_cli_js"}
	betaFeatures := []string{
		"terminal_resize_reflow",
		"responses_compact",
		"stream_options",
		"web_search",
		"image_generation",
	}
	envs := make([]CodexApplicationEnvironment, 0, CodexApplicationEnvironmentDefaultSeedCount)
	for i := 0; i < CodexApplicationEnvironmentDefaultSeedCount; i++ {
		idx := i + 1
		sessionID := fmt.Sprintf("sess-env-%03d", idx)
		windowID := fmt.Sprintf("win-env-%03d", idx)
		platform := platforms[i%len(platforms)]
		appVersion := appVersions[i%len(appVersions)]
		userAgent := userAgents[i%len(userAgents)]
		originator := originators[i%len(originators)]
		betaFeature := betaFeatures[i%len(betaFeatures)]
		turnMetadata := map[string]any{
			"environment_id": idx,
			"environment_name": fmt.Sprintf("codex-env-%03d", idx),
			"platform": platform,
			"app_version": appVersion,
			"session_id": sessionID,
			"window_id": windowID,
			"originator": originator,
		}
		turnMetadataJSON, _ := common.Marshal(turnMetadata)
		envs = append(envs, CodexApplicationEnvironment{
			Name:         fmt.Sprintf("codex-env-%03d", idx),
			Platform:     platform,
			AppVersion:   appVersion,
			UserAgent:    userAgent,
			Originator:   originator,
			SessionID:    sessionID,
			WindowID:     windowID,
			BetaFeatures: betaFeature,
			TurnMetadata: string(turnMetadataJSON),
			Enabled:      true,
			Source:       CodexApplicationEnvironmentSystemSource,
			Remark:       fmt.Sprintf("seeded environment %03d", idx),
		})
	}
	return envs
}

func UpdateCodexApplicationEnvironment(env *CodexApplicationEnvironment) error {
	if env == nil || env.Id <= 0 {
		return fmt.Errorf("codex application environment is invalid")
	}
	if err := DB.Save(env).Error; err != nil {
		return err
	}
	cacheCodexApplicationEnvironment(env)
	return nil
}

func DeleteCodexApplicationEnvironment(id int) error {
	if id <= 0 {
		return nil
	}
	if err := DB.Delete(&CodexApplicationEnvironment{}, id).Error; err != nil {
		return err
	}
	clearCodexApplicationEnvironmentCache(id)
	return nil
}
