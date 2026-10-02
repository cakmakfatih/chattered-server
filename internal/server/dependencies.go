package server

import (
	"context"
	"regexp"
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

// NewWithDependencies is the injection point for the planned onboarding routes.
func NewWithDependencies(_ Dependencies) *gin.Engine {
	return New()
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
