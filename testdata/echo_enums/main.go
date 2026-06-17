// Package main exercises echo route groups together with enum types (string
// constants → schema enum), `oneof` validation, and query/path params — a clean
// enum scenario on a properly-supported router.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Role enumerates account roles via string constants.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// Account is the resource: a Role enum field plus an oneof-validated plan.
type Account struct {
	ID     int    `json:"id"`
	Email  string `json:"email" validate:"required,email"`
	Role   Role   `json:"role"`
	Plan   string `json:"plan" validate:"required,oneof=free pro enterprise"`
	Active bool   `json:"active"`
}

// listAccounts returns accounts, optionally filtered by role query param.
func listAccounts(c echo.Context) error {
	_ = c.QueryParam("role")
	return c.JSON(http.StatusOK, []Account{})
}

// getAccount returns one account by id.
func getAccount(c echo.Context) error {
	_ = c.Param("id")
	return c.JSON(http.StatusOK, Account{})
}

// createAccount creates an account from the bound body.
func createAccount(c echo.Context) error {
	var a Account
	if err := c.Bind(&a); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad request"})
	}
	return c.JSON(http.StatusCreated, a)
}

func main() {
	e := echo.New()
	v1 := e.Group("/api/v1")
	accounts := v1.Group("/accounts")
	accounts.GET("", listAccounts)
	accounts.GET("/:id", getAccount)
	accounts.POST("", createAccount)
	_ = e.Start(":8080")
}
