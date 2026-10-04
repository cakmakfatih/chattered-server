package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/clerk/clerk-sdk-go/v2"
)

func TestAuthGuardRequiresSessionOnEveryNewEndpoint(t *testing.T) {
	routes := []struct {
		name   string
		method string
		path   string
	}{
		{name: "registration status", method: http.MethodGet, path: "/api/v1/me"},
		{name: "username check", method: http.MethodPost, path: "/api/v1/onboarding/username/check"},
		{name: "bio validation", method: http.MethodPost, path: "/api/v1/onboarding/bio/validate"},
		{name: "photo validation", method: http.MethodPost, path: "/api/v1/onboarding/photo/validate"},
		{name: "photo upload authorization", method: http.MethodPut, path: "/api/v1/me/profile-photo"},
		{name: "registration completion", method: http.MethodPost, path: "/api/v1/onboarding/complete"},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			// Arrange: build the API without a session token.
			verifier := validSessionVerifier()
			users := &fakeUserRepository{}
			router := testRouter(verifier, users)

			// Act: call the protected endpoint.
			response := requestJSON(t, router, route.method, route.path, "", map[string]any{})

			// Assert: authentication stops the request before business logic runs.
			assertAPIError(t, response, http.StatusUnauthorized, "unauthorized", "")
			if verifier.calls != 0 || len(users.getUserIDs) != 0 || len(users.lookedUpNames) != 0 || users.createCalls != 0 {
				t.Fatalf("unauthenticated request reached dependencies: verifier=%d, users=%#v", verifier.calls, users)
			}
		})
	}
}

func TestAuthGuardRejectsMalformedAuthorization(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
	}{
		{name: "wrong scheme", authorization: "Basic credentials"},
		{name: "empty bearer token", authorization: "Bearer "},
		{name: "missing bearer prefix", authorization: "test-session-token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: provide an authorization header that is not a bearer token.
			verifier := validSessionVerifier()
			users := &fakeUserRepository{}
			router := testRouter(verifier, users)

			// Act: request the authenticated identity.
			response := requestJSON(t, router, http.MethodGet, "/api/v1/me", test.authorization, nil)

			// Assert: malformed credentials never reach the token verifier or repository.
			assertAPIError(t, response, http.StatusUnauthorized, "unauthorized", "")
			if verifier.calls != 0 || len(users.getUserIDs) != 0 {
				t.Fatalf("malformed authorization reached dependencies: verifier=%d, lookups=%d", verifier.calls, len(users.getUserIDs))
			}
		})
	}
}

func TestAuthGuardRejectsVerifierFailures(t *testing.T) {
	reasons := []string{
		"malformed JWT",
		"invalid signature",
		"expired token",
		"token not valid yet",
		"untrusted Clerk signing key",
		"verification key unavailable",
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			// Arrange: make the verifier report this token failure.
			verifier := validSessionVerifier()
			verifier.err = errors.New(reason)
			users := &fakeUserRepository{}
			router := testRouter(verifier, users)

			// Act: send a bearer token to a protected endpoint.
			response := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)

			// Assert: verification fails closed and never leaks credentials or internal errors.
			assertAPIError(t, response, http.StatusUnauthorized, "unauthorized", "")
			if verifier.calls != 1 || len(users.getUserIDs) != 0 {
				t.Fatalf("verification failure reached business logic: verifier=%d, lookups=%d", verifier.calls, len(users.getUserIDs))
			}
			if strings.Contains(response.Body.String(), "test-session-token") || strings.Contains(response.Body.String(), reason) {
				t.Fatalf("authentication error leaked sensitive details: %s", response.Body.String())
			}
		})
	}
}

func TestAuthGuardRejectsIncompleteSessionClaims(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		sessionID string
	}{
		{name: "missing user ID", sessionID: "clerk-session-1"},
		{name: "missing session ID", userID: "clerk-user-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: return signed claims without a complete user session identity.
			verifier := &fakeSessionVerifier{claims: &clerk.SessionClaims{
				RegisteredClaims: clerk.RegisteredClaims{Subject: test.userID},
				Claims:           clerk.Claims{SessionID: test.sessionID},
			}}
			users := &fakeUserRepository{}
			router := testRouter(verifier, users)

			// Act: request the current user.
			response := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)

			// Assert: the API requires both the Clerk user and session IDs.
			assertAPIError(t, response, http.StatusUnauthorized, "unauthorized", "")
			if len(users.getUserIDs) != 0 {
				t.Fatalf("repository was called with incomplete claims: %#v", users.getUserIDs)
			}
		})
	}
}

func TestAuthGuardUsesVerifiedTokenIdentity(t *testing.T) {
	// Arrange: give the verifier a trusted Clerk user identity.
	verifier := validSessionVerifier()
	users := &fakeUserRepository{}
	router := testRouter(verifier, users)

	// Act: request the current user's registration status.
	response := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)

	// Assert: the bearer token is verified and the repository receives its subject.
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if verifier.calls != 1 || len(verifier.tokens) != 1 || verifier.tokens[0] != "test-session-token" {
		t.Fatalf("verifier calls/tokens = %d/%#v, want one session token", verifier.calls, verifier.tokens)
	}
	if len(users.getUserIDs) != 1 || users.getUserIDs[0] != "clerk-user-1" {
		t.Fatalf("repository Clerk IDs = %#v, want verified subject", users.getUserIDs)
	}
}

func TestAuthenticatedUserWithoutLocalProfileCanUseOnboarding(t *testing.T) {
	// Arrange: authenticate a Clerk user who has no application profile.
	verifier := validSessionVerifier()
	users := &fakeUserRepository{}
	router := testRouter(verifier, users)

	// Act: request profile status and check an onboarding username.
	status := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)
	username := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/username/check", testAuthorization, map[string]any{"username": "alice_1"})

	// Assert: a local user record is not required to reach onboarding.
	if status.Code != http.StatusOK || username.Code != http.StatusOK {
		t.Fatalf("status codes = %d/%d, want 200/200", status.Code, username.Code)
	}
	if got := responseJSON(t, status)["registration_complete"]; got != false {
		t.Errorf("registration_complete = %v, want false", got)
	}
}

func TestCORSPreflightDoesNotRequireSession(t *testing.T) {
	// Arrange: prepare a preflight request to an authenticated POST endpoint.
	verifier := validSessionVerifier()
	users := &fakeUserRepository{}
	router := testRouter(verifier, users)

	// Act: send OPTIONS without a bearer token.
	response := requestJSON(t, router, http.MethodOptions, "/api/v1/onboarding/complete", "", nil)

	// Assert: preflight succeeds without running authentication or business logic.
	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", response.Code, http.StatusNoContent)
	}
	allowedMethods := response.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{"POST", "PUT"} {
		if !strings.Contains(allowedMethods, method) {
			t.Errorf("allowed methods = %q, want %s", allowedMethods, method)
		}
	}
	if verifier.calls != 0 || users.createCalls != 0 {
		t.Fatalf("preflight reached protected dependencies: verifier=%d, creates=%d", verifier.calls, users.createCalls)
	}
}

func TestHelloExampleRouteIsRemoved(t *testing.T) {
	// Arrange: build the production API router.
	router := testRouter(validSessionVerifier(), &fakeUserRepository{})

	// Act: request the former example route.
	response := requestJSON(t, router, http.MethodGet, "/api/hello", testAuthorization, nil)

	// Assert: the example endpoint is not part of the production API.
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNotFound, response.Body.String())
	}
}
