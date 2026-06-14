package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

// https://github.com/songquanpeng/one-api/issues/79

type OpenAISubscriptionResponse struct {
	Object             string  `json:"object"`
	HasPaymentMethod   bool    `json:"has_payment_method"`
	SoftLimitUSD       float64 `json:"soft_limit_usd"`
	HardLimitUSD       float64 `json:"hard_limit_usd"`
	SystemHardLimitUSD float64 `json:"system_hard_limit_usd"`
	AccessUntil        int64   `json:"access_until"`
}

type OpenAIUsageDailyCost struct {
	Timestamp float64 `json:"timestamp"`
	LineItems []struct {
		Name string  `json:"name"`
		Cost float64 `json:"cost"`
	}
}

type OpenAICreditGrants struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalAvailable float64 `json:"total_available"`
}

type OpenAIUsageResponse struct {
	Object string `json:"object"`
	//DailyCosts []OpenAIUsageDailyCost `json:"daily_costs"`
	TotalUsage float64 `json:"total_usage"` // unit: 0.01 dollar
}

type OpenAISBUsageResponse struct {
	Msg  string `json:"msg"`
	Data *struct {
		Credit string `json:"credit"`
	} `json:"data"`
}

type AIProxyUserOverviewResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	ErrorCode int    `json:"error_code"`
	Data      struct {
		TotalPoints float64 `json:"totalPoints"`
	} `json:"data"`
}

type API2GPTUsageResponse struct {
	Object         string  `json:"object"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
	TotalRemaining float64 `json:"total_remaining"`
}

type APGC2DGPTUsageResponse struct {
	//Grants         interface{} `json:"grants"`
	Object         string  `json:"object"`
	TotalAvailable float64 `json:"total_available"`
	TotalGranted   float64 `json:"total_granted"`
	TotalUsed      float64 `json:"total_used"`
}

type SiliconFlowUsageResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  bool   `json:"status"`
	Data    struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Image         string `json:"image"`
		Email         string `json:"email"`
		IsAdmin       bool   `json:"isAdmin"`
		Balance       string `json:"balance"`
		Status        string `json:"status"`
		Introduction  string `json:"introduction"`
		Role          string `json:"role"`
		ChargeBalance string `json:"chargeBalance"`
		TotalBalance  string `json:"totalBalance"`
		Category      string `json:"category"`
	} `json:"data"`
}

type DeepSeekUsageResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

type OpenRouterCreditResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

type channelBalanceResult struct {
	Balance  float64
	Source   string
	Endpoint string
	Currency string
	RawUnit  string
}

type channelBalanceProbe struct {
	Source string
	URL    string
	Parse  func(body []byte, probe channelBalanceProbe) (channelBalanceResult, error)
}

// GetAuthHeader get auth header
func GetAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return h
}

// GetClaudeAuthHeader get claude auth header
func GetClaudeAuthHeader(token string) http.Header {
	h := http.Header{}
	h.Add("x-api-key", token)
	h.Add("anthropic-version", "2023-06-01")
	return h
}

func GetResponseBody(method, url string, channel *model.Channel, headers http.Header) ([]byte, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	for k := range headers {
		req.Header.Add(k, headers.Get(k))
	}
	client, err := service.NewProxyHttpClient(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func fetchChannelCloseAIBalance(channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("%s/dashboard/billing/credit_grants", channel.GetBaseURL())
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := OpenAICreditGrants{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	return response.TotalAvailable, nil
}

func updateChannelCloseAIBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelCloseAIBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelOpenAISBBalance(channel *model.Channel) (float64, error) {
	url := fmt.Sprintf("https://api.openai-sb.com/sb-api/user/status?api_key=%s", channel.Key)
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenAISBUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Data == nil {
		return 0, errors.New(response.Msg)
	}
	balance, err := strconv.ParseFloat(response.Data.Credit, 64)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func updateChannelOpenAISBBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelOpenAISBBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelAIProxyBalance(channel *model.Channel) (float64, error) {
	url := "https://aiproxy.io/api/report/getUserOverview"
	headers := http.Header{}
	headers.Add("Api-Key", channel.Key)
	body, err := GetResponseBody("GET", url, channel, headers)
	if err != nil {
		return 0, err
	}
	response := AIProxyUserOverviewResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Success {
		return 0, fmt.Errorf("code: %d, message: %s", response.ErrorCode, response.Message)
	}
	return response.Data.TotalPoints, nil
}

func updateChannelAIProxyBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelAIProxyBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelAPI2GPTBalance(channel *model.Channel) (float64, error) {
	url := "https://api.api2gpt.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))

	if err != nil {
		return 0, err
	}
	response := API2GPTUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	return response.TotalRemaining, nil
}

func updateChannelAPI2GPTBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelAPI2GPTBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelSiliconFlowBalance(channel *model.Channel) (float64, error) {
	url := "https://api.siliconflow.cn/v1/user/info"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := SiliconFlowUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if response.Code != 20000 {
		return 0, fmt.Errorf("code: %d, message: %s", response.Code, response.Message)
	}
	balance, err := strconv.ParseFloat(response.Data.TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func updateChannelSiliconFlowBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelSiliconFlowBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelDeepSeekBalance(channel *model.Channel) (float64, error) {
	url := "https://api.deepseek.com/user/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := DeepSeekUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	index := -1
	for i, balanceInfo := range response.BalanceInfos {
		if balanceInfo.Currency == "CNY" {
			index = i
			break
		}
	}
	if index == -1 {
		return 0, errors.New("currency CNY not found")
	}
	balance, err := strconv.ParseFloat(response.BalanceInfos[index].TotalBalance, 64)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func updateChannelDeepSeekBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelDeepSeekBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelAIGC2DBalance(channel *model.Channel) (float64, error) {
	url := "https://api.aigc2d.com/dashboard/billing/credit_grants"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := APGC2DGPTUsageResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	return response.TotalAvailable, nil
}

func updateChannelAIGC2DBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelAIGC2DBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelOpenRouterBalance(channel *model.Channel) (float64, error) {
	url := "https://openrouter.ai/api/v1/credits"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}
	response := OpenRouterCreditResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	balance := response.Data.TotalCredits - response.Data.TotalUsage
	return balance, nil
}

func updateChannelOpenRouterBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelOpenRouterBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func fetchChannelMoonshotBalance(channel *model.Channel) (float64, error) {
	url := "https://api.moonshot.cn/v1/users/me/balance"
	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return 0, err
	}

	type MoonshotBalanceData struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	}

	type MoonshotBalanceResponse struct {
		Code   int                 `json:"code"`
		Data   MoonshotBalanceData `json:"data"`
		Scode  string              `json:"scode"`
		Status bool                `json:"status"`
	}

	response := MoonshotBalanceResponse{}
	err = common.Unmarshal(body, &response)
	if err != nil {
		return 0, err
	}
	if !response.Status || response.Code != 0 {
		return 0, fmt.Errorf("failed to update moonshot balance, status: %v, code: %d, scode: %s", response.Status, response.Code, response.Scode)
	}
	availableBalanceCny := response.Data.AvailableBalance
	availableBalanceUsd := decimal.NewFromFloat(availableBalanceCny).Div(decimal.NewFromFloat(operation_setting.Price)).InexactFloat64()
	return availableBalanceUsd, nil
}

func updateChannelMoonshotBalance(channel *model.Channel) (float64, error) {
	balance, err := fetchChannelMoonshotBalance(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(balance)
	return balance, nil
}

func balanceResult(balance float64, source string, endpoint string, currency string, rawUnit string) channelBalanceResult {
	return channelBalanceResult{
		Balance:  balance,
		Source:   source,
		Endpoint: endpoint,
		Currency: currency,
		RawUnit:  rawUnit,
	}
}

func normalizeBalanceBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func baseURLWithoutVersionSuffix(baseURL string) string {
	baseURL = normalizeBalanceBaseURL(baseURL)
	lower := strings.ToLower(baseURL)
	for _, suffix := range []string{"/v1", "/v3", "/v4", "/api/v1", "/api/v3", "/api/v4"} {
		if strings.HasSuffix(lower, suffix) {
			return strings.TrimRight(baseURL[:len(baseURL)-len(suffix)], "/")
		}
	}
	return baseURL
}

func uniqueBalanceBaseURLs(baseURL string) []string {
	root := baseURLWithoutVersionSuffix(baseURL)
	base := normalizeBalanceBaseURL(baseURL)
	result := make([]string, 0, 2)
	for _, candidate := range []string{root, base} {
		if candidate == "" {
			continue
		}
		seen := false
		for _, existing := range result {
			if existing == candidate {
				seen = true
				break
			}
		}
		if !seen {
			result = append(result, candidate)
		}
	}
	return result
}

func channelBaseURLForBalance(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = *channel.BaseURL
	}
	if strings.TrimSpace(baseURL) == "" && channel.Type >= 0 && channel.Type < len(constant.ChannelBaseURLs) {
		baseURL = constant.ChannelBaseURLs[channel.Type]
	}
	return normalizeBalanceBaseURL(baseURL)
}

func fetchOpenAIUsageBalanceResult(channel *model.Channel, baseURL string) (channelBalanceResult, error) {
	baseURL = normalizeBalanceBaseURL(baseURL)
	if baseURL == "" {
		return channelBalanceResult{}, errors.New("base url is empty")
	}
	url := fmt.Sprintf("%s/v1/dashboard/billing/subscription", baseURL)

	body, err := GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return channelBalanceResult{}, err
	}
	subscription := OpenAISubscriptionResponse{}
	err = common.Unmarshal(body, &subscription)
	if err != nil {
		return channelBalanceResult{}, err
	}
	now := time.Now()
	startDate := fmt.Sprintf("%s-01", now.Format("2006-01"))
	endDate := now.Format("2006-01-02")
	if !subscription.HasPaymentMethod {
		startDate = now.AddDate(0, 0, -100).Format("2006-01-02")
	}
	url = fmt.Sprintf("%s/v1/dashboard/billing/usage?start_date=%s&end_date=%s", baseURL, startDate, endDate)
	body, err = GetResponseBody("GET", url, channel, GetAuthHeader(channel.Key))
	if err != nil {
		return channelBalanceResult{}, err
	}
	usage := OpenAIUsageResponse{}
	err = common.Unmarshal(body, &usage)
	if err != nil {
		return channelBalanceResult{}, err
	}
	balance := subscription.HardLimitUSD - usage.TotalUsage/100
	return balanceResult(balance, "openai_usage", "/v1/dashboard/billing/subscription,/v1/dashboard/billing/usage", "USD", "usd"), nil
}

func fetchGenericChannelBalance(channel *model.Channel, baseURL string) (channelBalanceResult, error) {
	baseURLs := uniqueBalanceBaseURLs(baseURL)
	if len(baseURLs) == 0 {
		return channelBalanceResult{}, errors.New("base url is empty")
	}
	probes := make([]channelBalanceProbe, 0, len(baseURLs)*7)
	for _, candidateBaseURL := range baseURLs {
		probes = append(probes,
			channelBalanceProbe{Source: "new_api_token_usage", URL: candidateBaseURL + "/api/usage/token/", Parse: parseGenericQuotaBalance},
			channelBalanceProbe{Source: "new_api_token_usage", URL: candidateBaseURL + "/api/usage/token", Parse: parseGenericQuotaBalance},
			channelBalanceProbe{Source: "new_api_user_self", URL: candidateBaseURL + "/api/user/self", Parse: parseGenericQuotaBalance},
			channelBalanceProbe{Source: "new_api_user_self", URL: candidateBaseURL + "/api/user/self?with_balance=true", Parse: parseGenericQuotaBalance},
			channelBalanceProbe{Source: "new_api_user_status", URL: candidateBaseURL + "/api/user/status", Parse: parseGenericQuotaBalance},
			channelBalanceProbe{Source: "openai_credit_grants", URL: candidateBaseURL + "/dashboard/billing/credit_grants", Parse: parseCreditGrantBalance},
			channelBalanceProbe{Source: "openai_credit_grants", URL: candidateBaseURL + "/v1/dashboard/billing/credit_grants", Parse: parseCreditGrantBalance},
		)
	}

	var lastErr error
	for _, probe := range probes {
		body, err := GetResponseBody("GET", probe.URL, channel, GetAuthHeader(channel.Key))
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", probe.Source, err)
			continue
		}
		result, err := probe.Parse(body, probe)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", probe.Source, err)
			continue
		}
		return result, nil
	}
	if lastErr != nil {
		return channelBalanceResult{}, lastErr
	}
	return channelBalanceResult{}, errors.New("no balance probe configured")
}

func parseCreditGrantBalance(body []byte, probe channelBalanceProbe) (channelBalanceResult, error) {
	root := map[string]any{}
	if err := common.Unmarshal(body, &root); err != nil {
		return channelBalanceResult{}, err
	}
	if errMessage := genericBalanceErrorMessage(root); errMessage != "" {
		return channelBalanceResult{}, errors.New(errMessage)
	}
	data := unwrapBalanceData(root)
	if balance, ok := numericField(data, "total_available", "total_remaining", "available_balance", "remaining_balance"); ok {
		return balanceResult(balance, probe.Source, probe.URL, "USD", "usd"), nil
	}
	if total, ok := numericField(data, "total_credits", "total_granted"); ok {
		used, _ := numericField(data, "total_usage", "total_used", "used")
		return balanceResult(total-used, probe.Source, probe.URL, "USD", "usd"), nil
	}
	return channelBalanceResult{}, errors.New("balance field not found")
}

func parseGenericQuotaBalance(body []byte, probe channelBalanceProbe) (channelBalanceResult, error) {
	root := map[string]any{}
	if err := common.Unmarshal(body, &root); err != nil {
		return channelBalanceResult{}, err
	}
	if errMessage := genericBalanceErrorMessage(root); errMessage != "" {
		return channelBalanceResult{}, errors.New(errMessage)
	}
	data := unwrapBalanceData(root)

	if unlimited, ok := boolField(data, "unlimited_quota"); ok && unlimited {
		return channelBalanceResult{}, errors.New("unlimited quota cannot be converted to balance")
	}
	if value, ok := numericField(data, "total_available", "remain_quota", "remaining_quota", "quota", "available_quota"); ok {
		return balanceResult(value/common.QuotaPerUnit, probe.Source, probe.URL, "USD", "quota"), nil
	}
	if total, ok := numericField(data, "total_granted", "total_quota"); ok {
		used, _ := numericField(data, "total_used", "used_quota")
		return balanceResult((total-used)/common.QuotaPerUnit, probe.Source, probe.URL, "USD", "quota"), nil
	}
	if value, ok := numericField(data, "available_balance", "remaining_balance", "balance", "credit", "credits", "total_available_usd"); ok {
		return balanceResult(value, probe.Source, probe.URL, "USD", "usd"), nil
	}
	return channelBalanceResult{}, errors.New("balance field not found")
}

func unwrapBalanceData(root map[string]any) map[string]any {
	current := root
	for _, key := range []string{"data", "user", "account", "result"} {
		if nested, ok := mapField(current, key); ok {
			current = nested
		}
	}
	return current
}

func genericBalanceErrorMessage(root map[string]any) string {
	if success, ok := boolField(root, "success"); ok && !success {
		if message, ok := stringField(root, "message", "msg", "error"); ok {
			return message
		}
		return "request failed"
	}
	if code, ok := numericField(root, "code"); ok && code != 0 && code != 200 && code != 20000 {
		if message, ok := stringField(root, "message", "msg", "error"); ok {
			return message
		}
		return fmt.Sprintf("code: %.0f", code)
	}
	return ""
}

func mapField(values map[string]any, key string) (map[string]any, bool) {
	if values == nil {
		return nil, false
	}
	value, ok := values[key]
	if !ok {
		return nil, false
	}
	nested, ok := value.(map[string]any)
	return nested, ok
}

func stringField(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v, true
			}
		case map[string]any:
			if message, ok := stringField(v, "message", "msg"); ok {
				return message, true
			}
		}
	}
	return "", false
}

func boolField(values map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case bool:
			return v, true
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(v))
			if err == nil {
				return parsed, true
			}
		}
	}
	return false, false
}

func numericField(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok || value == nil {
			continue
		}
		if number, ok := numericValue(value); ok {
			return number, true
		}
	}
	return 0, false
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func isOpenAICompatibleBalanceChannel(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeOpenAIMax,
		constant.ChannelTypeOhMyGPT,
		constant.ChannelTypeCustom,
		constant.ChannelTypeAILS,
		constant.ChannelTypeAIProxyLibrary,
		constant.ChannelTypeLingYiWanWu,
		constant.ChannelType360,
		constant.ChannelTypePerplexity,
		constant.ChannelTypeMistral,
		constant.ChannelTypeDeepSeek,
		constant.ChannelTypeXai,
		constant.ChannelTypeSubmodel,
		constant.ChannelTypeSora:
		return true
	default:
		return false
	}
}

func fetchChannelBalanceResult(channel *model.Channel) (channelBalanceResult, error) {
	if channel == nil {
		return channelBalanceResult{}, errors.New("channel is nil")
	}
	baseURL := channelBaseURLForBalance(channel)
	switch channel.Type {
	case constant.ChannelTypeAzure:
		return channelBalanceResult{}, errors.New("尚未实现")
	case constant.ChannelTypeAIProxy:
		balance, err := fetchChannelAIProxyBalance(channel)
		if err != nil {
			return channelBalanceResult{}, err
		}
		return balanceResult(balance, "aiproxy", "https://aiproxy.io/api/report/getUserOverview", "USD", "points"), nil
	case constant.ChannelTypeAPI2GPT:
		balance, err := fetchChannelAPI2GPTBalance(channel)
		if err != nil {
			return channelBalanceResult{}, err
		}
		return balanceResult(balance, "api2gpt", "https://api.api2gpt.com/dashboard/billing/credit_grants", "USD", "usd"), nil
	case constant.ChannelTypeAIGC2D:
		balance, err := fetchChannelAIGC2DBalance(channel)
		if err != nil {
			return channelBalanceResult{}, err
		}
		return balanceResult(balance, "aigc2d", "https://api.aigc2d.com/dashboard/billing/credit_grants", "USD", "usd"), nil
	case constant.ChannelTypeSiliconFlow:
		balance, err := fetchChannelSiliconFlowBalance(channel)
		if err != nil {
			return channelBalanceResult{}, err
		}
		return balanceResult(balance, "siliconflow", "https://api.siliconflow.cn/v1/user/info", "CNY", "cny"), nil
	case constant.ChannelTypeOpenRouter:
		balance, err := fetchChannelOpenRouterBalance(channel)
		if err != nil {
			return channelBalanceResult{}, err
		}
		return balanceResult(balance, "openrouter", "https://openrouter.ai/api/v1/credits", "USD", "usd"), nil
	case constant.ChannelTypeMoonshot:
		balance, err := fetchChannelMoonshotBalance(channel)
		if err != nil {
			return channelBalanceResult{}, err
		}
		return balanceResult(balance, "moonshot", "https://api.moonshot.cn/v1/users/me/balance", "USD", "cny_converted"), nil
	case constant.ChannelTypeDeepSeek:
		balance, err := fetchChannelDeepSeekBalance(channel)
		if err == nil {
			return balanceResult(balance, "deepseek", "https://api.deepseek.com/user/balance", "CNY", "cny"), nil
		}
		if genericResult, genericErr := fetchGenericChannelBalance(channel, baseURL); genericErr == nil {
			return genericResult, nil
		}
		return channelBalanceResult{}, err
	default:
		if !isOpenAICompatibleBalanceChannel(channel.Type) {
			return channelBalanceResult{}, errors.New("尚未实现")
		}
	}

	if genericResult, err := fetchGenericChannelBalance(channel, baseURL); err == nil {
		return genericResult, nil
	}
	return fetchOpenAIUsageBalanceResult(channel, baseURLWithoutVersionSuffix(baseURL))
}

func fetchChannelBalance(channel *model.Channel) (float64, error) {
	result, err := fetchChannelBalanceResult(channel)
	if err != nil {
		return 0, err
	}
	return result.Balance, nil
}

func updateChannelBalance(channel *model.Channel) (float64, error) {
	result, err := fetchChannelBalanceResult(channel)
	if err != nil {
		return 0, err
	}
	channel.UpdateBalance(result.Balance)
	return result.Balance, nil
}

func UpdateChannelBalance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "多密钥渠道不支持余额查询",
		})
		return
	}
	balance, err := updateChannelBalance(channel)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	reconcileChannelBalanceStatus(channel, balance)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"balance": balance,
	})
}

func shouldCheckChannelBalance(channel *model.Channel) bool {
	if channel == nil || channel.ChannelInfo.IsMultiKey {
		return false
	}
	if channel.Status == common.ChannelStatusEnabled {
		return true
	}
	return service.IsBalanceInsufficientPausedChannel(channel)
}

func reconcileChannelBalanceStatus(channel *model.Channel, balance float64) {
	if channel == nil {
		return
	}
	changed := false
	if balance > 0 {
		if service.ClearChannelBalanceInsufficientForChannel(channel.Id) > 0 {
			changed = true
		}
		if service.IsBalanceInsufficientStatusReason(service.ChannelStatusReason(channel)) &&
			!service.IsBalanceInsufficientPausedChannel(channel) {
			if model.UpdateChannelStatusWholeChannelWithInfo(channel.Id, channel.Status, "", nil) {
				changed = true
			}
		}
	}
	if service.IsBalanceInsufficientPausedChannel(channel) && service.ShouldResumeBalancePausedChannel(balance) {
		service.EnableChannel(channel.Id, "", channel.Name)
		changed = true
	}
	if changed {
		modelgatewayintegration.RefreshDefaultRoutingCaches(modelgatewayintegration.RoutingCacheRefreshOptions{
			Reason: "channel_balance_recovered",
		})
	}
}

func updateAllChannelsBalance() error {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if !shouldCheckChannelBalance(channel) {
			continue
		}
		// TODO: support Azure
		//if channel.Type != common.ChannelTypeOpenAI && channel.Type != common.ChannelTypeCustom {
		//	continue
		//}
		balance, err := updateChannelBalance(channel)
		if err != nil {
			continue
		}
		reconcileChannelBalanceStatus(channel, balance)
		time.Sleep(common.RequestInterval)
	}
	return nil
}

func UpdateAllChannelsBalance(c *gin.Context) {
	// TODO: make it async
	err := updateAllChannelsBalance()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func AutomaticallyUpdateChannels(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Minute)
		common.SysLog("updating all channels")
		_ = updateAllChannelsBalance()
		common.SysLog("channels update done")
	}
}
