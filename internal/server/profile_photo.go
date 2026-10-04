package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxPhotoPixels = 40_000_000

func profilePhotoObjectKeyForUser(clerkID string) string {
	return "users/" + url.PathEscape(clerkID) + "/profile/avatar"
}

func (deps Dependencies) createProfilePhotoUploadAuthorization(ctx context.Context, clerkID string, photo *photoMetadata) (profilePhotoUploadAuthorizationResponse, *apiError) {
	uploadURL, expiresAt, err := deps.ProfilePhotos.AuthorizeUpload(
		ctx,
		profilePhotoObjectKeyForUser(clerkID),
		photo.MIMEType,
		photo.SizeBytes,
	)
	if err != nil {
		return profilePhotoUploadAuthorizationResponse{}, &apiError{
			code:    "internal_error",
			message: "Could not authorize the profile photo upload",
		}
	}

	return profilePhotoUploadAuthorizationResponse{
		UploadURL: uploadURL,
		Method:    http.MethodPut,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		Headers: map[string]string{
			"Content-Type":   photo.MIMEType,
			"Content-Length": strconv.FormatInt(photo.SizeBytes, 10),
		},
	}, nil
}

func (deps Dependencies) verifyProfilePhoto(ctx context.Context, clerkID string, photo *photoMetadata) (*apiError, int) {
	if deps.ProfilePhotos == nil {
		return profilePhotoUploadIncomplete(), 409
	}

	object, contentType, storedSize, err := deps.ProfilePhotos.OpenObject(ctx, profilePhotoObjectKeyForUser(clerkID))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return profilePhotoUploadIncomplete(), 409
		}
		return profilePhotoVerificationFailed(), 500
	}
	if object == nil {
		return profilePhotoVerificationFailed(), 500
	}
	defer object.Close()

	expectedMIME, _ := expectedPhotoMIME(photo.FileName)
	return validateStoredProfilePhoto(object, contentType, storedSize, photo, expectedMIME)
}

func validateStoredProfilePhoto(object io.Reader, contentType string, storedSize int64, photo *photoMetadata, expectedMIME string) (*apiError, int) {
	if contentType != expectedMIME {
		return invalidPhoto("photo.mime_type", "Stored photo MIME type does not match the selected file"), 422
	}
	if storedSize <= 0 || storedSize > maxPhotoBytes {
		return invalidPhotoSize(), 422
	}
	if storedSize != photo.SizeBytes {
		return invalidPhoto("photo.size_bytes", "Stored photo size does not match the selected file"), 422
	}

	content, err := io.ReadAll(io.LimitReader(object, maxPhotoBytes+1))
	if err != nil {
		return profilePhotoVerificationFailed(), 500
	}
	if int64(len(content)) != storedSize || int64(len(content)) > maxPhotoBytes {
		return invalidPhoto("photo.size_bytes", "Stored photo size does not match the selected file"), 422
	}
	if photoContentError(content, expectedMIME) != nil {
		return invalidPhoto("photo", "Stored content is not a valid supported image"), 422
	}
	return nil, 0
}

func photoContentError(content []byte, mimeType string) error {
	if mimeType == "image/heic" {
		return validateHEICContent(content)
	}
	return validateDecodedImageContent(content, mimeType)
}

func validateHEICContent(content []byte) error {
	if len(content) < 16 || string(content[4:8]) != "ftyp" {
		return errors.New("invalid HEIC container")
	}

	brands := string(content[8:12]) + string(content[16:])
	for _, supportedBrand := range []string{"heic", "heix", "hevc", "hevx"} {
		if strings.Contains(brands, supportedBrand) {
			return nil
		}
	}
	return errors.New("unsupported HEIC brand")
}

func validateDecodedImageContent(content []byte, mimeType string) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return err
	}

	expectedFormat := map[string]string{"image/jpeg": "jpeg", "image/png": "png"}[mimeType]
	if expectedFormat == "" || format != expectedFormat {
		return fmt.Errorf("image format %q does not match MIME type %q", format, mimeType)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxPhotoPixels {
		return errors.New("image dimensions exceed the supported limit")
	}
	return nil
}

func profilePhotoUploadIncomplete() *apiError {
	return &apiError{code: "photo_upload_incomplete", field: "photo", message: "Profile photo has not been uploaded"}
}

func profilePhotoVerificationFailed() *apiError {
	return &apiError{code: "internal_error", message: "Could not verify the profile photo"}
}
