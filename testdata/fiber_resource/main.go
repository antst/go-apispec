// Package main exercises fiber route groups, c.Params / c.Query / c.BodyParser,
// and DTOs carrying pointer (optional) fields, a nested object, and a map —
// scenarios the flat fiber fixture doesn't cover.
package main

import (
	"github.com/gofiber/fiber/v2"
)

// Geo is an optional nested location.
type Geo struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Store has pointer (optional) fields, a nested object, and a map field.
type Store struct {
	ID       int               `json:"id"`
	Name     string            `json:"name"`
	Location *Geo              `json:"location,omitempty"`
	Hours    map[string]string `json:"hours"`
	Manager  *string           `json:"manager,omitempty"`
}

// listStores returns stores, optionally filtered by city.
func listStores(c *fiber.Ctx) error {
	_ = c.Query("city")
	return c.JSON([]Store{})
}

// getStore returns a single store by id.
func getStore(c *fiber.Ctx) error {
	_ = c.Params("id")
	return c.JSON(Store{})
}

// createStore creates a store from the parsed body.
func createStore(c *fiber.Ctx) error {
	var s Store
	if err := c.BodyParser(&s); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bad request"})
	}
	return c.Status(fiber.StatusCreated).JSON(s)
}

func main() {
	app := fiber.New()
	v1 := app.Group("/api/v1")
	stores := v1.Group("/stores")
	stores.Get("/", listStores)
	stores.Get("/:id", getStore)
	stores.Post("/", createStore)
	_ = app.Listen(":8080")
}
