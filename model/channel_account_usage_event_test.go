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
	require.NoError(t, db.AutoMigrate(
		&Channel{},
		&ChannelAccountUsageEvent{},
		&ModelExecutionRecord{},
		&ModelGatewayScoreEvent{},
		&ModelGatewayRequestCostSummary{},
		&ModelGatewayUserRequestSummary{},
	))
	require.NoError(t, EnsureModelExecutionRecordRequestMetaCapacity(db))

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

func TestChannelAccountRequestReconcileQueriesOmitHeavyPayloads(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	requestID := "req-reconcile-light-query"
	largeJSON := strings.Repeat(`{"payload":"x"}`, 128)

	require.NoError(t, db.Create(&ModelExecutionRecord{
		RequestId:       requestID,
		AttemptIndex:    0,
		ChannelId:       12,
		Success:         true,
		RequestMeta:     largeJSON,
		ScoreBreakdown:  largeJSON,
		CandidateGroups: largeJSON,
	}).Error)
	require.NoError(t, db.Create(&ModelGatewayScoreEvent{
		TraceID:            "trace-light-query",
		RequestID:          requestID,
		ChannelID:          12,
		CredentialIndex:    1,
		SampleDecisionJSON: largeJSON,
		ChangedItemsJSON:   largeJSON,
		ContextJSON:        largeJSON,
	}).Error)
	require.NoError(t, db.Create(&ModelGatewayRequestCostSummary{
		RequestId:     requestID,
		ChannelID:     12,
		BreakdownJSON: largeJSON,
		CostSource:    "profile",
		CostAccuracy:  "precise",
	}).Error)

	executionRecords, err := QueryModelExecutionRecordsByRequestId(requestID, 20)
	require.NoError(t, err)
	require.Len(t, executionRecords, 1)
	require.Empty(t, executionRecords[0].RequestMeta)
	require.Empty(t, executionRecords[0].ScoreBreakdown)
	require.Empty(t, executionRecords[0].CandidateGroups)

	scoreEvents, err := QueryModelGatewayScoreEventsByRequestId(requestID, 20)
	require.NoError(t, err)
	require.Len(t, scoreEvents, 1)
	require.Empty(t, scoreEvents[0].SampleDecisionJSON)
	require.Empty(t, scoreEvents[0].ChangedItemsJSON)
	require.Empty(t, scoreEvents[0].ContextJSON)

	costSummary, err := GetModelGatewayRequestCostSummaryByRequestId(requestID)
	require.NoError(t, err)
	require.NotNil(t, costSummary)
	require.Empty(t, costSummary.BreakdownJSON)
}

func TestChannelAccountUsageBillingBackfillsIdentityFromChannelCredentialIndex(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	oldCryptoSecret := common.CryptoSecret
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldChannelsIDM := channelsIDM
	common.CryptoSecret = "test-secret"
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.CryptoSecret = oldCryptoSecret
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelsIDM = oldChannelsIDM
	})

	key := `{"account_id":"acct-123","email":"a@example.com","access_token":"access-a","refresh_token":"refresh-a"}`
	channelsIDM = map[int]*Channel{
		88: {
			Id:     88,
			Type:   constant.ChannelTypeCodex,
			Key:    key,
			Status: common.ChannelStatusEnabled,
			ChannelInfo: ChannelInfo{
				IsMultiKey:   true,
				MultiKeySize: 1,
			},
		},
	}
	requestID := "req-account-usage-billing-identity-fallback"

	require.NoError(t, UpsertChannelAccountUsageCost(ModelGatewayRequestCostSummary{
		RequestId:         requestID,
		ChannelID:         88,
		UpstreamModel:     "gpt-5.5",
		UpstreamCostTotal: 0.0125,
		CostSource:        "profile",
		CostAccuracy:      "precise",
		CalculatedAt:      90,
	}))
	require.NoError(t, UpsertChannelAccountUsageBilling(ChannelAccountUsageEvent{
		RequestId:        requestID,
		ChannelID:        88,
		CredentialIndex:  0,
		RequestedModel:   "gpt-5.5",
		CompletedAt:      100,
		PromptTokens:     300,
		CompletionTokens: 120,
		TotalTokens:      420,
		Quota:            4200,
	}))

	expectedSubjectFP := common.GenerateHMAC("codex:account_id:acct-123")
	var row ChannelAccountUsageEvent
	require.NoError(t, db.First(&row, "request_id = ?", requestID).Error)
	require.Equal(t, 88, row.ChannelID)
	require.Equal(t, 0, row.CredentialIndex)
	require.Equal(t, "codex_oauth:codex:"+expectedSubjectFP, row.AccountIdentityKey)
	require.Equal(t, row.AccountIdentityKey, row.AccountID)
	require.Equal(t, expectedSubjectFP, row.CredentialSubjectFingerprint)
	require.Equal(t, common.GenerateHMAC(key), row.CredentialFingerprint)
	require.Equal(t, "oauth_account", row.AccountType)
	require.Equal(t, "codex", row.Brand)
	require.Equal(t, "codex_oauth", row.Provider)
	require.Equal(t, int64(420), row.TotalTokens)
	require.InEpsilon(t, 0.0125, row.UpstreamCostTotal, 0.0001)
	require.Equal(t, "profile", row.CostSource)
}

func TestRefreshChannelAccountUsageAttributionBackfillsRecentRows(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	oldCryptoSecret := common.CryptoSecret
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldChannelsIDM := channelsIDM
	common.CryptoSecret = "test-secret"
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.CryptoSecret = oldCryptoSecret
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelsIDM = oldChannelsIDM
	})

	key := "sk-refresh-primary\n" + `{"account_id":"acct-refresh","email":"refresh@example.com","access_token":"access-refresh","refresh_token":"refresh-refresh"}`
	channelsIDM = map[int]*Channel{
		89: {
			Id:     89,
			Name:   "refresh codex",
			Type:   constant.ChannelTypeCodex,
			Key:    key,
			Status: common.ChannelStatusEnabled,
			ChannelInfo: ChannelInfo{
				IsMultiKey:   true,
				MultiKeySize: 2,
			},
		},
	}
	require.NoError(t, db.Create(&ChannelAccountUsageEvent{
		RequestId:       "req-refresh-attribution",
		ChannelID:       89,
		CredentialIndex: 1,
		CompletedAt:     200,
		Success:         true,
		StatusCode:      http.StatusOK,
	}).Error)

	result, err := RefreshChannelAccountUsageAttribution(89, 1, 100, 100)
	require.NoError(t, err)
	require.Equal(t, 1, result.Scanned)
	require.Equal(t, 1, result.Updated)

	rows, err := QueryChannelAccountUsageRecentEvents(89, 1, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "refresh codex", rows[0].ChannelName)
	require.Equal(t, common.GenerateHMAC("codex:account_id:acct-refresh"), rows[0].CredentialSubjectFingerprint)
	require.NotEmpty(t, rows[0].AccountIdentityKey)
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

func TestChannelAccountUsageAttemptKeepsNewestSuccessfulAttribution(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	requestID := "req-account-usage-stale-attempt"

	require.NoError(t, UpsertChannelAccountUsageAttempt(ChannelAccountUsageEvent{
		RequestId:          requestID,
		AttemptIndex:       1,
		ChannelID:          42,
		CredentialIndex:    1,
		AccountIdentityKey: "account-final",
		CompletedAt:        130,
		Success:            true,
		StatusCode:         http.StatusOK,
		DurationMs:         900,
	}))
	require.NoError(t, UpsertChannelAccountUsageAttempt(ChannelAccountUsageEvent{
		RequestId:          requestID,
		AttemptIndex:       0,
		ChannelID:          42,
		CredentialIndex:    0,
		AccountIdentityKey: "account-stale",
		CompletedAt:        120,
		Success:            false,
		StatusCode:         http.StatusGatewayTimeout,
		ErrorCategory:      "timeout",
		DurationMs:         3000,
	}))

	var row ChannelAccountUsageEvent
	require.NoError(t, db.First(&row, "request_id = ?", requestID).Error)
	require.Equal(t, 1, row.AttemptIndex)
	require.Equal(t, 1, row.CredentialIndex)
	require.Equal(t, "account-final", row.AccountIdentityKey)
	require.True(t, row.Success)
	require.Equal(t, http.StatusOK, row.StatusCode)
	require.Equal(t, int64(900), row.DurationMs)
}

func TestChannelAccountUsageAttemptDoesNotOverwriteBilledFinalAccount(t *testing.T) {
	db := setupChannelAccountUsageEventTestDB(t)
	requestID := "req-account-usage-billed-final"

	require.NoError(t, UpsertChannelAccountUsageBilling(ChannelAccountUsageEvent{
		RequestId:          requestID,
		ChannelID:          42,
		CredentialIndex:    1,
		AccountIdentityKey: "account-final",
		CompletedAt:        130,
		PromptTokens:       10,
		CompletionTokens:   20,
		TotalTokens:        30,
		Quota:              300,
	}))
	require.NoError(t, UpsertChannelAccountUsageAttempt(ChannelAccountUsageEvent{
		RequestId:          requestID,
		AttemptIndex:       0,
		ChannelID:          42,
		CredentialIndex:    0,
		AccountIdentityKey: "account-stale",
		CompletedAt:        120,
		Success:            false,
		StatusCode:         http.StatusGatewayTimeout,
		ErrorCategory:      "timeout",
	}))

	var row ChannelAccountUsageEvent
	require.NoError(t, db.First(&row, "request_id = ?", requestID).Error)
	require.Equal(t, 1, row.CredentialIndex)
	require.Equal(t, "account-final", row.AccountIdentityKey)
	require.Equal(t, int64(30), row.TotalTokens)
	require.Equal(t, int64(300), row.Quota)
	require.False(t, row.Success)
	require.Zero(t, row.StatusCode)
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
