package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type modelGatewayProxyAPIResponse struct {
	Success bool                      `json:"success"`
	Message string                    `json:"message"`
	Data    ModelGatewayProxyResponse `json:"data"`
}

type modelGatewayProxyListAPIResponse struct {
	Success bool                        `json:"success"`
	Message string                      `json:"message"`
	Data    []ModelGatewayProxyResponse `json:"data"`
}

func setupModelGatewayProxyControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ModelGatewayProxy{},
		&model.ModelGatewayProxyUsage{},
	))

	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.InvalidateModelGatewayProxyCache()
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestModelGatewayProxyListMasksSecretAndIncludesUsage(t *testing.T) {
	db := setupModelGatewayProxyControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       101,
		Name:     "proxy masked",
		Protocol: "socks5",
		Address:  "user:inline-pass@127.0.0.1:1080",
		Username: "owner",
		Password: "stored-password",
		Enabled:  true,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxyUsage{
		ProxyID:                      101,
		ChannelID:                    7,
		Brand:                        "OpenAI",
		Provider:                     "openai",
		AccountID:                    "acct-1",
		CredentialIndex:              2,
		CredentialSubjectFingerprint: "subject-fp",
		CredentialFingerprint:        "credential-fp",
		LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/proxies", ListModelGatewayProxies)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/proxies", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyListResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data, 1)
	require.Equal(t, "socks5://127.0.0.1:1080", payload.Data[0].MaskedAddress)
	require.Equal(t, payload.Data[0].MaskedAddress, payload.Data[0].Address)
	require.True(t, payload.Data[0].PasswordSet)
	require.NotContains(t, recorder.Body.String(), "inline-pass")
	require.NotContains(t, recorder.Body.String(), "stored-password")
	require.Len(t, payload.Data[0].BrandUsage, 1)
	require.Equal(t, "OpenAI", payload.Data[0].BrandUsage[0].Brand)
	require.Equal(t, 2, payload.Data[0].BrandUsage[0].CredentialIndex)
	require.Nil(t, payload.Data[0].ReuseRisks)
}

func TestUpdateModelGatewayProxyKeepsExistingAddressAndPasswordWhenBlank(t *testing.T) {
	db := setupModelGatewayProxyControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       102,
		Name:     "proxy keep",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Username: "old-user",
		Password: "old-password",
		Enabled:  true,
		Remark:   "old remark",
	}).Error)

	router := gin.New()
	router.PUT("/api/model_gateway/proxies/:proxy_id", UpdateModelGatewayProxy)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name":"proxy keep edited","protocol":"socks5h","address":"","username":"","password":"","enabled":false,"remark":""}`)
	req := httptest.NewRequest(http.MethodPut, "/api/model_gateway/proxies/102", body)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, "proxy keep edited", payload.Data.Name)
	require.Equal(t, "socks5h://127.0.0.1:1080", payload.Data.MaskedAddress)
	require.False(t, payload.Data.Enabled)
	require.True(t, payload.Data.PasswordSet)
	require.NotContains(t, recorder.Body.String(), "old-password")

	var updated model.ModelGatewayProxy
	require.NoError(t, db.First(&updated, "id = ?", 102).Error)
	require.Equal(t, "127.0.0.1:1080", updated.Address)
	require.Equal(t, "old-user", updated.Username)
	require.Equal(t, "old-password", updated.Password)
	require.Equal(t, "old remark", updated.Remark)
	require.Equal(t, "socks5h://old-user:old-password@127.0.0.1:1080", mustProxyURL(t, updated))
}

func TestCreateModelGatewayProxyWithSchemeAndAuthBuildsAuthenticatedURL(t *testing.T) {
	db := setupModelGatewayProxyControllerTestDB(t)

	router := gin.New()
	router.POST("/api/model_gateway/proxies", CreateModelGatewayProxy)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name":"proxy scheme","protocol":"socks5","address":"socks5://127.0.0.1:1080","username":"user","password":"pass","enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/proxies", body)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, "socks5://127.0.0.1:1080", payload.Data.MaskedAddress)
	require.True(t, payload.Data.PasswordSet)
	require.NotContains(t, recorder.Body.String(), `"password":"pass"`)
	require.NotContains(t, recorder.Body.String(), "socks5://user:pass")

	var created model.ModelGatewayProxy
	require.NoError(t, db.First(&created, "id = ?", payload.Data.ID).Error)
	require.Equal(t, "socks5://user:pass@127.0.0.1:1080", mustProxyURL(t, created))
}

func TestCreateModelGatewayProxyRejectsAddressWithoutPort(t *testing.T) {
	setupModelGatewayProxyControllerTestDB(t)

	router := gin.New()
	router.POST("/api/model_gateway/proxies", CreateModelGatewayProxy)
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name":"proxy no port","protocol":"socks5","address":"206.123.156.217","enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/proxies", body)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "请填写端口")
}

func TestModelGatewayProxyListReportsSameBrandReuseRisk(t *testing.T) {
	db := setupModelGatewayProxyControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       103,
		Name:     "shared exit",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxyUsage{
		ProxyID:                      103,
		ChannelID:                    7,
		Brand:                        "openai",
		Provider:                     "openai",
		AccountID:                    "acct-a",
		CredentialIndex:              0,
		CredentialSubjectFingerprint: "subject-a",
		CredentialFingerprint:        "credential-a",
		LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
		LastUsedAt:                   1700000100,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayProxyUsage{
		ProxyID:                      103,
		ChannelID:                    8,
		Brand:                        "openai",
		Provider:                     "openai",
		AccountID:                    "acct-b",
		CredentialIndex:              1,
		CredentialSubjectFingerprint: "subject-b",
		CredentialFingerprint:        "credential-b",
		LastStatus:                   model.ModelGatewayProxyUsageStatusBound,
		LastUsedAt:                   1700000200,
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/proxies", ListModelGatewayProxies)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/proxies", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyListResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data, 1)
	require.Len(t, payload.Data[0].ReuseRisks, 1)
	require.Equal(t, "openai", payload.Data[0].ReuseRisks[0].Brand)
	require.Equal(t, 2, payload.Data[0].ReuseRisks[0].AccountCount)
	require.Equal(t, 2, payload.Data[0].ReuseRisks[0].CredentialCount)
	require.Equal(t, 2, payload.Data[0].ReuseRisks[0].DistinctSubjectCount)
	require.Equal(t, 2, payload.Data[0].ReuseRisks[0].ChannelCount)
	require.Equal(t, "same_brand_multi_account", payload.Data[0].ReuseRisks[0].Reason)
}

func TestModelGatewayProxyListIncludesGeoFields(t *testing.T) {
	db := setupModelGatewayProxyControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:           104,
		Name:         "hk exit",
		Protocol:     "socks5",
		Address:      "127.0.0.1:1080",
		Enabled:      true,
		ExitIP:       "66.93.162.84",
		RegionCode:   "US",
		RegionName:   "California",
		CountryName:  "United States",
		City:         "Los Angeles",
		Timezone:     "America/Los_Angeles",
		GeoCheckedAt: 1700000300,
		GeoStatus:    "success",
	}).Error)

	router := gin.New()
	router.GET("/api/model_gateway/proxies", ListModelGatewayProxies)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/model_gateway/proxies", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyListResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Len(t, payload.Data, 1)
	require.Equal(t, "US", payload.Data[0].RegionCode)
	require.Equal(t, "66.93.162.84", payload.Data[0].ExitIP)
	require.Equal(t, "Los Angeles", payload.Data[0].City)
	require.Equal(t, int64(1700000300), payload.Data[0].GeoCheckedAt)
}

func TestDetectModelGatewayProxyGeoSavesRegionCode(t *testing.T) {
	db := setupModelGatewayProxyControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       105,
		Name:     "geo direct",
		Protocol: "http",
		Address:  "127.0.0.1:18080",
		Enabled:  true,
	}).Error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"66.93.162.84","country_code":"us","country_name":"United States","region":"California","city":"Los Angeles","timezone":"America/Los_Angeles"}`))
	}))
	defer server.Close()
	originalEndpoints := modelGatewayProxyGeoEndpoints
	modelGatewayProxyGeoEndpoints = []string{server.URL}
	originalFactory := modelGatewayProxyGeoHTTPClientFactory
	modelGatewayProxyGeoHTTPClientFactory = func(proxyURL string) (*http.Client, error) {
		require.NotEmpty(t, proxyURL)
		return server.Client(), nil
	}
	t.Cleanup(func() {
		modelGatewayProxyGeoEndpoints = originalEndpoints
		modelGatewayProxyGeoHTTPClientFactory = originalFactory
	})

	router := gin.New()
	router.POST("/api/model_gateway/proxies/:proxy_id/detect", DetectModelGatewayProxyGeo)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/proxies/105/detect", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyResponse(t, recorder)
	require.True(t, payload.Success, recorder.Body.String())
	require.Equal(t, "US", payload.Data.RegionCode)
	require.Equal(t, "66.93.162.84", payload.Data.ExitIP)
	require.Equal(t, "success", payload.Data.GeoStatus)
	require.NotZero(t, payload.Data.GeoCheckedAt)

	var updated model.ModelGatewayProxy
	require.NoError(t, db.First(&updated, "id = ?", 105).Error)
	require.Equal(t, "US", updated.RegionCode)
	require.Equal(t, "United States", updated.CountryName)
	require.Equal(t, "Los Angeles", updated.City)
	require.Equal(t, "success", updated.GeoStatus)
}

func TestDetectModelGatewayProxyGeoSavesFailureReason(t *testing.T) {
	db := setupModelGatewayProxyControllerTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       106,
		Name:     "geo fail",
		Protocol: "http",
		Address:  "127.0.0.1:18080",
		Enabled:  true,
	}).Error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad geo", http.StatusBadGateway)
	}))
	defer server.Close()
	originalEndpoints := modelGatewayProxyGeoEndpoints
	modelGatewayProxyGeoEndpoints = []string{server.URL}
	originalFactory := modelGatewayProxyGeoHTTPClientFactory
	modelGatewayProxyGeoHTTPClientFactory = func(proxyURL string) (*http.Client, error) {
		require.NotEmpty(t, proxyURL)
		return server.Client(), nil
	}
	t.Cleanup(func() {
		modelGatewayProxyGeoEndpoints = originalEndpoints
		modelGatewayProxyGeoHTTPClientFactory = originalFactory
	})

	router := gin.New()
	router.POST("/api/model_gateway/proxies/:proxy_id/detect", DetectModelGatewayProxyGeo)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/model_gateway/proxies/106/detect", nil)
	router.ServeHTTP(recorder, req)

	payload := decodeModelGatewayProxyResponse(t, recorder)
	require.False(t, payload.Success, recorder.Body.String())
	require.Contains(t, payload.Message, "代理地区检测失败")

	var updated model.ModelGatewayProxy
	require.NoError(t, db.First(&updated, "id = ?", 106).Error)
	require.Equal(t, "failed", updated.GeoStatus)
	require.Contains(t, updated.GeoError, "HTTP 502")
	require.Equal(t, int64(1), updated.FailureCount)
	require.NotZero(t, updated.GeoCheckedAt)
}

func decodeModelGatewayProxyResponse(t *testing.T, recorder *httptest.ResponseRecorder) modelGatewayProxyAPIResponse {
	t.Helper()
	var payload modelGatewayProxyAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func decodeModelGatewayProxyListResponse(t *testing.T, recorder *httptest.ResponseRecorder) modelGatewayProxyListAPIResponse {
	t.Helper()
	var payload modelGatewayProxyListAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

func mustProxyURL(t *testing.T, proxy model.ModelGatewayProxy) string {
	t.Helper()
	proxyURL, err := proxy.ProxyURL()
	require.NoError(t, err)
	return proxyURL
}
