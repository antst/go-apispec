package main

import (
	"github.com/labstack/echo/v4"

	"github.com/antst/go-apispec/testdata/echo_handler_factory/api"
	"github.com/antst/go-apispec/testdata/echo_handler_factory/handlers"
)

func main() {
	e := echo.New()
	v1 := e.Group("/api/v1")
	api.RegisterRoutes(v1, handlers.New())
	_ = e.Start(":8080")
}
