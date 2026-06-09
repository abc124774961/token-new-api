package classic

import (
	"embed"

	"github.com/QuantumNous/new-api/router"
)

//go:embed dist
var classicBuildFS embed.FS

//go:embed dist/index.html
var classicIndexPage []byte

func ThemeAssets() router.ThemeAssets {
	return router.ThemeAssets{
		ClassicBuildFS:   classicBuildFS,
		ClassicIndexPage: classicIndexPage,
		ClassicAssetRoot: "dist",
	}
}
