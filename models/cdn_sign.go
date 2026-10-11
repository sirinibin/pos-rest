package models

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/env"
)

// Signed /cdn links. ZATCA invoice XML files (/cdn/zatca/<storeId>/…) are
// opened straight in a browser tab, which sends no Authorization header, so
// the API hands out links carrying an expiry and an HMAC of the path
// (?exp=<unix>&sig=<hex>) instead of serving the files to anyone.

// CdnLinkTTL is how long a signed link works.
const CdnLinkTTL = 24 * time.Hour

func cdnSig(path string, exp int64) string {
	m := hmac.New(sha256.New, []byte("cdn-link:"+env.GetJWTAccessSecret()))
	m.Write([]byte(path + "\n" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

// SignCdnURL returns link ("/cdn/…", without a query) with an expiry and a
// signature; other values are returned unchanged.
func SignCdnURL(link string, now time.Time) string {
	if !strings.HasPrefix(link, "/cdn/") || strings.Contains(link, "?") {
		return link
	}
	exp := now.Add(CdnLinkTTL).Unix()
	return link + "?exp=" + strconv.FormatInt(exp, 10) + "&sig=" + cdnSig(link, exp)
}

// ValidCdnSignature reports whether q carries an unexpired signature for path
// ("/cdn/…").
func ValidCdnSignature(path string, q url.Values, now time.Time) bool {
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	if err != nil || now.Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(q.Get("sig")), []byte(cdnSig(path, exp)))
}
