package integration

import (
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/gin-gonic/gin"
)

const selectedPlanContextKey = "modelgateway_selected_plan"
const failedStickyPlanContextKey = "modelgateway_failed_sticky_plan"

func SetSelectedPlan(c *gin.Context, plan *core.DispatchPlan) {
	if c == nil || plan == nil {
		return
	}
	c.Set(selectedPlanContextKey, plan)
}

func ClearSelectedPlan(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(selectedPlanContextKey, nil)
}

func GetSelectedPlan(c *gin.Context) (*core.DispatchPlan, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(selectedPlanContextKey)
	if !ok {
		return nil, false
	}
	plan, ok := value.(*core.DispatchPlan)
	return plan, ok && plan != nil
}

func SetFailedStickyPlan(c *gin.Context, plan *core.DispatchPlan) {
	if c == nil || plan == nil {
		return
	}
	c.Set(failedStickyPlanContextKey, plan)
}

func ClearFailedStickyPlan(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(failedStickyPlanContextKey, nil)
}

func GetFailedStickyPlan(c *gin.Context) (*core.DispatchPlan, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(failedStickyPlanContextKey)
	if !ok {
		return nil, false
	}
	plan, ok := value.(*core.DispatchPlan)
	return plan, ok && plan != nil
}
