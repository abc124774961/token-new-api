package operation_setting

import (
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled               bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes               float64 `json:"auto_test_channel_minutes"`
	ChannelBalanceMonitorEnabled         bool    `json:"channel_balance_monitor_enabled"`
	ChannelBalanceMonitorIntervalMinutes float64 `json:"channel_balance_monitor_interval_minutes"`
	ChannelBalanceWarningThreshold       float64 `json:"channel_balance_warning_threshold"`
	ChannelRatioSyncEnabled              bool    `json:"channel_ratio_sync_enabled"`
	ChannelRatioSyncIntervalMinutes      float64 `json:"channel_ratio_sync_interval_minutes"`
	ChannelRatioSyncTrustedAutoApply     bool    `json:"channel_ratio_sync_trusted_auto_apply"`
	ChannelBalanceMonitorRetentionDays   int     `json:"channel_balance_monitor_retention_days"`
}

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:               false,
	AutoTestChannelMinutes:               10,
	ChannelBalanceMonitorEnabled:         false,
	ChannelBalanceMonitorIntervalMinutes: 10,
	ChannelBalanceWarningThreshold:       10,
	ChannelRatioSyncEnabled:              false,
	ChannelRatioSyncIntervalMinutes:      60,
	ChannelRatioSyncTrustedAutoApply:     true,
	ChannelBalanceMonitorRetentionDays:   30,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
		}
	}
	return &monitorSetting
}

func SetMonitorSettingForTest(setting MonitorSetting) func() {
	previous := monitorSetting
	monitorSetting = setting
	return func() {
		monitorSetting = previous
	}
}
