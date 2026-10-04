package server

import "github.com/cakmakfatih/chattered-server/internal/database/models"

type registrationStatusResponse struct {
	RegistrationComplete bool                 `json:"registration_complete"`
	User                 *userProfileResponse `json:"user"`
}

type userProfileResponse struct {
	Username string  `json:"username"`
	Bio      *string `json:"bio"`
	Gender   string  `json:"gender"`
}

type profilePhotoUploadAuthorizationResponse struct {
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	ExpiresAt string            `json:"expires_at"`
	Headers   map[string]string `json:"headers"`
}

func profileResponse(user *models.User) *userProfileResponse {
	return &userProfileResponse{
		Username: user.Username,
		Bio:      user.Bio,
		Gender:   user.Gender,
	}
}
