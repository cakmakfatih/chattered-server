package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestBioValidationTreatsEmptyAndNullAsAbsent(t *testing.T) {
	tests := []struct {
		name string
		bio  any
	}{
		{name: "empty string", bio: ""},
		{name: "null", bio: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: provide an optional biography with no content.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)

			// Act: validate the biography.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/bio/validate", testAuthorization, map[string]any{"bio": test.bio})

			// Assert: the absence of a biography is valid and normalized to null.
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			if got := responseJSON(t, response)["normalized_bio"]; got != nil {
				t.Errorf("normalized_bio = %#v, want null", got)
			}
			if users.createCalls != 0 {
				t.Errorf("create calls = %d, want zero", users.createCalls)
			}
		})
	}
}

func TestBioValidationCountsUnicodeCharacters(t *testing.T) {
	tests := []struct {
		name   string
		bio    string
		status int
	}{
		{name: "exactly 750 ASCII characters", bio: strings.Repeat("a", 750), status: http.StatusOK},
		{name: "exactly 750 Turkish characters", bio: strings.Repeat("ğ", 750), status: http.StatusOK},
		{name: "751 ASCII characters", bio: strings.Repeat("a", 751), status: http.StatusUnprocessableEntity},
		{name: "751 Turkish characters", bio: strings.Repeat("ğ", 751), status: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare a biography at or beyond the 750-character limit.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)

			// Act: validate the biography.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/bio/validate", testAuthorization, map[string]any{"bio": test.bio})

			// Assert: the limit counts Unicode characters rather than UTF-8 bytes.
			if test.status == http.StatusOK {
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
				}
				if got := responseJSON(t, response)["normalized_bio"]; got != test.bio {
					t.Errorf("normalized_bio length/content differs from accepted input")
				}
			} else {
				assertAPIError(t, response, http.StatusUnprocessableEntity, "invalid_bio", "bio")
			}
			if users.createCalls != 0 {
				t.Errorf("create calls = %d, want zero", users.createCalls)
			}
		})
	}
}

func TestBioValidationAllowsPunctuationAndNewlines(t *testing.T) {
	// Arrange: prepare a short, readable, multiline biography.
	bio := "Merhaba!\nCoffee, books & code."
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)

	// Act: validate the biography.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/bio/validate", testAuthorization, map[string]any{"bio": bio})

	// Assert: punctuation and line breaks remain intact and no data is saved.
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := responseJSON(t, response)["normalized_bio"]; got != bio {
		t.Errorf("normalized_bio = %#v, want %#v", got, bio)
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestBioValidationRejectsControlCharacters(t *testing.T) {
	tests := []struct {
		name string
		bio  string
	}{
		{name: "NUL", bio: "hello\x00world"},
		{name: "SOH", bio: "hello\x01world"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare a biography containing a non-printable control character.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)

			// Act: validate the biography.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/bio/validate", testAuthorization, map[string]any{"bio": test.bio})

			// Assert: invalid content is rejected without a write.
			assertAPIError(t, response, http.StatusUnprocessableEntity, "invalid_bio", "bio")
			if users.createCalls != 0 {
				t.Errorf("create calls = %d, want zero", users.createCalls)
			}
		})
	}
}
