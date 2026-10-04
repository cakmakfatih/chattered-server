package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cakmakfatih/chattered-server/internal/database/models"
)

const profilePhotoObjectKey = "users/clerk-user-1/profile/avatar"
const profilePhotoAuthorizationPath = "/api/v1/me/profile-photo"

type profilePhotoAuthorizationCall struct {
	objectKey string
	mimeType  string
	sizeBytes int64
}

type fakeProfilePhotoStore struct {
	mu             sync.Mutex
	authorizeFn    func(context.Context, string, string, int64) (string, time.Time, error)
	openObjectFn   func(context.Context, string) (io.ReadCloser, string, int64, error)
	authorizeCalls []profilePhotoAuthorizationCall
	openedKeys     []string
}

func (f *fakeProfilePhotoStore) AuthorizeUpload(ctx context.Context, objectKey, mimeType string, sizeBytes int64) (string, time.Time, error) {
	f.mu.Lock()
	f.authorizeCalls = append(f.authorizeCalls, profilePhotoAuthorizationCall{
		objectKey: objectKey,
		mimeType:  mimeType,
		sizeBytes: sizeBytes,
	})
	f.mu.Unlock()
	if f.authorizeFn != nil {
		return f.authorizeFn(ctx, objectKey, mimeType, sizeBytes)
	}
	return "https://uploads.example.test/profile-photo", time.Now().Add(10 * time.Minute), nil
}

func (f *fakeProfilePhotoStore) OpenObject(ctx context.Context, objectKey string) (io.ReadCloser, string, int64, error) {
	f.mu.Lock()
	f.openedKeys = append(f.openedKeys, objectKey)
	f.mu.Unlock()
	if f.openObjectFn != nil {
		return f.openObjectFn(ctx, objectKey)
	}
	return nil, "", 0, fs.ErrNotExist
}

func TestAuthorizeProfilePhotoUploadReturnsConstrainedDirectUpload(t *testing.T) {
	// Arrange: prepare valid metadata and a private storage authorization.
	expiresAt := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	photos := &fakeProfilePhotoStore{authorizeFn: func(context.Context, string, string, int64) (string, time.Time, error) {
		return "https://uploads.example.test/profile-photo?signature=opaque", expiresAt, nil
	}}
	users := &fakeUserRepository{}
	router := testRouterWithProfilePhotos(t, validSessionVerifier(), users, photos)

	// Act: request permission to upload directly to Tigris.
	response := requestJSON(t, router, http.MethodPut, profilePhotoAuthorizationPath, testAuthorization, map[string]any{
		"photo": validPhotoMetadata(),
	})

	// Assert: the client receives only the constrained PUT instructions it needs.
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	body := responseJSON(t, response)
	if body["upload_url"] != "https://uploads.example.test/profile-photo?signature=opaque" {
		t.Errorf("upload_url = %v, want the signed storage URL", body["upload_url"])
	}
	if body["method"] != http.MethodPut {
		t.Errorf("method = %v, want PUT", body["method"])
	}
	if body["expires_at"] != expiresAt.Format(time.RFC3339) {
		t.Errorf("expires_at = %v, want %s", body["expires_at"], expiresAt.Format(time.RFC3339))
	}
	headers, ok := body["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %#v, want an object", body["headers"])
	}
	if headers["Content-Type"] != "image/jpeg" || headers["Content-Length"] != "1024" {
		t.Errorf("headers = %#v, want signed content type and length", headers)
	}
	for _, forbidden := range []string{"bucket", "object_key", "access_key", "secret_key"} {
		if _, exposed := body[forbidden]; exposed {
			t.Errorf("response exposes %q: %#v", forbidden, body[forbidden])
		}
	}
	if len(photos.authorizeCalls) != 1 {
		t.Fatalf("authorization calls = %d, want one", len(photos.authorizeCalls))
	}
	call := photos.authorizeCalls[0]
	if call.objectKey != profilePhotoObjectKey || call.mimeType != "image/jpeg" || call.sizeBytes != 1024 {
		t.Errorf("authorization call = %#v, want server key, image/jpeg, and 1024 bytes", call)
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero before completion", users.createCalls)
	}
}

func TestProfilePhotoAuthorizationIsNotAnOnboardingRoute(t *testing.T) {
	// Arrange: configure the API with a usable upload store.
	photos := &fakeProfilePhotoStore{}
	router := testRouterWithProfilePhotos(t, validSessionVerifier(), &fakeUserRepository{}, photos)

	// Act: call the former onboarding upload route.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/photo/upload", testAuthorization, map[string]any{
		"photo": validPhotoMetadata(),
	})

	// Assert: upload authorization is only available on the authenticated user resource.
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if len(photos.authorizeCalls) != 0 {
		t.Errorf("authorization calls = %d, want zero", len(photos.authorizeCalls))
	}
}

func TestAuthorizeProfilePhotoUploadAcceptsSupportedFormats(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		mimeType string
	}{
		{name: "JPEG", fileName: "avatar.jpg", mimeType: "image/jpeg"},
		{name: "PNG", fileName: "avatar.png", mimeType: "image/png"},
		{name: "HEIC", fileName: "avatar.heic", mimeType: "image/heic"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: select one supported profile-photo format.
			photos := &fakeProfilePhotoStore{}
			router := testRouterWithProfilePhotos(t, validSessionVerifier(), &fakeUserRepository{}, photos)
			photo := map[string]any{"file_name": test.fileName, "mime_type": test.mimeType, "size_bytes": 2048}

			// Act: request a direct-upload authorization.
			response := requestJSON(t, router, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": photo})

			// Assert: the signed upload is constrained to the submitted format and length.
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			if len(photos.authorizeCalls) != 1 {
				t.Fatalf("authorization calls = %d, want one", len(photos.authorizeCalls))
			}
			call := photos.authorizeCalls[0]
			if call.mimeType != test.mimeType || call.sizeBytes != 2048 {
				t.Errorf("authorization call = %#v, want %s and 2048 bytes", call, test.mimeType)
			}
		})
	}
}

func TestAuthorizeProfilePhotoUploadRevalidatesMetadata(t *testing.T) {
	tests := []struct {
		name  string
		photo any
		field string
	}{
		{name: "missing photo", photo: nil, field: "photo"},
		{name: "empty photo", photo: map[string]any{}, field: "photo.file_name"},
		{name: "unsupported extension", photo: map[string]any{"file_name": "avatar.gif", "mime_type": "image/gif", "size_bytes": 1024}, field: "photo.file_name"},
		{name: "MIME mismatch", photo: map[string]any{"file_name": "avatar.jpg", "mime_type": "image/png", "size_bytes": 1024}, field: "photo.mime_type"},
		{name: "empty file", photo: map[string]any{"file_name": "avatar.jpg", "mime_type": "image/jpeg", "size_bytes": 0}, field: "photo.size_bytes"},
		{name: "oversized file", photo: map[string]any{"file_name": "avatar.jpg", "mime_type": "image/jpeg", "size_bytes": maxTestPhotoBytes + 1}, field: "photo.size_bytes"},
		{name: "path as filename", photo: map[string]any{"file_name": "../avatar.jpg", "mime_type": "image/jpeg", "size_bytes": 1024}, field: "photo.file_name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare metadata that must not authorize storage access.
			photos := &fakeProfilePhotoStore{}
			router := testRouterWithProfilePhotos(t, validSessionVerifier(), &fakeUserRepository{}, photos)

			// Act: bypass preliminary validation and request an upload directly.
			response := requestJSON(t, router, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": test.photo})

			// Assert: the upload endpoint independently rejects the metadata.
			assertAPIError(t, response, http.StatusUnprocessableEntity, "invalid_photo", test.field)
			if len(photos.authorizeCalls) != 0 {
				t.Errorf("authorization calls = %d, want zero", len(photos.authorizeCalls))
			}
		})
	}
}

func TestAuthorizeProfilePhotoUploadRejectsStorageControls(t *testing.T) {
	for _, field := range []string{"bucket", "object_key", "acl"} {
		t.Run(field, func(t *testing.T) {
			// Arrange: a client attempts to control a server-owned storage setting.
			photos := &fakeProfilePhotoStore{}
			router := testRouterWithProfilePhotos(t, validSessionVerifier(), &fakeUserRepository{}, photos)
			payload := map[string]any{"photo": validPhotoMetadata(), field: "client-controlled"}

			// Act: request an upload with the untrusted setting.
			response := requestJSON(t, router, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, payload)

			// Assert: storage location and access policy cannot be selected by the client.
			assertAPIError(t, response, http.StatusUnprocessableEntity, "unexpected_field", field)
			if len(photos.authorizeCalls) != 0 {
				t.Errorf("authorization calls = %d, want zero", len(photos.authorizeCalls))
			}
		})
	}
}

func TestAuthorizeProfilePhotoUploadReusesOneServerOwnedObjectKey(t *testing.T) {
	// Arrange: prepare two requests from one authenticated user and one from another.
	photos := &fakeProfilePhotoStore{}
	users := &fakeUserRepository{}
	aliceRouter := testRouterWithProfilePhotos(t, validSessionVerifier(), users, photos)
	bobVerifier := validSessionVerifier()
	bobVerifier.claims.RegisteredClaims.Subject = "clerk-user-2"
	bobRouter := testRouterWithProfilePhotos(t, bobVerifier, users, photos)

	// Act: authorize a retry for Alice and an independent upload for Bob.
	first := requestJSON(t, aliceRouter, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": validPhotoMetadata()})
	retryPhoto := map[string]any{"file_name": "replacement.png", "mime_type": "image/png", "size_bytes": 2048}
	retry := requestJSON(t, aliceRouter, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": retryPhoto})
	otherUser := requestJSON(t, bobRouter, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": validPhotoMetadata()})

	// Assert: retries overwrite one stable location while different users remain isolated.
	responses := []struct {
		name     string
		response *httptest.ResponseRecorder
	}{
		{name: "first", response: first},
		{name: "retry", response: retry},
		{name: "other user", response: otherUser},
	}
	for _, result := range responses {
		name, response := result.name, result.response
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body = %s", name, response.Code, http.StatusOK, response.Body.String())
		}
	}
	if len(photos.authorizeCalls) != 3 {
		t.Fatalf("authorization calls = %d, want three", len(photos.authorizeCalls))
	}
	if photos.authorizeCalls[0].objectKey != photos.authorizeCalls[1].objectKey {
		t.Errorf("retry keys = %q/%q, want one stable key", photos.authorizeCalls[0].objectKey, photos.authorizeCalls[1].objectKey)
	}
	if photos.authorizeCalls[0].objectKey == photos.authorizeCalls[2].objectKey {
		t.Errorf("different users share object key %q", photos.authorizeCalls[0].objectKey)
	}
	for _, call := range photos.authorizeCalls {
		if strings.Contains(call.objectKey, "avatar.jpg") {
			t.Errorf("object key contains client filename: %q", call.objectKey)
		}
	}
}

func TestConcurrentProfilePhotoUploadAuthorizationsUseOneObjectKey(t *testing.T) {
	// Arrange: prepare several simultaneous authorizations for the same Clerk user.
	const requestCount = 16
	photos := &fakeProfilePhotoStore{}
	handlers := make([]http.Handler, requestCount)
	bodies := make([][]byte, requestCount)
	for index := 0; index < requestCount; index++ {
		handlers[index] = testRouterWithProfilePhotos(t, validSessionVerifier(), &fakeUserRepository{}, photos)
		photo := map[string]any{
			"file_name":  fmt.Sprintf("avatar-%d.jpg", index),
			"mime_type":  "image/jpeg",
			"size_bytes": 1024 + index,
		}
		var err error
		bodies[index], err = json.Marshal(map[string]any{"photo": photo})
		if err != nil {
			t.Fatalf("encode request %d: %v", index, err)
		}
	}
	responses := make(chan *httptest.ResponseRecorder, requestCount)
	var requests sync.WaitGroup
	requests.Add(requestCount)

	// Act: issue concurrent upload requests with different valid filenames.
	for index := 0; index < requestCount; index++ {
		go func(index int) {
			defer requests.Done()
			request := httptest.NewRequest(http.MethodPut, "/api/v1/me/profile-photo", bytes.NewReader(bodies[index]))
			request.Header.Set("Authorization", testAuthorization)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handlers[index].ServeHTTP(response, request)
			responses <- response
		}(index)
	}
	requests.Wait()
	close(responses)

	// Assert: concurrency can refresh authorization but cannot allocate more object locations.
	for response := range responses {
		if response.Code != http.StatusOK {
			t.Errorf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
		}
	}
	photos.mu.Lock()
	defer photos.mu.Unlock()
	if len(photos.authorizeCalls) != requestCount {
		t.Fatalf("authorization calls = %d, want %d", len(photos.authorizeCalls), requestCount)
	}
	for _, call := range photos.authorizeCalls {
		if call.objectKey != profilePhotoObjectKey {
			t.Errorf("concurrent object key = %q, want %q", call.objectKey, profilePhotoObjectKey)
		}
	}
}

func TestAuthorizeProfilePhotoUploadRejectsCompletedProfile(t *testing.T) {
	// Arrange: the authenticated user already has a completed local profile.
	users := &fakeUserRepository{getUserFn: func(_ context.Context, clerkID string) (*models.User, error) {
		return &models.User{ClerkUserID: clerkID, Username: "alice_1"}, nil
	}}
	photos := &fakeProfilePhotoStore{}
	router := testRouterWithProfilePhotos(t, validSessionVerifier(), users, photos)

	// Act: attempt to create another onboarding upload authorization.
	response := requestJSON(t, router, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": validPhotoMetadata()})

	// Assert: completed users cannot use onboarding to overwrite their photo.
	assertAPIError(t, response, http.StatusConflict, "profile_already_complete", "")
	if len(photos.authorizeCalls) != 0 {
		t.Errorf("authorization calls = %d, want zero", len(photos.authorizeCalls))
	}
}

func TestAuthorizeProfilePhotoUploadDoesNotLeakStorageFailure(t *testing.T) {
	// Arrange: make Tigris authorization fail with sensitive provider details.
	providerFailure := "Tigris secret access key ABC123 was rejected"
	photos := &fakeProfilePhotoStore{authorizeFn: func(context.Context, string, string, int64) (string, time.Time, error) {
		return "", time.Time{}, errors.New(providerFailure)
	}}
	router := testRouterWithProfilePhotos(t, validSessionVerifier(), &fakeUserRepository{}, photos)

	// Act: request a direct upload authorization.
	response := requestJSON(t, router, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": validPhotoMetadata()})

	// Assert: the API fails closed without returning storage credentials or provider diagnostics.
	assertAPIError(t, response, http.StatusInternalServerError, "internal_error", "")
	if strings.Contains(response.Body.String(), providerFailure) || strings.Contains(response.Body.String(), "ABC123") {
		t.Fatalf("storage error leaked sensitive details: %s", response.Body.String())
	}
}

func TestPhotoUploadAuthorizationDoesNotCompleteRegistration(t *testing.T) {
	// Arrange: authenticate a user without a local profile.
	users := &fakeUserRepository{}
	photos := &fakeProfilePhotoStore{}
	router := testRouterWithProfilePhotos(t, validSessionVerifier(), users, photos)

	// Act: authorize an upload, then reopen registration status without completing it.
	authorization := requestJSON(t, router, http.MethodPut, "/api/v1/me/profile-photo", testAuthorization, map[string]any{"photo": validPhotoMetadata()})
	status := requestJSON(t, router, http.MethodGet, "/api/v1/me", testAuthorization, nil)

	// Assert: an abandoned direct upload never traps or falsely completes registration.
	if authorization.Code != http.StatusOK {
		t.Fatalf("authorization status = %d, want %d; body = %s", authorization.Code, http.StatusOK, authorization.Body.String())
	}
	if status.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", status.Code, http.StatusOK, status.Body.String())
	}
	if got := responseJSON(t, status)["registration_complete"]; got != false {
		t.Errorf("registration_complete = %v, want false", got)
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestCompleteRegistrationSavesVerifiedProfilePhoto(t *testing.T) {
	for _, test := range supportedPhotoContents(t) {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: Tigris contains the exact photo declared by the client.
			photos := storedProfilePhoto(test.content, test.mimeType, int64(len(test.content)))
			users := &fakeUserRepository{}
			router := testRouterWithProfilePhotos(t, validSessionVerifier(), users, photos)
			payload := validCompletionPayload()
			payload["photo"] = map[string]any{
				"file_name":  test.fileName,
				"mime_type":  test.mimeType,
				"size_bytes": len(test.content),
			}

			// Act: complete registration after the direct upload finishes.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

			// Assert: completion verifies the object and saves its server-owned key.
			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
			}
			if len(photos.openedKeys) != 1 || photos.openedKeys[0] != profilePhotoObjectKey {
				t.Fatalf("opened keys = %#v, want %q", photos.openedKeys, profilePhotoObjectKey)
			}
			if len(users.saved) != 1 || users.saved[0].ProfileImageKey == nil || *users.saved[0].ProfileImageKey != profilePhotoObjectKey {
				t.Fatalf("saved profile image key = %#v, want %q", users.saved, profilePhotoObjectKey)
			}
		})
	}
}

func TestCompleteRegistrationRejectsUnverifiedProfilePhoto(t *testing.T) {
	validJPEG := encodeTestImage(t, "jpeg")
	tests := []struct {
		name     string
		metadata map[string]any
		store    *fakeProfilePhotoStore
		status   int
		code     string
		field    string
	}{
		{
			name:     "object not uploaded",
			metadata: photoMetadataFor("avatar.jpg", "image/jpeg", len(validJPEG)),
			store: &fakeProfilePhotoStore{openObjectFn: func(context.Context, string) (io.ReadCloser, string, int64, error) {
				return nil, "", 0, fs.ErrNotExist
			}},
			status: http.StatusConflict,
			code:   "photo_upload_incomplete",
			field:  "photo",
		},
		{
			name:     "storage unavailable",
			metadata: photoMetadataFor("avatar.jpg", "image/jpeg", len(validJPEG)),
			store: &fakeProfilePhotoStore{openObjectFn: func(context.Context, string) (io.ReadCloser, string, int64, error) {
				return nil, "", 0, errors.New("Tigris unavailable with secret ABC123")
			}},
			status: http.StatusInternalServerError,
			code:   "internal_error",
		},
		{
			name:     "stored MIME differs",
			metadata: photoMetadataFor("avatar.jpg", "image/jpeg", len(validJPEG)),
			store:    storedProfilePhoto(validJPEG, "image/png", int64(len(validJPEG))),
			status:   http.StatusUnprocessableEntity,
			code:     "invalid_photo",
			field:    "photo.mime_type",
		},
		{
			name:     "stored length differs",
			metadata: photoMetadataFor("avatar.jpg", "image/jpeg", len(validJPEG)+1),
			store:    storedProfilePhoto(validJPEG, "image/jpeg", int64(len(validJPEG))),
			status:   http.StatusUnprocessableEntity,
			code:     "invalid_photo",
			field:    "photo.size_bytes",
		},
		{
			name:     "stored object exceeds limit",
			metadata: photoMetadataFor("avatar.jpg", "image/jpeg", maxTestPhotoBytes),
			store:    storedProfilePhoto(validJPEG, "image/jpeg", maxTestPhotoBytes+1),
			status:   http.StatusUnprocessableEntity,
			code:     "invalid_photo",
			field:    "photo.size_bytes",
		},
		{
			name:     "content signature differs",
			metadata: photoMetadataFor("avatar.jpg", "image/jpeg", len("not a jpeg")),
			store:    storedProfilePhoto([]byte("not a jpeg"), "image/jpeg", int64(len("not a jpeg"))),
			status:   http.StatusUnprocessableEntity,
			code:     "invalid_photo",
			field:    "photo",
		},
		{
			name:     "truncated image",
			metadata: photoMetadataFor("avatar.jpg", "image/jpeg", 4),
			store:    storedProfilePhoto([]byte{0xff, 0xd8, 0xff, 0xdb}, "image/jpeg", 4),
			status:   http.StatusUnprocessableEntity,
			code:     "invalid_photo",
			field:    "photo",
		},
		{
			name:     "excessive pixel dimensions",
			metadata: photoMetadataFor("avatar.png", "image/png", len(oversizedPNGHeader(t))),
			store:    storedProfilePhoto(oversizedPNGHeader(t), "image/png", int64(len(oversizedPNGHeader(t)))),
			status:   http.StatusUnprocessableEntity,
			code:     "invalid_photo",
			field:    "photo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: expose an absent, inconsistent, or unsafe object from storage.
			users := &fakeUserRepository{}
			router := testRouterWithProfilePhotos(t, validSessionVerifier(), users, test.store)
			payload := validCompletionPayload()
			payload["photo"] = test.metadata

			// Act: attempt to complete registration with the unverified object.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/complete", testAuthorization, payload)

			// Assert: no local profile points at missing or unsafe content.
			assertAPIError(t, response, test.status, test.code, test.field)
			if users.createCalls != 0 || len(users.saved) != 0 {
				t.Errorf("create calls/saved = %d/%d, want 0/0", users.createCalls, len(users.saved))
			}
			if strings.Contains(response.Body.String(), "ABC123") {
				t.Fatalf("storage error leaked sensitive details: %s", response.Body.String())
			}
		})
	}
}

func testRouterWithProfilePhotos(t *testing.T, verifier *fakeSessionVerifier, users *fakeUserRepository, photos *fakeProfilePhotoStore) http.Handler {
	t.Helper()
	deps := Dependencies{
		Verifier:      verifier,
		Users:         users,
		ProfilePhotos: photos,
		Validator:     NewOnboardingValidator(),
	}
	return NewWithDependencies(deps)
}

type storedPhoto struct {
	name     string
	fileName string
	mimeType string
	content  []byte
}

func supportedPhotoContents(t *testing.T) []storedPhoto {
	t.Helper()
	return []storedPhoto{
		{name: "JPEG", fileName: "avatar.jpg", mimeType: "image/jpeg", content: encodeTestImage(t, "jpeg")},
		{name: "PNG", fileName: "avatar.png", mimeType: "image/png", content: encodeTestImage(t, "png")},
		{name: "HEIC", fileName: "avatar.heic", mimeType: "image/heic", content: minimalHEIC()},
	}
}

func storedProfilePhoto(content []byte, mimeType string, sizeBytes int64) *fakeProfilePhotoStore {
	return &fakeProfilePhotoStore{openObjectFn: func(context.Context, string) (io.ReadCloser, string, int64, error) {
		return io.NopCloser(bytes.NewReader(content)), mimeType, sizeBytes, nil
	}}
}

func photoMetadataFor(fileName, mimeType string, sizeBytes int) map[string]any {
	return map[string]any{"file_name": fileName, "mime_type": mimeType, "size_bytes": sizeBytes}
}

func encodeTestImage(t *testing.T, format string) []byte {
	t.Helper()
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageData.Set(0, 0, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&encoded, imageData, nil)
	case "png":
		err = png.Encode(&encoded, imageData)
	default:
		t.Fatalf("unsupported test image format %q", format)
	}
	if err != nil {
		t.Fatalf("encode %s test image: %v", format, err)
	}
	return encoded.Bytes()
}

func minimalHEIC() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18,
		'f', 't', 'y', 'p',
		'h', 'e', 'i', 'c',
		0x00, 0x00, 0x00, 0x00,
		'h', 'e', 'i', 'c',
		'm', 'i', 'f', '1',
	}
}

func oversizedPNGHeader(t *testing.T) []byte {
	t.Helper()
	var encoded bytes.Buffer
	encoded.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	var ihdr bytes.Buffer
	if err := binary.Write(&ihdr, binary.BigEndian, uint32(100_000)); err != nil {
		t.Fatalf("write PNG width: %v", err)
	}
	if err := binary.Write(&ihdr, binary.BigEndian, uint32(100_000)); err != nil {
		t.Fatalf("write PNG height: %v", err)
	}
	ihdr.Write([]byte{8, 2, 0, 0, 0})
	if err := binary.Write(&encoded, binary.BigEndian, uint32(ihdr.Len())); err != nil {
		t.Fatalf("write IHDR length: %v", err)
	}
	encoded.WriteString("IHDR")
	encoded.Write(ihdr.Bytes())
	checksumInput := append([]byte("IHDR"), ihdr.Bytes()...)
	if err := binary.Write(&encoded, binary.BigEndian, crc32.ChecksumIEEE(checksumInput)); err != nil {
		t.Fatalf("write IHDR checksum: %v", err)
	}
	return encoded.Bytes()
}
