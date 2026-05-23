package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type channelCostProfileAPIResponse struct {
	Success bool                                 `json:"success"`
	Message string                               `json:"message"`
	Data    model.ModelGatewayChannelCostProfile `json:"data"`
}

type channelCostProfileListAPIResponse struct {
	Success bool                                   `json:"success"`
	Message string                                 `json:"message"`
	Data    []model.ModelGatewayChannelCostProfile `json:"data"`
}

type channelCostQuoteAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Models  []string                                     `json:"models"`
		Quotes  map[string]modelgatewaycost.SystemRatioQuote `json:"quotes"`
		Default model.ModelGatewayChannelCostProfile         `json:"default_profile"`
	} `json:"data"`
}

type channelListAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Items []struct {
			Id                  int `json:"id"`
			UpstreamCostDisplay struct {
				Configured            bool    `json:"configured"`
				PriceConfigured       bool    `json:"price_configured"`
				Model                 string  `json:"model"`
				PricingModel          string  `json:"pricing_model"`
				CostCoefficient       float64 `json:"cost_coefficient"`
				FeeMultiplier         float64 `json:"fee_multiplier"`
				ActualTokenMultiplier float64 `json:"actual_token_multiplier"`
				InputPerMillion       float64 `json:"input_per_million"`
				OutputPerMillion      float64 `json:"output_per_million"`
				CacheReadPerMillion   float64 `json:"cache_read_per_million"`
			} `json:"upstream_cost_display"`
		} `json:"items"`
	} `json:"data"`
}

func setupChannelCostProfileControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	ratio_setting.InitRatioSettings()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ModelGatewayChannelCostProfile{}))

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

func TestChannelUpstreamCostProfileCRUD(t *testing.T) {
	db := setupChannelCostProfileControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 7, Name: "primary", Key: "sk", Models: "gpt-5.5", Group: "default"}).Error)

	router := gin.New()
	router.GET("/api/channel/:id/upstream_cost_profiles", ListChannelUpstreamCostProfiles)
	router.POST("/api/channel/:id/upstream_cost_profiles", SaveChannelUpstreamCostProfile)
	router.PUT("/api/channel/:id/upstream_cost_profiles", SaveChannelUpstreamCostProfile)
	router.DELETE("/api/channel/:id/upstream_cost_profiles/:profile_id", DeleteChannelUpstreamCostProfile)
	router.GET("/api/channel/:id/upstream_cost_quote", GetChannelUpstreamCostQuote)
	router.POST("/api/channel/upstream_cost_quote", GetUnsavedChannelUpstreamCostQuote)

	createBody, err := common.Marshal(model.ModelGatewayChannelCostProfile{
		UpstreamModel:        " gpt-5.5 ",
		Currency:             "",
		CostCoefficient:      0.5,
		TokenMultiplier:      0.06,
		InputCostMultiplier:  0.05,
		OutputCostMultiplier: 0.08,
		CacheReadMultiplier:  0.02,
		CacheWriteMultiplier: 0.03,
		RequestPrice:         0.003,
		RechargeMultiplier:   0.8,
		MetadataJSON:         `{"recharge_paid_amount":50.6,"recharge_received_amount":100,"recharge_multiplier_source":"paid_received_ratio"}`,
	})
	require.NoError(t, err)

	createRecorder := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/channel/7/upstream_cost_profiles", bytes.NewReader(createBody))
	router.ServeHTTP(createRecorder, createReq)
	createPayload := decodeChannelCostProfileResponse(t, createRecorder)
	require.True(t, createPayload.Success, createRecorder.Body.String())
	require.Equal(t, 7, createPayload.Data.ChannelID)
	require.Equal(t, "*", createPayload.Data.UpstreamModel)
	require.Equal(t, "USD", createPayload.Data.Currency)
	require.Equal(t, "system_ratio", createPayload.Data.Source)
	require.Equal(t, "token", createPayload.Data.PricingMode)
	require.Equal(t, "estimated", createPayload.Data.Accuracy)
	require.Equal(t, 1, createPayload.Data.Version)
	require.Equal(t, 0.5, createPayload.Data.CostCoefficient)
	require.Equal(t, 0.06, createPayload.Data.TokenMultiplier)
	require.Equal(t, 0.06, createPayload.Data.InputCostMultiplier)
	require.Equal(t, 0.06, createPayload.Data.OutputCostMultiplier)
	require.Equal(t, 0.06, createPayload.Data.CacheReadMultiplier)
	require.Equal(t, 0.06, createPayload.Data.CacheWriteMultiplier)
	require.Equal(t, 1.0, createPayload.Data.RequestCostMultiplier)
	require.Equal(t, 0.003, createPayload.Data.RequestPrice)
	require.Equal(t, 0.8, createPayload.Data.RechargeMultiplier)
	require.Contains(t, createPayload.Data.MetadataJSON, "recharge_paid_amount")
	require.Contains(t, createPayload.Data.MetadataJSON, "paid_received_ratio")
	require.Zero(t, createPayload.Data.InputPerMillion)
	require.Zero(t, createPayload.Data.OutputPerMillion)

	updateBody, err := common.Marshal(model.ModelGatewayChannelCostProfile{
		Id:                  createPayload.Data.Id,
		UpstreamModel:       "gpt-5.5",
		Currency:            "usd",
		CostCoefficient:     0.25,
		InputCostMultiplier: 0.1,
		RequestPrice:        0.004,
		RechargeMultiplier:  1,
	})
	require.NoError(t, err)
	updateRecorder := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPut, "/api/channel/7/upstream_cost_profiles", bytes.NewReader(updateBody))
	router.ServeHTTP(updateRecorder, updateReq)
	updatePayload := decodeChannelCostProfileResponse(t, updateRecorder)
	require.True(t, updatePayload.Success, updateRecorder.Body.String())
	require.Equal(t, 0.25, updatePayload.Data.CostCoefficient)
	require.Equal(t, 0.1, updatePayload.Data.TokenMultiplier)
	require.Equal(t, 0.1, updatePayload.Data.InputCostMultiplier)
	require.Equal(t, 0.1, updatePayload.Data.OutputCostMultiplier)
	require.Equal(t, 0.1, updatePayload.Data.CacheReadMultiplier)
	require.Equal(t, 0.1, updatePayload.Data.CacheWriteMultiplier)
	require.Equal(t, 1.0, updatePayload.Data.RequestCostMultiplier)
	require.Equal(t, 0.004, updatePayload.Data.RequestPrice)
	require.Equal(t, 1.0, updatePayload.Data.RechargeMultiplier)
	require.Zero(t, updatePayload.Data.InputPerMillion)
	require.Equal(t, "system_ratio", updatePayload.Data.Source)

	listRecorder := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/channel/7/upstream_cost_profiles", nil)
	router.ServeHTTP(listRecorder, listReq)
	listPayload := decodeChannelCostProfileListResponse(t, listRecorder)
	require.True(t, listPayload.Success, listRecorder.Body.String())
	require.Len(t, listPayload.Data, 1)
	require.Equal(t, "*", listPayload.Data[0].UpstreamModel)

	quoteRecorder := httptest.NewRecorder()
	quoteReq := httptest.NewRequest(http.MethodGet, "/api/channel/7/upstream_cost_quote", nil)
	router.ServeHTTP(quoteRecorder, quoteReq)
	require.Contains(t, quoteRecorder.Body.String(), `"gpt-5.5"`)
	require.Contains(t, quoteRecorder.Body.String(), `"system_ratio"`)

	unsavedQuoteBody, err := common.Marshal(gin.H{
		"models": []string{"gpt-4o", "gpt-4o", " "},
	})
	require.NoError(t, err)
	unsavedQuoteRecorder := httptest.NewRecorder()
	unsavedQuoteReq := httptest.NewRequest(http.MethodPost, "/api/channel/upstream_cost_quote", bytes.NewReader(unsavedQuoteBody))
	router.ServeHTTP(unsavedQuoteRecorder, unsavedQuoteReq)
	require.Contains(t, unsavedQuoteRecorder.Body.String(), `"gpt-4o"`)
	require.Contains(t, unsavedQuoteRecorder.Body.String(), `"input_per_million"`)
	require.Contains(t, unsavedQuoteRecorder.Body.String(), `"default_profile"`)

	defaultRuleBody, err := common.Marshal(model.ModelGatewayChannelCostProfile{
		UpstreamModel:   " ",
		EffectiveTime:   common.GetTimestamp() - 1,
		Version:         2,
		TokenMultiplier: 0.05,
	})
	require.NoError(t, err)
	defaultRuleRecorder := httptest.NewRecorder()
	defaultRuleReq := httptest.NewRequest(http.MethodPost, "/api/channel/7/upstream_cost_profiles", bytes.NewReader(defaultRuleBody))
	router.ServeHTTP(defaultRuleRecorder, defaultRuleReq)
	defaultPayload := decodeChannelCostProfileResponse(t, defaultRuleRecorder)
	require.True(t, defaultPayload.Success, defaultRuleRecorder.Body.String())
	require.Equal(t, "*", defaultPayload.Data.UpstreamModel)
	require.Equal(t, 1, defaultPayload.Data.Version)
	require.Equal(t, createPayload.Data.Id, defaultPayload.Data.Id)

	deleteRecorder := httptest.NewRecorder()
	deleteReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/channel/7/upstream_cost_profiles/%d", defaultPayload.Data.Id), nil)
	router.ServeHTTP(deleteRecorder, deleteReq)
	require.Contains(t, deleteRecorder.Body.String(), `"success":true`)

	var count int64
	require.NoError(t, db.Model(&model.ModelGatewayChannelCostProfile{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestChannelUpstreamCostProfileSystemRatioDerivation(t *testing.T) {
	db := setupChannelCostProfileControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 9, Name: "primary", Key: "sk", Models: "gpt-4o", Group: "default"}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/upstream_cost_profiles", SaveChannelUpstreamCostProfile)

	body, err := common.Marshal(model.ModelGatewayChannelCostProfile{
		UpstreamModel:      "gpt-4o",
		CostCoefficient:    0.5,
		TokenMultiplier:    0.05,
		RechargeMultiplier: 0.8,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/9/upstream_cost_profiles", bytes.NewReader(body))
	router.ServeHTTP(recorder, req)
	payload := decodeChannelCostProfileResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, "system_ratio", payload.Data.Source)
	require.Equal(t, "estimated", payload.Data.Accuracy)
	require.Equal(t, 0.5, payload.Data.CostCoefficient)
	require.Equal(t, 0.05, payload.Data.TokenMultiplier)
	require.Equal(t, 0.05, payload.Data.InputCostMultiplier)
	require.Equal(t, 0.05, payload.Data.OutputCostMultiplier)
	require.Equal(t, 0.05, payload.Data.CacheReadMultiplier)
	require.Equal(t, 0.05, payload.Data.CacheWriteMultiplier)
	require.Equal(t, 1.0, payload.Data.RequestCostMultiplier)
	require.Equal(t, 0.8, payload.Data.RechargeMultiplier)
	require.Zero(t, payload.Data.InputPerMillion)
	require.Zero(t, payload.Data.OutputPerMillion)
	require.Zero(t, payload.Data.CacheReadPerMillion)

	derived := modelgatewaycost.DeriveSystemRatioProfile("gpt-4o", payload.Data)
	require.InEpsilon(t, 0.078125, derived.InputPerMillion, 0.000001)
	require.InEpsilon(t, 0.3125, derived.OutputPerMillion, 0.000001)
	require.InEpsilon(t, 0.0390625, derived.CacheReadPerMillion, 0.000001)
}

func TestChannelUpstreamCostQuoteUsesMappedUpstreamPricingModel(t *testing.T) {
	db := setupChannelCostProfileControllerTestDB(t)
	modelMapping := `{"codex-plus":"gpt-5.4"}`
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.4":1.25}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-5.4":6}`))
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"gpt-5.4":0.1}`))
	require.NoError(t, db.Create(&model.Channel{
		Id:           10,
		Name:         "mapped",
		Key:          "sk",
		Models:       "codex-plus",
		ModelMapping: &modelMapping,
		Group:        "default",
	}).Error)

	router := gin.New()
	router.GET("/api/channel/:id/upstream_cost_quote", GetChannelUpstreamCostQuote)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/10/upstream_cost_quote", nil)
	router.ServeHTTP(recorder, req)
	payload := decodeChannelCostQuoteResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Data.Quotes, "codex-plus")
	quote := payload.Data.Quotes["codex-plus"]
	require.Equal(t, "codex-plus", quote.Model)
	require.Equal(t, "gpt-5.4", quote.PricingModel)
	require.Equal(t, "model_ratio", quote.PriceSource)
	require.InEpsilon(t, 2.5, quote.InputPerMillion, 0.000001)
	require.InEpsilon(t, 15.0, quote.OutputPerMillion, 0.000001)
	require.InEpsilon(t, 0.25, quote.CacheReadPerMillion, 0.000001)

	router.POST("/api/channel/upstream_cost_quote", GetUnsavedChannelUpstreamCostQuote)
	unsavedBody, err := common.Marshal(gin.H{
		"models":        []string{"codex-plus"},
		"model_mapping": modelMapping,
	})
	require.NoError(t, err)
	unsavedRecorder := httptest.NewRecorder()
	unsavedReq := httptest.NewRequest(http.MethodPost, "/api/channel/upstream_cost_quote", bytes.NewReader(unsavedBody))
	router.ServeHTTP(unsavedRecorder, unsavedReq)
	unsavedPayload := decodeChannelCostQuoteResponse(t, unsavedRecorder)
	require.True(t, unsavedPayload.Success, unsavedRecorder.Body.String())
	unsavedQuote := unsavedPayload.Data.Quotes["codex-plus"]
	require.Equal(t, "codex-plus", unsavedQuote.Model)
	require.Equal(t, "gpt-5.4", unsavedQuote.PricingModel)
	require.InEpsilon(t, 2.5, unsavedQuote.InputPerMillion, 0.000001)
}

func TestChannelListIncludesOneToOneUpstreamCostDisplay(t *testing.T) {
	db := setupChannelCostProfileControllerTestDB(t)
	modelMapping := `{"codex-plus":"gpt-5.4"}`
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.4":1.25}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-5.4":6}`))
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"gpt-5.4":0.1}`))
	require.NoError(t, db.Create(&model.Channel{
		Id:           4,
		Name:         "toioto",
		Key:          "sk",
		Models:       "codex-plus",
		ModelMapping: &modelMapping,
		Group:        "default",
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:          4,
		UpstreamModel:      "*",
		Currency:           "USD",
		PricingMode:        "token",
		Source:             "system_ratio",
		Accuracy:           "estimated",
		CostCoefficient:    0.05,
		TokenMultiplier:    4.1,
		RechargeMultiplier: 2,
		Version:            1,
		CreatedAt:          common.GetTimestamp(),
		UpdatedAt:          common.GetTimestamp(),
	}).Error)

	router := gin.New()
	router.GET("/api/channel/", GetAllChannels)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/channel/?p=1&page_size=20", nil)
	router.ServeHTTP(recorder, req)
	payload := decodeChannelListResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data.Items, 1)
	display := payload.Data.Items[0].UpstreamCostDisplay
	require.True(t, display.Configured)
	require.True(t, display.PriceConfigured)
	require.Equal(t, "codex-plus", display.Model)
	require.Equal(t, "gpt-5.4", display.PricingModel)
	require.InEpsilon(t, 0.05, display.CostCoefficient, 0.000001)
	require.InEpsilon(t, 4.1, display.FeeMultiplier, 0.000001)
	require.InEpsilon(t, 0.1025, display.ActualTokenMultiplier, 0.000001)
	require.InEpsilon(t, 0.25625, display.InputPerMillion, 0.000001)
	require.InEpsilon(t, 1.5375, display.OutputPerMillion, 0.000001)
	require.InEpsilon(t, 0.025625, display.CacheReadPerMillion, 0.000001)
}

func TestChannelUpstreamCostProfileValidation(t *testing.T) {
	db := setupChannelCostProfileControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 8, Name: "primary", Key: "sk", Models: "gpt-5.5", Group: "default"}).Error)

	router := gin.New()
	router.POST("/api/channel/:id/upstream_cost_profiles", SaveChannelUpstreamCostProfile)

	negativeBody, err := common.Marshal(model.ModelGatewayChannelCostProfile{
		UpstreamModel:       "gpt-5.5",
		InputCostMultiplier: -1,
		RechargeMultiplier:  1,
	})
	require.NoError(t, err)
	negativeRecorder := httptest.NewRecorder()
	negativeReq := httptest.NewRequest(http.MethodPost, "/api/channel/8/upstream_cost_profiles", bytes.NewReader(negativeBody))
	router.ServeHTTP(negativeRecorder, negativeReq)
	require.Contains(t, negativeRecorder.Body.String(), `"success":false`)
	require.Contains(t, negativeRecorder.Body.String(), "倍率必须是非负数")

	invalidMetadataBody, err := common.Marshal(model.ModelGatewayChannelCostProfile{
		UpstreamModel: "gpt-5.5",
		MetadataJSON:  `[]`,
	})
	require.NoError(t, err)
	invalidMetadataRecorder := httptest.NewRecorder()
	invalidMetadataReq := httptest.NewRequest(http.MethodPost, "/api/channel/8/upstream_cost_profiles", bytes.NewReader(invalidMetadataBody))
	router.ServeHTTP(invalidMetadataRecorder, invalidMetadataReq)
	require.Contains(t, invalidMetadataRecorder.Body.String(), `"success":false`)
	require.Contains(t, invalidMetadataRecorder.Body.String(), "元数据 JSON")
}

func decodeChannelCostProfileResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelCostProfileAPIResponse {
	t.Helper()
	var payload channelCostProfileAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}

func decodeChannelCostProfileListResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelCostProfileListAPIResponse {
	t.Helper()
	var payload channelCostProfileListAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}

func decodeChannelCostQuoteResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelCostQuoteAPIResponse {
	t.Helper()
	var payload channelCostQuoteAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}

func decodeChannelListResponse(t *testing.T, recorder *httptest.ResponseRecorder) channelListAPIResponse {
	t.Helper()
	var payload channelListAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	return payload
}
