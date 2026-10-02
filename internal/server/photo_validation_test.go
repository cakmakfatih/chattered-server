package server

import (
	"net/http"
	"testing"
)

const maxTestPhotoBytes = 10 << 20

func TestPhotoValidationAcceptsNoPhoto(t *testing.T) {
	// Arrange: photo selection is optional during onboarding.
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)

	// Act: validate an absent photo.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/photo/validate", testAuthorization, map[string]any{"photo": nil})

	// Assert: absence is valid and does not create a record.
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := responseJSON(t, response)["valid"]; got != true {
		t.Errorf("valid = %v, want true", got)
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestPhotoValidationAcceptsSupportedMetadata(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		mimeType string
	}{
		{name: "JPEG jpg", fileName: "avatar.jpg", mimeType: "image/jpeg"},
		{name: "JPEG jpeg", fileName: "avatar.jpeg", mimeType: "image/jpeg"},
		{name: "PNG", fileName: "avatar.png", mimeType: "image/png"},
		{name: "WebP", fileName: "avatar.webp", mimeType: "image/webp"},
		{name: "HEIC", fileName: "avatar.heic", mimeType: "image/heic"},
		{name: "uppercase extension", fileName: "avatar.JPG", mimeType: "image/jpeg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare supported photo metadata without file bytes.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)
			photo := map[string]any{"file_name": test.fileName, "mime_type": test.mimeType, "size_bytes": 1024}

			// Act: submit metadata for prevalidation.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/photo/validate", testAuthorization, map[string]any{"photo": photo})

			// Assert: supported metadata is accepted without persistence.
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			if got := responseJSON(t, response)["valid"]; got != true {
				t.Errorf("valid = %v, want true", got)
			}
			if users.createCalls != 0 || len(users.lookedUpNames) != 0 {
				t.Errorf("prevalidation touched repository: creates=%d, lookups=%#v", users.createCalls, users.lookedUpNames)
			}
		})
	}
}

func TestPhotoValidationEnforcesSizeLimit(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		status int
	}{
		{name: "one byte", size: 1, status: http.StatusOK},
		{name: "exactly ten MiB", size: maxTestPhotoBytes, status: http.StatusOK},
		{name: "one byte above ten MiB", size: maxTestPhotoBytes + 1, status: http.StatusUnprocessableEntity},
		{name: "zero bytes", size: 0, status: http.StatusUnprocessableEntity},
		{name: "negative bytes", size: -1, status: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare otherwise valid metadata at the selected byte size.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)
			photo := validPhotoMetadata()
			photo["size_bytes"] = test.size

			// Act: validate the photo metadata.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/photo/validate", testAuthorization, map[string]any{"photo": photo})

			// Assert: the byte limit is inclusive and errors identify the size field.
			if test.status == http.StatusOK {
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
				}
			} else {
				assertAPIError(t, response, http.StatusUnprocessableEntity, "invalid_photo", "photo.size_bytes")
			}
			if users.createCalls != 0 {
				t.Errorf("create calls = %d, want zero", users.createCalls)
			}
		})
	}
}

func TestPhotoValidationExplainsMaximumSize(t *testing.T) {
	// Arrange: provide a photo whose metadata exceeds the ten MiB policy.
	users := &fakeUserRepository{}
	router := testRouter(validSessionVerifier(), users)
	photo := validPhotoMetadata()
	photo["size_bytes"] = maxTestPhotoBytes + 1

	// Act: validate the oversized photo metadata.
	response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/photo/validate", testAuthorization, map[string]any{"photo": photo})

	// Assert: the error identifies the size field and gives the client the exact limit.
	assertAPIError(t, response, http.StatusUnprocessableEntity, "invalid_photo", "photo.size_bytes")
	detail := responseJSON(t, response)["error"].(map[string]any)
	limits, ok := detail["details"].(map[string]any)
	if !ok || limits["max_size_bytes"] != float64(maxTestPhotoBytes) {
		t.Errorf("error details = %#v, want max_size_bytes=%d", detail["details"], maxTestPhotoBytes)
	}
	if users.createCalls != 0 {
		t.Errorf("create calls = %d, want zero", users.createCalls)
	}
}

func TestPhotoValidationRejectsBadFileMetadata(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		mimeType string
		field    string
	}{
		{name: "unsupported extension", fileName: "avatar.gif", mimeType: "image/gif", field: "photo.file_name"},
		{name: "missing extension", fileName: "avatar", mimeType: "image/jpeg", field: "photo.file_name"},
		{name: "path instead of file name", fileName: "../avatar.jpg", mimeType: "image/jpeg", field: "photo.file_name"},
		{name: "mismatched MIME", fileName: "avatar.png", mimeType: "image/jpeg", field: "photo.mime_type"},
		{name: "missing MIME", fileName: "avatar.jpg", mimeType: "", field: "photo.mime_type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: prepare inconsistent or unsupported photo metadata.
			users := &fakeUserRepository{}
			router := testRouter(validSessionVerifier(), users)
			photo := map[string]any{"file_name": test.fileName, "mime_type": test.mimeType, "size_bytes": 1024}

			// Act: submit the metadata for validation.
			response := requestJSON(t, router, http.MethodPost, "/api/v1/onboarding/photo/validate", testAuthorization, map[string]any{"photo": photo})

			// Assert: the response identifies the invalid field and saves nothing.
			assertAPIError(t, response, http.StatusUnprocessableEntity, "invalid_photo", test.field)
			if users.createCalls != 0 {
				t.Errorf("create calls = %d, want zero", users.createCalls)
			}
		})
	}
}
