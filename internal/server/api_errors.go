package server

import "github.com/gin-gonic/gin"

type apiError struct {
	code    string
	field   string
	message string
	details gin.H
}

func respondError(c *gin.Context, status int, apiErr apiError) {
	problem := gin.H{"code": apiErr.code, "message": apiErr.message}
	if apiErr.field != "" {
		problem["field"] = apiErr.field
	}
	if apiErr.details != nil {
		problem["details"] = apiErr.details
	}
	c.JSON(status, gin.H{"error": problem})
}
