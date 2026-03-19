// Package handler contains HTTP request handlers for the Olds API.
// In Go, a "package" groups related code — all files in this directory
// share the same package name and can access each other's unexported identifiers.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health handles GET /health. It returns a simple status payload so that
// Docker, load balancers, and curl can verify the server is alive.
//
// The function signature `func(c *gin.Context)` is what Gin expects for all
// route handlers. Think of *gin.Context as the request+response object combined —
// analogous to (req, res) in Express, but merged into one parameter.
func Health(c *gin.Context) {
	// gin.H is a shorthand for map[string]any — just a convenience alias.
	// http.StatusOK is the constant 200 from the standard library's net/http package.
	// Using named constants instead of magic numbers is idiomatic Go.
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}
