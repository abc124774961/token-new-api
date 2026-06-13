package credential

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestResolveChannelCredentialSelectsSpecificMultiKeyWithoutAdvancingPolling(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     7,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b\nsk-c",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeyPollingIndex: 2,
		},
	}
	ref := core.CredentialRef{
		ResourceID:            "platform:channel:7",
		CredentialIndex:       1,
		CredentialFingerprint: common.GenerateHMAC("sk-b"),
		Resolver:              "channel_key",
	}

	resolved, apiErr := ResolveChannelCredential(channel, ref)

	require.Nil(t, apiErr)
	require.Equal(t, "sk-b", resolved.Key)
	require.Equal(t, 1, resolved.CredentialIndex)
	require.True(t, resolved.ChannelIsMultiKey)
	require.Equal(t, 2, channel.ChannelInfo.MultiKeyPollingIndex)
	require.Equal(t, "acct-"+common.GenerateHMAC("sk-b")[:8], resolved.CredentialUID)
	require.Equal(t, "openai-acct-"+common.GenerateHMAC("sk-b")[:8], resolved.CredentialLabel)
}

func TestResolveChannelCredentialRejectsDisabledMultiKey(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     8,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyStatusList: map[int]int{
				1: common.ChannelStatusManuallyDisabled,
			},
		},
	}
	ref := core.CredentialRef{
		ResourceID:      "platform:channel:8",
		CredentialIndex: 1,
		Resolver:        "channel_key",
	}

	_, apiErr := ResolveChannelCredential(channel, ref)

	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr.Err, ErrCredentialDisabled)
}

func TestResolveChannelCredentialRejectsSingleKeyAuthErrorCapability(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     18,
		Key:    `{"access_token":"auth","account_id":"auth"}`,
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
				},
			},
		},
	}
	ref := core.CredentialRef{
		ResourceID:      "platform:channel:18",
		CredentialIndex: 0,
		Resolver:        "channel_key",
	}

	_, apiErr := ResolveChannelCredential(channel, ref)

	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr.Err, ErrCredentialDisabled)
}

func TestResolveChannelCredentialRejectsFingerprintMismatch(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     9,
		Key:    "sk-live",
		Status: common.ChannelStatusEnabled,
	}
	ref := core.CredentialRef{
		ResourceID:            "platform:channel:9",
		CredentialIndex:       0,
		CredentialFingerprint: common.GenerateHMAC("sk-old"),
		Resolver:              "channel_key",
	}

	_, apiErr := ResolveChannelCredential(channel, ref)

	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr.Err, ErrCredentialFingerprint)
}

func TestResolveChannelCredentialRejectsChannelMismatch(t *testing.T) {
	channel := &model.Channel{
		Id:     12,
		Key:    "sk-live",
		Status: common.ChannelStatusEnabled,
	}
	ref := core.CredentialRef{
		ResourceID:      "platform:channel:13",
		CredentialIndex: 0,
		Resolver:        "channel_key",
	}

	_, apiErr := ResolveChannelCredential(channel, ref)

	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr.Err, ErrCredentialChannelMismatch)
}

func TestChannelKeyResolverLoadsChannelByResourceID(t *testing.T) {
	common.CryptoSecret = "test-secret"
	loader := fakeChannelLoader{
		channel: &model.Channel{
			Id:     10,
			Key:    "sk-loaded",
			Status: common.ChannelStatusEnabled,
		},
	}
	resolver := NewChannelKeyResolver(&loader)

	resolved, apiErr := resolver.Resolve(context.Background(), core.CredentialRef{
		ResourceID:      "platform:channel:10",
		CredentialIndex: 0,
		Resolver:        "channel_key",
	})

	require.Nil(t, apiErr)
	require.Equal(t, "sk-loaded", resolved.Key)
	require.Equal(t, 10, loader.requestedID)
	require.True(t, loader.selectAll)
}

func TestApplyResolvedCredentialToContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}

	ApplyResolvedCredentialToContext(ctx, ResolvedCredential{
		Key:                          "sk-context",
		CredentialIndex:              3,
		ChannelIsMultiKey:            true,
		CodexEnvironmentID:           42,
		AccountIdentityKey:           "openai:openai:subject",
		CredentialSubjectFingerprint: "abcdef1234567890",
		CredentialUID:                "acct-abcdef12",
		CredentialLabel:              "openai-acct-abcdef12",
	})

	require.Equal(t, "sk-context", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey))
	require.Equal(t, 3, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	require.Equal(t, 42, common.GetContextKeyInt(ctx, constant.ContextKeyChannelAccountCodexEnvironmentID))
	require.Equal(t, "acct-abcdef12", common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountCredentialUID))
	require.Equal(t, "openai-acct-abcdef12", common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountCredentialLabel))
	require.Equal(t, "openai:openai:subject", common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountIdentityKey))
	require.Equal(t, "abcdef1234567890", common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountCredentialSubjectFP))
}

func TestResolveChannelCredentialCarriesCodexEnvironmentID(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     16,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
		},
	}
	accountUniqueKey := modelgatewayaccount.AccountIdentityForChannelKey(channel, 1, "sk-b").AccountUniqueKey
	channel.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys = map[string]int{accountUniqueKey: 77}

	resolved, apiErr := ResolveChannelCredential(channel, core.CredentialRef{
		ResourceID:      "platform:channel:16",
		CredentialIndex: 1,
		Resolver:        "channel_key",
	})

	require.Nil(t, apiErr)
	require.Equal(t, 77, resolved.CodexEnvironmentID)
}

func TestResolveChannelCredentialFallsBackToLegacyCodexEnvironmentIndex(t *testing.T) {
	common.CryptoSecret = "test-secret"
	channel := &model.Channel{
		Id:     17,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:                  true,
			MultiKeyCodexEnvironmentIDs: map[int]int{1: 88},
		},
	}

	resolved, apiErr := ResolveChannelCredential(channel, core.CredentialRef{
		ResourceID:      "platform:channel:17",
		CredentialIndex: 1,
		Resolver:        "channel_key",
	})

	require.Nil(t, apiErr)
	require.Equal(t, 88, resolved.CodexEnvironmentID)
}

func TestResolveChannelCredentialAppliesAccountProxy(t *testing.T) {
	common.CryptoSecret = "test-secret"
	db := setupCredentialResolverProxyTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       77,
		Name:     "proxy one",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)
	channel := &model.Channel{
		Id:     15,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:       true,
			MultiKeyProxyIDs: map[int]int{1: 77},
		},
	}

	resolved, apiErr := ResolveChannelCredential(channel, core.CredentialRef{
		ResourceID:      "platform:channel:15",
		CredentialIndex: 1,
		Resolver:        "channel_key",
	})

	require.Nil(t, apiErr)
	require.Equal(t, 77, resolved.ProxyID)
	require.Equal(t, "socks5://127.0.0.1:1080", resolved.ProxyURL)
}

func TestApplyResolvedCredentialToContextIncludesProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}

	ApplyResolvedCredentialToContext(ctx, ResolvedCredential{
		Key:               "sk-context",
		CredentialIndex:   1,
		ChannelIsMultiKey: true,
		ProxyID:           77,
		ProxyURL:          "socks5://127.0.0.1:1080",
	})

	require.Equal(t, 77, common.GetContextKeyInt(ctx, constant.ContextKeyChannelAccountProxyID))
	require.Equal(t, "socks5://127.0.0.1:1080", common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountProxyURL))
}

func setupCredentialResolverProxyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayProxy{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

type fakeChannelLoader struct {
	channel     *model.Channel
	requestedID int
	selectAll   bool
}

func (l *fakeChannelLoader) GetChannelByID(channelID int, selectAll bool) (*model.Channel, error) {
	l.requestedID = channelID
	l.selectAll = selectAll
	return l.channel, nil
}
