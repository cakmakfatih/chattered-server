package server

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"apartmanim/server/internal/database/models"
	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// Dependencies lists the services the onboarding router will use.
type Dependencies struct {
	Verifier interface {
		VerifySession(context.Context, string) (*clerk.SessionClaims, error)
	}
	Users interface {
		GetByClerkID(context.Context, string) (*models.User, error)
		UsernameExists(context.Context, string) (bool, error)
		Create(context.Context, *models.User) error
	}
	Validator *validator.Validate
}

// NewWithDependencies assembles the onboarding API with injectable services.
func NewWithDependencies(deps Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), corsMiddleware())
	api := router.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			respondError(c, http.StatusUnauthorized, "unauthorized", "", "A valid Clerk session is required", nil)
			c.Abort()
			return
		}
		claims, err := deps.Verifier.VerifySession(c.Request.Context(), parts[1])
		if err != nil || claims == nil || claims.Subject == "" || claims.SessionID == "" {
			respondError(c, http.StatusUnauthorized, "unauthorized", "", "A valid Clerk session is required", nil)
			c.Abort()
			return
		}
		c.Set("clerk_user_id", claims.Subject)
		c.Next()
	})
	api.GET("/me", deps.me)
	api.POST("/onboarding/username/check", deps.checkUsername)
	api.POST("/onboarding/bio/validate", deps.validateBio)
	api.POST("/onboarding/photo/validate", deps.validatePhoto)
	api.POST("/onboarding/complete", deps.complete)
	return router
}

// NewOnboardingValidator registers rules shared by the onboarding steps.
func NewOnboardingValidator() *validator.Validate {
	validate := validator.New()
	usernamePattern := regexp.MustCompile(`^[a-z][a-z0-9_]{2,29}$`)
	if err := validate.RegisterValidation("username", func(field validator.FieldLevel) bool {
		return usernamePattern.MatchString(field.Field().String())
	}); err != nil {
		panic(err)
	}
	if err := validate.RegisterValidation("bio", func(field validator.FieldLevel) bool {
		bio := field.Field().String()
		if utf8.RuneCountInString(bio) > 750 {
			return false
		}
		for _, character := range bio {
			if unicode.IsControl(character) && character != '\n' && character != '\t' {
				return false
			}
		}
		return true
	}); err != nil {
		panic(err)
	}
	if err := validate.RegisterValidation("gender", func(field validator.FieldLevel) bool {
		switch field.Field().String() {
		case "male", "female", "other":
			return true
		default:
			return false
		}
	}); err != nil {
		panic(err)
	}
	return validate
}
