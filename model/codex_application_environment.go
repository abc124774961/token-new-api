package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/codexauth"
	"gorm.io/gorm"
)

const (
	CodexApplicationEnvironmentSystemSource      = "system_seed"
	CodexApplicationEnvironmentRealRequestSource = "real_request"
	CodexApplicationEnvironmentCustomSource      = "custom"

	codexApplicationEnvironmentLogScanLimit = 1000
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
	add("x-codex-beta-features", env.BetaFeatures)
	if extra := parseCodexApplicationEnvironmentHeaderJSON(env.HeadersJSON); len(extra) > 0 {
		for key, value := range extra {
			if isStableCodexEnvironmentHeader(key) {
				add(canonicalCodexEnvironmentHeaderName(key), value)
			}
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
		query = query.Where("enabled = ? AND (source IS NULL OR source = ? OR source <> ?)", true, "", CodexApplicationEnvironmentSystemSource)
	}
	envs := make([]CodexApplicationEnvironment, 0)
	if err := query.Order("id ASC").Find(&envs).Error; err != nil {
		return nil, err
	}
	for i := range envs {
		cacheCodexApplicationEnvironment(&envs[i])
	}
	return envs, nil
}

func ListRealCodexApplicationEnvironments() ([]CodexApplicationEnvironment, error) {
	envs := make([]CodexApplicationEnvironment, 0)
	err := DB.Model(&CodexApplicationEnvironment{}).
		Where("enabled = ? AND source = ?", true, CodexApplicationEnvironmentRealRequestSource).
		Order("id ASC").
		Find(&envs).Error
	if err != nil {
		return nil, err
	}
	for i := range envs {
		cacheCodexApplicationEnvironment(&envs[i])
	}
	return envs, nil
}

func SelectCodexApplicationEnvironmentForIndex(index int) (*CodexApplicationEnvironment, error) {
	envs, err := ListRealCodexApplicationEnvironments()
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
	envs, err := ListRealCodexApplicationEnvironments()
	if err != nil {
		return err
	}
	if len(envs) == 0 {
		return nil
	}
	assignIndex := 0
	for _, credentialIndex := range credentialIndexes {
		if credentialIndex < 0 {
			continue
		}
		keys := channel.GetKeys()
		if credentialIndex >= len(keys) || !shouldBindCodexApplicationEnvironment(channel, keys[credentialIndex]) {
			continue
		}
		accountUniqueKey := codexApplicationEnvironmentAccountUniqueKey(channel, credentialIndex, keys[credentialIndex])
		if accountUniqueKey == "" {
			continue
		}
		if channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys == nil {
			channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys = make(map[string]int)
		}
		env := envs[assignIndex%len(envs)]
		assignIndex++
		channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys[accountUniqueKey] = env.Id
		if channel.ChannelInfo.MultiKeyCodexEnvironmentIDs != nil {
			delete(channel.ChannelInfo.MultiKeyCodexEnvironmentIDs, credentialIndex)
		}
	}
	CleanupCodexApplicationEnvironmentIDs(&channel.ChannelInfo, len(channel.GetKeys()))
	return nil
}

func CleanupCodexApplicationEnvironmentIDs(info *ChannelInfo, maxSize int) {
	if info == nil {
		return
	}
	if info.MultiKeyCodexEnvironmentIDs != nil {
		for idx := range info.MultiKeyCodexEnvironmentIDs {
			if idx < 0 || idx >= maxSize {
				delete(info.MultiKeyCodexEnvironmentIDs, idx)
			}
		}
		if len(info.MultiKeyCodexEnvironmentIDs) == 0 {
			info.MultiKeyCodexEnvironmentIDs = nil
		}
	}
	if info.MultiKeyCodexEnvironmentAccountUniqueKeys != nil {
		for accountUniqueKey, environmentID := range info.MultiKeyCodexEnvironmentAccountUniqueKeys {
			if strings.TrimSpace(accountUniqueKey) == "" || environmentID <= 0 {
				delete(info.MultiKeyCodexEnvironmentAccountUniqueKeys, accountUniqueKey)
			}
		}
		if len(info.MultiKeyCodexEnvironmentAccountUniqueKeys) == 0 {
			info.MultiKeyCodexEnvironmentAccountUniqueKeys = nil
		}
	}
}

func SyncCodexApplicationEnvironments() error {
	if err := disableSystemSeedCodexApplicationEnvironments(); err != nil {
		return err
	}
	if LOG_DB != nil {
		if _, err := CollectCodexApplicationEnvironmentsFromLogs(codexApplicationEnvironmentLogScanLimit); err != nil {
			common.SysLog(fmt.Sprintf("failed to collect real codex environments from logs: %v", err))
		}
	}
	return ReconcileCodexApplicationEnvironmentBindings()
}

func SeedDefaultCodexApplicationEnvironments() error {
	return SyncCodexApplicationEnvironments()
}

func CollectCodexApplicationEnvironmentsFromLogs(limit int) (int, error) {
	if DB == nil || LOG_DB == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = codexApplicationEnvironmentLogScanLimit
	}
	rows := make([]Log, 0, limit)
	if err := LOG_DB.Model(&Log{}).
		Select("id", "other").
		Where("type IN ? AND other <> ?", []int{LogTypeConsume, LogTypeError}, "").
		Order("id desc").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	collected := 0
	for _, row := range rows {
		headers := codexApplicationEnvironmentHeadersFromLogOther(row.Other)
		if len(headers) == 0 {
			continue
		}
		if _, changed, err := UpsertCodexApplicationEnvironmentFromRequestHeaders(headers); err != nil {
			return collected, err
		} else if changed {
			collected++
		}
	}
	return collected, nil
}

func UpsertCodexApplicationEnvironmentFromRequestHeaders(headers map[string]string) (*CodexApplicationEnvironment, bool, error) {
	if DB == nil {
		return nil, false, nil
	}
	env, ok := BuildCodexApplicationEnvironmentFromRequestHeaders(headers)
	if !ok {
		return nil, false, nil
	}
	var existing CodexApplicationEnvironment
	err := DB.Where("name = ?", env.Name).First(&existing).Error
	if err == nil {
		changed := existing.UserAgent != env.UserAgent ||
			existing.Originator != env.Originator ||
			existing.BetaFeatures != env.BetaFeatures ||
			existing.HeadersJSON != env.HeadersJSON ||
			existing.Platform != env.Platform ||
			existing.AppVersion != env.AppVersion ||
			existing.Source != CodexApplicationEnvironmentRealRequestSource ||
			!existing.Enabled
		existing.Platform = env.Platform
		existing.AppVersion = env.AppVersion
		existing.UserAgent = env.UserAgent
		existing.Originator = env.Originator
		existing.SessionID = ""
		existing.WindowID = ""
		existing.BetaFeatures = env.BetaFeatures
		existing.TurnMetadata = ""
		existing.HeadersJSON = env.HeadersJSON
		existing.Enabled = true
		existing.Source = CodexApplicationEnvironmentRealRequestSource
		existing.Remark = env.Remark
		if saveErr := DB.Save(&existing).Error; saveErr != nil {
			return nil, false, saveErr
		}
		cacheCodexApplicationEnvironment(&existing)
		return &existing, changed, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if err := DB.Create(&env).Error; err != nil {
		if fetchErr := DB.Where("name = ?", env.Name).First(&existing).Error; fetchErr == nil {
			cacheCodexApplicationEnvironment(&existing)
			return &existing, false, nil
		}
		return nil, false, err
	}
	cacheCodexApplicationEnvironment(&env)
	return &env, true, nil
}

func BuildCodexApplicationEnvironmentFromRequestHeaders(headers map[string]string) (CodexApplicationEnvironment, bool) {
	stable := stableCodexApplicationEnvironmentHeaders(headers)
	if len(stable) == 0 || !codexEnvironmentHeadersLookReal(stable) {
		return CodexApplicationEnvironment{}, false
	}
	userAgent := stable["user-agent"]
	originator := stable["originator"]
	betaFeatures := stable["x-codex-beta-features"]
	extras := make(map[string]string)
	for key, value := range stable {
		switch key {
		case "user-agent", "originator", "x-codex-beta-features":
			continue
		default:
			extras[canonicalCodexEnvironmentHeaderName(key)] = value
		}
	}
	headersJSON := ""
	if len(extras) > 0 {
		if extraBytes, err := common.Marshal(extras); err == nil {
			headersJSON = string(extraBytes)
		}
	}
	fingerprint := codexApplicationEnvironmentFingerprint(stable)
	if len(fingerprint) > 12 {
		fingerprint = fingerprint[:12]
	}
	env := CodexApplicationEnvironment{
		Name:         "codex-real-" + fingerprint,
		Platform:     codexApplicationEnvironmentPlatform(userAgent),
		AppVersion:   codexApplicationEnvironmentAppVersion(userAgent),
		UserAgent:    userAgent,
		Originator:   originator,
		BetaFeatures: betaFeatures,
		HeadersJSON:  headersJSON,
		Enabled:      true,
		Source:       CodexApplicationEnvironmentRealRequestSource,
		Remark:       "captured from real Codex request headers",
	}
	return env, true
}

func ReconcileCodexApplicationEnvironmentBindings() error {
	if DB == nil {
		return nil
	}
	invalidIDs, err := codexApplicationEnvironmentIDsBySource(CodexApplicationEnvironmentSystemSource)
	if err != nil {
		return err
	}
	if len(invalidIDs) == 0 {
		return nil
	}
	realEnvs, err := ListRealCodexApplicationEnvironments()
	if err != nil {
		return err
	}
	channels := make([]Channel, 0)
	if err := DB.Find(&channels).Error; err != nil {
		return err
	}
	changed := false
	for i := range channels {
		channel := &channels[i]
		if cleanupCodexApplicationEnvironmentBindingForChannel(channel, invalidIDs, realEnvs) {
			changed = true
			if err := DB.Model(channel).Update("channel_info", channel.ChannelInfo).Error; err != nil {
				return err
			}
		}
	}
	if changed {
		InitChannelCache()
	}
	return nil
}

func disableSystemSeedCodexApplicationEnvironments() error {
	if DB == nil {
		return nil
	}
	now := common.GetTimestamp()
	if err := DB.Model(&CodexApplicationEnvironment{}).
		Where("source = ?", CodexApplicationEnvironmentSystemSource).
		Updates(map[string]interface{}{"enabled": false, "updated_at": now}).Error; err != nil {
		return err
	}
	clearCodexApplicationEnvironmentCacheAll()
	return nil
}

func clearCodexApplicationEnvironmentCacheAll() {
	codexApplicationEnvironmentCache.Range(func(key, value any) bool {
		codexApplicationEnvironmentCache.Delete(key)
		return true
	})
}

func codexApplicationEnvironmentIDsBySource(source string) (map[int]struct{}, error) {
	ids := make([]int, 0)
	if err := DB.Model(&CodexApplicationEnvironment{}).
		Where("source = ?", strings.TrimSpace(source)).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	result := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			result[id] = struct{}{}
		}
	}
	return result, nil
}

func cleanupCodexApplicationEnvironmentBindingForChannel(channel *Channel, invalidIDs map[int]struct{}, realEnvs []CodexApplicationEnvironment) bool {
	if channel == nil || len(invalidIDs) == 0 {
		return false
	}
	changed := false
	removeInvalidID := func(environmentID int) bool {
		if environmentID <= 0 {
			return false
		}
		_, invalid := invalidIDs[environmentID]
		return invalid
	}
	if channel.ChannelInfo.MultiKeyCodexEnvironmentIDs != nil {
		for index, environmentID := range channel.ChannelInfo.MultiKeyCodexEnvironmentIDs {
			if removeInvalidID(environmentID) {
				delete(channel.ChannelInfo.MultiKeyCodexEnvironmentIDs, index)
				changed = true
			}
		}
		if len(channel.ChannelInfo.MultiKeyCodexEnvironmentIDs) == 0 {
			channel.ChannelInfo.MultiKeyCodexEnvironmentIDs = nil
		}
	}
	if channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys != nil {
		for accountUniqueKey, environmentID := range channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys {
			if removeInvalidID(environmentID) {
				delete(channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys, accountUniqueKey)
				changed = true
			}
		}
	}
	if len(realEnvs) > 0 {
		keys := channel.GetKeys()
		if channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys == nil {
			channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys = make(map[string]int)
		}
		assignIndex := 0
		for index, key := range keys {
			if !shouldBindCodexApplicationEnvironment(channel, key) {
				continue
			}
			accountUniqueKey := codexApplicationEnvironmentAccountUniqueKey(channel, index, key)
			if accountUniqueKey == "" {
				continue
			}
			if channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys[accountUniqueKey] > 0 {
				continue
			}
			env := realEnvs[assignIndex%len(realEnvs)]
			assignIndex++
			channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys[accountUniqueKey] = env.Id
			changed = true
		}
	}
	if channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys != nil && len(channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys) == 0 {
		channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys = nil
	}
	return changed
}

func shouldBindCodexApplicationEnvironment(channel *Channel, rawKey string) bool {
	if channel == nil {
		return false
	}
	if channel.Type == constant.ChannelTypeCodex {
		return true
	}
	key := strings.TrimSpace(rawKey)
	if channel.Type == constant.ChannelTypeOpenAI && codexauth.LooksLikeOAuthJSONCredential(key) {
		return true
	}
	if channel.Type == constant.ChannelTypeOpenAI && channel.GetOtherSettings().UsesCodexCompatibilityMode() {
		return true
	}
	return false
}

func codexApplicationEnvironmentAccountUniqueKey(channel *Channel, credentialIndex int, rawKey string) string {
	if channel == nil || credentialIndex < 0 {
		return ""
	}
	brand := codexApplicationEnvironmentBrandForChannelKey(channel, rawKey)
	subjectSource := codexApplicationEnvironmentSubjectSourceForChannelKey(channel, rawKey)
	subjectFP := strings.TrimSpace(common.GenerateHMAC(subjectSource))
	if brand == "" || subjectFP == "" {
		return ""
	}
	return strings.Join([]string{fmt.Sprint(channel.Id), brand, subjectFP}, ":")
}

func codexApplicationEnvironmentBrandForChannelKey(channel *Channel, rawKey string) string {
	if channel == nil {
		return ""
	}
	if channel.Type == constant.ChannelTypeOpenAI && codexauth.LooksLikeOAuthJSONCredential(rawKey) {
		return "codex"
	}
	switch channel.Type {
	case constant.ChannelTypeCodex:
		return "codex"
	case constant.ChannelTypeOpenAI:
		if channel.GetOtherSettings().UsesCodexCompatibilityMode() {
			return "codex"
		}
		return "openai"
	default:
		name := strings.TrimSpace(constant.GetChannelTypeName(channel.Type))
		if name == "" || strings.EqualFold(name, "unknown") {
			return "unknown"
		}
		return strings.ToLower(strings.ReplaceAll(name, " ", "_"))
	}
}

func codexApplicationEnvironmentSubjectSourceForChannelKey(channel *Channel, rawKey string) string {
	key := strings.TrimSpace(rawKey)
	if channel != nil && (channel.Type == constant.ChannelTypeCodex || channel.Type == constant.ChannelTypeOpenAI) && strings.HasPrefix(key, "{") {
		if oauthKey, ok := codexauth.ParseOAuthJSONCredentialLoose(key); ok {
			accountID := strings.TrimSpace(oauthKey.AccountID)
			if accountID != "" {
				return "codex:account_id:" + accountID
			}
			email := strings.TrimSpace(strings.ToLower(oauthKey.Email))
			if email != "" {
				return "codex:email:" + email
			}
			refresh := strings.TrimSpace(oauthKey.RefreshToken)
			if refresh != "" {
				return "codex:refresh:" + refresh
			}
		}
	}
	return key
}

func codexApplicationEnvironmentHeadersFromLogOther(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var other map[string]any
	if err := common.UnmarshalJsonStr(raw, &other); err != nil {
		return nil
	}
	adminInfo, _ := other["admin_info"].(map[string]any)
	clientRequest, _ := adminInfo["client_request"].(map[string]any)
	headersRaw, _ := clientRequest["headers"].(map[string]any)
	if len(headersRaw) == 0 {
		return nil
	}
	headers := make(map[string]string, len(headersRaw))
	for key, value := range headersRaw {
		key = strings.TrimSpace(key)
		valueText := strings.TrimSpace(fmt.Sprint(value))
		if key == "" || valueText == "" || valueText == "<nil>" {
			continue
		}
		headers[key] = valueText
	}
	return headers
}

func stableCodexApplicationEnvironmentHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	stable := make(map[string]string)
	for key, value := range headers {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if normalizedKey == "" || value == "" || !isStableCodexEnvironmentHeader(normalizedKey) {
			continue
		}
		stable[normalizedKey] = value
	}
	if len(stable) == 0 {
		return nil
	}
	return stable
}

func codexEnvironmentHeadersLookReal(headers map[string]string) bool {
	for key, value := range headers {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		lowerValue := strings.ToLower(strings.TrimSpace(value))
		if strings.Contains(lowerKey, "codex") || strings.Contains(lowerValue, "codex") {
			return true
		}
	}
	return false
}

func isStableCodexEnvironmentHeader(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" || isSensitiveCodexEnvironmentHeader(key) || isDynamicCodexEnvironmentHeader(key) {
		return false
	}
	switch key {
	case "user-agent", "originator", "x-codex-beta-features", "openai-beta":
		return true
	}
	return strings.HasPrefix(key, "x-stainless-") ||
		strings.HasPrefix(key, "codex-") ||
		strings.HasPrefix(key, "x-codex-")
}

func isSensitiveCodexEnvironmentHeader(key string) bool {
	for _, marker := range []string{"authorization", "api-key", "apikey", "token", "cookie", "secret", "key"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func isDynamicCodexEnvironmentHeader(key string) bool {
	for _, marker := range []string{"metadata", "session", "window", "request", "trace", "turn", "thread"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func canonicalCodexEnvironmentHeaderName(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "user-agent":
		return "User-Agent"
	case "originator":
		return "originator"
	case "x-codex-beta-features":
		return "x-codex-beta-features"
	case "openai-beta":
		return "openai-beta"
	default:
		return key
	}
}

func codexApplicationEnvironmentFingerprint(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+headers[key])
	}
	return common.Sha1([]byte(strings.Join(parts, "\n")))
}

func codexApplicationEnvironmentAppVersion(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	lower := strings.ToLower(userAgent)
	for _, marker := range []string{"codex desktop/", "codex cli/", "codex-cli/", "codex_cli_rs/", "codex_cli_js/"} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		start := idx + len(marker)
		if start >= len(userAgent) {
			return ""
		}
		rest := userAgent[start:]
		end := len(rest)
		for i, r := range rest {
			if r == ' ' || r == ';' || r == ')' || r == '(' {
				end = i
				break
			}
		}
		return strings.Trim(rest[:end], " ,;")
	}
	return ""
}

func codexApplicationEnvironmentPlatform(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	if start := strings.Index(userAgent, "("); start >= 0 {
		if end := strings.Index(userAgent[start+1:], ")"); end >= 0 {
			inside := strings.TrimSpace(userAgent[start+1 : start+1+end])
			if inside != "" {
				if semi := strings.Index(inside, ";"); semi >= 0 {
					inside = strings.TrimSpace(inside[:semi])
				}
				return inside
			}
		}
	}
	lower := strings.ToLower(userAgent)
	switch {
	case strings.Contains(lower, "mac"):
		return "macOS"
	case strings.Contains(lower, "windows"):
		return "Windows"
	case strings.Contains(lower, "linux"):
		return "Linux"
	default:
		return ""
	}
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
