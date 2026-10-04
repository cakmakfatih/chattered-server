package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (deps Dependencies) me(c *gin.Context) {
	user, err := deps.Users.GetByClerkID(c.Request.Context(), c.GetString("clerk_user_id"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, apiError{code: "internal_error", message: "Could not load the user profile"})
		return
	}
	if user == nil {
		c.JSON(http.StatusOK, registrationStatusResponse{})
		return
	}
	c.JSON(http.StatusOK, registrationStatusResponse{
		RegistrationComplete: true,
		User:                 profileResponse(user),
	})
}

func (deps Dependencies) checkUsername(c *gin.Context) {
	fields, ok := readRequestObject(c, "username")
	if !ok {
		return
	}
	username, ok := decodeString(fields["username"])
	if !ok {
		respondError(c, http.StatusUnprocessableEntity, *invalidUsername())
		return
	}
	if validationErr := deps.usernameValidationError(username); validationErr != nil {
		respondError(c, http.StatusUnprocessableEntity, *validationErr)
		return
	}
	available, availabilityErr := deps.usernameAvailability(c.Request.Context(), username)
	if availabilityErr != nil {
		respondError(c, http.StatusInternalServerError, *availabilityErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": available})
}

func (deps Dependencies) validateBio(c *gin.Context) {
	fields, ok := readRequestObject(c, "bio")
	if !ok {
		return
	}
	bio, ok := decodeOptionalString(fields["bio"])
	if !ok {
		respondError(c, http.StatusUnprocessableEntity, *invalidBiography())
		return
	}
	if validationErr := deps.biographyValidationError(bio); validationErr != nil {
		respondError(c, http.StatusUnprocessableEntity, *validationErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true, "normalized_bio": bio})
}

func (deps Dependencies) validatePhoto(c *gin.Context) {
	fields, ok := readRequestObject(c, "photo")
	if !ok {
		return
	}
	if photoErr := deps.validatePhotoMetadata(fields["photo"]); photoErr != nil {
		respondError(c, http.StatusUnprocessableEntity, *photoErr)
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true})
}

func (deps Dependencies) authorizeProfilePhotoUpload(c *gin.Context) {
	fields, ok := readRequestObject(c, "photo")
	if !ok {
		return
	}
	if isAbsentJSON(fields["photo"]) {
		respondError(c, http.StatusUnprocessableEntity, *invalidPhoto("photo", "Photo metadata is required"))
		return
	}
	if photoErr := deps.validatePhotoMetadata(fields["photo"]); photoErr != nil {
		respondError(c, http.StatusUnprocessableEntity, *photoErr)
		return
	}
	if deps.ProfilePhotos == nil {
		respondError(c, http.StatusInternalServerError, apiError{code: "internal_error", message: "Could not authorize the profile photo upload"})
		return
	}

	var photo photoMetadata
	if err := json.Unmarshal(fields["photo"], &photo); err != nil {
		respondError(c, http.StatusUnprocessableEntity, *invalidPhoto("photo", "Invalid photo metadata"))
		return
	}
	clerkID := c.GetString("clerk_user_id")
	user, err := deps.Users.GetByClerkID(c.Request.Context(), clerkID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, apiError{code: "internal_error", message: "Could not check registration status"})
		return
	}
	if user != nil {
		respondError(c, http.StatusConflict, *profileAlreadyExists())
		return
	}

	objectKey := profilePhotoObjectKeyForUser(clerkID)
	uploadURL, expiresAt, err := deps.ProfilePhotos.AuthorizeUpload(c.Request.Context(), objectKey, photo.MIMEType, photo.SizeBytes)
	if err != nil {
		respondError(c, http.StatusInternalServerError, apiError{code: "internal_error", message: "Could not authorize the profile photo upload"})
		return
	}
	c.JSON(http.StatusOK, profilePhotoUploadAuthorizationResponse{
		UploadURL: uploadURL,
		Method:    http.MethodPut,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		Headers: map[string]string{
			"Content-Type":   photo.MIMEType,
			"Content-Length": strconv.FormatInt(photo.SizeBytes, 10),
		},
	})
}

func (deps Dependencies) complete(c *gin.Context) {
	fields, ok := readRequestObject(c, "username", "gender", "bio", "photo")
	if !ok {
		return
	}
	input, validationErr := deps.parseCompletionInput(fields)
	if validationErr != nil {
		respondError(c, http.StatusUnprocessableEntity, *validationErr)
		return
	}
	clerkID := c.GetString("clerk_user_id")
	existing, lookupErr := deps.Users.GetByClerkID(c.Request.Context(), clerkID)
	if lookupErr != nil {
		respondError(c, http.StatusInternalServerError, apiError{code: "internal_error", message: "Could not check registration status"})
		return
	}
	if existing != nil {
		if completionMatchesProfile(existing, input) {
			c.JSON(http.StatusOK, registrationStatusResponse{RegistrationComplete: true, User: profileResponse(existing)})
			return
		}
		respondError(c, http.StatusConflict, *profileAlreadyExists())
		return
	}
	if input.photo != nil {
		photoErr, status := deps.verifyProfilePhoto(c.Request.Context(), clerkID, input.photo)
		if photoErr != nil {
			respondError(c, status, *photoErr)
			return
		}
	}
	user, status, createErr := deps.createProfile(c.Request.Context(), clerkID, input)
	if createErr != nil {
		respondError(c, status, *createErr)
		return
	}
	c.JSON(http.StatusCreated, registrationStatusResponse{
		RegistrationComplete: true,
		User:                 profileResponse(user),
	})
}
