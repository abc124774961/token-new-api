package scheduler

import (
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type NoopSmartChannelSelector struct{}

func NewNoopSmartChannelSelector() *NoopSmartChannelSelector {
	return &NoopSmartChannelSelector{}
}

func (s *NoopSmartChannelSelector) Select(c *gin.Context, param *service.RetryParam, policy core.GroupSmartPolicy) (*core.DispatchPlan, bool, *types.NewAPIError) {
	return nil, false, nil
}
