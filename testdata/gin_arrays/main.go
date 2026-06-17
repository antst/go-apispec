// Package main exercises array request and response bodies in gin: a POST whose
// body is a slice of DTOs (bulk create) and responses that are slices.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Point is one element.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// bulkCreate accepts an array body ([]Point) and returns the created array.
func bulkCreate(c *gin.Context) {
	var pts []Point
	if err := c.ShouldBindJSON(&pts); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, pts)
}

// listPoints returns an array response.
func listPoints(c *gin.Context) {
	c.JSON(http.StatusOK, []Point{})
}

func main() {
	r := gin.New()
	r.POST("/points/bulk", bulkCreate)
	r.GET("/points", listPoints)
	_ = r.Run(":8080")
}
