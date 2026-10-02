package server

import "apartmanim/server/internal/database/models"

type registrationStatusResponse struct {
	RegistrationComplete bool                 `json:"registration_complete"`
	User                 *userProfileResponse `json:"user"`
}

type userProfileResponse struct {
	Username string  `json:"username"`
	Bio      *string `json:"bio"`
	Gender   string  `json:"gender"`
}

func profileResponse(user *models.User) *userProfileResponse {
	return &userProfileResponse{
		Username: user.Username,
		Bio:      user.Bio,
		Gender:   user.Gender,
	}
}
