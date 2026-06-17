// Package main exercises deeply nested gin route groups (/api → /v1 → /projects,
// /admin) so the group-prefix resolution composes several levels into the final
// path, including a nested resource path param.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Project is a resource.
type Project struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Member is a sub-resource.
type Member struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
}

func listProjects(c *gin.Context) {
	c.JSON(http.StatusOK, []Project{})
}

func createProject(c *gin.Context) {
	var p Project
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, p)
}

func listMembers(c *gin.Context) {
	_ = c.Param("pid")
	c.JSON(http.StatusOK, []Member{})
}

func adminStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func main() {
	r := gin.New()
	api := r.Group("/api")
	{
		v1 := api.Group("/v1")
		{
			projects := v1.Group("/projects")
			{
				projects.GET("", listProjects)
				projects.POST("", createProject)
				projects.GET("/:pid/members", listMembers)
			}
			admin := v1.Group("/admin")
			{
				admin.GET("/stats", adminStats)
			}
		}
	}
	_ = r.Run(":8080")
}
