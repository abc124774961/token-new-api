package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const modelGatewayProxyGeoDetectTimeout = 12 * time.Second

var modelGatewayProxyGeoEndpoints = []string{
	"https://ipapi.co/json/",
	"https://ipwho.is/",
}

var modelGatewayProxyGeoHTTPClientFactory = service.NewProxyHttpClient

type ModelGatewayProxyResponse struct {
	ID             int                           `json:"id"`
	Name           string                        `json:"name"`
	Protocol       string                        `json:"protocol"`
	Address        string                        `json:"address"`
	MaskedAddress  string                        `json:"masked_address"`
	Username       string                        `json:"username,omitempty"`
	Enabled        bool                          `json:"enabled"`
	Remark         string                        `json:"remark,omitempty"`
	LastUsedAt     int64                         `json:"last_used_at,omitempty"`
	LastSuccessAt  int64                         `json:"last_success_at,omitempty"`
	LastFailureAt  int64                         `json:"last_failure_at,omitempty"`
	FailureCount   int64                         `json:"failure_count,omitempty"`
	UseCount       int64                         `json:"use_count,omitempty"`
	ExitIP         string                        `json:"exit_ip,omitempty"`
	RegionCode     string                        `json:"region_code,omitempty"`
	RegionName     string                        `json:"region_name,omitempty"`
	CountryName    string                        `json:"country_name,omitempty"`
	City           string                        `json:"city,omitempty"`
	Timezone       string                        `json:"timezone,omitempty"`
	GeoCheckedAt   int64                         `json:"geo_checked_at,omitempty"`
	GeoStatus      string                        `json:"geo_status,omitempty"`
	GeoError       string                        `json:"geo_error,omitempty"`
	CreatedTime    int64                         `json:"created_time"`
	UpdatedTime    int64                         `json:"updated_time"`
	BrandUsage     []ModelGatewayProxyUsageBrief `json:"brand_usage,omitempty"`
	ReuseRisks     []ModelGatewayProxyReuseRisk  `json:"reuse_risks,omitempty"`
	PasswordSet    bool                          `json:"password_set"`
	PasswordMasked bool                          `json:"password_masked"`
}

type ModelGatewayProxyUsageBrief struct {
	Brand                        string `json:"brand,omitempty"`
	Provider                     string `json:"provider,omitempty"`
	ChannelID                    int    `json:"channel_id,omitempty"`
	AccountID                    string `json:"account_id,omitempty"`
	CredentialIndex              int    `json:"credential_index,omitempty"`
	CredentialSubjectFingerprint string `json:"credential_subject_fingerprint,omitempty"`
	LastUsedAt                   int64  `json:"last_used_at,omitempty"`
	UseCount                     int64  `json:"use_count,omitempty"`
	LastStatus                   string `json:"last_status,omitempty"`
}

type ModelGatewayProxyReuseRisk struct {
	Brand                string `json:"brand,omitempty"`
	Provider             string `json:"provider,omitempty"`
	AccountCount         int    `json:"account_count"`
	CredentialCount      int    `json:"credential_count"`
	DistinctSubjectCount int    `json:"distinct_subject_count"`
	ChannelCount         int    `json:"channel_count"`
	LastUsedAt           int64  `json:"last_used_at,omitempty"`
	Reason               string `json:"reason,omitempty"`
}

type SaveModelGatewayProxyRequest struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  *bool  `json:"enabled"`
	Remark   string `json:"remark"`
}

func ListModelGatewayProxies(c *gin.Context) {
	enabledOnly := parseBoolQuery(c.Query("enabled_only"), false)
	proxies, err := model.ListModelGatewayProxies(enabledOnly)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usages, err := model.ListModelGatewayProxyUsages(proxyIDsFromModelGatewayProxies(proxies))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildModelGatewayProxyResponses(proxies, usages))
}

func CreateModelGatewayProxy(c *gin.Context) {
	var request SaveModelGatewayProxyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	proxy, err := modelGatewayProxyFromRequest(request, nil)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.DB.Create(proxy).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateModelGatewayProxyCache(proxy.ID)
	common.ApiSuccess(c, buildModelGatewayProxyResponse(*proxy, nil))
}

func UpdateModelGatewayProxy(c *gin.Context) {
	proxyID, ok := parseModelGatewayProxyIDParam(c)
	if !ok {
		return
	}
	existing, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil {
		common.ApiErrorMsg(c, "代理不存在")
		return
	}
	var request SaveModelGatewayProxyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	proxy, err := modelGatewayProxyFromRequest(request, existing)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.DB.Save(proxy).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateModelGatewayProxyCache(proxy.ID)
	common.ApiSuccess(c, buildModelGatewayProxyResponse(*proxy, nil))
}

func DetectModelGatewayProxyGeo(c *gin.Context) {
	proxyID, ok := parseModelGatewayProxyIDParam(c)
	if !ok {
		return
	}
	proxy, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil {
		common.ApiErrorMsg(c, "代理不存在")
		return
	}
	result, detectErr := detectModelGatewayProxyGeo(c.Request.Context(), *proxy)
	if detectErr != nil {
		now := common.GetTimestamp()
		proxy.GeoCheckedAt = now
		proxy.GeoStatus = "failed"
		proxy.GeoError = truncateModelGatewayProxyGeoError(detectErr.Error())
		proxy.LastFailureAt = now
		proxy.FailureCount++
		if err := model.DB.Save(proxy).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		model.InvalidateModelGatewayProxyCache(proxy.ID)
		common.ApiErrorMsg(c, "代理地区检测失败："+proxy.GeoError)
		return
	}
	applyModelGatewayProxyGeoResult(proxy, result)
	if err := model.DB.Save(proxy).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.InvalidateModelGatewayProxyCache(proxy.ID)
	common.ApiSuccess(c, buildModelGatewayProxyResponse(*proxy, nil))
}

func parseModelGatewayProxyIDParam(c *gin.Context) (int, bool) {
	proxyID, err := strconv.Atoi(c.Param("proxy_id"))
	if err != nil || proxyID <= 0 {
		common.ApiError(c, fmt.Errorf("代理 ID 无效"))
		return 0, false
	}
	return proxyID, true
}

func modelGatewayProxyFromRequest(request SaveModelGatewayProxyRequest, existing *model.ModelGatewayProxy) (*model.ModelGatewayProxy, error) {
	name := strings.TrimSpace(request.Name)
	address := model.NormalizeModelGatewayProxyAddress(request.Address)
	protocol := model.NormalizeModelGatewayProxyProtocol(request.Protocol)
	username := strings.TrimSpace(request.Username)
	password := request.Password
	if name == "" && existing != nil {
		name = existing.Name
	}
	if address == "" && existing != nil {
		address = existing.Address
	}
	if username == "" && existing != nil {
		username = existing.Username
	}
	if password == "" && existing != nil {
		password = existing.Password
	}
	if name == "" {
		name = defaultModelGatewayProxyName(protocol, address)
	}
	if err := validateModelGatewayProxyAddress(protocol, address); err != nil {
		return nil, err
	}
	enabled := true
	if existing != nil {
		enabled = existing.Enabled
	}
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	proxy := &model.ModelGatewayProxy{
		Name:     name,
		Protocol: protocol,
		Address:  address,
		Username: username,
		Password: password,
		Enabled:  enabled,
		Remark:   strings.TrimSpace(request.Remark),
	}
	if existing != nil {
		proxy.ID = existing.ID
		proxy.LastUsedAt = existing.LastUsedAt
		proxy.LastSuccessAt = existing.LastSuccessAt
		proxy.LastFailureAt = existing.LastFailureAt
		proxy.FailureCount = existing.FailureCount
		proxy.UseCount = existing.UseCount
		proxy.ExitIP = existing.ExitIP
		proxy.RegionCode = existing.RegionCode
		proxy.RegionName = existing.RegionName
		proxy.CountryName = existing.CountryName
		proxy.City = existing.City
		proxy.Timezone = existing.Timezone
		proxy.GeoCheckedAt = existing.GeoCheckedAt
		proxy.GeoStatus = existing.GeoStatus
		proxy.GeoError = existing.GeoError
		proxy.CreatedTime = existing.CreatedTime
		if request.Remark == "" {
			proxy.Remark = existing.Remark
		}
	}
	return proxy, nil
}

func validateModelGatewayProxyAddress(protocol string, address string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("请填写代理地址")
	}
	candidate := address
	if !strings.Contains(candidate, "://") {
		candidate = protocol + "://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return fmt.Errorf("代理地址无效: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("代理地址无效")
	}
	if strings.TrimSpace(parsed.Port()) == "" {
		return fmt.Errorf("代理地址无效：请填写端口，例如 127.0.0.1:1080")
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
	case model.ModelGatewayProxyProtocolHTTP, model.ModelGatewayProxyProtocolHTTPS, model.ModelGatewayProxyProtocolSOCKS5, model.ModelGatewayProxyProtocolSOCKS5H:
		return nil
	default:
		return fmt.Errorf("代理协议仅支持 http、https、socks5、socks5h")
	}
}

func defaultModelGatewayProxyName(protocol string, address string) string {
	address = strings.TrimSpace(address)
	if strings.Contains(address, "://") {
		parsed, err := url.Parse(address)
		if err == nil && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host
		}
	}
	if address == "" {
		return strings.ToUpper(model.NormalizeModelGatewayProxyProtocol(protocol))
	}
	return model.NormalizeModelGatewayProxyProtocol(protocol) + "://" + address
}

func buildModelGatewayProxyResponses(proxies []model.ModelGatewayProxy, usages []model.ModelGatewayProxyUsage) []ModelGatewayProxyResponse {
	usageByProxy := make(map[int][]model.ModelGatewayProxyUsage)
	for _, usage := range usages {
		usageByProxy[usage.ProxyID] = append(usageByProxy[usage.ProxyID], usage)
	}
	responses := make([]ModelGatewayProxyResponse, 0, len(proxies))
	for _, proxy := range proxies {
		responses = append(responses, buildModelGatewayProxyResponse(proxy, usageByProxy[proxy.ID]))
	}
	return responses
}

func buildModelGatewayProxyResponse(proxy model.ModelGatewayProxy, usages []model.ModelGatewayProxyUsage) ModelGatewayProxyResponse {
	response := ModelGatewayProxyResponse{
		ID:             proxy.ID,
		Name:           proxy.Name,
		Protocol:       model.NormalizeModelGatewayProxyProtocol(proxy.Protocol),
		Address:        proxy.MaskedAddress(),
		MaskedAddress:  proxy.MaskedAddress(),
		Username:       proxy.Username,
		Enabled:        proxy.Enabled,
		Remark:         proxy.Remark,
		LastUsedAt:     proxy.LastUsedAt,
		LastSuccessAt:  proxy.LastSuccessAt,
		LastFailureAt:  proxy.LastFailureAt,
		FailureCount:   proxy.FailureCount,
		UseCount:       proxy.UseCount,
		ExitIP:         proxy.ExitIP,
		RegionCode:     proxy.RegionCode,
		RegionName:     proxy.RegionName,
		CountryName:    proxy.CountryName,
		City:           proxy.City,
		Timezone:       proxy.Timezone,
		GeoCheckedAt:   proxy.GeoCheckedAt,
		GeoStatus:      proxy.GeoStatus,
		GeoError:       proxy.GeoError,
		CreatedTime:    proxy.CreatedTime,
		UpdatedTime:    proxy.UpdatedTime,
		PasswordSet:    proxy.Password != "",
		PasswordMasked: true,
		BrandUsage:     make([]ModelGatewayProxyUsageBrief, 0, len(usages)),
	}
	for _, usage := range usages {
		response.BrandUsage = append(response.BrandUsage, ModelGatewayProxyUsageBrief{
			Brand:                        usage.Brand,
			Provider:                     usage.Provider,
			ChannelID:                    usage.ChannelID,
			AccountID:                    usage.AccountID,
			CredentialIndex:              usage.CredentialIndex,
			CredentialSubjectFingerprint: usage.CredentialSubjectFingerprint,
			LastUsedAt:                   usage.LastUsedAt,
			UseCount:                     usage.UseCount,
			LastStatus:                   usage.LastStatus,
		})
	}
	response.ReuseRisks = buildModelGatewayProxyReuseRisks(usages)
	if len(response.BrandUsage) == 0 {
		response.BrandUsage = nil
	}
	if len(response.ReuseRisks) == 0 {
		response.ReuseRisks = nil
	}
	return response
}

func buildModelGatewayProxyReuseRisks(usages []model.ModelGatewayProxyUsage) []ModelGatewayProxyReuseRisk {
	type aggregate struct {
		brand        string
		provider     string
		accountIDs   map[string]struct{}
		credentialFP map[string]struct{}
		subjectFP    map[string]struct{}
		channelIDs   map[int]struct{}
		lastUsedAt   int64
	}
	aggregates := make(map[string]*aggregate)
	for _, usage := range usages {
		brand := strings.TrimSpace(usage.Brand)
		provider := strings.TrimSpace(usage.Provider)
		key := strings.ToLower(strings.TrimSpace(brand))
		if key == "" {
			key = strings.ToLower(provider)
		}
		if key == "" {
			continue
		}
		item := aggregates[key]
		if item == nil {
			item = &aggregate{
				brand:        brand,
				provider:     provider,
				accountIDs:   make(map[string]struct{}),
				credentialFP: make(map[string]struct{}),
				subjectFP:    make(map[string]struct{}),
				channelIDs:   make(map[int]struct{}),
			}
			aggregates[key] = item
		}
		if accountID := strings.TrimSpace(usage.AccountID); accountID != "" {
			item.accountIDs[accountID] = struct{}{}
		}
		if credentialFP := strings.TrimSpace(usage.CredentialFingerprint); credentialFP != "" {
			item.credentialFP[credentialFP] = struct{}{}
		}
		if subjectFP := strings.TrimSpace(usage.CredentialSubjectFingerprint); subjectFP != "" {
			item.subjectFP[subjectFP] = struct{}{}
		}
		if usage.ChannelID > 0 {
			item.channelIDs[usage.ChannelID] = struct{}{}
		}
		if usage.LastUsedAt > item.lastUsedAt {
			item.lastUsedAt = usage.LastUsedAt
		}
	}
	risks := make([]ModelGatewayProxyReuseRisk, 0)
	for _, item := range aggregates {
		accountCount := len(item.accountIDs)
		credentialCount := len(item.credentialFP)
		subjectCount := len(item.subjectFP)
		if subjectCount <= 1 && credentialCount <= 1 && accountCount <= 1 {
			continue
		}
		risks = append(risks, ModelGatewayProxyReuseRisk{
			Brand:                item.brand,
			Provider:             item.provider,
			AccountCount:         accountCount,
			CredentialCount:      credentialCount,
			DistinctSubjectCount: subjectCount,
			ChannelCount:         len(item.channelIDs),
			LastUsedAt:           item.lastUsedAt,
			Reason:               "same_brand_multi_account",
		})
	}
	sort.SliceStable(risks, func(i, j int) bool {
		if risks[i].DistinctSubjectCount != risks[j].DistinctSubjectCount {
			return risks[i].DistinctSubjectCount > risks[j].DistinctSubjectCount
		}
		return risks[i].LastUsedAt > risks[j].LastUsedAt
	})
	return risks
}

type modelGatewayProxyGeoResult struct {
	ExitIP      string
	RegionCode  string
	RegionName  string
	CountryName string
	City        string
	Timezone    string
}

type modelGatewayProxyGeoAPIResponse struct {
	IP          string `json:"ip"`
	Query       string `json:"query"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`
	Region      string `json:"region"`
	RegionCode  string `json:"region_code"`
	RegionName  string `json:"region_name"`
	City        string `json:"city"`
	Timezone    string `json:"timezone"`
	Success     *bool  `json:"success"`
	Message     string `json:"message"`
	Reason      string `json:"reason"`
	Error       string `json:"error"`
}

func detectModelGatewayProxyGeo(parent context.Context, proxy model.ModelGatewayProxy) (modelGatewayProxyGeoResult, error) {
	proxyURL, err := proxy.ProxyURL()
	if err != nil {
		return modelGatewayProxyGeoResult{}, err
	}
	client, err := modelGatewayProxyGeoHTTPClientFactory(proxyURL)
	if err != nil {
		return modelGatewayProxyGeoResult{}, err
	}
	ctx := parent
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, modelGatewayProxyGeoDetectTimeout)
	defer cancel()

	var lastErr error
	for _, endpoint := range modelGatewayProxyGeoEndpoints {
		result, err := detectModelGatewayProxyGeoFromEndpoint(ctx, client, endpoint)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的地区检测服务")
	}
	return modelGatewayProxyGeoResult{}, lastErr
}

func detectModelGatewayProxyGeoFromEndpoint(ctx context.Context, client *http.Client, endpoint string) (modelGatewayProxyGeoResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return modelGatewayProxyGeoResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "new-api-proxy-geo-detector/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return modelGatewayProxyGeoResult{}, err
	}
	defer service.CloseResponseBodyGracefully(resp)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return modelGatewayProxyGeoResult{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return modelGatewayProxyGeoResult{}, fmt.Errorf("检测服务返回 HTTP %d", resp.StatusCode)
	}
	var payload modelGatewayProxyGeoAPIResponse
	if err := common.Unmarshal(body, &payload); err != nil {
		return modelGatewayProxyGeoResult{}, err
	}
	if payload.Success != nil && !*payload.Success {
		reason := strings.TrimSpace(firstNonEmptyString(payload.Message, payload.Reason, payload.Error))
		if reason == "" {
			reason = "检测服务返回失败"
		}
		return modelGatewayProxyGeoResult{}, fmt.Errorf("%s", reason)
	}
	result := normalizeModelGatewayProxyGeoResult(payload)
	if result.ExitIP == "" || result.RegionCode == "" {
		return modelGatewayProxyGeoResult{}, fmt.Errorf("检测服务未返回有效地区")
	}
	return result, nil
}

func normalizeModelGatewayProxyGeoResult(payload modelGatewayProxyGeoAPIResponse) modelGatewayProxyGeoResult {
	countryCode := normalizeModelGatewayProxyRegionCode(firstNonEmptyString(payload.CountryCode, payload.Country))
	countryName := strings.TrimSpace(firstNonEmptyString(payload.CountryName, payload.Country))
	if countryName == countryCode {
		countryName = ""
	}
	return modelGatewayProxyGeoResult{
		ExitIP:      strings.TrimSpace(firstNonEmptyString(payload.IP, payload.Query)),
		RegionCode:  countryCode,
		RegionName:  strings.TrimSpace(firstNonEmptyString(payload.RegionName, payload.Region)),
		CountryName: countryName,
		City:        strings.TrimSpace(payload.City),
		Timezone:    strings.TrimSpace(payload.Timezone),
	}
}

func applyModelGatewayProxyGeoResult(proxy *model.ModelGatewayProxy, result modelGatewayProxyGeoResult) {
	if proxy == nil {
		return
	}
	now := common.GetTimestamp()
	proxy.ExitIP = result.ExitIP
	proxy.RegionCode = result.RegionCode
	proxy.RegionName = result.RegionName
	proxy.CountryName = result.CountryName
	proxy.City = result.City
	proxy.Timezone = result.Timezone
	proxy.GeoCheckedAt = now
	proxy.GeoStatus = "success"
	proxy.GeoError = ""
	proxy.LastSuccessAt = now
}

func normalizeModelGatewayProxyRegionCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) > 16 {
		value = value[:16]
	}
	return value
}

func truncateModelGatewayProxyGeoError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 240 {
		return message
	}
	return message[:240]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func proxyIDsFromModelGatewayProxies(proxies []model.ModelGatewayProxy) []int {
	ids := make([]int, 0, len(proxies))
	for _, proxy := range proxies {
		if proxy.ID > 0 {
			ids = append(ids, proxy.ID)
		}
	}
	return ids
}

func parseBoolQuery(value string, fallback bool) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getModelGatewayProxyOrNil(proxyID int) (*model.ModelGatewayProxy, error) {
	if proxyID <= 0 {
		return nil, nil
	}
	proxy, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("代理不存在")
		}
		return nil, err
	}
	if !proxy.Enabled {
		return nil, fmt.Errorf("代理已禁用")
	}
	return proxy, nil
}
