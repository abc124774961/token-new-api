package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	requestId := c.Query("request_id")
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group, requestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	attachLogModelGatewayCostSummaries(logs)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

// Deprecated: SearchAllLogs 已废弃，前端未使用该接口。
func SearchAllLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

// Deprecated: SearchUserLogs 已废弃，前端未使用该接口。
func SearchUserLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "该接口已废弃",
	})
}

func attachLogModelGatewayCostSummaries(logs []*model.Log) {
	if len(logs) == 0 {
		return
	}
	requestIDs := make([]string, 0, len(logs))
	seen := make(map[string]bool, len(logs))
	channelIDs := make([]int, 0, len(logs))
	seenChannels := make(map[int]bool, len(logs))
	consumeLogs := make([]*model.Log, 0, len(logs))
	for _, log := range logs {
		if log == nil || log.Type != model.LogTypeConsume {
			continue
		}
		consumeLogs = append(consumeLogs, log)
		requestID := strings.TrimSpace(log.RequestId)
		if requestID != "" && !seen[requestID] {
			seen[requestID] = true
			requestIDs = append(requestIDs, requestID)
		}
		if log.ChannelId > 0 && !seenChannels[log.ChannelId] {
			seenChannels[log.ChannelId] = true
			channelIDs = append(channelIDs, log.ChannelId)
		}
	}
	if len(consumeLogs) == 0 {
		return
	}

	summaryByRequestID := loadLogModelGatewayCostSummaries(requestIDs)
	profileByChannelID := loadLogChannelCostProfiles(channelIDs)
	now := common.GetTimestamp()
	for _, log := range consumeLogs {
		if log == nil {
			continue
		}
		if summary, ok := summaryByRequestID[strings.TrimSpace(log.RequestId)]; ok {
			attachLogModelGatewayCostSummary(log, summary)
			continue
		}
		attachLogModelGatewayEstimatedCost(log, profileByChannelID, now)
	}
}

func loadLogModelGatewayCostSummaries(requestIDs []string) map[string]model.ModelGatewayRequestCostSummary {
	if len(requestIDs) == 0 || model.DB == nil {
		return nil
	}
	summaries := make([]model.ModelGatewayRequestCostSummary, 0, len(requestIDs))
	if err := model.DB.Where("request_id IN ?", requestIDs).Find(&summaries).Error; err != nil {
		common.SysLog("failed to load model gateway log cost summaries: " + err.Error())
		return nil
	}
	summaryByRequestID := make(map[string]model.ModelGatewayRequestCostSummary, len(summaries))
	for _, summary := range summaries {
		requestID := strings.TrimSpace(summary.RequestId)
		if requestID == "" {
			continue
		}
		summaryByRequestID[requestID] = summary
	}
	return summaryByRequestID
}

func loadLogChannelCostProfiles(channelIDs []int) map[int]model.ModelGatewayChannelCostProfile {
	if len(channelIDs) == 0 || model.DB == nil {
		return nil
	}
	profiles := make([]model.ModelGatewayChannelCostProfile, 0, len(channelIDs))
	if err := model.DB.
		Where("channel_id IN ? AND upstream_model = ?", channelIDs, defaultChannelCostModel).
		Find(&profiles).Error; err != nil {
		common.SysLog("failed to load model gateway log channel cost profiles: " + err.Error())
		return nil
	}
	now := common.GetTimestamp()
	profileByChannelID := make(map[int]model.ModelGatewayChannelCostProfile, len(profiles))
	for _, profile := range profiles {
		if profile.ChannelID <= 0 || profile.EffectiveTime > now {
			continue
		}
		if current, ok := profileByChannelID[profile.ChannelID]; !ok || betterChannelCostDisplayProfile(profile, current) {
			profileByChannelID[profile.ChannelID] = profile
		}
	}
	return profileByChannelID
}

func attachLogModelGatewayCostSummary(log *model.Log, summary model.ModelGatewayRequestCostSummary) {
	if log == nil {
		return
	}
	other := logOtherMap(log)
	costSummary := map[string]interface{}{
		"channel_id":          summary.ChannelID,
		"upstream_model":      strings.TrimSpace(summary.UpstreamModel),
		"upstream_cost_total": summary.UpstreamCostTotal,
		"cost_source":         strings.TrimSpace(summary.CostSource),
		"cost_accuracy":       strings.TrimSpace(summary.CostAccuracy),
		"calculated_at":       summary.CalculatedAt,
	}
	if breakdown := compactLogModelGatewayCostBreakdown(summary.BreakdownJSON); len(breakdown) > 0 {
		costSummary["breakdown"] = breakdown
	}
	costSummary["billing_quota"] = log.Quota
	other["model_gateway_cost"] = costSummary
	log.Other = common.MapToJsonStr(other)
}

func attachLogModelGatewayEstimatedCost(log *model.Log, profileByChannelID map[int]model.ModelGatewayChannelCostProfile, now int64) {
	if log == nil {
		return
	}
	usage := modelgatewaycost.UsageSnapshotFromLog(*log)
	var profile *model.ModelGatewayChannelCostProfile
	if usage.ChannelID > 0 {
		if cachedProfile, ok := profileByChannelID[usage.ChannelID]; ok {
			profileCopy := cachedProfile
			profile = &profileCopy
		}
	}
	if profile == nil {
		profile = modelgatewaycost.DefaultSystemRatioProfile(usage.ChannelID)
	}
	result := modelgatewaycost.Calculate(usage, profile)
	summary := result.Summary(now)
	attachLogModelGatewayCostSummary(log, summary)
}

func logOtherMap(log *model.Log) map[string]interface{} {
	other := make(map[string]interface{})
	if log == nil || strings.TrimSpace(log.Other) == "" {
		return other
	}
	if err := common.UnmarshalJsonStr(log.Other, &other); err != nil || other == nil {
		return map[string]interface{}{}
	}
	return other
}

func compactLogModelGatewayCostBreakdown(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	breakdown := map[string]interface{}{}
	if err := common.UnmarshalJsonStr(raw, &breakdown); err != nil {
		return nil
	}
	compact := make(map[string]interface{}, 6)
	for _, key := range []string{
		"currency",
		"usage_semantic",
		"cost_coefficient",
		"fee_multiplier",
		"token_multiplier",
		"recharge_multiplier",
	} {
		value, ok := breakdown[key]
		if !ok || isEmptyLogCostValue(value) {
			continue
		}
		compact[key] = value
	}
	return compact
}

func isEmptyLogCostValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case float64:
		return typed == 0
	case float32:
		return typed == 0
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case int32:
		return typed == 0
	case uint:
		return typed == 0
	case uint64:
		return typed == 0
	case uint32:
		return typed == 0
	default:
		return false
	}
}

func GetLogByKey(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		c.JSON(200, gin.H{
			"success": false,
			"message": "无效的令牌",
		})
		return
	}
	logs, err := model.GetLogByTokenId(tokenId)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	stat, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": stat.Quota,
			"rpm":   stat.Rpm,
			"tpm":   stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	username := c.GetString("username")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group := c.Query("group")
	quotaNum, err := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota": quotaNum.Quota,
			"rpm":   quotaNum.Rpm,
			"tpm":   quotaNum.Tpm,
			//"token": tokenNum,
		},
	})
	return
}

func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}
