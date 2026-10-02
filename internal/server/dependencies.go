package server

import (
	"context"

	"github.com/cakmakfatih/chattered-server/internal/database/models"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Dependencies lists the services the onboarding router will use.
type Dependencies struct {
	Verifier  sessionVerifier
	Users     userRepository
	Validator *validator.Validate
}

type userRepository interface {
	GetByClerkID(context.Context, string) (*models.User, error)
	UsernameExists(context.Context, string) (bool, error)
	Create(context.Context, *models.User) error
}

// NewWithDependencies assembles the onboarding API with injectable services.
func NewWithDependencies(deps Dependencies) *gin.Engine {
	router := newRouter()
	api := router.Group("/api/v1")
	api.Use(authGuard(deps.Verifier))
	api.GET("/me", deps.me)
	api.POST("/onboarding/username/check", deps.checkUsername)
	api.POST("/onboarding/bio/validate", deps.validateBio)
	api.POST("/onboarding/photo/validate", deps.validatePhoto)
	api.POST("/onboarding/complete", deps.complete)
	return router
}
