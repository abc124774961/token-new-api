package main

import (
	"github.com/QuantumNous/new-api/internal/app"
	classic "github.com/QuantumNous/new-api/web/classic"
)

func main() {
	app.Run(app.RunConfig{
		Role:   app.RoleWeb,
		Assets: classic.ThemeAssets(),
	})
}
