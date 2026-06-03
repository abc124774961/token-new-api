package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
)

const credentialWritebackPath = "/api/internal/token-account-automation/credential"
const desktopProxyListPath = "/api/internal/token-account-automation/proxies"
const accountProfilePath = "/api/internal/token-account-automation/account-profile"
const invalidAccountPoolListPath = "/api/internal/token-account-automation/account-pools/invalid"
const invalidAccountArchivePath = "/api/internal/token-account-automation/account-pools/invalid/archive"
const invalidAccountReauthorizePrefix = "/api/internal/token-account-automation/account-pools/invalid/"
const discardedAccountArchivePath = "/api/internal/token-account-automation/account-pools/discarded/archive"

type Client struct {
	baseURL string
	token   string
	timeout time.Duration
	client  *http.Client
}

type CredentialWritebackRequest struct {
	ChannelID       int            `json:"channel_id"`
	CredentialIndex int            `json:"credential_index"`
	CredentialType  string         `json:"credential_type,omitempty"`
	Credential      any            `json:"credential"`
	SourceJobID     string         `json:"source_job_id,omitempty"`
	SecretRef       string         `json:"secret_ref,omitempty"`
	Fingerprint     string         `json:"fingerprint,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type CredentialWritebackResult struct {
	ChannelID           int    `json:"channel_id"`
	CredentialIndex     int    `json:"credential_index"`
	AccountType         string `json:"account_type,omitempty"`
	ChannelStatus       int    `json:"channel_status"`
	AccountEnabled      bool   `json:"account_enabled"`
	ClearedAuthError    bool   `json:"cleared_auth_error"`
	ClearedAutoDisabled bool   `json:"cleared_auto_disabled"`
}

type DesktopProxy struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
	ProxyRules    string `json:"proxy_rules"`
	MaskedAddress string `json:"masked_address"`
	Enabled       bool   `json:"enabled"`
	Remark        string `json:"remark,omitempty"`
	ExitIP        string `json:"exit_ip,omitempty"`
	RegionCode    string `json:"region_code,omitempty"`
	RegionName    string `json:"region_name,omitempty"`
	CountryName   string `json:"country_name,omitempty"`
	City          string `json:"city,omitempty"`
	GeoStatus     string `json:"geo_status,omitempty"`
	GeoCheckedAt  int64  `json:"geo_checked_at,omitempty"`
	LastSuccessAt int64  `json:"last_success_at,omitempty"`
	LastFailureAt int64  `json:"last_failure_at,omitempty"`
	FailureCount  int64  `json:"failure_count,omitempty"`
	UseCount      int64  `json:"use_count,omitempty"`
	UpdatedTime   int64  `json:"updated_time,omitempty"`
}

type ChannelAccountArchiveTarget struct {
	ChannelID       int `json:"channel_id"`
	CredentialIndex int `json:"credential_index"`
}

type AccountPoolArchiveRequest struct {
	Targets     []ChannelAccountArchiveTarget `json:"targets"`
	Reason      string                        `json:"reason,omitempty"`
	Note        string                        `json:"note,omitempty"`
	SourceJobID string                        `json:"source_job_id,omitempty"`
}

type AccountPoolArchiveOperation struct {
	Type            string `json:"type,omitempty"`
	Action          string `json:"action,omitempty"`
	Requested       int    `json:"requested,omitempty"`
	Affected        int    `json:"affected,omitempty"`
	Deleted         int    `json:"deleted,omitempty"`
	ChannelDisabled bool   `json:"channel_disabled,omitempty"`
	ChannelRestored bool   `json:"channel_restored,omitempty"`
	CredentialIndex *int   `json:"credential_index,omitempty"`
	OriginalIndex   *int   `json:"original_index,omitempty"`
	ChannelID       int    `json:"channel_id,omitempty"`
	ChannelStatus   int    `json:"channel_status,omitempty"`
	AccountEnabled  bool   `json:"account_enabled,omitempty"`
	AccountStatus   int    `json:"account_status,omitempty"`
	PoolID          int64  `json:"pool_id,omitempty"`
	ArchivePool     string `json:"archive_pool,omitempty"`
	ArchivePoolID   int64  `json:"archive_pool_id,omitempty"`
}

type AccountPoolArchiveResult struct {
	Operation *AccountPoolArchiveOperation `json:"operation,omitempty"`
}

type AccountPoolListParams struct {
	Page        int
	PageSize    int
	Keyword     string
	ChannelID   int
	AccountType string
	Brand       string
	Provider    string
}

type AccountPoolListItem struct {
	ID                           int            `json:"id"`
	Pool                         string         `json:"pool"`
	ChannelID                    int            `json:"channel_id"`
	ChannelName                  string         `json:"channel_name,omitempty"`
	CredentialIndex              int            `json:"credential_index"`
	AccountID                    string         `json:"account_id,omitempty"`
	AccountIdentityKey           string         `json:"account_identity_key,omitempty"`
	CredentialSubjectFingerprint string         `json:"credential_subject_fingerprint,omitempty"`
	CredentialFingerprint        string         `json:"credential_fingerprint,omitempty"`
	SubjectShort                 string         `json:"subject_short,omitempty"`
	CredentialShort              string         `json:"credential_short,omitempty"`
	CredentialMasked             string         `json:"credential_masked,omitempty"`
	AccountType                  string         `json:"account_type,omitempty"`
	Brand                        string         `json:"brand,omitempty"`
	Provider                     string         `json:"provider,omitempty"`
	ResourceID                   string         `json:"resource_id,omitempty"`
	ResourceType                 string         `json:"resource_type,omitempty"`
	ProxyID                      int            `json:"proxy_id,omitempty"`
	CodexEnvironmentID           int            `json:"codex_environment_id,omitempty"`
	Capabilities                 map[string]any `json:"capabilities,omitempty"`
	Reason                       string         `json:"reason,omitempty"`
	Note                         string         `json:"note,omitempty"`
	ArchivedAt                   int64          `json:"archived_at,omitempty"`
	UpdatedAt                    int64          `json:"updated_at,omitempty"`
}

type AccountPoolListResult struct {
	Page          int                   `json:"page"`
	PageSize      int                   `json:"page_size"`
	Total         int64                 `json:"total"`
	FilteredTotal int64                 `json:"filtered_total,omitempty"`
	Items         []AccountPoolListItem `json:"items"`
}

type InvalidAccountReauthorizeRequest struct {
	ChannelID int    `json:"channel_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type InvalidAccountReauthorizeResult struct {
	Operation  *AccountPoolArchiveOperation `json:"operation,omitempty"`
	Automation any                          `json:"automation,omitempty"`
}

type AccountProfileResult struct {
	ChannelID       int            `json:"channel_id"`
	ChannelName     string         `json:"channel_name,omitempty"`
	ChannelStatus   int            `json:"channel_status,omitempty"`
	CredentialIndex int            `json:"credential_index"`
	ResourceRef     map[string]any `json:"resource_ref,omitempty"`
	Account         map[string]any `json:"account,omitempty"`
	SnapshotAt      int64          `json:"snapshot_at,omitempty"`
}

func New(baseURL string, token string, timeoutSeconds int) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		timeout: timeout,
		client:  &http.Client{},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != "" && c.token != ""
}

func (c *Client) WriteCredential(ctx context.Context, req CredentialWritebackRequest) (*CredentialWritebackResult, error) {
	if !c.Enabled() {
		return nil, nil
	}
	if req.ChannelID <= 0 || req.CredentialIndex < 0 {
		return nil, errors.New("channel_id and credential_index are required for gateway writeback")
	}
	payload, err := jsonx.Marshal(req)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.baseURL+credentialWritebackPath, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var envelope struct {
		Success bool                      `json:"success"`
		Message string                    `json:"message,omitempty"`
		Data    CredentialWritebackResult `json:"data,omitempty"`
	}
	_ = jsonx.Decode(resp.Body, &envelope)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("gateway credential writeback failed: status=%d message=%s", resp.StatusCode, message)
	}
	return &envelope.Data, nil
}

func (c *Client) ListDesktopProxies(ctx context.Context, enabledOnly bool) ([]DesktopProxy, error) {
	if !c.Enabled() {
		return nil, errors.New("gateway callback is not configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	url := c.baseURL + desktopProxyListPath
	if enabledOnly {
		url += "?enabled_only=true"
	}
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Success bool           `json:"success"`
		Message string         `json:"message,omitempty"`
		Data    []DesktopProxy `json:"data,omitempty"`
	}
	_ = jsonx.Decode(resp.Body, &envelope)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("gateway proxy list failed: status=%d message=%s", resp.StatusCode, message)
	}
	return envelope.Data, nil
}

func (c *Client) GetAccountProfile(ctx context.Context, channelID int, credentialIndex int) (*AccountProfileResult, error) {
	if !c.Enabled() {
		return nil, errors.New("gateway callback is not configured")
	}
	if channelID <= 0 || credentialIndex < 0 {
		return nil, errors.New("channel_id and credential_index are required for account profile")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	requestURL := c.baseURL + accountProfilePath
	query := url.Values{}
	query.Set("channel_id", strconv.Itoa(channelID))
	query.Set("credential_index", strconv.Itoa(credentialIndex))
	requestURL += "?" + query.Encode()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Success bool                 `json:"success"`
		Message string               `json:"message,omitempty"`
		Data    AccountProfileResult `json:"data,omitempty"`
	}
	_ = jsonx.Decode(resp.Body, &envelope)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("gateway account profile failed: status=%d message=%s", resp.StatusCode, message)
	}
	return &envelope.Data, nil
}

func (c *Client) ArchiveAccountToInvalidPool(ctx context.Context, req AccountPoolArchiveRequest) (*AccountPoolArchiveResult, error) {
	return c.archiveAccount(ctx, invalidAccountArchivePath, req)
}

func (c *Client) ArchiveAccountToDiscardedPool(ctx context.Context, req AccountPoolArchiveRequest) (*AccountPoolArchiveResult, error) {
	return c.archiveAccount(ctx, discardedAccountArchivePath, req)
}

func (c *Client) archiveAccount(ctx context.Context, path string, req AccountPoolArchiveRequest) (*AccountPoolArchiveResult, error) {
	if !c.Enabled() {
		return nil, errors.New("gateway callback is not configured")
	}
	if len(req.Targets) == 0 {
		return nil, errors.New("channel account target is required")
	}
	for _, target := range req.Targets {
		if target.ChannelID <= 0 || target.CredentialIndex < 0 {
			return nil, errors.New("channel_id and credential_index are required for account archive")
		}
	}
	payload, err := jsonx.Marshal(req)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Success bool                     `json:"success"`
		Message string                   `json:"message,omitempty"`
		Data    AccountPoolArchiveResult `json:"data,omitempty"`
	}
	_ = jsonx.Decode(resp.Body, &envelope)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("gateway account archive failed: status=%d message=%s", resp.StatusCode, message)
	}
	return &envelope.Data, nil
}

func (c *Client) ListInvalidAccountPool(ctx context.Context, params AccountPoolListParams) (*AccountPoolListResult, error) {
	if !c.Enabled() {
		return nil, errors.New("gateway callback is not configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	requestURL := c.baseURL + invalidAccountPoolListPath
	query := accountPoolListQuery(params)
	if query != "" {
		requestURL += "?" + query
	}
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Success bool                  `json:"success"`
		Message string                `json:"message,omitempty"`
		Data    AccountPoolListResult `json:"data,omitempty"`
	}
	_ = jsonx.Decode(resp.Body, &envelope)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("gateway invalid account pool list failed: status=%d message=%s", resp.StatusCode, message)
	}
	return &envelope.Data, nil
}

func (c *Client) ReauthorizeInvalidAccount(ctx context.Context, poolID int, req InvalidAccountReauthorizeRequest) (*InvalidAccountReauthorizeResult, error) {
	if !c.Enabled() {
		return nil, errors.New("gateway callback is not configured")
	}
	if poolID <= 0 {
		return nil, errors.New("pool_id is required")
	}
	payload, err := jsonx.Marshal(req)
	if err != nil {
		return nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	path := invalidAccountReauthorizePrefix + strconv.Itoa(poolID) + "/reauthorize"
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Success bool                            `json:"success"`
		Message string                          `json:"message,omitempty"`
		Data    InvalidAccountReauthorizeResult `json:"data,omitempty"`
	}
	_ = jsonx.Decode(resp.Body, &envelope)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("gateway invalid account reauthorize failed: status=%d message=%s", resp.StatusCode, message)
	}
	return &envelope.Data, nil
}

func accountPoolListQuery(params AccountPoolListParams) string {
	values := url.Values{}
	if params.Page > 0 {
		values.Set("page", strconv.Itoa(params.Page))
	}
	if params.PageSize > 0 {
		values.Set("page_size", strconv.Itoa(params.PageSize))
	}
	if strings.TrimSpace(params.Keyword) != "" {
		values.Set("keyword", strings.TrimSpace(params.Keyword))
	}
	if params.ChannelID > 0 {
		values.Set("channel_id", strconv.Itoa(params.ChannelID))
	}
	if strings.TrimSpace(params.AccountType) != "" {
		values.Set("account_type", strings.TrimSpace(params.AccountType))
	}
	if strings.TrimSpace(params.Brand) != "" {
		values.Set("brand", strings.TrimSpace(params.Brand))
	}
	if strings.TrimSpace(params.Provider) != "" {
		values.Set("provider", strings.TrimSpace(params.Provider))
	}
	return values.Encode()
}
