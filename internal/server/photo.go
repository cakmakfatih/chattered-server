package server

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxPhotoBytes = 10 << 20

var photoSizeValidationTag = fmt.Sprintf("gt=0,lte=%d", maxPhotoBytes)

var photoMIME = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
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
	if deps.Validator.Var(photo.MIMEType, "required,oneof=image/jpeg image/png image/webp image/heic") != nil || expectedMIME != photo.MIMEType {
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
