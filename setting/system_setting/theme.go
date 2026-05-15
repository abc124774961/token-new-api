package system_setting

import (
	"github.com/QuantumNous/new-api/setting/config"
)

type ThemeSettings struct {
	Frontend string `json:"frontend"`
}

var themeSettings = ThemeSettings{
	Frontend: "classic",
}

func init() {
	config.GlobalConfig.Register("theme", &themeSettings)
}

func normalizeThemeSettings() {
	themeSettings.Frontend = "classic"
}

func GetThemeSettings() *ThemeSettings {
	normalizeThemeSettings()
	return &themeSettings
}

// UpdateAndSyncTheme normalizes legacy theme config after DB load.
func UpdateAndSyncTheme() {
	normalizeThemeSettings()
}
