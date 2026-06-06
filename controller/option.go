package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
}

func isVisiblePublicKeyOption(key string) bool {
	switch key {
	case "WaffoPancakeWebhookPublicKey", "WaffoPancakeWebhookTestKey":
		return true
	default:
		return false
	}
}

func isSensitiveOptionKey(key string) bool {
	if isVisiblePublicKeyOption(key) {
		return false
	}
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	return strings.HasSuffix(key, "Token") ||
		strings.HasSuffix(key, "Secret") ||
		strings.HasSuffix(key, "Key") ||
		strings.HasSuffix(lowerKey, "secret") ||
		strings.HasSuffix(lowerKey, "api_key")
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		value := common.Interface2String(v)
		if isSensitiveOptionKey(k) {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
	return
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type optionAuditSnapshot struct {
	Exists bool
	Value  string
}

func getOptionAuditSnapshot(key string) (optionAuditSnapshot, error) {
	common.OptionMapRWMutex.RLock()
	if common.OptionMap != nil {
		if value, ok := common.OptionMap[key]; ok {
			common.OptionMapRWMutex.RUnlock()
			return optionAuditSnapshot{Exists: true, Value: value}, nil
		}
	}
	common.OptionMapRWMutex.RUnlock()

	if model.DB == nil {
		return optionAuditSnapshot{}, nil
	}
	var option model.Option
	err := model.DB.Where("key = ?", key).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return optionAuditSnapshot{}, nil
	}
	if err != nil {
		return optionAuditSnapshot{}, err
	}
	return optionAuditSnapshot{Exists: true, Value: option.Value}, nil
}

func optionAuditValueKind(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "empty"
	}
	if trimmed == "true" || trimmed == "false" {
		return "bool"
	}
	if _, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return "number"
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var parsed any
		if err := common.UnmarshalJsonStr(trimmed, &parsed); err == nil {
			return "json"
		}
	}
	if strings.Contains(trimmed, ",") {
		return "list"
	}
	return "string"
}

func optionAuditValueFingerprint(value string) string {
	if value == "" {
		return ""
	}
	hash := common.Sha1([]byte(value))
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func optionAuditRequestValueType(value any) string {
	switch value.(type) {
	case bool:
		return "bool"
	case float64:
		return "number"
	case int:
		return "number"
	case string:
		return "string"
	case nil:
		return "null"
	default:
		return "other"
	}
}

func setOptionAuditSnapshotSummary(c *gin.Context, prefix string, snapshot optionAuditSnapshot, sensitive bool) {
	trimmed := strings.TrimSpace(snapshot.Value)
	valueKind := optionAuditValueKind(snapshot.Value)
	middleware.SetAdminAuditSummary(c, prefix+"exists", snapshot.Exists)
	middleware.SetAdminAuditSummary(c, prefix+"empty", trimmed == "")
	middleware.SetAdminAuditSummary(c, prefix+"value_length", len(snapshot.Value))
	middleware.SetAdminAuditSummary(c, prefix+"value_kind", valueKind)
	if snapshot.Value != "" {
		middleware.SetAdminAuditSummary(c, prefix+"value_fingerprint", optionAuditValueFingerprint(snapshot.Value))
	}
	if !sensitive {
		switch valueKind {
		case "bool":
			middleware.SetAdminAuditSummary(c, prefix+"bool_value", trimmed == "true")
		case "number":
			if numberValue, err := strconv.ParseFloat(trimmed, 64); err == nil {
				middleware.SetAdminAuditSummary(c, prefix+"number_value", numberValue)
			}
		}
	}
}

func setUpdateOptionAuditSummary(c *gin.Context, key string, requestValueType string, before optionAuditSnapshot, after optionAuditSnapshot, beforeErr error, afterErr error) {
	sensitive := isSensitiveOptionKey(key)
	middleware.SetAdminAuditSummary(c, "operation", "update_option")
	middleware.SetAdminAuditSummary(c, "option_key", key)
	middleware.SetAdminAuditSummary(c, "request_value_type", requestValueType)
	middleware.SetAdminAuditSummary(c, "sensitive_option", sensitive)
	middleware.SetAdminAuditSummary(c, "value_changed", before.Value != after.Value || before.Exists != after.Exists)
	middleware.SetAdminAuditSummary(c, "value_fingerprint_changed", optionAuditValueFingerprint(before.Value) != optionAuditValueFingerprint(after.Value))
	setOptionAuditSnapshotSummary(c, "before_", before, sensitive)
	setOptionAuditSnapshotSummary(c, "after_", after, sensitive)
	if beforeErr != nil || afterErr != nil {
		middleware.SetAdminAuditSummary(c, "snapshot_error", true)
	}
}

func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	err := common.DecodeJson(c.Request.Body, &option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	requestValueType := optionAuditRequestValueType(option.Value)
	switch option.Value.(type) {
	case bool:
		option.Value = common.Interface2String(option.Value.(bool))
	case float64:
		option.Value = common.Interface2String(option.Value.(float64))
	case int:
		option.Value = common.Interface2String(option.Value.(int))
	default:
		option.Value = fmt.Sprintf("%v", option.Value)
	}
	beforeSnapshot, beforeSnapshotErr := getOptionAuditSnapshot(option.Key)
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！",
			})
			return
		}
	case "discord.enabled":
		if option.Value == "true" && system_setting.GetDiscordSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！",
			})
			return
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！",
			})
			return
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！",
			})
			return
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名限制，请先填入限制的邮箱域名！",
			})
			return
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})

			return
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && common.TelegramBotToken == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Telegram OAuth，请先填入 Telegram Bot Token！",
			})
			return
		}
	case "theme.frontend":
		if option.Value != "classic" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "当前版本仅支持 classic 经典前端",
			})
			return
		}
	case "GroupRatio":
		err = ratio_setting.CheckGroupRatio(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "图片倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频倍率设置失败: " + err.Error(),
			})
			return
		}
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "音频补全倍率设置失败: " + err.Error(),
			})
			return
		}
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "缓存创建倍率设置失败: " + err.Error(),
			})
			return
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticDisableStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticRetryStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.uptime_kuma_groups":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "UptimeKumaGroups")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.support_contacts":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "SupportContacts")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	err = model.UpdateOption(option.Key, option.Value.(string))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	afterSnapshot, afterSnapshotErr := getOptionAuditSnapshot(option.Key)
	setUpdateOptionAuditSummary(c, option.Key, requestValueType, beforeSnapshot, afterSnapshot, beforeSnapshotErr, afterSnapshotErr)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}
