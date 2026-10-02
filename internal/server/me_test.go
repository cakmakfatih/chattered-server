package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"apartmanim/server/internal/database/models"
)

func TestMeReportsIncompleteRegistration(t *testing.T) {
	// Arrange: the authenticated Clerk user has no local user record.
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)

	// Act: request the current user.
	response := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)

	// Assert: the client is told to continue onboarding and no record is written.
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := responseJSON(t, response)["registration_complete"]; got != false {
		t.Errorf("registration_complete = %v, want false", got)
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestMeReportsCompletedRegistration(t *testing.T) {
	// Arrange: the authenticated Clerk user has a local profile.
	users := &fakeUserRepository{getUserFn: func(_ context.Context, clerkID string) (*models.User, error) {
		return &models.User{ClerkUserID: clerkID, Username: "alice_1"}, nil
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: request the current user.
	response := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)

	// Assert: the response identifies a completed profile without writing it again.
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	body := responseJSON(t, response)
	if got := body["registration_complete"]; got != true {
		t.Errorf("registration_complete = %v, want true", got)
	}
	profile, ok := body["user"].(map[string]any)
	if !ok || profile["username"] != "alice_1" {
		t.Errorf("user = %#v, want username alice_1", body["user"])
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestMeDoesNotTreatRepositoryFailureAsMissingProfile(t *testing.T) {
	// Arrange: make the local user lookup fail.
	users := &fakeUserRepository{getUserFn: func(context.Context, string) (*models.User, error) {
		return nil, errors.New("database unavailable")
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: request the current user.
	response := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)

	// Assert: a database error is not reported as an incomplete registration.
	assertAPIError(t, response, http.StatusInternalServerError, "internal_error", "")
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}
