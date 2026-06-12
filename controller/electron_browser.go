package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
	"github.com/gin-gonic/gin"
)

const electronBrowserDefaultPageSize = 500
const electronBrowserMaxPageSize = 1000

type ElectronBrowserAccountsResponse struct {
	Items      []ElectronBrowserAccountItem `json:"items"`
	Channels   []ChannelAccountChannelItem  `json:"channels,omitempty"`
	Total      int                          `json:"total"`
	SnapshotAt int64                        `json:"snapshot_at"`
}

type ElectronBrowserAccountItem struct {
	ChannelAccountItem
	ProfileKey string `json:"profile_key"`
	OpenURL    string `json:"open_url"`
	ProxyRules string `json:"proxy_rules,omitempty"`
}

func ListElectronBrowserAccounts(c *gin.Context) {
	if !requireTokenAccountAutomationCallbackToken(c) {
		return
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	params := parseChannelAccountsQuery(c)
	desiredPageSize := parsePositiveQueryInt(c, "page_size", electronBrowserDefaultPageSize)
	if desiredPageSize > electronBrowserMaxPageSize {
		desiredPageSize = electronBrowserMaxPageSize
	}
	common.ApiSuccess(c, buildElectronBrowserAccountsResponse(channels, params, desiredPageSize))
}

func buildElectronBrowserAccountsResponse(channels []*model.Channel, params channelAccountsQuery, desiredPageSize int) ElectronBrowserAccountsResponse {
	if desiredPageSize <= 0 {
		desiredPageSize = electronBrowserDefaultPageSize
	}
	if desiredPageSize > electronBrowserMaxPageSize {
		desiredPageSize = electronBrowserMaxPageSize
	}
	params = normalizeChannelAccountsQuery(params)
	registry := modelgatewayaccount.NewRegistry()
	channelByID := make(map[int]*model.Channel, len(channels))
	accountTotalsByChannel := make(map[int]int, len(channels))
	allAccounts := make([]modelgatewayaccount.ChannelAccount, 0)
	response := ElectronBrowserAccountsResponse{
		Channels:   make([]ChannelAccountChannelItem, 0, len(channels)),
		SnapshotAt: common.GetTimestamp(),
	}
	for _, channel := range channels {
		if channel == nil || channel.Id <= 0 {
			continue
		}
		accounts := registry.AccountsForChannel(channel)
		response.Channels = append(response.Channels, buildChannelAccountChannelItem(channel, accounts))
		channelByID[channel.Id] = channel
		accountTotalsByChannel[channel.Id] = len(accounts)
		if params.ChannelID > 0 && channel.Id != params.ChannelID {
			continue
		}
		allAccounts = append(allAccounts, accounts...)
	}
	filtered := filterChannelAccounts(allAccounts, params)
	sortChannelAccounts(filtered, params, nil)
	response.Total = len(filtered)
	start := (params.Page - 1) * desiredPageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + desiredPageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	pageAccounts := filtered[start:end]
	proxiesByID := electronBrowserProxiesByID(pageAccounts)
	response.Items = make([]ElectronBrowserAccountItem, 0, len(pageAccounts))
	for _, account := range pageAccounts {
		channel := channelByID[account.ChannelID]
		item := buildChannelAccountItem(account, nil, accountTotalsByChannel[account.ChannelID] == 1)
		if channel != nil {
			item.ChannelName = channel.Name
		}
		var proxy *model.ModelGatewayProxy
		if account.ProxyRef.ProxyID > 0 {
			if proxyConfig, ok := proxiesByID[account.ProxyRef.ProxyID]; ok {
				proxy = &proxyConfig
				proxyResponse := buildModelGatewayProxyResponse(proxyConfig, nil)
				item.Proxy = &proxyResponse
			} else {
				item.Proxy = &ModelGatewayProxyResponse{
					ID:             account.ProxyRef.ProxyID,
					Name:           fmt.Sprintf("Proxy #%d", account.ProxyRef.ProxyID),
					Enabled:        false,
					PasswordMasked: true,
				}
			}
		}
		response.Items = append(response.Items, buildElectronBrowserAccountItem(item, proxy))
	}
	return response
}

func electronBrowserProxiesByID(accounts []modelgatewayaccount.ChannelAccount) map[int]model.ModelGatewayProxy {
	proxySet := make(map[int]struct{})
	for _, account := range accounts {
		if account.ProxyRef.ProxyID > 0 {
			proxySet[account.ProxyRef.ProxyID] = struct{}{}
		}
	}
	proxyIDs := make([]int, 0, len(proxySet))
	for proxyID := range proxySet {
		proxyIDs = append(proxyIDs, proxyID)
	}
	proxiesByID, err := model.GetModelGatewayProxiesByIDs(proxyIDs)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to load electron browser proxies: error=%v", err))
		return nil
	}
	return proxiesByID
}

func buildElectronBrowserAccountItem(item ChannelAccountItem, proxy *model.ModelGatewayProxy) ElectronBrowserAccountItem {
	browserItem := ElectronBrowserAccountItem{
		ChannelAccountItem: item,
		ProfileKey:         electronBrowserProfileKey(item),
		OpenURL:            electronBrowserDefaultOpenURL(item),
	}
	if proxy != nil && proxy.Enabled {
		if proxyRules, err := proxy.ProxyURL(); err == nil {
			browserItem.ProxyRules = proxyRules
		}
	}
	return browserItem
}

func electronBrowserProfileKey(item ChannelAccountItem) string {
	return fmt.Sprintf("channel-%d-credential-%d", item.ChannelID, item.CredentialIndex)
}

func electronBrowserDefaultOpenURL(item ChannelAccountItem) string {
	values := []string{
		item.AccountIdentity.Brand,
		item.AccountIdentity.Provider,
		item.AccountIdentity.AccountType,
		item.ResourceRef.ResourceType,
		item.ChannelName,
		item.CredentialLabel,
	}
	joined := strings.ToLower(strings.Join(values, " "))
	switch {
	case strings.Contains(joined, "claude") || strings.Contains(joined, "anthropic"):
		return "https://claude.ai/"
	case strings.Contains(joined, "gemini") || strings.Contains(joined, "google"):
		return "https://aistudio.google.com/"
	case strings.Contains(joined, "openai") || strings.Contains(joined, "chatgpt") || strings.Contains(joined, "codex"):
		return "https://chatgpt.com/"
	default:
		return "https://chatgpt.com/"
	}
}
