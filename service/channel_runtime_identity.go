package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

// ChannelRuntimeIdentity identifies a channel candidate at account/runtime
// granularity without importing modelgateway/core into service.
type ChannelRuntimeIdentity struct {
	ChannelID           int
	RequestedModel      string
	SelectedGroup       string
	EndpointType        constant.EndpointType
	AccountID           string
	CredentialIndex     int
	CredentialIndexSet  bool
	CredentialSubjectFP string
	CredentialFP        string
}

func NormalizeChannelRuntimeIdentity(identity ChannelRuntimeIdentity) ChannelRuntimeIdentity {
	identity.RequestedModel = strings.TrimSpace(identity.RequestedModel)
	identity.SelectedGroup = strings.TrimSpace(identity.SelectedGroup)
	identity.EndpointType = constant.EndpointType(strings.TrimSpace(string(identity.EndpointType)))
	identity.AccountID = strings.TrimSpace(identity.AccountID)
	identity.CredentialSubjectFP = strings.TrimSpace(identity.CredentialSubjectFP)
	identity.CredentialFP = strings.TrimSpace(identity.CredentialFP)
	if identity.EndpointType == "" {
		identity.EndpointType = constant.EndpointTypeOpenAI
	}
	if identity.CredentialIndex < 0 {
		identity.CredentialIndex = 0
		identity.CredentialIndexSet = false
	}
	return identity
}

func ChannelOnlyRuntimeIdentity(channelID int) ChannelRuntimeIdentity {
	return ChannelRuntimeIdentity{ChannelID: channelID}
}

func ChannelRuntimeIdentityFromContext(c *gin.Context, channelID int) ChannelRuntimeIdentity {
	identity := ChannelRuntimeIdentity{ChannelID: channelID}
	if c == nil {
		return NormalizeChannelRuntimeIdentity(identity)
	}
	if identity.ChannelID <= 0 {
		identity.ChannelID = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	}
	identity.RequestedModel = strings.TrimSpace(c.GetString("original_model"))
	identity.SelectedGroup = strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
	if identity.SelectedGroup == "" {
		identity.SelectedGroup = strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
	}
	if common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey) {
		identity.CredentialIndex = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		identity.CredentialIndexSet = true
	}
	return NormalizeChannelRuntimeIdentity(identity)
}

func (identity ChannelRuntimeIdentity) Normalize() ChannelRuntimeIdentity {
	return NormalizeChannelRuntimeIdentity(identity)
}

func (identity ChannelRuntimeIdentity) Valid() bool {
	return identity.Normalize().ChannelID > 0
}

func (identity ChannelRuntimeIdentity) HasAccountScope() bool {
	identity = identity.Normalize()
	return identity.AccountID != "" ||
		identity.CredentialSubjectFP != "" ||
		identity.CredentialFP != "" ||
		identity.CredentialIndexSet
}

func (identity ChannelRuntimeIdentity) ChannelScope() ChannelRuntimeIdentity {
	return ChannelRuntimeIdentity{ChannelID: identity.ChannelID}
}

func (identity ChannelRuntimeIdentity) AccountScope() ChannelRuntimeIdentity {
	identity = identity.Normalize()
	if !identity.HasAccountScope() {
		return identity.ChannelScope()
	}
	identity.RequestedModel = ""
	identity.SelectedGroup = ""
	identity.EndpointType = ""
	return identity
}
