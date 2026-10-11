package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// RequireProcurementAccess guards the procurement message routes (messages,
// threads, unread badges, email send/sync). They used to answer without a
// sign-in, so anyone with a store id could read a store's supplier emails and
// WhatsApp messages. It answers 401 without a valid access token and 403 when
// the store the request names, in the query, a multipart form or through the
// message in /v1/procurement-messages/{id}, is not one the user may use
// (JSON bodies are already checked by StoreAccessMiddleware).
func RequireProcurementAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := authenticateProcurementRequest(r)
		if err != nil {
			procurementAccessError(w, http.StatusUnauthorized, "access_token", "Invalid access token: "+err.Error())
			return
		}
		user, err := userFromClaims(claims)
		if err != nil || user == nil {
			procurementAccessError(w, http.StatusUnauthorized, "access_token", "Invalid access token")
			return
		}

		raw := r.URL.Query()["store_id"]
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(32 << 20); err == nil && r.MultipartForm != nil {
				raw = append(raw, r.MultipartForm.Value["store_id"]...)
			}
		}
		var ids []primitive.ObjectID
		for _, s := range raw {
			if id, err := primitive.ObjectIDFromHex(strings.TrimSpace(s)); err == nil && !id.IsZero() {
				ids = append(ids, id)
			}
		}
		if msgID, ok := procurementMessageIDFromRoute(r); ok {
			storeID, err := procurementMessageStoreID(msgID)
			switch {
			case err == nil:
				ids = append(ids, storeID)
			case err != mongo.ErrNoDocuments:
				procurementAccessError(w, http.StatusInternalServerError, "id", err.Error())
				return
			}
			// An unknown message is left to the handler, which answers 404.
		}
		for _, id := range ids {
			if !userMayUseStore(user.Role, user.StoreIDs, id) {
				procurementAccessError(w, http.StatusForbidden, "store_id", "You don't have access to this store")
				return
			}
		}
		next(w, r)
	}
}

// Swapped out in tests.
var (
	authenticateProcurementRequest = models.AuthenticateByAccessToken
	userFromClaims                 = models.UserFromClaims
	procurementMessageStoreID      = models.ProcurementMessageStoreID
)

// procurementMessageIDFromRoute returns the {id} of a /v1/procurement-messages/{id}... route.
func procurementMessageIDFromRoute(r *http.Request) (primitive.ObjectID, bool) {
	route := mux.CurrentRoute(r)
	if route == nil {
		return primitive.NilObjectID, false
	}
	tpl, _ := route.GetPathTemplate()
	if !strings.HasPrefix(tpl, "/v1/procurement-messages/{id}") {
		return primitive.NilObjectID, false
	}
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		return primitive.NilObjectID, false
	}
	return id, true
}

func procurementAccessError(w http.ResponseWriter, status int, field, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(models.Response{Status: false, Errors: map[string]string{field: msg}})
}
