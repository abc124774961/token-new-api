package dto

import (
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type Request interface {
	GetTokenCountMeta() *types.TokenCountMeta
	IsStream(c *gin.Context) bool
	SetModelName(modelName string)
	GetModelName() string
}

type BaseRequest struct {
}

func (b *BaseRequest) GetTokenCountMeta() *types.TokenCountMeta {
	return &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
}
func (b *BaseRequest) IsStream(c *gin.Context) bool {
	return false
}
func (b *BaseRequest) SetModelName(modelName string) {}
func (b *BaseRequest) GetModelName() string          { return "" }
