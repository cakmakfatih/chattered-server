package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestOnboardingValidatorRegistersDomainRules(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		tag     string
		wantErr bool
	}{
		{name: "valid username", value: "alice_1", tag: "username", wantErr: false},
		{name: "short username", value: "ab", tag: "username", wantErr: true},
		{name: "uppercase username", value: "Alice", tag: "username", wantErr: true},
		{name: "empty optional bio", value: "", tag: "bio", wantErr: false},
		{name: "multiline bio", value: "Hello!\nMerhaba.", tag: "bio", wantErr: false},
		{name: "Unicode bio at limit", value: strings.Repeat("ğ", 750), tag: "bio", wantErr: false},
		{name: "Unicode bio over limit", value: strings.Repeat("ğ", 751), tag: "bio", wantErr: true},
		{name: "control character in bio", value: "hello\x00world", tag: "bio", wantErr: true},
		{name: "supported gender", value: "other", tag: "gender", wantErr: false},
		{name: "unsupported gender", value: "unknown", tag: "gender", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: construct the validator shared by onboarding endpoints.
			var validate *validator.Validate = NewOnboardingValidator()

			// Act: apply one domain rule to the candidate value.
			err := validate.Var(test.value, test.tag)

			// Assert: the validator enforces the same domain limits as the API.
			if (err != nil) != test.wantErr {
				t.Errorf("validation error = %v, want error %t", err, test.wantErr)
			}
			if test.wantErr {
				var validationErrors validator.ValidationErrors
				if !errors.As(err, &validationErrors) {
					t.Errorf("validation error = %T, want validator.ValidationErrors", err)
				}
			}
		})
	}
}

func TestOnboardingEndpointsUseInjectedValidator(t *testing.T) {
	// Arrange: override the shared username rule to reject an otherwise valid name.
	validate := NewOnboardingValidator()
	if err := validate.RegisterValidation("username", func(validator.FieldLevel) bool { return false }); err != nil {
		t.Fatalf("register username rule: %v", err)
	}
	users := &fakeUserRepository{}
	router := NewWithDependencies(Dependencies{Verifier: validSessionVerifier(), Users: users, Validator: validate})

	// Act: submit the same name through prevalidation and final registration.
	precheck := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/username/check", testAuthorization, map[string]any{"username": "alice_1"})
	completion := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())

	// Assert: both paths use the injected rule and stop before persistence.
	assertAPIError(t, precheck, http.StatusUnprocessableEntity, "invalid_username", "username")
	assertAPIError(t, completion, http.StatusUnprocessableEntity, "invalid_username", "username")
	if len(users.lookedUpNames) != 0 || users.createCalls != 0 {
		t.Errorf("repository calls = lookups %#v, creates %d; want no persistence calls", users.lookedUpNames, users.createCalls)
	}
}
