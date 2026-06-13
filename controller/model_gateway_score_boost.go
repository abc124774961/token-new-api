package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/gin-gonic/gin"
)

type ModelGatewayChannelScoreBoostResponse struct {
	ChannelID         int                        `json:"channel_id"`
	ChannelName       string                     `json:"channel_name,omitempty"`
	SmartScoreBoosts  map[string]float64         `json:"smart_score_boosts"`
	AllowedScoreItems []scheduler.ScoreBoostItem `json:"allowed_score_items"`
}

type UpdateModelGatewayChannelScoreBoostRequest struct {
	SmartScoreBoosts *map[string]float64 `json:"smart_score_boosts"`
}

func GetModelGatewayChannelScoreBoosts(c *gin.Context) {
	channel, ok := modelGatewayScoreBoostChannelFromParam(c, false)
	if !ok {
		return
	}
	common.ApiSuccess(c, buildModelGatewayChannelScoreBoostResponse(channel))
}

func UpdateModelGatewayChannelScoreBoosts(c *gin.Context) {
	channel, ok := modelGatewayScoreBoostChannelFromParam(c, true)
	if !ok {
		return
	}
	var request UpdateModelGatewayChannelScoreBoostRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	if request.SmartScoreBoosts == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少渠道分值加成配置",
		})
		return
	}

	scoreBoostService := scheduler.NewScoreBoostService()
	settings := channel.GetOtherSettings()
	before := scoreBoostService.Normalize(settings.SmartScoreBoosts)
	after := scoreBoostService.Normalize(*request.SmartScoreBoosts)
	settings.SmartScoreBoosts = after
	channel.SetOtherSettings(settings)
	if err := model.DB.Model(&model.Channel{}).
		Where("id = ?", channel.Id).
		Update("settings", channel.OtherSettings).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	if updatedChannel, err := model.GetChannelById(channel.Id, true); err == nil {
		model.CacheUpdateChannel(updatedChannel)
		channel = updatedChannel
	}
	modelgatewayintegration.RefreshDefaultRoutingCaches(modelgatewayintegration.RoutingCacheRefreshOptions{
		Reason: "model_gateway_score_boost",
	})
	recordModelGatewayScoreBoostLog(c, channel, before, after)
	common.ApiSuccess(c, buildModelGatewayChannelScoreBoostResponse(channel))
}

func modelGatewayScoreBoostChannelFromParam(c *gin.Context, selectAll bool) (*model.Channel, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "渠道 ID 无效")
		return nil, false
	}
	channel, err := model.GetChannelById(id, selectAll)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return channel, true
}

func buildModelGatewayChannelScoreBoostResponse(channel *model.Channel) ModelGatewayChannelScoreBoostResponse {
	boosts := scheduler.NewScoreBoostService().BoostsForChannel(channel)
	if boosts == nil {
		boosts = map[string]float64{}
	}
	response := ModelGatewayChannelScoreBoostResponse{
		SmartScoreBoosts:  boosts,
		AllowedScoreItems: scheduler.AllowedScoreBoostItems(),
	}
	if channel != nil {
		response.ChannelID = channel.Id
		response.ChannelName = channel.Name
	}
	return response
}

func recordModelGatewayScoreBoostLog(c *gin.Context, channel *model.Channel, before map[string]float64, after map[string]float64) {
	if model.LOG_DB == nil || channel == nil {
		return
	}
	userID := c.GetInt("id")
	changes := modelGatewayScoreBoostLogChanges(before, after)
	model.RecordLogWithAdminInfo(userID, model.LogTypeSystem, fmt.Sprintf(
		"更新渠道分值加成 (渠道ID: %d, 渠道名称: %s, 变更: %s)",
		channel.Id,
		channel.Name,
		modelGatewayScoreBoostLogJSON(changes),
	), map[string]interface{}{
		"channel_id":          channel.Id,
		"channel_name":        channel.Name,
		"old_score_boosts":    modelGatewayScoreBoostLogMap(before),
		"new_score_boosts":    modelGatewayScoreBoostLogMap(after),
		"score_boost_changes": changes,
	})
}

func modelGatewayScoreBoostLogMap(boosts map[string]float64) map[string]float64 {
	if len(boosts) == 0 {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(boosts))
	for key, value := range boosts {
		out[key] = value
	}
	return out
}

func modelGatewayScoreBoostLogChanges(before map[string]float64, after map[string]float64) []map[string]interface{} {
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	changes := make([]map[string]interface{}, 0, len(ordered))
	for _, key := range ordered {
		oldValue := before[key]
		newValue := after[key]
		if oldValue == newValue {
			continue
		}
		changes = append(changes, map[string]interface{}{
			"score_item": key,
			"old_value":  oldValue,
			"new_value":  newValue,
		})
	}
	return changes
}

func modelGatewayScoreBoostLogJSON(value interface{}) string {
	switch typed := value.(type) {
	case []map[string]interface{}:
		if len(typed) == 0 {
			return "[]"
		}
	case map[string]float64:
		if len(typed) == 0 {
			return "{}"
		}
	}
	data, err := common.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
