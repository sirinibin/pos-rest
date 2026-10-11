package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// cdnPrefixes are the folders files are stored under (images, attachments,
// ZATCA XML). /cdn/ used to serve any file below the working directory
// (configuration, keys, source) when S3 is off.
var cdnPrefixes = []string{"images/", "attachments/", "zatca/"}

func cdnServable(path string) bool {
	for _, p := range cdnPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// cdnAccess decides whether r may read /cdn/<path>; 0 = yes, otherwise the
// HTTP status to answer. Images and attachments stay public links (they are
// shown in <img> tags and shared). ZATCA invoice XML (zatca/<storeId>/…)
// needs either a signed link (?exp=&sig=, what the API now returns as
// xmlUrl) or a valid access token (Authorization header or ?access_token=)
// of a user who may use that store. Files outside a store folder need a
// platform admin's token.
func cdnAccess(r *http.Request, path string) int {
	if !strings.HasPrefix(path, "zatca/") {
		return 0
	}
	if models.ValidCdnSignature("/cdn/"+path, r.URL.Query(), time.Now()) {
		return 0
	}
	_, user, err := models.AuthenticatedUser(r)
	if err != nil {
		return http.StatusUnauthorized
	}
	seg := strings.SplitN(strings.TrimPrefix(path, "zatca/"), "/", 2)[0]
	if id, err := primitive.ObjectIDFromHex(seg); err == nil {
		if mayUseStore(user, &id) {
			return 0
		}
		return http.StatusForbidden
	}
	if isAdminUser(user) {
		return 0
	}
	return http.StatusForbidden
}
