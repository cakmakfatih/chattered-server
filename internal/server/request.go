package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
)

const maxRequestBodyBytes = 16 << 10

type requestFailure struct {
	status  int
	problem apiError
}

func readRequestObject(c *gin.Context, allowedFields ...string) (map[string]json.RawMessage, bool) {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	fields, failure := decodeJSONObject(body)
	if failure == nil {
		if field := firstUnexpectedField(fields, allowedFields); field != "" {
			failure = &requestFailure{
				status:  http.StatusUnprocessableEntity,
				problem: apiError{code: "unexpected_field", field: field, message: "Field is not accepted"},
			}
		}
	}
	if failure != nil {
		respondError(c, failure.status, failure.problem)
		return nil, false
	}
	return fields, true
}

func decodeJSONObject(body io.Reader) (map[string]json.RawMessage, *requestFailure) {
	decoder := json.NewDecoder(body)
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, invalidJSONObject()
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, multipleJSONObjectFailure()
	}
	return fields, nil
}

func firstUnexpectedField(fields map[string]json.RawMessage, allowedFields []string) string {
	allowed := make(map[string]struct{}, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = struct{}{}
	}

	keys := make([]string, 0, len(fields))
	for field := range fields {
		keys = append(keys, field)
	}
	sort.Strings(keys)
	for _, field := range keys {
		if _, ok := allowed[field]; !ok {
			return field
		}
	}
	return ""
}

func decodeString(raw json.RawMessage) (string, bool) {
	var value string
	if len(raw) == 0 || isAbsentJSON(raw) || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func decodeOptionalString(raw json.RawMessage) (*string, bool) {
	if isAbsentJSON(raw) {
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

func invalidJSONObject() *requestFailure {
	return &requestFailure{
		status:  http.StatusBadRequest,
		problem: apiError{code: "invalid_request", message: "Expected a JSON object"},
	}
}

func multipleJSONObjectFailure() *requestFailure {
	return &requestFailure{
		status:  http.StatusBadRequest,
		problem: apiError{code: "invalid_request", message: "Expected one JSON object"},
	}
}

func isAbsentJSON(value json.RawMessage) bool {
	return len(value) == 0 || bytes.Equal(value, []byte("null"))
}
