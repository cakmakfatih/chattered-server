package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestUsernameCheckAcceptsValidNamesAtBoundaries(t *testing.T) {
	names := []string{"abc", "a_2", strings.Repeat("a", 30)}
	for _, username := range names {
		t.Run(username, func(t *testing.T) {
			// Arrange: the candidate username is not in use.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)

			// Act: check the candidate's format and availability.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/username/check", testAuthorization, map[string]any{"username": username})

			// Assert: a valid free username is available and only read operations ran.
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			if got := responseJSON(t, response)["available"]; got != true {
				t.Errorf("available = %v, want true", got)
			}
			if len(users.lookedUpNames) != 1 || users.lookedUpNames[0] != username || users.createCalls != 0 {
				t.Errorf("repository calls = lookups %#v, creates %d; want one lookup and no writes", users.lookedUpNames, users.createCalls)
			}
		})
	}
}

func TestUsernameCheckRejectsInvalidFormatsBeforeLookup(t *testing.T) {
	names := []struct {
		name     string
		username string
	}{
		{name: "empty", username: ""},
		{name: "two characters", username: "ab"},
		{name: "thirty-one characters", username: strings.Repeat("a", 31)},
		{name: "starts with digit", username: "1alice"},
		{name: "starts with underscore", username: "_alice"},
		{name: "uppercase letter", username: "Alice"},
		{name: "space", username: "alice bob"},
		{name: "hyphen", username: "alice-bob"},
		{name: "non-ASCII character", username: "aliçe"},
	}
	for _, test := range names {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare a malformed username and an observable repository.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)

			// Act: submit the username for an availability check.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/username/check", testAuthorization, map[string]any{"username": test.username})

			// Assert: format validation stops before any database lookup or write.
			assertAPIError(t, response, http.StatusUnprocessableEntity, "invalid_username", "username")
			if len(users.lookedUpNames) != 0 || users.createCalls != 0 {
				t.Errorf("invalid username reached repository: lookups=%#v, creates=%d", users.lookedUpNames, users.createCalls)
			}
		})
	}
}

func TestUsernameCheckReportsTakenName(t *testing.T) {
	// Arrange: the candidate username already exists.
	users := &fakeUserRepository{usernameExistsFn: func(context.Context, string) (bool, error) {
		return true, nil
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: check the taken username.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/username/check", testAuthorization, map[string]any{"username": "alice_1"})

	// Assert: expected unavailability is reported without creating a user.
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := responseJSON(t, response)["available"]; got != false {
		t.Errorf("available = %v, want false", got)
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestUsernameCheckReportsRepositoryFailure(t *testing.T) {
	// Arrange: make the availability lookup fail.
	users := &fakeUserRepository{usernameExistsFn: func(context.Context, string) (bool, error) {
		return false, errors.New("database unavailable")
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: check a correctly formatted username.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/username/check", testAuthorization, map[string]any{"username": "alice_1"})

	// Assert: an unknown availability state is not reported as available.
	assertAPIError(t, response, http.StatusInternalServerError, "internal_error", "")
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}
