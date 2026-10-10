package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// maxPeekBody is the largest JSON body the store check reads to find a
// store_id; bigger bodies (bulk imports) pass through unchecked by it.
const maxPeekBody = 8 << 20

// StoreAccessMiddleware refuses (403) a signed-in user's request that names a
// store the user may not use. Handlers read the store from the query
// (search[store_id]), the body (store_id) or the /v1/store/{id} path, and
// each store has its own database, so without this any signed-in user could
// read or change any other store by passing its id. A user may use a store
// when they are an Admin, when the store is in their store_ids, or when
// their store_ids is empty (the same rule the store list applies).
// Requests without a valid access token pass through: the handler answers
// them with 401 as before.
func StoreAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := requestedStoreIDs(r)
		if len(ids) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		user := storeAccessUser(r)
		if user == nil {
			next.ServeHTTP(w, r)
			return
		}
		for _, id := range ids {
			if !userMayUseStore(user.Role, user.StoreIDs, id) {
				var response models.Response
				response.Status = false
				response.Errors = map[string]string{"store_id": "You don't have access to this store"}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(response)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// userMayUseStore is the access rule; see StoreAccessMiddleware.
func userMayUseStore(role string, storeIDs []*primitive.ObjectID, id primitive.ObjectID) bool {
	if role == "Admin" || len(storeIDs) == 0 {
		return true
	}
	for _, s := range storeIDs {
		if s != nil && *s == id {
			return true
		}
	}
	return false
}

// requestedStoreIDs lists every valid, non-zero store id the request names.
// Malformed ids are left to the handlers, which reject them.
func requestedStoreIDs(r *http.Request) []primitive.ObjectID {
	var raw []string
	q := r.URL.Query()
	raw = append(raw, q["search[store_id]"]...)
	raw = append(raw, q["store_id"]...)

	tpl := ""
	if route := mux.CurrentRoute(r); route != nil {
		tpl, _ = route.GetPathTemplate()
	}
	if strings.HasPrefix(tpl, "/v1/store/{id}") {
		raw = append(raw, mux.Vars(r)["id"])
	}

	storeID, id := bodyStoreID(r)
	if storeID != "" {
		raw = append(raw, storeID)
	}
	// ZATCA connect/disconnect name the store as "id".
	if strings.HasPrefix(tpl, "/v1/store/zatca/") && id != "" {
		raw = append(raw, id)
	}

	var ids []primitive.ObjectID
	for _, s := range raw {
		id, err := primitive.ObjectIDFromHex(strings.TrimSpace(s))
		if err == nil && !id.IsZero() {
			ids = append(ids, id)
		}
	}
	return ids
}

// bodyStoreID returns the top-level "store_id" and "id" of a JSON object
// body and leaves the body readable for the handler.
func bodyStoreID(r *http.Request) (storeID, id string) {
	if r.Body == nil || r.Body == http.NoBody {
		return "", ""
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		return "", ""
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "json") && !strings.HasPrefix(ct, "text/plain") {
		return "", ""
	}
	if r.ContentLength > maxPeekBody {
		return "", ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPeekBody+1))
	rest := r.Body
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(body), rest), rest}
	if err != nil || len(body) > maxPeekBody {
		return "", ""
	}
	var probe struct {
		StoreID interface{} `json:"store_id"`
		ID      interface{} `json:"id"`
	}
	if json.Unmarshal(body, &probe) != nil {
		return "", ""
	}
	storeID, _ = probe.StoreID.(string)
	id, _ = probe.ID.(string)
	return storeID, id
}

// storeAccessUser returns the user behind the request's access token, or nil
// when there is no valid one.
func storeAccessUser(r *http.Request) *models.User {
	tokenStr, err := models.ParseAccessTokenFromRequest(r)
	if err != nil {
		return nil
	}
	claims, err := models.AuthenticateByJWTToken(tokenStr)
	if err != nil || claims.Type != "access_token" {
		return nil
	}
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil
	}
	user, err := models.FindUserByID(&userID, bson.M{"role": 1, "store_ids": 1})
	if err != nil {
		return nil
	}
	return user
}
