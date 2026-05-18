package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type listModelsResponse struct {
	Success bool               `json:"success"`
	Data    []dto.OpenAIModels `json:"data"`
	Object  string             `json:"object"`
}

type codexModelsResponse struct {
	Models []dto.CodexModelInfo `json:"models"`
}

type stringListResponse struct {
	Success bool     `json:"success"`
	Data    []string `json:"data"`
}

func setupModelListControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	initModelListColumnNames(t)

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}, &model.Token{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func initModelListColumnNames(t *testing.T) {
	t.Helper()

	originalIsMasterNode := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	defer func() {
		common.IsMasterNode = originalIsMasterNode
		common.SQLitePath = originalSQLitePath
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	}()

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s_init?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, os.Setenv("SQL_DSN", "local"))

	require.NoError(t, model.InitDB())
	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func withTieredBillingConfig(t *testing.T, modes map[string]string, exprs map[string]string) {
	t.Helper()

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if strings.HasPrefix(key, "billing_setting.") {
			saved[key] = value
		}
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		model.InvalidatePricingCache()
	})

	modeBytes, err := common.Marshal(modes)
	require.NoError(t, err)
	exprBytes, err := common.Marshal(exprs)
	require.NoError(t, err)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": string(modeBytes),
		"billing_setting.billing_expr": string(exprBytes),
	}))
	model.InvalidatePricingCache()
}

func withSelfUseModeDisabled(t *testing.T) {
	t.Helper()

	original := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = false
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = original
	})
}

func decodeListModelsResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]struct{} {
	t.Helper()

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, "list", payload.Object)

	ids := make(map[string]struct{}, len(payload.Data))
	for _, item := range payload.Data {
		ids[item.Id] = struct{}{}
	}
	return ids
}

func pricingByModelName(pricings []model.Pricing) map[string]model.Pricing {
	byName := make(map[string]model.Pricing, len(pricings))
	for _, pricing := range pricings {
		byName[pricing.ModelName] = pricing
	}
	return byName
}

func withMemoryCacheEnabled(t *testing.T, enabled bool) {
	t.Helper()

	original := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = enabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = original
	})
}

func withSelfUseModeEnabled(t *testing.T) {
	t.Helper()

	original := operation_setting.SelfUseModeEnabled
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() {
		operation_setting.SelfUseModeEnabled = original
	})
}

func TestListModelsIncludesTieredBillingModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, map[string]string{
		"zz-tiered-visible-model":      "tiered_expr",
		"zz-tiered-empty-expr-model":   "tiered_expr",
		"zz-tiered-missing-expr-model": "tiered_expr",
	}, map[string]string{
		"zz-tiered-visible-model":    `tier("base", p * 1 + c * 2)`,
		"zz-tiered-empty-expr-model": "   ",
	})

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       1001,
		Username: "model-list-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "zz-tiered-visible-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "zz-tiered-empty-expr-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "zz-tiered-missing-expr-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "zz-unpriced-model", ChannelId: 1, Enabled: true},
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 1001)

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "zz-tiered-visible-model")
	require.NotContains(t, ids, "zz-tiered-empty-expr-model")
	require.NotContains(t, ids, "zz-tiered-missing-expr-model")
	require.NotContains(t, ids, "zz-unpriced-model")

	pricingByName := pricingByModelName(model.GetPricing())
	visiblePricing, ok := pricingByName["zz-tiered-visible-model"]
	require.True(t, ok)
	require.Equal(t, "tiered_expr", visiblePricing.BillingMode)
	require.NotEmpty(t, visiblePricing.BillingExpr)

	emptyExprPricing, ok := pricingByName["zz-tiered-empty-expr-model"]
	require.True(t, ok)
	require.Empty(t, emptyExprPricing.BillingMode)
	require.Empty(t, emptyExprPricing.BillingExpr)

	missingExprPricing, ok := pricingByName["zz-tiered-missing-expr-model"]
	require.True(t, ok)
	require.Empty(t, missingExprPricing.BillingMode)
	require.Empty(t, missingExprPricing.BillingExpr)
}

func TestListModelsTokenLimitIncludesTieredBillingModel(t *testing.T) {
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, map[string]string{
		"zz-token-tiered-visible-model":      "tiered_expr",
		"zz-token-tiered-empty-expr-model":   "tiered_expr",
		"zz-token-tiered-missing-expr-model": "tiered_expr",
	}, map[string]string{
		"zz-token-tiered-visible-model":    `tier("base", p * 1 + c * 2)`,
		"zz-token-tiered-empty-expr-model": "",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, map[string]bool{
		"zz-token-tiered-visible-model":      true,
		"zz-token-tiered-empty-expr-model":   true,
		"zz-token-tiered-missing-expr-model": true,
		"zz-token-unpriced-model":            true,
	})

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "zz-token-tiered-visible-model")
	require.NotContains(t, ids, "zz-token-tiered-empty-expr-model")
	require.NotContains(t, ids, "zz-token-tiered-missing-expr-model")
	require.NotContains(t, ids, "zz-token-unpriced-model")
}

func TestListModelsFiltersStaleAbilitiesWithoutMemoryCache(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, false)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       2001,
		Username: "model-list-channel-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:     1,
		Name:   "primary-channel",
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "zz-real-routable-model",
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "zz-real-routable-model", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "zz-stale-ability-model", ChannelId: 999, Enabled: true},
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 2001)

	ListModels(ctx, constant.ChannelTypeOpenAI)

	ids := decodeListModelsResponse(t, recorder)
	require.Contains(t, ids, "zz-real-routable-model")
	require.NotContains(t, ids, "zz-stale-ability-model")
}

func TestGetTokenModelsUsesTokenEffectiveGroupAvailability(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, true)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       2002,
		Username: "token-model-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:     3001,
		UserId: 2002,
		Key:    "token-key",
		Name:   "token-models",
		Status: common.TokenStatusEnabled,
		Group:  "",
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:     2,
		Name:   "default-channel",
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "zz-token-routable-model",
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "default", Model: "zz-token-routable-model", ChannelId: 2, Enabled: true},
		{Group: "default", Model: "zz-token-stale-model", ChannelId: 998, Enabled: true},
	}).Error)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/token/3001/models", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "3001"}}
	ctx.Set("id", 2002)

	GetTokenModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload stringListResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "zz-token-routable-model")
	require.NotContains(t, payload.Data, "zz-token-stale-model")
}

func TestGetTokenModelsTreatsEmptyGroupAsAuto(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, true)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       2003,
		Username: "token-auto-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:     3002,
		UserId: 2003,
		Key:    "token-auto-key",
		Name:   "token-auto-models",
		Status: common.TokenStatusEnabled,
		Group:  "",
	}).Error)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["codex-plus","codex-pro"]`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	})
	require.NoError(t, db.Create(&[]model.Channel{
		{
			Id:     3,
			Name:   "codex-plus-channel",
			Key:    "test-key",
			Status: common.ChannelStatusEnabled,
			Group:  "codex-plus",
			Models: "zz-auto-codex-plus-model",
		},
		{
			Id:     4,
			Name:   "codex-pro-channel",
			Key:    "test-key",
			Status: common.ChannelStatusEnabled,
			Group:  "codex-pro",
			Models: "zz-auto-codex-pro-model",
		},
	}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "codex-plus", Model: "zz-auto-codex-plus-model", ChannelId: 3, Enabled: true},
		{Group: "codex-pro", Model: "zz-auto-codex-pro-model", ChannelId: 4, Enabled: true},
	}).Error)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/token/3002/models", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "3002"}}
	ctx.Set("id", 2003)

	GetTokenModels(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload stringListResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Contains(t, payload.Data, "zz-auto-codex-plus-model")
	require.Contains(t, payload.Data, "zz-auto-codex-pro-model")
}

func TestListModelsAdvertisesResponsesForOpenAIWireAPI(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, false)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       4001,
		Username: "wire-api-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:            10,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "wire-api-channel",
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Group:         "default",
		Models:        "zz-wire-api-model",
		OtherSettings: `{"wire_api":"responses"}`,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "zz-wire-api-model",
		ChannelId: 10,
		Enabled:   true,
	}).Error)
	model.RefreshPricing()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 4001)

	ListModels(ctx, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	var found bool
	for _, item := range payload.Data {
		if item.Id != "zz-wire-api-model" {
			continue
		}
		found = true
		require.Contains(t, item.SupportedEndpointTypes, constant.EndpointTypeOpenAIResponse)
		require.Contains(t, item.SupportedEndpointTypes, constant.EndpointTypeOpenAI)
		require.Equal(t, constant.EndpointTypeOpenAIResponse, item.SupportedEndpointTypes[0])
	}
	require.True(t, found, "expected wire-api model to appear in /v1/models")
}

func TestListModelsAdvertisesSessionModesAndActualReturnedModel(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, false)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       5001,
		Username: "model-capability-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	modelMapping := `{"gpt-5.5":"gpt-5.4"}`
	require.NoError(t, db.Create(&model.Channel{
		Id:            20,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "mapped-response-channel",
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Group:         "default",
		Models:        "gpt-5.5",
		ModelMapping:  &modelMapping,
		OtherSettings: `{"wire_api":"responses"}`,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: 20,
		Enabled:   true,
	}).Error)
	model.RefreshPricing()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 5001)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	ListModels(ctx, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	var found bool
	for _, item := range payload.Data {
		if item.Id != "gpt-5.5" {
			continue
		}
		found = true
		require.Equal(t, []string{"chat_completions", "responses"}, item.SupportedSessionModes)
		require.Equal(t, map[string]string{
			"chat_completions": "gpt-5.4",
			"responses":        "gpt-5.4",
		}, item.ActualModelReturned)
	}
	require.True(t, found, "expected gpt-5.5 to appear in /v1/models")
}

func TestListModelsAdvertisesImageSessionModes(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, false)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       5002,
		Username: "image-capability-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:            21,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "image-channel",
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Group:         "default",
		Models:        "gpt-image-2",
		OtherSettings: `{"codex_compatibility_mode":true,"codex_image_generation_tool_supported":true}`,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-image-2",
		ChannelId: 21,
		Enabled:   true,
	}).Error)
	model.RefreshPricing()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 5002)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	ListModels(ctx, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	var found bool
	for _, item := range payload.Data {
		if item.Id != "gpt-image-2" {
			continue
		}
		found = true
		require.Contains(t, item.SupportedEndpointTypes, constant.EndpointTypeImageGeneration)
		require.Contains(t, item.SupportedEndpointTypes, constant.EndpointTypeImageEdit)
		require.Equal(t, []string{"image_generation", "image_edit"}, item.SupportedSessionModes)
		require.Equal(t, map[string]bool{dto.BuildInToolImageGeneration: true}, item.Capabilities)
		require.Contains(t, item.ExperimentalSupportedTools, dto.BuildInToolImageGeneration)
		require.Equal(t, []string{"text", "image"}, item.InputModalities)
		require.Equal(t, []string{"image"}, item.OutputModalities)
		require.Equal(t, []string{"text", "image"}, item.SupportedModalities)
		require.Nil(t, item.ActualModelReturned)
	}
	require.True(t, found, "expected gpt-image-2 to appear in /v1/models")
}

func TestListModelsDoesNotAdvertiseImageSessionModesWithoutCodexCompatibility(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, false)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       5004,
		Username: "plain-image-capability-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:     23,
		Type:   constant.ChannelTypeOpenAI,
		Name:   "plain-image-channel",
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-image-2",
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-image-2",
		ChannelId: 23,
		Enabled:   true,
	}).Error)
	model.RefreshPricing()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	ctx.Set("id", 5004)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	ListModels(ctx, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload listModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	var found bool
	for _, item := range payload.Data {
		if item.Id != "gpt-image-2" {
			continue
		}
		found = true
		require.NotContains(t, item.SupportedEndpointTypes, constant.EndpointTypeImageGeneration)
		require.NotContains(t, item.SupportedEndpointTypes, constant.EndpointTypeImageEdit)
		require.Empty(t, item.SupportedSessionModes)
		require.Nil(t, item.Capabilities)
		require.Empty(t, item.ExperimentalSupportedTools)
		require.Equal(t, []string{"text", "image"}, item.InputModalities)
		require.Equal(t, []string{"text"}, item.OutputModalities)
	}
	require.True(t, found, "expected gpt-image-2 to appear in /v1/models")
}

func TestListModelsCodexFormatAdvertisesImageGenerationToolOnlyOnImageModels(t *testing.T) {
	withSelfUseModeEnabled(t)
	withMemoryCacheEnabled(t, false)

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       5003,
		Username: "codex-models-user",
		Password: "password",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:            22,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "codex-text-channel",
		Key:           "test-key",
		Status:        common.ChannelStatusEnabled,
		Group:         "default",
		Models:        "gpt-5.5,gpt-image-2",
		OtherSettings: `{"codex_compatibility_mode":true,"codex_image_generation_tool_supported":true}`,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.5",
		ChannelId: 22,
		Enabled:   true,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-image-2",
		ChannelId: 22,
		Enabled:   true,
	}).Error)
	model.RefreshPricing()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models?format=codex", nil)
	ctx.Set("id", 5003)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	ListModels(ctx, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload codexModelsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))

	var foundText bool
	var foundImage bool
	for _, item := range payload.Models {
		switch item.Slug {
		case "gpt-5.5":
			foundText = true
			require.Contains(t, item.InputModalities, "image")
			require.NotContains(t, item.ExperimentalSupportedTools, dto.BuildInToolImageGeneration)
			require.Contains(t, item.SupportedSessionModes, "responses")
		case "gpt-image-2":
			foundImage = true
			require.Contains(t, item.SupportedEndpointTypes, constant.EndpointTypeImageGeneration)
			require.Contains(t, item.SupportedSessionModes, "image_generation")
			require.Contains(t, item.ExperimentalSupportedTools, dto.BuildInToolImageGeneration)
			require.Equal(t, map[string]bool{dto.BuildInToolImageGeneration: true}, item.Capabilities)
			require.Equal(t, []string{"text", "image"}, item.InputModalities)
			require.Equal(t, []string{"image"}, item.OutputModalities)
			require.Equal(t, []string{"text", "image"}, item.SupportedModalities)
		}
	}
	require.True(t, foundText, "expected gpt-5.5 to appear in codex /models response")
	require.True(t, foundImage, "expected gpt-image-2 to appear in codex /models response")
}
