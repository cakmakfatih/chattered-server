package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cakmakfatih/chattered-server/internal/database/models"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCompleteRegistrationCreatesUserFromVerifiedSession(t *testing.T) {
	// Arrange: prepare a valid registration and a verified Clerk session.
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)
	payload := validCompletionPayload()

	// Act: complete onboarding.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

	// Assert: exactly one profile is saved with the verified Clerk identity.
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if users.createCalls != 1 || len(users.saved) != 1 {
		t.Fatalf("create calls/saved = %d/%d, want 1/1", users.createCalls, len(users.saved))
	}
	user := users.saved[0]
	if user.ClerkUserID != "clerk-user-1" || user.Username != "alice_1" {
		t.Errorf("saved identity = %q/%q, want clerk-user-1/alice_1", user.ClerkUserID, user.Username)
	}
	if user.Bio == nil || *user.Bio != "Hello from Chattered" {
		t.Errorf("saved bio = %v, want submitted biography", user.Bio)
	}
	if user.ProfileImageKey != nil {
		t.Errorf("profile image key = %v, want nil before upload integration", user.ProfileImageKey)
	}
	assertSavedGender(t, user, "female")
}

func TestCompleteRegistrationAcceptsEveryGenderValue(t *testing.T) {
	for _, gender := range []string{"male", "female", "other"} {
		t.Run(gender, func(t *testing.T) {
			// Arrange: choose one of the supported gender values.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)
			payload := validCompletionPayload()
			payload["gender"] = gender

			// Act: complete the registration.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

			// Assert: the accepted enum value is saved without conversion.
			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
			}
			if len(users.saved) != 1 {
				t.Fatalf("saved users = %d, want one", len(users.saved))
			}
			assertSavedGender(t, users.saved[0], gender)
		})
	}
}

func TestCompleteRegistrationNormalizesEmptyBio(t *testing.T) {
	tests := []struct {
		name string
		bio  any
	}{
		{name: "empty string", bio: ""},
		{name: "null", bio: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare an otherwise valid registration without biography content.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)
			payload := validCompletionPayload()
			payload["bio"] = test.bio

			// Act: complete onboarding.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

			// Assert: an optional empty biography is stored as null.
			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
			}
			if len(users.saved) != 1 || users.saved[0].Bio != nil {
				t.Fatalf("saved bio = %#v, want nil", users.saved)
			}
		})
	}
}

func TestCompleteRegistrationAcceptsBioAtUnicodeLimit(t *testing.T) {
	// Arrange: provide exactly 300 Unicode characters in the final payload.
	bio := strings.Repeat("ğ", 300)
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)
	payload := validCompletionPayload()
	payload["bio"] = bio

	// Act: complete onboarding without a preliminary bio check.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

	// Assert: final validation uses the same inclusive character limit.
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if len(users.saved) != 1 || users.saved[0].Bio == nil || *users.saved[0].Bio != bio {
		t.Fatalf("saved bio = %#v, want the 300-character biography", users.saved)
	}
}

func TestCompleteRegistrationValidatesPhotoMetadataWithoutSavingImage(t *testing.T) {
	// Arrange: include valid photo metadata without photo bytes or Tigris storage.
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)
	payload := validCompletionPayload()
	payload["photo"] = validPhotoMetadata()

	// Act: complete onboarding.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

	// Assert: metadata is accepted but cannot create a stored image key.
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if len(users.saved) != 1 || users.saved[0].ProfileImageKey != nil {
		t.Fatalf("saved profile image key = %#v, want nil", users.saved)
	}
}

func TestCompleteRegistrationRevalidatesEveryInput(t *testing.T) {
	tests := []struct {
		name   string
		change func(map[string]any)
		code   string
		field  string
	}{
		{name: "missing username", change: func(p map[string]any) { delete(p, "username") }, code: "invalid_username", field: "username"},
		{name: "short username", change: func(p map[string]any) { p["username"] = "ab" }, code: "invalid_username", field: "username"},
		{name: "long username", change: func(p map[string]any) { p["username"] = strings.Repeat("a", 16) }, code: "invalid_username", field: "username"},
		{name: "missing gender", change: func(p map[string]any) { delete(p, "gender") }, code: "invalid_gender", field: "gender"},
		{name: "unknown gender", change: func(p map[string]any) { p["gender"] = "unknown" }, code: "invalid_gender", field: "gender"},
		{name: "uppercase gender", change: func(p map[string]any) { p["gender"] = "Female" }, code: "invalid_gender", field: "gender"},
		{name: "long bio", change: func(p map[string]any) { p["bio"] = strings.Repeat("a", 301) }, code: "invalid_bio", field: "bio"},
		{name: "control character in bio", change: func(p map[string]any) { p["bio"] = "hello\x00world" }, code: "invalid_bio", field: "bio"},
		{name: "oversized photo", change: func(p map[string]any) {
			photo := validPhotoMetadata()
			photo["size_bytes"] = maxTestPhotoBytes + 1
			p["photo"] = photo
		}, code: "invalid_photo", field: "photo.size_bytes"},
		{name: "MIME mismatch", change: func(p map[string]any) {
			photo := validPhotoMetadata()
			photo["mime_type"] = "image/png"
			p["photo"] = photo
		}, code: "invalid_photo", field: "photo.mime_type"},
		{name: "unsupported photo extension", change: func(p map[string]any) {
			photo := validPhotoMetadata()
			photo["file_name"] = "avatar.gif"
			photo["mime_type"] = "image/gif"
			p["photo"] = photo
		}, code: "invalid_photo", field: "photo.file_name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: alter one field in an otherwise valid completion request.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)
			payload := validCompletionPayload()
			test.change(payload)

			// Act: submit all onboarding fields together.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

			// Assert: final validation rejects the field and performs no insert.
			assertAPIError(t, response, http.StatusUnprocessableEntity, test.code, test.field)
			if users.createCalls != 0 {
				t.Errorf("create calls = %d, want zero", users.createCalls)
			}
		})
	}
}

func TestCompleteRegistrationRechecksUsernameAfterPrevalidation(t *testing.T) {
	// Arrange: a username becomes taken after a successful preliminary check.
	lookupCount := 0
	users := &fakeUserRepository{usernameExistsFn: func(context.Context, string) (bool, error) {
		lookupCount++
		return lookupCount > 1, nil
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: check the name, then submit the complete registration.
	precheck := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/username/check", testAuthorization, map[string]any{"username": "alice_1"})
	completion := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())

	// Assert: the earlier availability result does not reserve the username.
	if precheck.Code != http.StatusOK || responseJSON(t, precheck)["available"] != true {
		t.Fatalf("precheck = %d/%s, want available", precheck.Code, precheck.Body.String())
	}
	assertAPIError(t, completion, http.StatusConflict, "username_taken", "username")
	if lookupCount < 2 || users.createCalls != 0 {
		t.Errorf("lookups/creates = %d/%d, want at least two lookups and no insert", lookupCount, users.createCalls)
	}
}

func TestCompleteRegistrationMapsUsernameUniqueConstraintRace(t *testing.T) {
	// Arrange: another request wins the username race during insertion.
	users := &fakeUserRepository{createFn: func(context.Context, *models.User) error {
		return &pgconn.PgError{Code: pgerrcode.UniqueViolation, ConstraintName: "ux_users_username"}
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: submit a valid registration.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())

	// Assert: the database constraint is mapped to a username conflict.
	assertAPIError(t, response, http.StatusConflict, "username_taken", "username")
	if users.createCalls != 1 || len(users.saved) != 0 {
		t.Errorf("create calls/saved = %d/%d, want 1/0", users.createCalls, len(users.saved))
	}
}

func TestCompleteRegistrationMapsClerkIDUniqueConstraintRace(t *testing.T) {
	// Arrange: another request creates the same Clerk user during insertion.
	users := &fakeUserRepository{createFn: func(context.Context, *models.User) error {
		return &pgconn.PgError{Code: pgerrcode.UniqueViolation, ConstraintName: "ux_users_clerk_user_id"}
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: submit a valid registration.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())

	// Assert: duplicate application accounts receive a conflict response.
	assertAPIError(t, response, http.StatusConflict, "profile_already_complete", "")
	if users.createCalls != 1 || len(users.saved) != 0 {
		t.Errorf("create calls/saved = %d/%d, want 1/0", users.createCalls, len(users.saved))
	}
}

func TestCompleteRegistrationRejectsExistingClerkUser(t *testing.T) {
	// Arrange: this Clerk user already has a local account.
	users := &fakeUserRepository{getUserFn: func(_ context.Context, clerkID string) (*models.User, error) {
		return &models.User{ClerkUserID: clerkID, Username: "existing_user"}, nil
	}}
	router := testRouter(validSessionVerifier(), users)

	// Act: attempt to complete registration again.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())

	// Assert: no second local user is created.
	assertAPIError(t, response, http.StatusConflict, "profile_already_complete", "")
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestCompleteRegistrationReturnsCompletedProfileForRepeatedSubmission(t *testing.T) {
	// Arrange: the fake repository exposes a successful first insert on later reads.
	users := &fakeUserRepository{}
	users.getUserFn = func(context.Context, string) (*models.User, error) {
		if len(users.saved) == 0 {
			return nil, nil
		}
		return users.saved[0], nil
	}
	router := testRouter(validSessionVerifier(), users)

	// Act: submit the same valid completion request twice.
	first := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())
	second := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())

	// Assert: a lost success response can be retried without creating another account.
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d; body = %s", first.Code, http.StatusCreated, first.Body.String())
	}
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d; body = %s", second.Code, http.StatusOK, second.Body.String())
	}
	if got := responseJSON(t, second)["registration_complete"]; got != true {
		t.Errorf("second registration_complete = %v, want true", got)
	}
	if users.createCalls != 1 || len(users.saved) != 1 {
		t.Errorf("create calls/saved = %d/%d, want 1/1", users.createCalls, len(users.saved))
	}
}

func TestCompleteRegistrationRejectsClientSuppliedClerkID(t *testing.T) {
	// Arrange: a client attempts to choose a different Clerk user ID.
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)
	payload := validCompletionPayload()
	payload["clerk_user_id"] = "another-clerk-user"

	// Act: submit the untrusted identity in the request body.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

	// Assert: the API rejects client-controlled identity and writes nothing.
	assertAPIError(t, response, http.StatusUnprocessableEntity, "unexpected_field", "clerk_user_id")
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestCompleteRegistrationDoesNotSucceedOnRepositoryFailure(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*fakeUserRepository)
		wantWrites int
	}{
		{name: "existing user lookup", configure: func(users *fakeUserRepository) {
			users.getUserFn = func(context.Context, string) (*models.User, error) {
				return nil, errors.New("database unavailable")
			}
		}},
		{name: "username lookup", configure: func(users *fakeUserRepository) {
			users.usernameExistsFn = func(context.Context, string) (bool, error) {
				return false, errors.New("database unavailable")
			}
		}},
		{name: "user insert", configure: func(users *fakeUserRepository) {
			users.createFn = func(context.Context, *models.User) error {
				return errors.New("database unavailable")
			}
		}, wantWrites: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: fail one persistence operation during completion.
			users := &fakeUserRepository{}
			test.configure(users)
			router := testRouter(validSessionVerifier(), users)

			// Act: submit a valid registration request.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, validCompletionPayload())

			// Assert: the API reports failure without claiming to have saved a user.
			assertAPIError(t, response, http.StatusInternalServerError, "internal_error", "")
			if users.createCalls != test.wantWrites || len(users.saved) != 0 {
				t.Errorf("create calls/saved = %d/%d, want %d/0", users.createCalls, len(users.saved), test.wantWrites)
			}
		})
	}
}

func assertSavedGender(t *testing.T, user *models.User, want string) {
	t.Helper()
	field := reflect.ValueOf(user).Elem().FieldByName("Gender")
	if !field.IsValid() {
		t.Fatal("User.Gender is missing from the model")
	}
	if got := fmt.Sprint(field.Interface()); got != want {
		t.Errorf("saved gender = %q, want %q", got, want)
	}
}
