package service

import (
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/constant"
)

const (
	channelConfigIsolationFailureThreshold = 2
	channelConfigIsolationDefaultTTL       = 0
)

type ChannelConfigIsolationKey struct {
	ChannelID           int                   `json:"channel_id"`
	RequestedModel      string                `json:"requested_model"`
	SelectedGroup       string                `json:"selected_group"`
	EndpointType        constant.EndpointType `json:"endpoint_type"`
	AccountID           string                `json:"account_id,omitempty"`
	CredentialIndex     int                   `json:"credential_index,omitempty"`
	CredentialIndexSet  bool                  `json:"credential_index_set,omitempty"`
	CredentialSubjectFP string                `json:"credential_subject_fingerprint,omitempty"`
	CredentialFP        string                `json:"credential_fingerprint,omitempty"`
}

type ChannelConfigIsolationStatus struct {
	Active       bool   `json:"active"`
	Reason       string `json:"reason,omitempty"`
	Until        int64  `json:"until,omitempty"`
	RemainingSec int64  `json:"remaining_seconds,omitempty"`
	FailureCount int    `json:"failure_count,omitempty"`
	LastErrorAt  int64  `json:"last_error_at,omitempty"`
}

type channelConfigIsolationEntry struct {
	reason       string
	until        time.Time
	failureCount int
	lastErrorAt  time.Time
}

type ChannelConfigIsolationManager struct {
	mu               sync.RWMutex
	entries          map[ChannelConfigIsolationKey]*channelConfigIsolationEntry
	ttl              time.Duration
	failureThreshold int
	now              func() time.Time
}

var defaultChannelConfigIsolationManager = NewChannelConfigIsolationManager()

func NewChannelConfigIsolationKey(channelID int, requestedModel, selectedGroup string, endpointType constant.EndpointType) ChannelConfigIsolationKey {
	return ChannelConfigIsolationKey{
		ChannelID:      channelID,
		RequestedModel: strings.TrimSpace(requestedModel),
		SelectedGroup:  strings.TrimSpace(selectedGroup),
		EndpointType:   normalizeChannelConfigIsolationEndpoint(endpointType),
	}
}

func NewChannelRuntimeConfigIsolationKey(identity ChannelRuntimeIdentity, requestedModel, selectedGroup string, endpointType constant.EndpointType) ChannelConfigIsolationKey {
	identity = identity.Normalize()
	key := NewChannelConfigIsolationKey(identity.ChannelID, requestedModel, selectedGroup, endpointType)
	if strings.TrimSpace(key.RequestedModel) == "" {
		key.RequestedModel = identity.RequestedModel
	}
	if strings.TrimSpace(key.SelectedGroup) == "" {
		key.SelectedGroup = identity.SelectedGroup
	}
	if key.EndpointType == "" {
		key.EndpointType = normalizeChannelConfigIsolationEndpoint(identity.EndpointType)
	}
	if identity.HasAccountScope() {
		key.AccountID = identity.AccountID
		key.CredentialIndex = identity.CredentialIndex
		key.CredentialIndexSet = identity.CredentialIndexSet
		key.CredentialSubjectFP = identity.CredentialSubjectFP
		key.CredentialFP = identity.CredentialFP
	}
	return key
}

func NewChannelConfigIsolationManager() *ChannelConfigIsolationManager {
	return newChannelConfigIsolationManager(channelConfigIsolationDefaultTTL, channelConfigIsolationFailureThreshold, time.Now)
}

func newChannelConfigIsolationManager(ttl time.Duration, failureThreshold int, now func() time.Time) *ChannelConfigIsolationManager {
	if ttl < 0 {
		ttl = channelConfigIsolationDefaultTTL
	}
	if failureThreshold <= 0 {
		failureThreshold = channelConfigIsolationFailureThreshold
	}
	if now == nil {
		now = time.Now
	}
	return &ChannelConfigIsolationManager{
		entries:          make(map[ChannelConfigIsolationKey]*channelConfigIsolationEntry),
		ttl:              ttl,
		failureThreshold: failureThreshold,
		now:              now,
	}
}

func RecordChannelConfigAuthError(key ChannelConfigIsolationKey, reason string) *ChannelConfigIsolationStatus {
	return defaultChannelConfigIsolationManager.RecordAuthError(key, reason)
}

func RecordChannelConfigSuccess(key ChannelConfigIsolationKey) {
	defaultChannelConfigIsolationManager.RecordSuccess(key)
}

func GetChannelConfigIsolationStatus(key ChannelConfigIsolationKey) *ChannelConfigIsolationStatus {
	return defaultChannelConfigIsolationManager.GetStatus(key)
}

func IsChannelConfigIsolated(key ChannelConfigIsolationKey) bool {
	return defaultChannelConfigIsolationManager.IsIsolated(key)
}

func ClearChannelConfigIsolation(key ChannelConfigIsolationKey) {
	defaultChannelConfigIsolationManager.Clear(key)
}

func ClearChannelConfigIsolationForChannel(channelID int) {
	defaultChannelConfigIsolationManager.ClearForChannel(channelID)
}

func ClearChannelConfigIsolationForAccountIndex(channelID int, credentialIndex int) {
	defaultChannelConfigIsolationManager.ClearForAccountIndex(channelID, credentialIndex)
}

func (m *ChannelConfigIsolationManager) RecordAuthError(key ChannelConfigIsolationKey, reason string) *ChannelConfigIsolationStatus {
	if m == nil || !key.valid() {
		return nil
	}
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.deleteExpiredLocked(key, now)
	entry, ok := m.entries[key]
	if !ok {
		entry = &channelConfigIsolationEntry{}
		m.entries[key] = entry
	}
	entry.failureCount++
	entry.reason = reason
	entry.lastErrorAt = now
	entry.until = time.Time{}
	return statusFromChannelConfigIsolationEntry(entry, now)
}

func (m *ChannelConfigIsolationManager) RecordSuccess(key ChannelConfigIsolationKey) {
	if m == nil || !key.valid() {
		return
	}
	m.Clear(key)
}

func (m *ChannelConfigIsolationManager) GetStatus(key ChannelConfigIsolationKey) *ChannelConfigIsolationStatus {
	if m == nil || !key.valid() {
		return nil
	}
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.deleteExpiredLocked(key, now)
	entry, ok := m.entries[key]
	if !ok {
		return nil
	}
	return statusFromChannelConfigIsolationEntry(entry, now)
}

func (m *ChannelConfigIsolationManager) IsIsolated(key ChannelConfigIsolationKey) bool {
	status := m.GetStatus(key)
	return status != nil && status.Active
}

func (m *ChannelConfigIsolationManager) Clear(key ChannelConfigIsolationKey) {
	if m == nil || !key.valid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
}

func (m *ChannelConfigIsolationManager) ClearForChannel(channelID int) {
	if m == nil || channelID <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.entries {
		if key.ChannelID == channelID {
			delete(m.entries, key)
		}
	}
}

func (m *ChannelConfigIsolationManager) ClearForAccountIndex(channelID int, credentialIndex int) {
	if m == nil || channelID <= 0 || credentialIndex < 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.entries {
		if key.ChannelID == channelID && key.CredentialIndexSet && key.CredentialIndex == credentialIndex {
			delete(m.entries, key)
		}
	}
}

func (m *ChannelConfigIsolationManager) clearAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = make(map[ChannelConfigIsolationKey]*channelConfigIsolationEntry)
}

func (m *ChannelConfigIsolationManager) deleteExpiredLocked(key ChannelConfigIsolationKey, now time.Time) {
	entry, ok := m.entries[key]
	if !ok {
		return
	}
	if entry.until.IsZero() || entry.until.After(now) {
		return
	}
	delete(m.entries, key)
}

func statusFromChannelConfigIsolationEntry(entry *channelConfigIsolationEntry, now time.Time) *ChannelConfigIsolationStatus {
	if entry == nil {
		return nil
	}
	status := &ChannelConfigIsolationStatus{
		Reason:       entry.reason,
		FailureCount: entry.failureCount,
	}
	if !entry.lastErrorAt.IsZero() {
		status.LastErrorAt = entry.lastErrorAt.Unix()
	}
	if entry.until.After(now) {
		status.Active = true
		status.Until = entry.until.Unix()
		status.RemainingSec = int64(entry.until.Sub(now).Seconds())
		if status.RemainingSec < 0 {
			status.RemainingSec = 0
		}
	}
	return status
}

func (key ChannelConfigIsolationKey) valid() bool {
	return key.ChannelID > 0
}

func normalizeChannelConfigIsolationEndpoint(endpointType constant.EndpointType) constant.EndpointType {
	endpointType = constant.EndpointType(strings.TrimSpace(string(endpointType)))
	if endpointType == "" {
		return constant.EndpointTypeOpenAI
	}
	return endpointType
}

func clearAllChannelConfigIsolationForTest() {
	defaultChannelConfigIsolationManager.clearAll()
}
