// Package main exercises echo nested sub-resources: /authors/:aid/books/:bid with
// two path params, plus a top-level collection.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Author is the parent resource.
type Author struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Book is the nested resource.
type Book struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func listAuthors(c echo.Context) error { return c.JSON(http.StatusOK, []Author{}) }
func getAuthor(c echo.Context) error   { _ = c.Param("aid"); return c.JSON(http.StatusOK, Author{}) }
func listBooks(c echo.Context) error   { _ = c.Param("aid"); return c.JSON(http.StatusOK, []Book{}) }
func getBook(c echo.Context) error {
	_ = c.Param("aid")
	_ = c.Param("bid")
	return c.JSON(http.StatusOK, Book{})
}

func main() {
	e := echo.New()
	authors := e.Group("/authors")
	authors.GET("", listAuthors)
	authors.GET("/:aid", getAuthor)
	authors.GET("/:aid/books", listBooks)
	authors.GET("/:aid/books/:bid", getBook)
	_ = e.Start(":8080")
}
