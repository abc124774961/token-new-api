package service

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

var channelConcurrency sync.Map           // channel_id -> *channelConcurrencyCounter
var channelConcurrencyLearnLocks sync.Map // channel_id -> *sync.Mutex

type channelConcurrencyCounter struct {
	mu     sync.Mutex
	active int
}

type ChannelConcurrencyLease struct {
	ChannelID   int
	Limit       int
	activeAtHit int
	counter     *channelConcurrencyCounter
	released    bool
}

func getChannelConcurrencyCounter(channelID int) *channelConcurrencyCounter {
	actual, _ := channelConcurrency.LoadOrStore(channelID, &channelConcurrencyCounter{})
	return actual.(*channelConcurrencyCounter)
}

func getChannelConcurrencyLearnLock(channelID int) *sync.Mutex {
	actual, _ := channelConcurrencyLearnLocks.LoadOrStore(channelID, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func GetChannelActiveConcurrency(channelID int) int {
	if channelID <= 0 {
		return 0
	}
	value, ok := channelConcurrency.Load(channelID)
	if !ok {
		return 0
	}
	counter := value.(*channelConcurrencyCounter)
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.active
}

func IsChannelConcurrencyFull(channelID int, setting dto.ChannelSettings) bool {
	limit := setting.MaxConcurrency
	if channelID <= 0 || limit <= 0 {
		return false
	}
	return GetChannelActiveConcurrency(channelID) >= limit
}

func TryAcquireChannelConcurrency(channelID int, setting dto.ChannelSettings) (*ChannelConcurrencyLease, bool) {
	limit := setting.MaxConcurrency
	if channelID <= 0 {
		return &ChannelConcurrencyLease{ChannelID: channelID, Limit: limit}, true
	}
	counter := getChannelConcurrencyCounter(channelID)
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if limit > 0 && counter.active >= limit {
		return &ChannelConcurrencyLease{
			ChannelID:   channelID,
			Limit:       limit,
			activeAtHit: counter.active,
			counter:     counter,
		}, false
	}
	counter.active++
	return &ChannelConcurrencyLease{
		ChannelID:   channelID,
		Limit:       limit,
		activeAtHit: counter.active,
		counter:     counter,
	}, true
}

func (lease *ChannelConcurrencyLease) Release() {
	if lease == nil || lease.released || lease.counter == nil {
		return
	}
	lease.counter.mu.Lock()
	if lease.counter.active > 0 {
		lease.counter.active--
	}
	lease.counter.mu.Unlock()
	lease.released = true
}

func (lease *ChannelConcurrencyLease) ActiveAtHit() int {
	if lease == nil {
		return 0
	}
	return lease.activeAtHit
}

func (lease *ChannelConcurrencyLease) CurrentActive() int {
	if lease == nil {
		return 0
	}
	if lease.counter == nil {
		return lease.activeAtHit
	}
	lease.counter.mu.Lock()
	defer lease.counter.mu.Unlock()
	return lease.counter.active
}

func IsUpstreamConcurrencyLimitError(err *types.NewAPIError) bool {
	if err == nil || err.StatusCode != http.StatusTooManyRequests {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "concurrency limit exceeded for user") ||
		strings.Contains(message, "too many pending requests")
}

func LearnChannelConcurrencyLimit(ctx *gin.Context, channelID int, activeAtLimit int, err *types.NewAPIError) {
	if channelID <= 0 || activeAtLimit <= 1 || !IsUpstreamConcurrencyLimitError(err) {
		return
	}
	learnedLimit := activeAtLimit - 1
	learnLock := getChannelConcurrencyLearnLock(channelID)
	learnLock.Lock()
	defer learnLock.Unlock()

	channel, getErr := model.GetChannelById(channelID, true)
	if getErr != nil || channel == nil {
		logChannelConcurrencyError(ctx, fmt.Sprintf("failed to learn channel concurrency limit: channel_id=%d, error=%v", channelID, getErr))
		return
	}
	setting := channel.GetSetting()
	if setting.MaxConcurrency > 0 && setting.MaxConcurrency <= learnedLimit {
		return
	}
	settingStr, marshalErr := buildLearnedChannelSetting(channel, learnedLimit)
	if marshalErr != nil {
		logChannelConcurrencyError(ctx, fmt.Sprintf("failed to marshal learned channel concurrency limit: channel_id=%d, error=%v", channelID, marshalErr))
		return
	}
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channelID).Update("setting", settingStr).Error; err != nil {
		logChannelConcurrencyError(ctx, fmt.Sprintf("failed to save learned channel concurrency limit: channel_id=%d, error=%v", channelID, err))
		return
	}
	channel.Setting = &settingStr
	model.CacheUpdateChannel(channel)
	logChannelConcurrencyInfo(ctx, fmt.Sprintf("learned channel #%d max concurrency: %d", channelID, learnedLimit))
}

func buildLearnedChannelSetting(channel *model.Channel, learnedLimit int) (string, error) {
	settingMap := map[string]any{}
	if channel.Setting != nil && strings.TrimSpace(*channel.Setting) != "" {
		if err := common.Unmarshal([]byte(*channel.Setting), &settingMap); err != nil {
			return "", err
		}
	}
	settingMap["max_concurrency"] = learnedLimit
	settingBytes, err := common.Marshal(settingMap)
	if err != nil {
		return "", err
	}
	return string(settingBytes), nil
}

func logChannelConcurrencyError(ctx *gin.Context, message string) {
	if ctx == nil {
		common.SysError(message)
		return
	}
	logger.LogError(ctx, message)
}

func logChannelConcurrencyInfo(ctx *gin.Context, message string) {
	if ctx == nil {
		common.SysLog(message)
		return
	}
	logger.LogInfo(ctx, message)
}

func ClearChannelConcurrencyForTest() {
	channelConcurrency.Range(func(key, value any) bool {
		channelConcurrency.Delete(key)
		return true
	})
	channelConcurrencyLearnLocks.Range(func(key, value any) bool {
		channelConcurrencyLearnLocks.Delete(key)
		return true
	})
}
