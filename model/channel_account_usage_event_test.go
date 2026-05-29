package model

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelAccountUsageEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ChannelAccountUsageEvent{}))

	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestChannelAccountUsageEventUpsertsMergeOutOfOrderRequestStages(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	requestID := "req-account-usage-merge"

	require.NoError(t, UpsertChannelAccountUsageCost(ModelGatewayRequestCostSummary{
		RequestId:         requestID,
		ChannelID:         42,
		UpstreamModel:     "gpt-5.4",
		UpstreamCostTotal: 0.03125,
		CostSource:        "profile",
		CostAccuracy:      "precise",
		CalculatedAt:      100,
	}))
	require.NoError(t, UpsertChannelAccountUsageBilling(ChannelAccountUsageEvent{
		RequestId:        requestID,
		ChannelID:        42,
		RequestedModel:   "gpt-5.4",
		RequestedGroup:   "vip",
		SelectedGroup:    "vip-a",
		CompletedAt:      110,
		PromptTokens:     120,
		CompletionTokens: 80,
		TotalTokens:      200,
		Quota:            2000,
	}))
	require.NoError(t, UpsertChannelAccountUsageDispatch(ChannelAccountUsageEvent{
		RequestId:                    requestID,
		ChannelID:                    42,
		ChannelName:                  "primary codex",
		CredentialIndex:              2,
		AccountID:                    "openai:codex:subject",
		AccountIdentityKey:           "openai:codex:subject",
		CredentialSubjectFingerprint: "subject",
		CredentialFingerprint:        "credential",
		AccountType:                  "oauth_account",
		Brand:                        "codex",
		Provider:                     "openai",
		RequestedModel:               "gpt-5.4",
		RequestedGroup:               "vip",
		SelectedGroup:                "vip-a",
		EndpointType:                 string(constant.EndpointTypeOpenAI),
	}))
	require.NoError(t, UpsertChannelAccountUsageAttempt(ChannelAccountUsageEvent{
		RequestId:          requestID,
		ChannelID:          42,
		CredentialIndex:    2,
		AccountIdentityKey: "openai:codex:subject",
		CompletedAt:        120,
		Success:            true,
		StatusCode:         http.StatusOK,
		DurationMs:         1500,
		TTFTMs:             360,
	}))

	var row ChannelAccountUsageEvent
	require.NoError(t, db.First(&row, "request_id = ?", requestID).Error)
	require.Equal(t, 42, row.ChannelID)
	require.Equal(t, "primary codex", row.ChannelName)
	require.Equal(t, 2, row.CredentialIndex)
	require.Equal(t, "openai:codex:subject", row.AccountIdentityKey)
	require.Equal(t, "credential", row.CredentialFingerprint)
	require.Equal(t, "gpt-5.4", row.RequestedModel)
	require.Equal(t, "vip", row.RequestedGroup)
	require.Equal(t, "vip-a", row.SelectedGroup)
	require.Equal(t, string(constant.EndpointTypeOpenAI), row.EndpointType)
	require.Equal(t, int64(120), row.CompletedAt)
	require.True(t, row.Success)
	require.Equal(t, http.StatusOK, row.StatusCode)
	require.Equal(t, int64(1500), row.DurationMs)
	require.Equal(t, int64(360), row.TTFTMs)
	require.Equal(t, int64(120), row.PromptTokens)
	require.Equal(t, int64(80), row.CompletionTokens)
	require.Equal(t, int64(200), row.TotalTokens)
	require.Equal(t, int64(2000), row.Quota)
	require.InEpsilon(t, 0.03125, row.UpstreamCostTotal, 0.0001)
	require.Equal(t, "profile", row.CostSource)
	require.Equal(t, "precise", row.CostAccuracy)
	require.Equal(t, int64(100), row.CostCalculatedAt)

	aggregates, err := QueryChannelAccountUsageWindowAggregates(42, []ChannelAccountUsageWindowSpec{{Name: "last_5h", Since: 1}}, false)
	require.NoError(t, err)
	require.Len(t, aggregates, 1)
	require.Equal(t, "last_5h", aggregates[0].Window)
	require.Equal(t, "openai:codex:subject", aggregates[0].AccountIdentityKey)
	require.Equal(t, int64(1), aggregates[0].Requests)
	require.Equal(t, int64(1), aggregates[0].SuccessRequests)
	require.Equal(t, int64(200), aggregates[0].TotalTokens)
	require.Equal(t, int64(2000), aggregates[0].Quota)
	require.InEpsilon(t, 0.03125, aggregates[0].UpstreamCostTotal, 0.0001)
	require.InEpsilon(t, 1500, aggregates[0].AvgDurationMs, 0.0001)
	require.InEpsilon(t, 360, aggregates[0].AvgTTFTMs, 0.0001)
}

func TestChannelAccountUsageAttemptDoesNotOverwritePositiveCompletedAtWithInvalidValue(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	requestID := "req-account-usage-invalid-completed-at"

	require.NoError(t, UpsertChannelAccountUsageBilling(ChannelAccountUsageEvent{
		RequestId:        requestID,
		ChannelID:        42,
		CredentialIndex:  2,
		CompletedAt:      110,
		PromptTokens:     12,
		CompletionTokens: 8,
		TotalTokens:      20,
		Quota:            200,
	}))
	require.NoError(t, UpsertChannelAccountUsageAttempt(ChannelAccountUsageEvent{
		RequestId:       requestID,
		ChannelID:       42,
		CredentialIndex: 2,
		CompletedAt:     -62135596800,
		Success:         true,
		StatusCode:      http.StatusOK,
		DurationMs:      1500,
	}))

	var row ChannelAccountUsageEvent
	require.NoError(t, db.First(&row, "request_id = ?", requestID).Error)
	require.Equal(t, int64(110), row.CompletedAt)
	require.True(t, row.Success)
	require.Equal(t, http.StatusOK, row.StatusCode)
	require.Equal(t, int64(1500), row.DurationMs)
}

func TestChannelAccountUsageWindowAggregatesExcludeHealthProbes(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	require.NoError(t, db.Create(&[]ChannelAccountUsageEvent{
		{
			RequestId:          "req-user",
			ChannelID:          77,
			CredentialIndex:    0,
			AccountIdentityKey: "account-a",
			CompletedAt:        100,
			Success:            true,
			StatusCode:         http.StatusOK,
			TotalTokens:        20,
		},
		{
			RequestId:          "req-probe",
			ChannelID:          77,
			CredentialIndex:    0,
			AccountIdentityKey: "account-a",
			CompletedAt:        110,
			Success:            true,
			StatusCode:         http.StatusOK,
			IsHealthProbe:      true,
			TotalTokens:        999,
		},
	}).Error)

	aggregates, err := QueryChannelAccountUsageWindowAggregates(77, []ChannelAccountUsageWindowSpec{{Name: "today", Since: 1}}, false)
	require.NoError(t, err)
	require.Len(t, aggregates, 1)
	require.Equal(t, int64(1), aggregates[0].Requests)
	require.Equal(t, int64(20), aggregates[0].TotalTokens)

	aggregates, err = QueryChannelAccountUsageWindowAggregates(77, []ChannelAccountUsageWindowSpec{{Name: "today", Since: 1}}, true)
	require.NoError(t, err)
	require.Len(t, aggregates, 1)
	require.Equal(t, int64(2), aggregates[0].Requests)
	require.Equal(t, int64(1019), aggregates[0].TotalTokens)
}

func TestChannelAccountUsageWindowAggregatesFallbackToCreatedAtForInvalidCompletedAt(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	require.NoError(t, db.Create(&[]ChannelAccountUsageEvent{
		{
			RequestId:          "req-invalid-completed",
			CreatedAt:          100,
			UpdatedAt:          -62135596800,
			ChannelID:          77,
			CredentialIndex:    0,
			AccountIdentityKey: "account-a",
			CompletedAt:        -62135596800,
			Success:            true,
			StatusCode:         http.StatusOK,
			TotalTokens:        20,
			Quota:              200,
		},
		{
			RequestId:          "req-pending-dispatch",
			CreatedAt:          120,
			UpdatedAt:          120,
			ChannelID:          77,
			CredentialIndex:    0,
			AccountIdentityKey: "account-a",
			CompletedAt:        0,
		},
	}).Error)

	aggregates, err := QueryChannelAccountUsageWindowAggregates(77, []ChannelAccountUsageWindowSpec{{Name: "today", Since: 90}}, false)
	require.NoError(t, err)
	require.Len(t, aggregates, 1)
	require.Equal(t, int64(1), aggregates[0].Requests)
	require.Equal(t, int64(20), aggregates[0].TotalTokens)
	require.Equal(t, int64(200), aggregates[0].Quota)
	require.Equal(t, int64(100), aggregates[0].LastActiveAt)

	aggregates, err = QueryChannelAccountUsageWindowAggregates(77, []ChannelAccountUsageWindowSpec{{Name: "today", Since: 110}}, false)
	require.NoError(t, err)
	require.Empty(t, aggregates)
}
