package integration

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

type LegacyChannelSelector struct{}

func NewLegacyChannelSelector() *LegacyChannelSelector {
	return &LegacyChannelSelector{}
}

func (s *LegacyChannelSelector) Select(param *service.RetryParam) (*model.Channel, string, error) {
	return service.CacheGetRandomSatisfiedChannel(param)
}
