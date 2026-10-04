package server

import (
	"context"
	"io"
	"time"

	"github.com/cakmakfatih/chattered-server/internal/database/models"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Dependencies lists the services the onboarding router will use.
type Dependencies struct {
	Verifier      sessionVerifier
	Users         userRepository
	ProfilePhotos profilePhotoStore
	Validator     *validator.Validate
}

type profilePhotoStore interface {
	AuthorizeUpload(context.Context, string, string, int64) (string, time.Time, error)
	OpenObject(context.Context, string) (io.ReadCloser, string, int64, error)
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
	api.PUT("/me/profile-photo", deps.authorizeProfilePhotoUpload)
	api.POST("/onboarding/complete", deps.complete)
	return router
}
