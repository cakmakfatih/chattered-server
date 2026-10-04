package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxPhotoBytes = 10 << 20
const maxPhotoPixels = 40_000_000
const profilePhotoUploadLifetime = 10 * time.Minute

var photoSizeValidationTag = fmt.Sprintf("gt=0,lte=%d", maxPhotoBytes)

var photoMIME = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".heic": "image/heic",
}

type photoMetadata struct {
	FileName  string `json:"file_name"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

func (deps Dependencies) validatePhotoMetadata(raw json.RawMessage) *apiError {
	if isAbsentJSON(raw) {
		return nil
	}

	var photo photoMetadata
	if err := json.Unmarshal(raw, &photo); err != nil {
		return invalidPhoto("photo", "Invalid photo metadata")
	}

	expectedMIME, err := expectedPhotoMIME(photo.FileName)
	if err != nil {
		return err
	}
	if deps.Validator.Var(photo.MIMEType, "required,oneof=image/jpeg image/png image/heic") != nil || expectedMIME != photo.MIMEType {
		return invalidPhoto("photo.mime_type", "Photo MIME type must match its extension")
	}
	if deps.Validator.Var(photo.SizeBytes, photoSizeValidationTag) != nil {
		return invalidPhotoSize()
	}
	return nil
}

func expectedPhotoMIME(fileName string) (string, *apiError) {
	if fileName == "" || filepath.Base(fileName) != fileName || strings.ContainsAny(fileName, `/\`) {
		return "", invalidPhoto("photo.file_name", "Invalid photo filename")
	}

	mimeType, supported := photoMIME[strings.ToLower(filepath.Ext(fileName))]
	if !supported {
		return "", invalidPhoto("photo.file_name", "Unsupported photo extension")
	}
	return mimeType, nil
}

func invalidPhoto(field, message string) *apiError {
	return &apiError{code: "invalid_photo", field: field, message: message}
}

func invalidPhotoSize() *apiError {
	return &apiError{
		code:    "invalid_photo",
		field:   "photo.size_bytes",
		message: "Photo size must be between 1 byte and 10 MiB",
		details: gin.H{"max_size_bytes": maxPhotoBytes},
	}
}

func profilePhotoObjectKeyForUser(clerkID string) string {
	return "users/" + url.PathEscape(clerkID) + "/profile/avatar"
}

func (deps Dependencies) verifyProfilePhoto(ctx context.Context, clerkID string, photo *photoMetadata) (*apiError, int) {
	if deps.ProfilePhotos == nil {
		return &apiError{code: "photo_upload_incomplete", field: "photo", message: "Profile photo has not been uploaded"}, 409
	}

	object, contentType, storedSize, err := deps.ProfilePhotos.OpenObject(ctx, profilePhotoObjectKeyForUser(clerkID))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &apiError{code: "photo_upload_incomplete", field: "photo", message: "Profile photo has not been uploaded"}, 409
		}
		return &apiError{code: "internal_error", message: "Could not verify the profile photo"}, 500
	}
	if object == nil {
		return &apiError{code: "internal_error", message: "Could not verify the profile photo"}, 500
	}
	defer object.Close()

	expectedMIME, _ := expectedPhotoMIME(photo.FileName)
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
		return &apiError{code: "internal_error", message: "Could not verify the profile photo"}, 500
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
		if len(content) < 16 || string(content[4:8]) != "ftyp" {
			return errors.New("invalid HEIC container")
		}
		majorBrand := string(content[8:12])
		compatibleBrands := string(content[16:])
		allBrands := majorBrand + compatibleBrands
		if !strings.Contains(allBrands, "heic") && !strings.Contains(allBrands, "heix") && !strings.Contains(allBrands, "hevc") && !strings.Contains(allBrands, "hevx") {
			return errors.New("unsupported HEIC brand")
		}
		return nil
	}

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
