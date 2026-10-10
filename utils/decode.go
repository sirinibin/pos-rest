package utils

import (
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/sirinibin/startpos/backend/models"
)

func Decode(w http.ResponseWriter, r *http.Request, v interface{}) bool {

	var response models.Response

	if err := json.NewDecoder(r.Body).Decode(&v); err != nil || v == nil || decodedToNil(v) {
		msg := "request body is empty or null"
		if err != nil {
			msg = err.Error()
		}

		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["input"] = "Invalid Data:" + msg
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return false
	}
	return true
}

// decodedToNil reports whether a JSON null left the caller's target pointer
// nil (e.g. a *Order decoded from "null"). Handlers dereference the target
// straight away, so such a body must be rejected like an empty one.
func decodedToNil(v interface{}) bool {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}
	return false
}
