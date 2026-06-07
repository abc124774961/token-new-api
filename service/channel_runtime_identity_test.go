package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFailureAvoidanceIsAccountScoped(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 60
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})
	clearAllChannelFailureAvoidanceForTest()

	accountA := testRuntimeIdentity(8101, "acct-a", 0)
	accountB := testRuntimeIdentity(8101, "acct-b", 1)

	require.NotNil(t, RecordChannelRuntimeFailureAvoidanceWithContext(accountA, "upstream_error:502", nil))
	require.NotNil(t, GetChannelRuntimeFailureAvoidanceStatus(accountA))
	require.Nil(t, GetChannelRuntimeFailureAvoidanceStatus(accountB))
	require.Nil(t, GetChannelRuntimeFailureAvoidanceStatus(ChannelOnlyRuntimeIdentity(8101)))

	require.True(t, ClearChannelRuntimeFailureAvoidanceOnRealSuccess(accountA))
	require.Nil(t, GetChannelRuntimeFailureAvoidanceStatus(accountA))

	require.NotNil(t, RecordChannelRuntimeFailureAvoidanceWithContext(accountB, "upstream_error:502", nil))
	require.False(t, ClearChannelRuntimeFailureAvoidanceOnRealSuccess(accountA))
	require.NotNil(t, GetChannelRuntimeFailureAvoidanceStatus(accountB))
}

func TestRuntimeFailureAvoidanceExpiredEntriesAreCleanedOnRead(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 60
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})
	clearAllChannelFailureAvoidanceForTest()

	accountA := testRuntimeIdentity(8103, "acct-a", 0)
	channelRuntimeFailureAvoidance.Store(accountA.Normalize(), channelAvoidanceEntry{
		until:        time.Now().Add(-time.Second),
		reason:       "upstream_error:502",
		failureCount: 1,
	})

	require.Nil(t, GetChannelRuntimeFailureAvoidanceStatus(accountA))
	_, runtimeExists := channelRuntimeFailureAvoidance.Load(accountA.Normalize())
	require.False(t, runtimeExists)
}

func TestRuntimeBalanceAndSelectionSkipScope(t *testing.T) {
	clearRuntimeBalanceInsufficientForTest()
	t.Cleanup(clearRuntimeBalanceInsufficientForTest)
	gin.SetMode(gin.TestMode)

	accountA := testRuntimeIdentity(8102, "acct-a", 0)
	accountB := testRuntimeIdentity(8102, "acct-b", 1)
	ctx, _ := gin.CreateTestContext(nil)

	MarkChannelRuntimeBalanceInsufficient(accountA)
	require.True(t, IsRuntimeBalanceInsufficientIdentity(accountA))
	require.False(t, IsRuntimeBalanceInsufficientIdentity(accountB))
	require.False(t, IsRuntimeBalanceInsufficientIdentity(ChannelOnlyRuntimeIdentity(8102)))

	MarkChannelRuntimeSelectionSkipped(ctx, accountA)
	require.True(t, IsChannelRuntimeSelectionSkipped(ctx, accountA))
	require.False(t, IsChannelRuntimeSelectionSkipped(ctx, accountB))
	sameAccountDifferentRoute := accountA
	sameAccountDifferentRoute.RequestedModel = "gpt-6"
	sameAccountDifferentRoute.SelectedGroup = "pro"
	sameAccountDifferentRoute.EndpointType = constant.EndpointTypeOpenAIResponse
	require.True(t, IsChannelRuntimeSelectionSkipped(ctx, sameAccountDifferentRoute))
	require.True(t, IsChannelRuntimeSelectionSkipped(ctx, ChannelRuntimeIdentity{
		ChannelID:          accountA.ChannelID,
		CredentialIndex:    accountA.CredentialIndex,
		CredentialIndexSet: true,
	}))
	require.False(t, IsChannelRuntimeSelectionSkipped(ctx, ChannelRuntimeIdentity{
		ChannelID:          accountA.ChannelID,
		CredentialIndex:    accountB.CredentialIndex,
		CredentialIndexSet: true,
	}))

	MarkChannelRuntimeSelectionSkipped(ctx, ChannelOnlyRuntimeIdentity(8102))
	require.True(t, IsChannelRuntimeSelectionSkipped(ctx, accountB))
}

func TestRuntimeAttemptedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountA := testRuntimeIdentity(8105, "acct-a", 0)
	accountB := testRuntimeIdentity(8105, "acct-b", 1)
	ctx, _ := gin.CreateTestContext(nil)

	MarkChannelRuntimeAttempted(ctx, accountA)
	require.True(t, IsChannelRuntimeAttempted(ctx, accountA))
	require.False(t, IsChannelRuntimeAttempted(ctx, accountB))
	require.True(t, IsChannelRuntimeAttempted(ctx, ChannelOnlyRuntimeIdentity(8105)))

	otherCtx, _ := gin.CreateTestContext(nil)
	MarkChannelRuntimeAttempted(otherCtx, ChannelOnlyRuntimeIdentity(8105))
	require.True(t, IsChannelRuntimeAttempted(otherCtx, accountA))
	require.True(t, IsChannelRuntimeAttempted(otherCtx, accountB))
}

func TestRuntimeBalanceInsufficientChannelCountsIncludeAccountScope(t *testing.T) {
	clearRuntimeBalanceInsufficientForTest()
	t.Cleanup(clearRuntimeBalanceInsufficientForTest)

	accountA := testRuntimeIdentity(8111, "acct-a", 0)
	accountB := testRuntimeIdentity(8111, "acct-b", 1)
	accountOther := testRuntimeIdentity(8112, "acct-c", 0)

	MarkChannelRuntimeBalanceInsufficient(accountA)
	MarkChannelRuntimeBalanceInsufficient(accountB)
	MarkChannelRuntimeBalanceInsufficient(accountOther)
	MarkChannelBalanceInsufficient(8111)

	counts := RuntimeBalanceInsufficientChannelCounts()
	require.Equal(t, 3, counts[8111])
	require.Equal(t, 1, counts[8112])
	require.Equal(t, 3, RuntimeBalanceInsufficientCountForChannel(8111))
}

func TestClearChannelBalanceInsufficientForChannelClearsAccountScope(t *testing.T) {
	clearRuntimeBalanceInsufficientForTest()
	t.Cleanup(clearRuntimeBalanceInsufficientForTest)

	accountA := testRuntimeIdentity(8121, "acct-a", 0)
	accountB := testRuntimeIdentity(8121, "acct-b", 1)
	accountOther := testRuntimeIdentity(8122, "acct-c", 0)

	MarkChannelRuntimeBalanceInsufficient(accountA)
	MarkChannelRuntimeBalanceInsufficient(accountB)
	MarkChannelRuntimeBalanceInsufficient(accountOther)
	MarkChannelBalanceInsufficient(8121)

	require.Equal(t, 3, ClearChannelBalanceInsufficientForChannel(8121))
	require.False(t, IsRuntimeBalanceInsufficientIdentity(accountA))
	require.False(t, IsRuntimeBalanceInsufficientIdentity(accountB))
	require.False(t, IsRuntimeBalanceInsufficientChannelID(8121))
	require.True(t, IsRuntimeBalanceInsufficientIdentity(accountOther))
}

func TestChannelRuntimeAccountScopeIgnoresDisplayMetadata(t *testing.T) {
	base := testRuntimeIdentity(8104, "acct-a", 0)
	withMetadata := base
	withMetadata.AccountType = "oauth_account"
	withMetadata.Brand = "codex"
	withMetadata.Provider = "codex_oauth"

	require.Equal(t, base.AccountScope(), withMetadata.AccountScope())
}

func testRuntimeIdentity(channelID int, accountID string, credentialIndex int) ChannelRuntimeIdentity {
	return ChannelRuntimeIdentity{
		ChannelID:           channelID,
		RequestedModel:      "gpt-5.5",
		SelectedGroup:       "default",
		EndpointType:        constant.EndpointTypeOpenAI,
		AccountID:           accountID,
		CredentialIndex:     credentialIndex,
		CredentialIndexSet:  true,
		CredentialSubjectFP: accountID + "-subject",
		CredentialFP:        accountID + "-credential",
	}
}

func clearRuntimeBalanceInsufficientForTest() {
	channelBalanceInsufficientRuntime.Range(func(key, value any) bool {
		channelBalanceInsufficientRuntime.Delete(key)
		return true
	})
	channelRuntimeBalanceInsufficient.Range(func(key, value any) bool {
		channelRuntimeBalanceInsufficient.Delete(key)
		return true
	})
}
