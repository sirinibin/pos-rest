package erp

import (
	"encoding/json"
	"net/http"
)

// APIError is the contract error envelope {"error":{"code","message","fields"}}.
type APIError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *APIError) Error() string { return e.Message }

func errf(status int, code, msg string, fields map[string]string) *APIError {
	return &APIError{Status: status, Code: code, Message: msg, Fields: fields}
}

func errBadRequest(msg string, fields map[string]string) *APIError {
	if msg == "" {
		msg = firstFieldMsg(fields, "Validation failed.")
	}
	return errf(http.StatusBadRequest, "validation", msg, fields)
}

func errUnauthorized(msg string) *APIError {
	if msg == "" {
		msg = "Authentication required."
	}
	return errf(http.StatusUnauthorized, "unauthorized", msg, nil)
}

func errForbidden(msg string) *APIError {
	if msg == "" {
		msg = "You do not have permission to do this."
	}
	return errf(http.StatusForbidden, "forbidden", msg, nil)
}

func errNotFound() *APIError { return errf(http.StatusNotFound, "not_found", "Not found.", nil) }

func errConflict(code, msg string) *APIError {
	if code == "" {
		code = "conflict"
	}
	if msg == "" {
		msg = "This record was changed by someone else."
	}
	return errf(http.StatusConflict, code, msg, nil)
}

func errUnsupported(msg string) *APIError {
	return errf(http.StatusConflict, "unsupported_legacy", msg, nil)
}

func errInternal(msg string) *APIError {
	if msg == "" {
		msg = "Server error."
	}
	return errf(http.StatusInternalServerError, "internal", msg, nil)
}

func firstFieldMsg(fields map[string]string, def string) string {
	for k, v := range fields {
		return k + ": " + v
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, err error) {
	ae, ok := err.(*APIError)
	if !ok {
		ae = errInternal(err.Error())
	}
	if ae.Status == 0 {
		ae.Status = http.StatusInternalServerError
	}
	writeJSON(w, ae.Status, map[string]interface{}{"error": ae})
}
