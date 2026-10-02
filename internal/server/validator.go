package server

import (
	"context"
	"fmt"
	"regexp"
	"unicode"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
)

const maxBioCharacters = 300
const maxUsernameCharacters = 15

var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,14}$`)

// NewOnboardingValidator registers the rules shared by onboarding endpoints.
func NewOnboardingValidator() *validator.Validate {
	validate := validator.New()
	registerValidation(validate, "username", usernameValidationRule)
	registerValidation(validate, "bio", biographyValidationRule)
	registerValidation(validate, "gender", genderValidationRule)
	return validate
}

func registerValidation(validate *validator.Validate, tag string, rule validator.Func) {
	if err := validate.RegisterValidation(tag, rule); err != nil {
		panic(fmt.Errorf("register %s validation: %w", tag, err))
	}
}

func isValidUsername(username string) bool {
	return usernamePattern.MatchString(username)
}

func usernameValidationRule(field validator.FieldLevel) bool {
	return isValidUsername(field.Field().String())
}

func isValidBio(bio string) bool {
	if utf8.RuneCountInString(bio) > maxBioCharacters {
		return false
	}

	for _, character := range bio {
		if isDisallowedControlCharacter(character) {
			return false
		}
	}
	return true
}

func biographyValidationRule(field validator.FieldLevel) bool {
	return isValidBio(field.Field().String())
}

func isDisallowedControlCharacter(character rune) bool {
	return unicode.IsControl(character) && character != '\n' && character != '\t'
}

func isValidGender(gender string) bool {
	switch gender {
	case "male", "female", "other":
		return true
	default:
		return false
	}
}

func genderValidationRule(field validator.FieldLevel) bool {
	return isValidGender(field.Field().String())
}

func (deps Dependencies) usernameValidationError(username string) *apiError {
	if deps.Validator.Var(username, "username") == nil {
		return nil
	}
	return invalidUsername()
}

func (deps Dependencies) biographyValidationError(bio *string) *apiError {
	if bio == nil || deps.Validator.Var(*bio, "bio") == nil {
		return nil
	}
	return invalidBiography()
}

func (deps Dependencies) genderValidationError(gender string) *apiError {
	if deps.Validator.Var(gender, "gender") == nil {
		return nil
	}
	return invalidGender()
}

func (deps Dependencies) usernameAvailability(ctx context.Context, username string) (bool, *apiError) {
	exists, err := deps.Users.UsernameExists(ctx, username)
	if err != nil {
		return false, &apiError{code: "internal_error", message: "Could not check username availability"}
	}
	return !exists, nil
}
