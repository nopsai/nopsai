package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

type RequiredField struct {
	Name    string
	Missing bool
}

func DecodeJSON(r *http.Request, dst interface{}) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return ensureSingleJSONValue(decoder)
}

func DecodeOptionalJSON(r *http.Request, dst interface{}) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return ensureSingleJSONValue(decoder)
}

func WriteJSON(w http.ResponseWriter, status int, payload interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

func WriteJSONError(w http.ResponseWriter, status int, message string) error {
	return WriteJSON(w, status, ErrorResponse{Error: message})
}

func RequiredString(name, value string) RequiredField {
	return RequiredField{
		Name:    name,
		Missing: strings.TrimSpace(value) == "",
	}
}

func RequiredInt64(name string, value int64) RequiredField {
	return RequiredField{
		Name:    name,
		Missing: value == 0,
	}
}

func ValidateRequired(fields ...RequiredField) error {
	missing := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Missing {
			missing = append(missing, field.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if len(missing) == 1 {
		return fmt.Errorf("%s is required", missing[0])
	}
	return fmt.Errorf("%s are required", joinWithAnd(missing))
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra struct{}
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return fmt.Errorf("request body must contain a single JSON value")
}

func joinWithAnd(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
	}
}
