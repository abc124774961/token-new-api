package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	"github.com/gin-gonic/gin"
)

type ConfirmModelGatewayDynamicBillingRequest struct {
	Group string `json:"group"`
}

func ConfirmModelGatewayDynamicBilling(c *gin.Context) {
	var request ConfirmModelGatewayDynamicBillingRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := common.DecodeJson(c.Request.Body, &request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "无效的参数",
			})
			return
		}
	}
	if strings.TrimSpace(request.Group) == "" {
		request.Group = c.Query("group")
	}
	confirmation, baseline, err := modelgatewaydynamicbilling.ConfirmManualRatio(
		model.DB,
		request.Group,
		c.GetInt("id"),
		c.GetString("username"),
	)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := modelgatewaydynamicbilling.RefreshDefaultNow(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"confirmation": confirmation,
		"baseline":     baseline,
		"baselines":    modelgatewaydynamicbilling.DefaultBaselineSnapshots(),
	})
}
