package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cakmakfatih/chattered-server/internal/database/models"
)

type completionInput struct {
	username string
	gender   string
	bio      *string
}

func (deps Dependencies) parseCompletionInput(fields map[string]json.RawMessage) (completionInput, *apiError) {
	username, ok := decodeString(fields["username"])
	if !ok {
		return completionInput{}, invalidUsername()
	}
	if apiErr := deps.usernameValidationError(username); apiErr != nil {
		return completionInput{}, apiErr
	}

	gender, ok := decodeString(fields["gender"])
	if !ok {
		return completionInput{}, invalidGender()
	}
	if apiErr := deps.genderValidationError(gender); apiErr != nil {
		return completionInput{}, apiErr
	}

	bio, ok := decodeOptionalString(fields["bio"])
	if !ok {
		return completionInput{}, invalidBiography()
	}
	if apiErr := deps.biographyValidationError(bio); apiErr != nil {
		return completionInput{}, apiErr
	}
	if apiErr := deps.validatePhotoMetadata(fields["photo"]); apiErr != nil {
		return completionInput{}, apiErr
	}

	return completionInput{username: username, gender: gender, bio: bio}, nil
}

func (deps Dependencies) createProfile(ctx context.Context, clerkID string, input completionInput) (*models.User, int, *apiError) {
	existing, err := deps.Users.GetByClerkID(ctx, clerkID)
	if err != nil {
		return nil, http.StatusInternalServerError, &apiError{code: "internal_error", message: "Could not check registration status"}
	}
	if existing != nil {
		return nil, http.StatusConflict, profileAlreadyExists()
	}

	available, apiErr := deps.usernameAvailability(ctx, input.username)
	if apiErr != nil {
		return nil, http.StatusInternalServerError, apiErr
	}
	if !available {
		return nil, http.StatusConflict, usernameAlreadyInUse()
	}

	user := &models.User{
		ClerkUserID: clerkID,
		Username:    input.username,
		Gender:      input.gender,
		Bio:         input.bio,
	}
	if err := deps.Users.Create(ctx, user); err != nil {
		status, apiErr := userCreationError(err)
		return nil, status, &apiErr
	}
	return user, http.StatusCreated, nil
}

func invalidUsername() *apiError {
	message := fmt.Sprintf("Username must be 3-%d characters, start with a lowercase letter, and use only lowercase letters, numbers, or underscores", maxUsernameCharacters)
	return &apiError{code: "invalid_username", field: "username", message: message}
}

func invalidGender() *apiError {
	return &apiError{code: "invalid_gender", field: "gender", message: "Invalid gender"}
}

func invalidBiography() *apiError {
	message := fmt.Sprintf("Biography must be valid and no longer than %d characters", maxBioCharacters)
	return &apiError{code: "invalid_bio", field: "bio", message: message}
}

func profileAlreadyExists() *apiError {
	return &apiError{code: "profile_already_complete", message: "Profile already exists"}
}

func usernameAlreadyInUse() *apiError {
	return &apiError{code: "username_taken", field: "username", message: "Username is already in use"}
}
