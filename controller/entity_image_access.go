package controller

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// imageStore authorizes a customer/vendor/product image upload or delete:
// the caller must be signed in (401) and may use the store (403). These
// routes used to accept anyone, so files could be written to and image URLs
// attached to any record of any store. It writes the error response and
// returns nil when the request may not go on.
func imageStore(w http.ResponseWriter, r *http.Request, storeID string) *models.Store {
	_, user, err := models.AuthenticatedUser(r)
	if err != nil {
		http.Error(w, "Invalid Access token:"+err.Error(), http.StatusUnauthorized)
		return nil
	}
	storeObjectID, err := primitive.ObjectIDFromHex(storeID)
	if err != nil || storeObjectID.IsZero() {
		http.Error(w, "invalid store id", http.StatusBadRequest)
		return nil
	}
	if !mayUseStore(user, &storeObjectID) {
		http.Error(w, "You don't have access to this store", http.StatusForbidden)
		return nil
	}
	store, err := models.FindStoreByID(&storeObjectID, bson.M{})
	if err != nil {
		http.Error(w, "invalid store", http.StatusNotFound)
		return nil
	}
	return store
}

// imageRecordExists reports whether the customer, vendor or product id
// exists in store; uploads for unknown ids used to answer 200 and leave an
// orphan file.
func imageRecordExists(store *models.Store, kind string, id *primitive.ObjectID) bool {
	var err error
	sel := bson.M{"_id": 1}
	switch kind {
	case "customer":
		_, err = store.FindCustomerByID(id, sel)
	case "vendor":
		_, err = store.FindVendorByID(id, sel)
	case "product":
		_, err = store.FindProductByID(id, sel)
	default:
		return false
	}
	return err == nil
}

// localImagePath is the file on disk behind an old-style local image URL
// ("/images/…"), or "" for anything else: deleting used to remove whatever
// path the client sent ("/../…"). "/cdn/…" URLs were never removed from
// disk by these routes and still aren't.
func localImagePath(imageURL string) string {
	if !strings.HasPrefix(imageURL, "/images/") || strings.Contains(imageURL, "..") {
		return ""
	}
	p := strings.TrimPrefix(imageURL, "/")
	p = filepath.ToSlash(filepath.Clean(p))
	if !strings.HasPrefix(p, "images/") {
		return ""
	}
	return p
}

func removeLocalImage(imageURL string) {
	if p := localImagePath(imageURL); p != "" {
		_ = os.Remove(p)
	}
}
