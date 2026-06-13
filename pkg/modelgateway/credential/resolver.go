package credential

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var (
	ErrMissingCredentialRef      = errors.New("missing credential reference")
	ErrUnsupportedResolver       = errors.New("unsupported credential resolver")
	ErrChannelNotFound           = errors.New("credential channel not found")
	ErrCredentialChannelMismatch = errors.New("credential channel mismatch")
	ErrCredentialIndexOutOfRange = errors.New("credential index out of range")
	ErrCredentialDisabled        = errors.New("credential disabled")
	ErrCredentialFingerprint     = errors.New("credential fingerprint mismatch")
)

type ResolvedCredential struct {
	Ref                          core.CredentialRef
	ChannelID                    int
	Key                          string
	CredentialIndex              int
	ChannelIsMultiKey            bool
	CodexEnvironmentID           int
	AccountID                    string
	AccountIdentityKey           string
	AccountUniqueKey             string
	AccountType                  string
	Brand                        string
	Provider                     string
	CredentialSubjectFingerprint string
	CredentialFingerprint        string
	CredentialUID                string
	CredentialLabel              string
	ProxyID                      int
	ProxyURL                     string
}

type Resolver interface {
	Resolve(ctx context.Context, ref core.CredentialRef) (ResolvedCredential, *types.NewAPIError)
}

type ChannelLoader interface {
	GetChannelByID(channelID int, selectAll bool) (*model.Channel, error)
}

type ModelChannelLoader struct{}

func (ModelChannelLoader) GetChannelByID(channelID int, selectAll bool) (*model.Channel, error) {
	return model.GetChannelById(channelID, selectAll)
}

type ChannelKeyResolver struct {
	loader ChannelLoader
}

func NewChannelKeyResolver(loader ChannelLoader) *ChannelKeyResolver {
	if loader == nil {
		loader = ModelChannelLoader{}
	}
	return &ChannelKeyResolver{loader: loader}
}

func (r *ChannelKeyResolver) Resolve(ctx context.Context, ref core.CredentialRef) (ResolvedCredential, *types.NewAPIError) {
	_ = ctx
	if ref.Resolver != "" && ref.Resolver != "channel_key" {
		return ResolvedCredential{}, resolverError(fmt.Errorf("%w: %s", ErrUnsupportedResolver, ref.Resolver), types.ErrorCodeChannelInvalidKey)
	}
	channelID := channelIDFromResourceID(ref.ResourceID)
	if channelID <= 0 {
		return ResolvedCredential{}, resolverError(ErrMissingCredentialRef, types.ErrorCodeChannelInvalidKey)
	}
	channel, err := r.loader.GetChannelByID(channelID, true)
	if err != nil || channel == nil {
		if err == nil {
			err = ErrChannelNotFound
		}
		return ResolvedCredential{}, resolverError(fmt.Errorf("%w: %v", ErrChannelNotFound, err), types.ErrorCodeGetChannelFailed)
	}
	resolved, apiErr := ResolveChannelCredential(channel, ref)
	if apiErr != nil {
		return ResolvedCredential{}, apiErr
	}
	return resolved, nil
}

func ResolveChannelCredential(channel *model.Channel, ref core.CredentialRef) (ResolvedCredential, *types.NewAPIError) {
	if channel == nil || channel.Id <= 0 {
		return ResolvedCredential{}, resolverError(ErrChannelNotFound, types.ErrorCodeGetChannelFailed)
	}
	if channel.Status != common.ChannelStatusEnabled {
		return ResolvedCredential{}, resolverError(fmt.Errorf("%w: channel_id=%d", ErrCredentialDisabled, channel.Id), types.ErrorCodeChannelNoAvailableKey)
	}
	if refChannelID := channelIDFromResourceID(ref.ResourceID); refChannelID > 0 && refChannelID != channel.Id {
		return ResolvedCredential{}, resolverError(fmt.Errorf("%w: ref_channel_id=%d channel_id=%d", ErrCredentialChannelMismatch, refChannelID, channel.Id), types.ErrorCodeChannelInvalidKey)
	}
	keys := channel.GetKeys()
	if len(keys) == 0 && strings.TrimSpace(channel.Key) != "" {
		keys = []string{channel.Key}
	}
	index := ref.CredentialIndex
	if !channel.ChannelInfo.IsMultiKey {
		index = 0
	}
	if index < 0 || index >= len(keys) {
		return ResolvedCredential{}, resolverError(fmt.Errorf("%w: index=%d size=%d", ErrCredentialIndexOutOfRange, index, len(keys)), types.ErrorCodeChannelNoAvailableKey)
	}
	key := strings.TrimSpace(keys[index])
	if key == "" {
		return ResolvedCredential{}, resolverError(fmt.Errorf("%w: empty key at index=%d", ErrCredentialIndexOutOfRange, index), types.ErrorCodeChannelNoAvailableKey)
	}
	if !channelKeyEnabled(channel, index) {
		return ResolvedCredential{}, resolverError(fmt.Errorf("%w: index=%d", ErrCredentialDisabled, index), types.ErrorCodeChannelNoAvailableKey)
	}
	identity := modelgatewayaccount.AccountIdentityForChannelKey(channel, index, key)
	resourceRef := modelgatewayaccount.ResourceRefForChannel(channel)
	credentialFP := fingerprint(key)
	if ref.CredentialFingerprint != "" && ref.CredentialFingerprint != credentialFP {
		return ResolvedCredential{}, resolverError(ErrCredentialFingerprint, types.ErrorCodeChannelInvalidKey)
	}
	subjectFP := strings.TrimSpace(ref.CredentialSubjectFingerprint)
	if subjectFP == "" {
		subjectFP = identity.CredentialSubjectFingerprint
	}
	proxyID, proxyURL, proxyErr := resolveChannelCredentialProxy(channel, index)
	if proxyErr != nil {
		return ResolvedCredential{}, resolverError(proxyErr, types.ErrorCodeGetChannelFailed)
	}
	envID := modelgatewayaccount.CodexEnvironmentIDForChannelKey(channel, index, key)
	credentialRef := ref
	if credentialRef.ResourceID == "" {
		credentialRef.ResourceID = resourceRef.ResourceID
	}
	if credentialRef.AccountID == "" {
		credentialRef.AccountID = identity.AccountID
	}
	credentialRef.CredentialIndex = index
	credentialRef.CredentialSubjectFingerprint = subjectFP
	credentialRef.CredentialFingerprint = credentialFP
	if credentialRef.Resolver == "" {
		credentialRef.Resolver = modelgatewayaccount.ResolverChannelKey
	}
	return ResolvedCredential{
		Ref:                          ref,
		ChannelID:                    channel.Id,
		Key:                          key,
		CredentialIndex:              index,
		ChannelIsMultiKey:            channel.ChannelInfo.IsMultiKey,
		CodexEnvironmentID:           envID,
		AccountID:                    identity.AccountID,
		AccountIdentityKey:           identity.AccountIdentityKey,
		AccountUniqueKey:             identity.AccountUniqueKey,
		AccountType:                  identity.AccountType,
		Brand:                        identity.Brand,
		Provider:                     identity.Provider,
		CredentialSubjectFingerprint: subjectFP,
		CredentialFingerprint:        credentialFP,
		CredentialUID:                modelgatewayaccount.CredentialUID(identity, credentialRef, resourceRef, channel.Id),
		CredentialLabel:              modelgatewayaccount.CredentialLabel(identity, credentialRef, resourceRef, channel.Id),
		ProxyID:                      proxyID,
		ProxyURL:                     proxyURL,
	}, nil
}

func ApplyResolvedCredentialToContext(c *gin.Context, resolved ResolvedCredential) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyChannelKey, resolved.Key)
	common.SetContextKey(c, constant.ContextKeyChannelAccountCodexEnvironmentID, resolved.CodexEnvironmentID)
	applyAccountIdentityToContext(c, resolved)
	if resolved.ProxyID > 0 && resolved.ProxyURL != "" {
		common.SetContextKey(c, constant.ContextKeyChannelAccountProxyID, resolved.ProxyID)
		common.SetContextKey(c, constant.ContextKeyChannelAccountProxyURL, resolved.ProxyURL)
	}
	if resolved.ChannelIsMultiKey {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, resolved.CredentialIndex)
		return
	}
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 0)
}

func ApplyChannelCredentialIdentityToContext(c *gin.Context, channel *model.Channel, credentialIndex int, rawKey string) {
	if c == nil || channel == nil || credentialIndex < 0 {
		return
	}
	key := strings.TrimSpace(rawKey)
	if key == "" {
		return
	}
	identity := modelgatewayaccount.AccountIdentityForChannelKey(channel, credentialIndex, key)
	resourceRef := modelgatewayaccount.ResourceRefForChannel(channel)
	credentialFP := fingerprint(key)
	credentialRef := core.CredentialRef{
		ResourceID:                   resourceRef.ResourceID,
		AccountID:                    identity.AccountID,
		CredentialIndex:              credentialIndex,
		CredentialSubjectFingerprint: identity.CredentialSubjectFingerprint,
		CredentialFingerprint:        credentialFP,
		Resolver:                     modelgatewayaccount.ResolverChannelKey,
	}
	applyAccountIdentityToContext(c, ResolvedCredential{
		ChannelID:                    channel.Id,
		CredentialIndex:              credentialIndex,
		AccountID:                    identity.AccountID,
		AccountIdentityKey:           identity.AccountIdentityKey,
		AccountUniqueKey:             identity.AccountUniqueKey,
		AccountType:                  identity.AccountType,
		Brand:                        identity.Brand,
		Provider:                     identity.Provider,
		CredentialSubjectFingerprint: identity.CredentialSubjectFingerprint,
		CredentialFingerprint:        credentialFP,
		CredentialUID:                modelgatewayaccount.CredentialUID(identity, credentialRef, resourceRef, channel.Id),
		CredentialLabel:              modelgatewayaccount.CredentialLabel(identity, credentialRef, resourceRef, channel.Id),
	})
}

func applyAccountIdentityToContext(c *gin.Context, resolved ResolvedCredential) {
	if c == nil {
		return
	}
	setString := func(key constant.ContextKey, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			common.SetContextKey(c, key, value)
		}
	}
	setString(constant.ContextKeyChannelAccountID, resolved.AccountID)
	setString(constant.ContextKeyChannelAccountIdentityKey, resolved.AccountIdentityKey)
	setString(constant.ContextKeyChannelAccountUniqueKey, resolved.AccountUniqueKey)
	setString(constant.ContextKeyChannelAccountType, resolved.AccountType)
	setString(constant.ContextKeyChannelAccountBrand, resolved.Brand)
	setString(constant.ContextKeyChannelAccountProvider, resolved.Provider)
	setString(constant.ContextKeyChannelAccountCredentialSubjectFP, resolved.CredentialSubjectFingerprint)
	setString(constant.ContextKeyChannelAccountCredentialFP, resolved.CredentialFingerprint)
	setString(constant.ContextKeyChannelAccountCredentialUID, resolved.CredentialUID)
	setString(constant.ContextKeyChannelAccountCredentialLabel, resolved.CredentialLabel)
}

func channelIDFromResourceID(resourceID string) int {
	const prefix = "platform:channel:"
	resourceID = strings.TrimSpace(resourceID)
	if !strings.HasPrefix(resourceID, prefix) {
		return 0
	}
	var channelID int
	_, _ = fmt.Sscanf(strings.TrimPrefix(resourceID, prefix), "%d", &channelID)
	return channelID
}

func resolveChannelCredentialProxy(channel *model.Channel, credentialIndex int) (int, string, error) {
	if channel == nil || channel.ChannelInfo.MultiKeyProxyIDs == nil {
		return 0, "", nil
	}
	proxyID := channel.ChannelInfo.MultiKeyProxyIDs[credentialIndex]
	if proxyID <= 0 {
		return 0, "", nil
	}
	proxyConfig, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil {
		return 0, "", fmt.Errorf("credential proxy not found: proxy_id=%d: %w", proxyID, err)
	}
	if !proxyConfig.Enabled {
		return 0, "", fmt.Errorf("credential proxy disabled: proxy_id=%d", proxyID)
	}
	proxyURL, err := proxyConfig.ProxyURL()
	if err != nil {
		return 0, "", fmt.Errorf("credential proxy invalid: proxy_id=%d: %w", proxyID, err)
	}
	return proxyID, proxyURL, nil
}

func channelKeyEnabled(channel *model.Channel, index int) bool {
	if channel == nil {
		return false
	}
	status := channel.Status
	if channel.ChannelInfo.IsMultiKey && channel.ChannelInfo.MultiKeyStatusList != nil {
		if value, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok {
			status = value
		}
	}
	if capability, ok := channel.ChannelInfo.MultiKeyCapabilities[index]; ok && channelCredentialCapabilityBlocksScheduling(capability) {
		return false
	}
	return status == common.ChannelStatusEnabled
}

func channelCredentialCapabilityBlocksScheduling(capability model.ChannelAccountCapability) bool {
	if capability.UsageLimitActiveAt(common.GetTimestamp()) {
		return true
	}
	return strings.TrimSpace(capability.CapabilityClassification) == channelcapability.ClassificationAuthError
}

func fingerprint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return common.GenerateHMAC(value)
}

func resolverError(err error, code types.ErrorCode) *types.NewAPIError {
	return types.NewError(err, code, types.ErrOptionWithSkipRetry())
}
