// Package main exercises gin route groups (nested, versioned), query-parameter
// extraction (pagination + filtering), and response bodies carrying array and
// nested-struct fields — scenarios the flat single-router gin fixture doesn't.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Tag is a nested object used inside Article.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Article is the resource — note the []string and []Tag array fields.
type Article struct {
	ID       int      `json:"id"`
	Title    string   `json:"title" validate:"required"`
	Authors  []string `json:"authors"`
	Tags     []Tag    `json:"tags"`
	Featured bool     `json:"featured"`
}

// ArticleList is a paginated response envelope with an array of items.
type ArticleList struct {
	Items []Article `json:"items"`
	Page  int       `json:"page"`
	Total int       `json:"total"`
}

// listArticles returns a paginated, optionally tag-filtered list.
func listArticles(c *gin.Context) {
	_ = c.Query("page")
	_ = c.Query("tag")
	_ = c.DefaultQuery("limit", "20")
	c.JSON(http.StatusOK, ArticleList{})
}

// getArticle returns a single article by id.
func getArticle(c *gin.Context) {
	_ = c.Param("id")
	c.JSON(http.StatusOK, Article{})
}

// createArticle creates a new article from the JSON body.
func createArticle(c *gin.Context) {
	var a Article
	if err := c.ShouldBindJSON(&a); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, a)
}

// deleteArticle removes an article.
func deleteArticle(c *gin.Context) {
	_ = c.Param("id")
	c.Status(http.StatusNoContent)
}

func main() {
	r := gin.New()
	v1 := r.Group("/api/v1")
	{
		articles := v1.Group("/articles")
		{
			articles.GET("", listArticles)
			articles.GET("/:id", getArticle)
			articles.POST("", createArticle)
			articles.DELETE("/:id", deleteArticle)
		}
	}
	_ = r.Run(":8080")
}
