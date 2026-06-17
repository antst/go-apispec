// Package main exercises a full echo REST CRUD lifecycle (GET list / GET by id /
// POST / PUT / DELETE) with c.Param, c.Bind, c.QueryParam and c.NoContent — the
// complete resource lifecycle the other echo fixtures don't cover.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Book is the resource.
type Book struct {
	ID     int    `json:"id"`
	Title  string `json:"title" validate:"required"`
	Author string `json:"author"`
	Year   int    `json:"year"`
}

func listBooks(c echo.Context) error {
	_ = c.QueryParam("author")
	return c.JSON(http.StatusOK, []Book{})
}

func getBook(c echo.Context) error {
	_ = c.Param("id")
	return c.JSON(http.StatusOK, Book{})
}

func createBook(c echo.Context) error {
	var b Book
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}
	return c.JSON(http.StatusCreated, b)
}

func updateBook(c echo.Context) error {
	var b Book
	if err := c.Bind(&b); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}
	return c.JSON(http.StatusOK, b)
}

func deleteBook(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func main() {
	e := echo.New()
	e.GET("/books", listBooks)
	e.GET("/books/:id", getBook)
	e.POST("/books", createBook)
	e.PUT("/books/:id", updateBook)
	e.DELETE("/books/:id", deleteBook)
	_ = e.Start(":8080")
}
