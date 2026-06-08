package main

import "github.com/QuantumNous/new-api/internal/app"

func main() {
	app.Run(app.RunConfig{
		Role: app.RoleGateway,
	})
}
