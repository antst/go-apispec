// Package main exercises a full fiber REST CRUD lifecycle (list / get / create /
// update / delete) with c.Params, c.Query, c.BodyParser and c.SendStatus.
package main

import (
	"github.com/gofiber/fiber/v2"
)

// Task is the resource.
type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

func listTasks(c *fiber.Ctx) error {
	_ = c.Query("status")
	return c.JSON([]Task{})
}

func getTask(c *fiber.Ctx) error {
	_ = c.Params("id")
	return c.JSON(Task{})
}

func createTask(c *fiber.Ctx) error {
	var t Task
	if err := c.BodyParser(&t); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bad request"})
	}
	return c.Status(fiber.StatusCreated).JSON(t)
}

func updateTask(c *fiber.Ctx) error {
	var t Task
	if err := c.BodyParser(&t); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bad request"})
	}
	return c.JSON(t)
}

func deleteTask(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func main() {
	app := fiber.New()
	app.Get("/tasks", listTasks)
	app.Get("/tasks/:id", getTask)
	app.Post("/tasks", createTask)
	app.Put("/tasks/:id", updateTask)
	app.Delete("/tasks/:id", deleteTask)
	_ = app.Listen(":8080")
}
