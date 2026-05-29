package model

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const modelGatewayProxyCacheTTLSeconds = 60

type modelGatewayProxyCacheEntry struct {
	proxy     ModelGatewayProxy
	expiresAt int64
}

var modelGatewayProxyCache sync.Map

const (
	ModelGatewayProxyProtocolSOCKS5  = "socks5"
	ModelGatewayProxyProtocolSOCKS5H = "socks5h"
	ModelGatewayProxyProtocolHTTP    = "http"
	ModelGatewayProxyProtocolHTTPS   = "https"
)

type ModelGatewayProxy struct {
	ID            int    `json:"id"`
	Name          string `json:"name" gorm:"size:128;not null;index"`
	Protocol      string `json:"protocol" gorm:"size:16;not null;default:socks5;index"`
	Address       string `json:"address" gorm:"type:text;not null"`
	Username      string `json:"username,omitempty" gorm:"size:255"`
	Password      string `json:"-" gorm:"type:text"`
	Enabled       bool   `json:"enabled" gorm:"default:true;index"`
	Remark        string `json:"remark,omitempty" gorm:"type:varchar(255)"`
	LastUsedAt    int64  `json:"last_used_at,omitempty" gorm:"bigint;index"`
	LastSuccessAt int64  `json:"last_success_at,omitempty" gorm:"bigint;index"`
	LastFailureAt int64  `json:"last_failure_at,omitempty" gorm:"bigint;index"`
	FailureCount  int64  `json:"failure_count,omitempty" gorm:"bigint;default:0"`
	UseCount      int64  `json:"use_count,omitempty" gorm:"bigint;default:0"`
	ExitIP        string `json:"exit_ip,omitempty" gorm:"size:64;index"`
	RegionCode    string `json:"region_code,omitempty" gorm:"size:16;index"`
	RegionName    string `json:"region_name,omitempty" gorm:"size:128"`
	CountryName   string `json:"country_name,omitempty" gorm:"size:128"`
	City          string `json:"city,omitempty" gorm:"size:128"`
	Timezone      string `json:"timezone,omitempty" gorm:"size:128"`
	GeoCheckedAt  int64  `json:"geo_checked_at,omitempty" gorm:"bigint;index"`
	GeoStatus     string `json:"geo_status,omitempty" gorm:"size:32;index"`
	GeoError      string `json:"geo_error,omitempty" gorm:"type:varchar(255)"`
	CreatedTime   int64  `json:"created_time" gorm:"bigint;index"`
	UpdatedTime   int64  `json:"updated_time" gorm:"bigint;index"`
}

func (p *ModelGatewayProxy) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if p.CreatedTime == 0 {
		p.CreatedTime = now
	}
	p.UpdatedTime = now
	p.Protocol = NormalizeModelGatewayProxyProtocol(p.Protocol)
	p.Address = NormalizeModelGatewayProxyAddress(p.Address)
	return nil
}

func (p *ModelGatewayProxy) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedTime = common.GetTimestamp()
	p.Protocol = NormalizeModelGatewayProxyProtocol(p.Protocol)
	p.Address = NormalizeModelGatewayProxyAddress(p.Address)
	return nil
}

func NormalizeModelGatewayProxyProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case ModelGatewayProxyProtocolHTTP, ModelGatewayProxyProtocolHTTPS, ModelGatewayProxyProtocolSOCKS5H:
		return protocol
	default:
		return ModelGatewayProxyProtocolSOCKS5
	}
}

func NormalizeModelGatewayProxyAddress(address string) string {
	address = strings.TrimSpace(address)
	address = strings.TrimRight(address, "/")
	return address
}

func (p ModelGatewayProxy) ProxyURL() (string, error) {
	protocol := NormalizeModelGatewayProxyProtocol(p.Protocol)
	address := NormalizeModelGatewayProxyAddress(p.Address)
	if address == "" {
		return "", fmt.Errorf("proxy address is empty")
	}
	if strings.Contains(address, "://") {
		parsed, err := url.Parse(address)
		if err != nil {
			return "", err
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return "", fmt.Errorf("proxy address is invalid")
		}
		applyModelGatewayProxyAuth(parsed, p.Username, p.Password)
		return parsed.String(), nil
	}
	if parsed, ok := parseProxyAddressWithInlineAuth(protocol, address); ok {
		applyModelGatewayProxyAuth(parsed, p.Username, p.Password)
		return parsed.String(), nil
	}
	result := &url.URL{
		Scheme: protocol,
		Host:   address,
	}
	if p.Username != "" {
		if p.Password != "" {
			result.User = url.UserPassword(p.Username, p.Password)
		} else {
			result.User = url.User(p.Username)
		}
	}
	return result.String(), nil
}

func applyModelGatewayProxyAuth(parsed *url.URL, username string, password string) {
	if parsed == nil || username == "" {
		return
	}
	if password != "" {
		parsed.User = url.UserPassword(username, password)
		return
	}
	parsed.User = url.User(username)
}

func (p ModelGatewayProxy) MaskedAddress() string {
	address := NormalizeModelGatewayProxyAddress(p.Address)
	if address == "" {
		return ""
	}
	if strings.Contains(address, "://") {
		parsed, err := url.Parse(address)
		if err == nil && parsed.Host != "" {
			parsed.User = nil
			return parsed.String()
		}
	}
	if parsed, ok := parseProxyAddressWithInlineAuth(NormalizeModelGatewayProxyProtocol(p.Protocol), address); ok {
		parsed.User = nil
		return parsed.String()
	}
	return NormalizeModelGatewayProxyProtocol(p.Protocol) + "://" + address
}

func parseProxyAddressWithInlineAuth(protocol string, address string) (*url.URL, bool) {
	if strings.TrimSpace(address) == "" || !strings.Contains(address, "@") {
		return nil, false
	}
	parsed, err := url.Parse(protocol + "://" + address)
	if err != nil || parsed.Host == "" || parsed.User == nil {
		return nil, false
	}
	return parsed, true
}

func ListModelGatewayProxies(enabledOnly bool) ([]ModelGatewayProxy, error) {
	proxies := make([]ModelGatewayProxy, 0)
	query := DB.Model(&ModelGatewayProxy{})
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("updated_time DESC").Find(&proxies).Error
	return proxies, err
}

func GetModelGatewayProxyByID(proxyID int) (*ModelGatewayProxy, error) {
	if proxyID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if common.MemoryCacheEnabled {
		if cached, ok := modelGatewayProxyCache.Load(proxyID); ok {
			entry, ok := cached.(modelGatewayProxyCacheEntry)
			if ok && entry.expiresAt > common.GetTimestamp() {
				proxy := entry.proxy
				return &proxy, nil
			}
			modelGatewayProxyCache.Delete(proxyID)
		}
	}
	var proxy ModelGatewayProxy
	if err := DB.First(&proxy, "id = ?", proxyID).Error; err != nil {
		return nil, err
	}
	if common.MemoryCacheEnabled {
		modelGatewayProxyCache.Store(proxyID, modelGatewayProxyCacheEntry{
			proxy:     proxy,
			expiresAt: common.GetTimestamp() + modelGatewayProxyCacheTTLSeconds,
		})
	}
	return &proxy, nil
}

func InvalidateModelGatewayProxyCache(proxyIDs ...int) {
	if len(proxyIDs) == 0 {
		modelGatewayProxyCache.Range(func(key, value interface{}) bool {
			modelGatewayProxyCache.Delete(key)
			return true
		})
		return
	}
	for _, proxyID := range proxyIDs {
		modelGatewayProxyCache.Delete(proxyID)
	}
}

func GetModelGatewayProxiesByIDs(proxyIDs []int) (map[int]ModelGatewayProxy, error) {
	result := make(map[int]ModelGatewayProxy)
	seen := make(map[int]struct{}, len(proxyIDs))
	ids := make([]int, 0, len(proxyIDs))
	for _, proxyID := range proxyIDs {
		if proxyID <= 0 {
			continue
		}
		if _, ok := seen[proxyID]; ok {
			continue
		}
		seen[proxyID] = struct{}{}
		ids = append(ids, proxyID)
	}
	if len(ids) == 0 {
		return result, nil
	}
	var proxies []ModelGatewayProxy
	if err := DB.Where("id IN ?", ids).Find(&proxies).Error; err != nil {
		return nil, err
	}
	for _, proxy := range proxies {
		result[proxy.ID] = proxy
	}
	return result, nil
}
