package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"apartmanim/server/internal/database/models"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

const maxPhotoBytes = 10 << 20

var photoMIME = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
	".webp": "image/webp", ".heic": "image/heic",
}

type photoMetadata struct {
	FileName  string `json:"file_name"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
}

func respondError(c *gin.Context, status int, code, field, message string, details gin.H) {
	problem := gin.H{"code": code, "message": message}
	if field != "" {
		problem["field"] = field
	}
	if details != nil {
		problem["details"] = details
	}
	c.JSON(status, gin.H{"error": problem})
}

func (deps Dependencies) me(c *gin.Context) {
	user, err := deps.Users.GetByClerkID(c.Request.Context(), c.GetString("clerk_user_id"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", "", "Could not load the user profile", nil)
		return
	}
	if user == nil {
		c.JSON(http.StatusOK, gin.H{"registration_complete": false, "user": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"registration_complete": true, "user": gin.H{"username": user.Username, "bio": user.Bio, "gender": user.Gender}})
}

func (deps Dependencies) checkUsername(c *gin.Context) {
	fields, ok := decodeObject(c, "username")
	if !ok {
		return
	}
	username, ok := decodeString(fields["username"])
	if !ok || deps.Validator.Var(username, "username") != nil {
		respondError(c, http.StatusUnprocessableEntity, "invalid_username", "username", "Invalid username", nil)
		return
	}
	exists, err := deps.Users.UsernameExists(c.Request.Context(), username)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", "", "Could not check username availability", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": !exists})
}

func (deps Dependencies) validateBio(c *gin.Context) {
	fields, ok := decodeObject(c, "bio")
	if !ok {
		return
	}
	bio, ok := decodeOptionalString(fields["bio"])
	if !ok || (bio != nil && deps.Validator.Var(*bio, "bio") != nil) {
		respondError(c, http.StatusUnprocessableEntity, "invalid_bio", "bio", "Invalid biography", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true, "normalized_bio": bio})
}

func (deps Dependencies) validatePhoto(c *gin.Context) {
	fields, ok := decodeObject(c, "photo")
	if !ok {
		return
	}
	if !deps.checkPhoto(c, fields["photo"]) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true})
}

func (deps Dependencies) complete(c *gin.Context) {
	fields, ok := decodeObject(c, "username", "gender", "bio", "photo")
	if !ok {
		return
	}
	username, ok := decodeString(fields["username"])
	if !ok || deps.Validator.Var(username, "username") != nil {
		respondError(c, http.StatusUnprocessableEntity, "invalid_username", "username", "Invalid username", nil)
		return
	}
	gender, ok := decodeString(fields["gender"])
	if !ok || deps.Validator.Var(gender, "gender") != nil {
		respondError(c, http.StatusUnprocessableEntity, "invalid_gender", "gender", "Invalid gender", nil)
		return
	}
	bio, ok := decodeOptionalString(fields["bio"])
	if !ok || (bio != nil && deps.Validator.Var(*bio, "bio") != nil) {
		respondError(c, http.StatusUnprocessableEntity, "invalid_bio", "bio", "Invalid biography", nil)
		return
	}
	if !deps.checkPhoto(c, fields["photo"]) {
		return
	}
	clerkID := c.GetString("clerk_user_id")
	existing, err := deps.Users.GetByClerkID(c.Request.Context(), clerkID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", "", "Could not check registration status", nil)
		return
	}
	if existing != nil {
		respondError(c, http.StatusConflict, "profile_already_complete", "", "Profile already exists", nil)
		return
	}
	exists, err := deps.Users.UsernameExists(c.Request.Context(), username)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", "", "Could not check username availability", nil)
		return
	}
	if exists {
		respondError(c, http.StatusConflict, "username_taken", "username", "Username is already in use", nil)
		return
	}
	user := &models.User{ClerkUserID: clerkID, Username: username, Gender: gender, Bio: bio}
	if err := deps.Users.Create(c.Request.Context(), user); err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) && pgError.Code == "23505" {
			switch pgError.ConstraintName {
			case "ux_users_username":
				respondError(c, http.StatusConflict, "username_taken", "username", "Username is already in use", nil)
				return
			case "ux_users_clerk_user_id":
				respondError(c, http.StatusConflict, "profile_already_complete", "", "Profile already exists", nil)
				return
			}
		}
		respondError(c, http.StatusInternalServerError, "internal_error", "", "Could not create the user profile", nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"registration_complete": true, "user": gin.H{"username": user.Username, "gender": user.Gender, "bio": user.Bio}})
}

func (deps Dependencies) checkPhoto(c *gin.Context, raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return true
	}
	var photo photoMetadata
	if err := json.Unmarshal(raw, &photo); err != nil {
		respondError(c, http.StatusUnprocessableEntity, "invalid_photo", "photo", "Invalid photo metadata", nil)
		return false
	}
	name := photo.FileName
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		respondError(c, http.StatusUnprocessableEntity, "invalid_photo", "photo.file_name", "Invalid photo filename", nil)
		return false
	}
	mime, supported := photoMIME[strings.ToLower(filepath.Ext(name))]
	if !supported {
		respondError(c, http.StatusUnprocessableEntity, "invalid_photo", "photo.file_name", "Unsupported photo extension", nil)
		return false
	}
	if deps.Validator.Var(photo.MIMEType, "required,oneof=image/jpeg image/png image/webp image/heic") != nil || mime != photo.MIMEType {
		respondError(c, http.StatusUnprocessableEntity, "invalid_photo", "photo.mime_type", "Photo MIME type must match its extension", nil)
		return false
	}
	if deps.Validator.Var(photo.SizeBytes, "gt=0,lte=10485760") != nil {
		respondError(c, http.StatusUnprocessableEntity, "invalid_photo", "photo.size_bytes", "Photo size must be between 1 byte and 10 MiB", gin.H{"max_size_bytes": maxPhotoBytes})
		return false
	}
	return true
}

func decodeObject(c *gin.Context, allowed ...string) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		respondError(c, http.StatusBadRequest, "invalid_request", "", "Expected a JSON object", nil)
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		respondError(c, http.StatusBadRequest, "invalid_request", "", "Expected one JSON object", nil)
		return nil, false
	}
	permitted := make(map[string]bool, len(allowed))
	for _, field := range allowed {
		permitted[field] = true
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !permitted[key] {
			respondError(c, http.StatusUnprocessableEntity, "unexpected_field", key, "Field is not accepted", nil)
			return nil, false
		}
	}
	return fields, true
}

func decodeString(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func decodeOptionalString(raw json.RawMessage) (*string, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, true
	}
	value, ok := decodeString(raw)
	if !ok {
		return nil, false
	}
	if value == "" {
		return nil, true
	}
	return &value, true
}
