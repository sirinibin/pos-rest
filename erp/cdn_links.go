package erp

import (
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

// signedXMLURL turns a stored ZATCA XML link (/cdn/zatca/<storeId>/…) into
// a signed, expiring one: /cdn/zatca/ files are no longer served without a
// login, and the web app opens these links in a browser tab (no
// Authorization header). The field keeps its name (xmlUrl).
func signedXMLURL(u string) string {
	if u == "" {
		return u
	}
	return models.SignCdnURL(u, time.Now())
}
