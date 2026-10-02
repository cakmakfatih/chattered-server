package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cakmakfatih/chattered-server/internal/database/models"
	"github.com/clerk/clerk-sdk-go/v2"
)

const testAuthorization = "Bearer test-session-token"

type fakeSessionVerifier struct {
	claims *clerk.SessionClaims
	err    error
	calls  int
	tokens []string
}

func validSessionVerifier() *fakeSessionVerifier {
	return &fakeSessionVerifier{claims: &clerk.SessionClaims{
		RegisteredClaims: clerk.RegisteredClaims{Subject: "clerk-user-1"},
		Claims:           clerk.Claims{SessionID: "clerk-session-1"},
	}}
}

func (f *fakeSessionVerifier) VerifySession(_ context.Context, token string) (*clerk.SessionClaims, error) {
	f.calls++
	f.tokens = append(f.tokens, token)
	return f.claims, f.err
}

type fakeUserRepository struct {
	getUserFn        func(context.Context, string) (*models.User, error)
	usernameExistsFn func(context.Context, string) (bool, error)
	createFn         func(context.Context, *models.User) error
	getUserIDs       []string
	lookedUpNames    []string
	createCalls      int
	saved            []*models.User
}

func (f *fakeUserRepository) GetByClerkID(ctx context.Context, clerkID string) (*models.User, error) {
	f.getUserIDs = append(f.getUserIDs, clerkID)
	if f.getUserFn != nil {
		return f.getUserFn(ctx, clerkID)
	}
	return nil, nil
}

func (f *fakeUserRepository) UsernameExists(ctx context.Context, username string) (bool, error) {
	f.lookedUpNames = append(f.lookedUpNames, username)
	if f.usernameExistsFn != nil {
		return f.usernameExistsFn(ctx, username)
	}
	return false, nil
}

func (f *fakeUserRepository) Create(ctx context.Context, user *models.User) error {
	f.createCalls++
	if f.createFn != nil {
		if err := f.createFn(ctx, user); err != nil {
			return err
		}
	}
	copyOfUser := *user
	f.saved = append(f.saved, &copyOfUser)
	return nil
}

func testRouter(verifier *fakeSessionVerifier, users *fakeUserRepository) http.Handler {
	return NewWithDependencies(Dependencies{Verifier: verifier, Users: users, Validator: NewOnboardingValidator()})
}

func requestJSON(t *testing.T, handler http.Handler, method, path, authorization string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		body = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, body)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func responseJSON(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode JSON response %q: %v", response.Body.String(), err)
	}
	return body
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code, field string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	body := responseJSON(t, response)
	detail, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error response = %#v, want an error object", body)
	}
	if got := detail["code"]; got != code {
		t.Errorf("error code = %v, want %q", got, code)
	}
	if message, ok := detail["message"].(string); !ok || message == "" {
		t.Errorf("error message = %v, want a non-empty explanation", detail["message"])
	}
	if field != "" && detail["field"] != field {
		t.Errorf("error field = %v, want %q", detail["field"], field)
	}
}

func validPhotoMetadata() map[string]any {
	return map[string]any{
		"file_name":  "avatar.jpg",
		"mime_type":  "image/jpeg",
		"size_bytes": 1024,
	}
}

func validCompletionPayload() map[string]any {
	return map[string]any{
		"username": "alice_1",
		"gender":   "female",
		"bio":      "Hello from Chattered",
		"photo":    nil,
	}
}
