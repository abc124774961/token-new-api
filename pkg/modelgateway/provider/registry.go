package provider

import (
	"sync"

	"github.com/QuantumNous/new-api/model"
)

type ProviderRegistry interface {
	Register(profile ProviderProfile)
	Match(channel *model.Channel, modelName string) []ProviderProfile
	Best(channel *model.Channel, modelName string) ProviderProfile
	Get(name string) (ProviderProfile, bool)
}

type DefaultProviderRegistry struct {
	mu       sync.RWMutex
	profiles []ProviderProfile
	byName   map[string]ProviderProfile
}

func NewDefaultProviderRegistry(profiles ...ProviderProfile) *DefaultProviderRegistry {
	r := &DefaultProviderRegistry{
		profiles: make([]ProviderProfile, 0, len(profiles)),
		byName:   map[string]ProviderProfile{},
	}
	for _, profile := range profiles {
		r.Register(profile)
	}
	return r
}

func NewStandardProviderRegistry() *DefaultProviderRegistry {
	return NewDefaultProviderRegistry(
		NewOpenAICodexProfile(),
		NewMiMoCodexChatProfile(),
		NewDeepSeekV4ProCodexChatProfile(),
		NewStandardOpenAICompatibleProfile(),
	)
}

func (r *DefaultProviderRegistry) Register(profile ProviderProfile) {
	if r == nil || profile == nil || profile.Name() == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byName == nil {
		r.byName = map[string]ProviderProfile{}
	}
	if _, exists := r.byName[profile.Name()]; exists {
		r.byName[profile.Name()] = profile
		for i, existing := range r.profiles {
			if existing != nil && existing.Name() == profile.Name() {
				r.profiles[i] = profile
				return
			}
		}
	}
	r.profiles = append(r.profiles, profile)
	r.byName[profile.Name()] = profile
}

func (r *DefaultProviderRegistry) Match(channel *model.Channel, modelName string) []ProviderProfile {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	matched := make([]ProviderProfile, 0, len(r.profiles))
	for _, profile := range r.profiles {
		if profile != nil && profile.Match(channel, modelName) {
			matched = append(matched, profile)
		}
	}
	return matched
}

func (r *DefaultProviderRegistry) Best(channel *model.Channel, modelName string) ProviderProfile {
	matched := r.Match(channel, modelName)
	if len(matched) == 0 {
		return nil
	}
	return matched[0]
}

func (r *DefaultProviderRegistry) Get(name string) (ProviderProfile, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	profile, ok := r.byName[name]
	return profile, ok
}

var _ ProviderRegistry = (*DefaultProviderRegistry)(nil)
