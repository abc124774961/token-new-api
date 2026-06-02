package scheduler

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const (
	stickySourceUser          = "user_sticky"
	stickySourceCacheAffinity = "cache_affinity"

	defaultStickyTTLSeconds      = 180
	defaultStickyKeepScoreRatio  = 0.85
	defaultCacheKeepScoreRatio   = 0.75
	defaultStickyStoreMaxEntries = 100000
	defaultStickyFailurePolicy   = StickyFailurePolicyClear

	stickyStoreCacheNamespace = "new-api:modelgateway:sticky:v1"
	stickyRoutingDisabledKey  = "modelgateway_sticky_routing_disabled"
)

type StickyStore interface {
	Get(key string) (StickyEntry, bool)
	Set(key string, entry StickyEntry)
}

type stickyStoreDeleter interface {
	Delete(key string)
}

type stickyStoreBulkDeleter interface {
	DeleteMany(keys []string) int
}

type StickyStoreEntry struct {
	Key            string
	KeyID          string
	ChannelID      int
	Group          string
	KeyFingerprint string
	ExpiresAt      time.Time
}

type stickyStoreLister interface {
	List() []StickyStoreEntry
}

type StickyRouterOptions struct {
	TTLSeconds           int
	StickyKeepScoreRatio float64
	CacheKeepScoreRatio  float64
	SaveOnSelect         bool
	RenewOnSuccess       bool
	FailurePolicy        StickyFailurePolicy
	MaxEntries           int
	Store                StickyStore
}

type MemoryStickyRouter struct {
	ttl                  time.Duration
	stickyKeepScoreRatio float64
	cacheKeepScoreRatio  float64
	saveOnSelect         bool
	renewOnSuccess       bool
	failurePolicy        StickyFailurePolicy
	store                StickyStore
	cacheAdapter         core.CacheAffinitySignalAdapter
}

type StickyFailurePolicy string

const (
	StickyFailurePolicyKeep  StickyFailurePolicy = "keep"
	StickyFailurePolicyClear StickyFailurePolicy = "clear"
)

func normalizeStickyFailurePolicy(policy StickyFailurePolicy) StickyFailurePolicy {
	switch StickyFailurePolicy(strings.ToLower(strings.TrimSpace(string(policy)))) {
	case StickyFailurePolicyKeep:
		return StickyFailurePolicyKeep
	case StickyFailurePolicyClear, "":
		return defaultStickyFailurePolicy
	default:
		return defaultStickyFailurePolicy
	}
}

type StickyEntry struct {
	ChannelID      int       `json:"channel_id"`
	Group          string    `json:"group,omitempty"`
	KeyFingerprint string    `json:"key_fingerprint,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
}

func NewMemoryStickyRouter(options StickyRouterOptions, cacheAdapter core.CacheAffinitySignalAdapter) *MemoryStickyRouter {
	ttlSeconds := options.TTLSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = defaultStickyTTLSeconds
	}
	stickyRatio := options.StickyKeepScoreRatio
	if stickyRatio <= 0 {
		stickyRatio = defaultStickyKeepScoreRatio
	}
	cacheRatio := options.CacheKeepScoreRatio
	if cacheRatio <= 0 {
		cacheRatio = defaultCacheKeepScoreRatio
	}
	failurePolicy := normalizeStickyFailurePolicy(options.FailurePolicy)
	maxEntries := options.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultStickyStoreMaxEntries
	}
	store := options.Store
	if store == nil {
		store = NewMemoryStickyStore(maxEntries)
	}
	return &MemoryStickyRouter{
		ttl:                  time.Duration(ttlSeconds) * time.Second,
		stickyKeepScoreRatio: stickyRatio,
		cacheKeepScoreRatio:  cacheRatio,
		saveOnSelect:         options.SaveOnSelect,
		renewOnSuccess:       options.RenewOnSuccess,
		failurePolicy:        failurePolicy,
		store:                store,
		cacheAdapter:         cacheAdapter,
	}
}

func (r *MemoryStickyRouter) Route(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) (core.StickyRoute, bool) {
	if r == nil || req == nil {
		return core.StickyRoute{}, false
	}
	if StickyRoutingDisabled(c) {
		return core.StickyRoute{}, false
	}
	if policy.CacheAffinityEnabled && r.cacheAdapter != nil {
		if signal, ok := r.cacheAdapter.Extract(c, req, policy); ok {
			if signal.PreferredChannelID > 0 {
				return core.StickyRoute{
					ChannelID:      signal.PreferredChannelID,
					Group:          signal.PreferredGroup,
					Source:         stickySourceCacheAffinity,
					Key:            signal.Key,
					KeyFingerprint: signal.KeyFingerprint,
					CacheAware:     true,
					KeepScoreRatio: r.cacheKeepScoreRatio,
				}, true
			}
		}
	}
	key := r.userStickyKey(c, req, policy)
	if key == "" {
		return core.StickyRoute{}, false
	}
	entry, ok := r.store.Get(key)
	if !ok {
		return core.StickyRoute{}, false
	}
	return core.StickyRoute{
		ChannelID:      entry.ChannelID,
		Group:          entry.Group,
		Source:         stickySourceUser,
		Key:            key,
		KeyFingerprint: entry.KeyFingerprint,
		KeepScoreRatio: r.stickyKeepScoreRatio,
	}, true
}

func (r *MemoryStickyRouter) Save(c *gin.Context, req *core.DispatchRequest, plan *core.DispatchPlan) {
	r.save(c, req, plan, true)
}

func (r *MemoryStickyRouter) save(c *gin.Context, req *core.DispatchRequest, plan *core.DispatchPlan, guardRetryAttempt bool) {
	if r == nil || req == nil || plan == nil || plan.Channel == nil || plan.Channel.Id <= 0 {
		return
	}
	if StickyRoutingDisabled(c) {
		return
	}
	if guardRetryAttempt && shouldSkipStickySaveForRetryAttempt(c, req, plan, nil) {
		return
	}
	key := r.userStickyKey(c, req, core.GroupSmartPolicy{})
	if key == "" {
		return
	}
	r.store.Set(key, StickyEntry{
		ChannelID:      plan.Channel.Id,
		Group:          plan.SelectedGroup,
		KeyFingerprint: affinityFingerprint(key),
		ExpiresAt:      time.Now().Add(r.ttl),
	})
}

func (r *MemoryStickyRouter) Clear(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) {
	if r == nil || req == nil {
		return
	}
	if StickyRoutingDisabled(c) {
		return
	}
	key := r.userStickyKey(c, req, policy)
	if key == "" || r.store == nil {
		return
	}
	if deleter, ok := r.store.(stickyStoreDeleter); ok {
		deleter.Delete(key)
		return
	}
	r.store.Set(key, StickyEntry{ExpiresAt: time.Now().Add(-time.Second)})
}

func (r *MemoryStickyRouter) Report(c *gin.Context, req *core.DispatchRequest, plan *core.DispatchPlan, result core.AttemptResult) {
	if r == nil || req == nil || plan == nil || plan.Channel == nil || plan.StickySource == "" {
		return
	}
	if StickyRoutingDisabled(c) {
		return
	}
	if plan.CacheAffinity || plan.StickySource == stickySourceCacheAffinity {
		return
	}
	if result.Success {
		if r.renewOnSuccess && !shouldSkipStickySaveForRetryAttempt(c, req, plan, &result) {
			r.save(c, req, plan, false)
		}
		return
	}
	if r.failurePolicy == StickyFailurePolicyClear {
		r.Clear(c, req, core.GroupSmartPolicy{RequestedGroup: req.RequestedGroup})
	}
}

func (r *MemoryStickyRouter) SaveOnSelect() bool {
	if r == nil {
		return false
	}
	return r.saveOnSelect
}

func shouldSkipStickySaveForRetryAttempt(c *gin.Context, req *core.DispatchRequest, plan *core.DispatchPlan, result *core.AttemptResult) bool {
	if req != nil {
		if req.Retry > 0 {
			return true
		}
		if req.RetryRoutingIntent != nil && req.RetryRoutingIntent.Active() {
			return true
		}
	}
	if plan != nil {
		if plan.RetryIntentApplied {
			return true
		}
		if plan.RetryRoutingIntent != nil && plan.RetryRoutingIntent.Active() {
			return true
		}
	}
	if result != nil {
		if result.AttemptIndex > 0 {
			return true
		}
		if len(result.UsedChannels) > 1 {
			return true
		}
		if c != nil && len(c.GetStringSlice("use_channel")) > 1 {
			return true
		}
		return false
	}
	return c != nil && len(c.GetStringSlice("use_channel")) > 0
}

func StickyRoutingDisabled(c *gin.Context) bool {
	if c == nil {
		return false
	}
	disabled, _ := c.Get(stickyRoutingDisabledKey)
	value, _ := disabled.(bool)
	return value
}

func WithStickyRoutingDisabled(c *gin.Context, fn func()) {
	if fn == nil {
		return
	}
	if c == nil {
		fn()
		return
	}
	previous, existed := c.Get(stickyRoutingDisabledKey)
	c.Set(stickyRoutingDisabledKey, true)
	defer func() {
		if existed {
			c.Set(stickyRoutingDisabledKey, previous)
			return
		}
		c.Set(stickyRoutingDisabledKey, nil)
	}()
	fn()
}

type MemoryStickyStore struct {
	mu         sync.RWMutex
	entries    map[string]StickyEntry
	maxEntries int
}

func NewMemoryStickyStore(maxEntries int) *MemoryStickyStore {
	if maxEntries <= 0 {
		maxEntries = defaultStickyStoreMaxEntries
	}
	return &MemoryStickyStore{
		entries:    map[string]StickyEntry{},
		maxEntries: maxEntries,
	}
}

func (s *MemoryStickyStore) Get(key string) (StickyEntry, bool) {
	if s == nil {
		return StickyEntry{}, false
	}
	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return StickyEntry{}, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		s.mu.Lock()
		if current, exists := s.entries[key]; exists && current.ExpiresAt.Equal(entry.ExpiresAt) {
			delete(s.entries, key)
		}
		s.mu.Unlock()
		return StickyEntry{}, false
	}
	return entry, true
}

func (s *MemoryStickyStore) Set(key string, entry StickyEntry) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = map[string]StickyEntry{}
	}
	if len(s.entries) >= s.maxEntries {
		s.evictExpiredLocked()
	}
	if len(s.entries) >= s.maxEntries {
		for k := range s.entries {
			delete(s.entries, k)
			break
		}
	}
	s.entries[key] = entry
}

func (s *MemoryStickyStore) Delete(key string) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
}

func (s *MemoryStickyStore) DeleteMany(keys []string) int {
	if s == nil || len(keys) == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := s.entries[key]; ok {
			delete(s.entries, key)
			deleted++
		}
	}
	return deleted
}

func (s *MemoryStickyStore) List() []StickyStoreEntry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	entries := make([]StickyStoreEntry, 0, len(s.entries))
	for key, entry := range s.entries {
		entries = append(entries, buildStickyStoreEntry(key, entry))
	}
	sortStickyStoreEntries(entries)
	return entries
}

func (s *MemoryStickyStore) evictExpiredLocked() {
	now := time.Now()
	for key, entry := range s.entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(s.entries, key)
		}
	}
}

type StickyEntryCodec struct{}

func (c StickyEntryCodec) Encode(v StickyEntry) (string, error) {
	body, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c StickyEntryCodec) Decode(s string) (StickyEntry, error) {
	var entry StickyEntry
	if err := common.UnmarshalJsonStr(s, &entry); err != nil {
		return StickyEntry{}, err
	}
	return entry, nil
}

type HybridStickyStore struct {
	cache *cachex.HybridCache[StickyEntry]
}

func NewHybridStickyStore(maxEntries int) *HybridStickyStore {
	if maxEntries <= 0 {
		maxEntries = defaultStickyStoreMaxEntries
	}
	return &HybridStickyStore{
		cache: cachex.NewHybridCache[StickyEntry](cachex.HybridCacheConfig[StickyEntry]{
			Namespace:  cachex.Namespace(stickyStoreCacheNamespace),
			Redis:      common.RDB,
			RedisCodec: StickyEntryCodec{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, StickyEntry] {
				return hot.NewHotCache[string, StickyEntry](hot.LRU, maxEntries).
					WithTTL(time.Duration(defaultStickyTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		}),
	}
}

func (s *HybridStickyStore) Get(key string) (StickyEntry, bool) {
	if s == nil || s.cache == nil {
		return StickyEntry{}, false
	}
	entry, found, err := s.cache.Get(key)
	if err != nil || !found {
		return StickyEntry{}, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		_, _ = s.cache.DeleteMany([]string{key})
		return StickyEntry{}, false
	}
	return entry, true
}

func (s *HybridStickyStore) Set(key string, entry StickyEntry) {
	if s == nil || s.cache == nil || strings.TrimSpace(key) == "" {
		return
	}
	ttl := time.Duration(defaultStickyTTLSeconds) * time.Second
	if !entry.ExpiresAt.IsZero() {
		ttl = time.Until(entry.ExpiresAt)
		if ttl <= 0 {
			return
		}
	}
	_ = s.cache.SetWithTTL(key, entry, ttl)
}

func (s *HybridStickyStore) Delete(key string) {
	if s == nil || s.cache == nil || strings.TrimSpace(key) == "" {
		return
	}
	_, _ = s.cache.DeleteMany([]string{key})
}

func (s *HybridStickyStore) DeleteMany(keys []string) int {
	if s == nil || s.cache == nil || len(keys) == 0 {
		return 0
	}
	result, err := s.cache.DeleteMany(keys)
	if err != nil {
		return 0
	}
	deleted := 0
	for _, ok := range result {
		if ok {
			deleted++
		}
	}
	return deleted
}

func (s *HybridStickyStore) List() []StickyStoreEntry {
	if s == nil || s.cache == nil {
		return nil
	}
	keys, err := s.cache.Keys()
	if err != nil {
		return nil
	}
	entries := make([]StickyStoreEntry, 0, len(keys))
	for _, key := range keys {
		entry, found := s.Get(key)
		if !found {
			continue
		}
		entries = append(entries, buildStickyStoreEntry(key, entry))
	}
	sortStickyStoreEntries(entries)
	return entries
}

func (r *MemoryStickyRouter) StickyEntries() []StickyStoreEntry {
	if r == nil || r.store == nil {
		return nil
	}
	lister, ok := r.store.(stickyStoreLister)
	if !ok {
		return nil
	}
	return lister.List()
}

func (r *MemoryStickyRouter) ClearStickyEntryByID(keyID string) bool {
	keyID = strings.TrimSpace(keyID)
	if r == nil || r.store == nil || keyID == "" {
		return false
	}
	deleter, ok := r.store.(stickyStoreDeleter)
	if !ok {
		return false
	}
	for _, entry := range r.StickyEntries() {
		if entry.KeyID == keyID {
			deleter.Delete(entry.Key)
			return true
		}
	}
	return false
}

type StickyClearFilter struct {
	Group     string
	ChannelID int
	KeyID     string
}

func (r *MemoryStickyRouter) ClearStickyEntries(filter StickyClearFilter) int {
	if r == nil || r.store == nil {
		return 0
	}
	keys := make([]string, 0)
	for _, entry := range r.StickyEntries() {
		if stickyEntryMatchesClearFilter(entry, filter) {
			keys = append(keys, entry.Key)
		}
	}
	if len(keys) == 0 {
		return 0
	}
	if bulk, ok := r.store.(stickyStoreBulkDeleter); ok {
		return bulk.DeleteMany(keys)
	}
	deleted := 0
	if deleter, ok := r.store.(stickyStoreDeleter); ok {
		for _, key := range keys {
			deleter.Delete(key)
			deleted++
		}
	}
	return deleted
}

func stickyEntryMatchesClearFilter(entry StickyStoreEntry, filter StickyClearFilter) bool {
	group := strings.TrimSpace(filter.Group)
	if group != "" && entry.Group != group {
		return false
	}
	if filter.ChannelID > 0 && entry.ChannelID != filter.ChannelID {
		return false
	}
	keyID := strings.TrimSpace(filter.KeyID)
	if keyID != "" && entry.KeyID != keyID {
		return false
	}
	return true
}

func buildStickyStoreEntry(key string, entry StickyEntry) StickyStoreEntry {
	return StickyStoreEntry{
		Key:            key,
		KeyID:          affinityFingerprint(key),
		ChannelID:      entry.ChannelID,
		Group:          entry.Group,
		KeyFingerprint: entry.KeyFingerprint,
		ExpiresAt:      entry.ExpiresAt,
	}
}

func sortStickyStoreEntries(entries []StickyStoreEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if !left.ExpiresAt.Equal(right.ExpiresAt) {
			return left.ExpiresAt.After(right.ExpiresAt)
		}
		if left.Group != right.Group {
			return left.Group < right.Group
		}
		if left.ChannelID != right.ChannelID {
			return left.ChannelID < right.ChannelID
		}
		return left.KeyID < right.KeyID
	})
}

func (r *MemoryStickyRouter) userStickyKey(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) string {
	actor := stickyActor(c)
	if actor == "" || req.ModelName == "" {
		return ""
	}
	group := req.RequestedGroup
	if policy.RequestedGroup != "" {
		group = policy.RequestedGroup
	}
	if group == "" {
		group = req.UserGroup
	}
	endpointType := req.EndpointType
	if endpointType == "" {
		endpointType = constant.EndpointTypeOpenAI
	}
	sessionKey := stickySessionKey(c)
	if sessionKey == "" {
		return ""
	}
	parts := []string{
		actor,
		group,
		req.ModelName,
		string(endpointType),
		sessionKey,
	}
	return strings.Join(parts, "\n")
}

var stickyBodyKeyPaths = []string{
	"prompt_cache_key",
	"previous_response_id",
	"session_id",
	"sessionId",
	"session.id",
	"conversation_id",
	"conversationId",
	"conversation",
	"conversation.id",
	"chat_id",
	"chatId",
	"chat.id",
	"thread_id",
	"threadId",
	"thread.id",
	"parent_id",
	"parentId",
	"parent.id",
	"metadata.session_id",
	"metadata.sessionId",
	"metadata.session.id",
	"metadata.conversation_id",
	"metadata.conversationId",
	"metadata.conversation.id",
	"metadata.chat_id",
	"metadata.chatId",
	"metadata.chat.id",
	"metadata.thread_id",
	"metadata.threadId",
	"metadata.thread.id",
	"metadata.parent_id",
	"metadata.parentId",
	"metadata.parent.id",
	"metadata.user_id",
	"metadata.userId",
	"extra_body.session_id",
	"extra_body.session.id",
	"extra_body.conversation_id",
	"extra_body.conversation.id",
	"extra_body.thread_id",
	"extra_body.thread.id",
}

var stickyHeaderKeys = []string{
	"Session_id",
	"Session-Id",
	"X-Session-Id",
	"X-Conversation-Id",
	"X-Thread-Id",
	"X-Chat-Id",
	"X-Parent-Id",
	"X-Codex-Session-Id",
	"X-Codex-Conversation-Id",
	"X-Codex-Thread-Id",
	"Mcp-Session-Id",
}

var stickyMetadataHeaderPaths = []string{
	"session_id",
	"sessionId",
	"conversation_id",
	"conversationId",
	"thread_id",
	"threadId",
	"chat_id",
	"chatId",
}

func stickySessionKey(c *gin.Context) string {
	if source, value := stickyHeaderSignal(c); value != "" {
		return stickySessionKeyPart(source, value)
	}
	if source, value := stickyBodySignal(c); value != "" {
		return stickySessionKeyPart(source, value)
	}
	return ""
}

func stickyHeaderSignal(c *gin.Context) (string, string) {
	if c == nil || c.Request == nil {
		return "", ""
	}
	if source, value := stickyMetadataHeaderSignal(c.Request.Header); value != "" {
		return source, value
	}
	for _, header := range stickyHeaderKeys {
		if value := normalizeStickySignalValue(c.GetHeader(header)); value != "" {
			return "header." + strings.ToLower(header), value
		}
	}
	return "", ""
}

func stickyMetadataHeaderSignal(header http.Header) (string, string) {
	if len(header) == 0 {
		return "", ""
	}
	raw := strings.TrimSpace(header.Get("X-Codex-Turn-Metadata"))
	if raw == "" {
		return "", ""
	}
	for _, path := range stickyMetadataHeaderPaths {
		result := gjson.Get(raw, path)
		if value := stickyJSONScalar(result); value != "" {
			return "header.x-codex-turn-metadata." + path, value
		}
	}
	return "", ""
}

func stickyBodySignal(c *gin.Context) (string, string) {
	if c == nil || c.Request == nil {
		return "", ""
	}
	if c.Request.Body == nil {
		return "", ""
	}
	contentType := c.Request.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(strings.ToLower(contentType), "json") {
		return "", ""
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return "", ""
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return "", ""
	}
	for _, path := range stickyBodyKeyPaths {
		result := gjson.GetBytes(body, path)
		if value := stickyJSONScalar(result); value != "" {
			return "body." + path, value
		}
	}
	return "", ""
}

func stickyJSONScalar(result gjson.Result) string {
	if !result.Exists() {
		return ""
	}
	switch result.Type {
	case gjson.String, gjson.Number:
		return normalizeStickySignalValue(result.String())
	default:
		return ""
	}
}

func normalizeStickySignalValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	switch strings.ToLower(value) {
	case "auto", "none", "null", "undefined", "false", "true", "{}", "[]":
		return ""
	default:
		return value
	}
}

func stickySessionKeyPart(source string, value string) string {
	source = strings.TrimSpace(source)
	value = normalizeStickySignalValue(value)
	if source == "" || value == "" {
		return ""
	}
	return "session:" + source + ":" + affinityFingerprint(value)
}

type ServiceCacheAffinitySignalAdapter struct{}

func NewServiceCacheAffinitySignalAdapter() *ServiceCacheAffinitySignalAdapter {
	return &ServiceCacheAffinitySignalAdapter{}
}

func (a *ServiceCacheAffinitySignalAdapter) Extract(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) (core.CacheAffinitySignal, bool) {
	if c == nil || req == nil || req.ModelName == "" {
		return core.CacheAffinitySignal{}, false
	}
	group := req.RequestedGroup
	if policy.RequestedGroup != "" {
		group = policy.RequestedGroup
	}
	if group == "auto" {
		group = req.CurrentAutoGroup
	}
	if group == "" {
		group = req.UserGroup
	}
	signal, ok := service.ResolveChannelAffinitySignal(c, req.ModelName, group)
	if !ok {
		return core.CacheAffinitySignal{}, false
	}
	return core.CacheAffinitySignal{
		Key:                signal.CacheKey,
		KeyFingerprint:     signal.KeyFingerprint,
		Source:             signal.RuleName,
		TTLSeconds:         signal.TTLSeconds,
		PreferredChannelID: signal.PreferredChannelID,
		PreferredGroup:     signal.UsingGroup,
	}, true
}

type StaticCacheAffinitySignalAdapter struct {
	Signal core.CacheAffinitySignal
	Found  bool
}

func NewStaticCacheAffinitySignalAdapter(signal core.CacheAffinitySignal, found bool) *StaticCacheAffinitySignalAdapter {
	return &StaticCacheAffinitySignalAdapter{Signal: signal, Found: found}
}

func (a *StaticCacheAffinitySignalAdapter) Extract(c *gin.Context, req *core.DispatchRequest, policy core.GroupSmartPolicy) (core.CacheAffinitySignal, bool) {
	if a == nil {
		return core.CacheAffinitySignal{}, false
	}
	return a.Signal, a.Found
}

func stickyActor(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId); tokenID > 0 {
		return fmt.Sprintf("token:%d", tokenID)
	}
	if userID := common.GetContextKeyInt(c, constant.ContextKeyUserId); userID > 0 {
		return fmt.Sprintf("user:%d", userID)
	}
	if tokenKey := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenKey)); tokenKey != "" {
		return "token_key:" + affinityFingerprint(tokenKey)
	}
	return ""
}

func affinityFingerprint(s string) string {
	if s == "" {
		return ""
	}
	sum := sha1.Sum([]byte(s))
	encoded := hex.EncodeToString(sum[:])
	if len(encoded) >= 8 {
		return encoded[:8]
	}
	return encoded
}

var _ core.StickyRouter = (*MemoryStickyRouter)(nil)
var _ core.CacheAffinitySignalAdapter = (*ServiceCacheAffinitySignalAdapter)(nil)
var _ core.CacheAffinitySignalAdapter = (*StaticCacheAffinitySignalAdapter)(nil)
