// Package main exercises nested fiber groups (/api/v1/teams/:tid/members) with a
// nested path param and a response DTO carrying a map field.
package main

import "github.com/gofiber/fiber/v2"

// Team is a resource with a map of role->count.
type Team struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Roles map[string]int `json:"roles"`
}

// Member is a sub-resource.
type Member struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

func getTeam(c *fiber.Ctx) error     { _ = c.Params("tid"); return c.JSON(Team{}) }
func listMembers(c *fiber.Ctx) error { _ = c.Params("tid"); return c.JSON([]Member{}) }

func main() {
	app := fiber.New()
	api := app.Group("/api")
	v1 := api.Group("/v1")
	teams := v1.Group("/teams")
	teams.Get("/:tid", getTeam)
	teams.Get("/:tid/members", listMembers)
	_ = app.Listen(":8080")
}
