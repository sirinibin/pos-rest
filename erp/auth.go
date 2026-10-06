package erp

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jameskeane/bcrypt"
	"github.com/sirinibin/startpos/backend/controller"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Capabilities returned by GET /meta (contract §2).
func Capabilities() M {
	return M{"clientIds": true, "serverNumbers": true, "serverStock": true, "serverTotals": true,
		"zatca": "server", "realtime": false}
}

func handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, M{"capabilities": Capabilities(), "adapter": "pos-rest/erp", "apiVersion": 1})
}

// findUserByEmailCI finds a non-deleted user by e-mail, case-insensitive and
// trimmed (legacy lookups are exact-match; stored e-mails are kept as-is).
func findUserByEmailCI(email string) M {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := mainDB().Collection("user").FindOne(ctx, bson.M{"email": email}).Decode(&raw); err == nil {
		return normDoc(raw)
	}
	re := "^" + regexp.QuoteMeta(email) + "$"
	if err := mainDB().Collection("user").FindOne(ctx, bson.M{"email": bson.M{"$regex": re, "$options": "i"}}).Decode(&raw); err == nil {
		return normDoc(raw)
	}
	return nil
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	email := strings.TrimSpace(str(body["email"]))
	password := str(body["password"])
	fields := map[string]string{}
	if email == "" {
		fields["email"] = "required"
	}
	if password == "" {
		fields["password"] = "required"
	}
	if len(fields) > 0 {
		writeErr(w, errBadRequest("Email and password are required.", fields))
		return
	}
	u := findUserByEmailCI(email)
	if u == nil || boolv(u["deleted"]) {
		writeErr(w, errf(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.", nil))
		return
	}
	if !bcrypt.Match(password, str(u["password"])) {
		writeErr(w, errf(http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.", nil))
		return
	}
	if userStatus(u) == "inactive" {
		writeErr(w, errf(http.StatusForbidden, "inactive", "This account is inactive. Contact your administrator.", nil))
		return
	}
	// Same token pair the legacy /v1/accesstoken issues (JWT + Redis session),
	// so the tokens are accepted by every existing v1 endpoint too.
	tok, err := models.GenerateAccesstoken(str(u["email"]))
	if err != nil {
		writeErr(w, errInternal("Unable to issue tokens."))
		return
	}
	id, _ := oidOf(u["_id"])
	ctx, cancel := dbctx()
	_, _ = mainDB().Collection("user").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"erp.x.lastLogin": nowFn().In(riyadh).Format(layoutDT)}})
	cancel()
	u, _ = loadUser(id)
	stores, _ := accessibleStores(u)
	writeJSON(w, http.StatusOK, M{"accessToken": tok.Token, "refreshToken": tok.RefreshToken,
		"expiresAt": tok.ExpiresAt, "refreshExpiresAt": tok.RefreshExpiresAt, "user": userToContractFull(u, stores)})
}

func handleRefresh(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	rt := strings.TrimSpace(str(body["refreshToken"]))
	if rt == "" {
		writeErr(w, errUnauthorized("refreshToken is required."))
		return
	}
	claims, err := models.AuthenticateByJWTToken(rt)
	if err != nil || claims.Type != "refresh_token" {
		writeErr(w, errUnauthorized("Invalid or expired refresh token."))
		return
	}
	uid, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		writeErr(w, errUnauthorized("Invalid refresh token."))
		return
	}
	u, err := loadUser(uid)
	if err != nil || u == nil || boolv(u["deleted"]) || userStatus(u) == "inactive" {
		writeErr(w, errUnauthorized("Account is not active."))
		return
	}
	tok, err := models.GenerateAccesstoken(str(u["email"]))
	if err != nil {
		writeErr(w, errInternal("Unable to issue tokens."))
		return
	}
	// rotate: the used refresh token is revoked
	_, _ = db.RedisClient.Del(claims.AccessUUID).Result()
	writeJSON(w, http.StatusOK, M{"accessToken": tok.Token, "refreshToken": tok.RefreshToken,
		"expiresAt": tok.ExpiresAt, "refreshExpiresAt": tok.RefreshExpiresAt})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, _ = db.RedisClient.Del(c.Claims.AccessUUID).Result()
	if body, err := readBody(r); err == nil {
		if rt := str(body["refreshToken"]); rt != "" {
			if cl, err := models.AuthenticateByJWTToken(rt); err == nil && cl.UserID == c.Claims.UserID {
				_, _ = db.RedisClient.Del(cl.AccessUUID).Result()
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meBody(c))
}

func meBody(c *Ctx) M {
	stores := []M{}
	x := newMapCtx(c, "")
	sb := newStoresResource().Backend.(*storesBackend)
	for _, s := range c.Stores {
		stores = append(stores, sb.render(x, s, false))
	}
	u := userToContractFull(c.User, c.Stores)
	u["perms"] = permsToM(c.Perms)
	return M{"user": u, "stores": stores}
}

// ---- signup (owner rule: VAT, CR, registered company name, mobile, National Address) ----

var signupTypes = map[string]bool{"retail": true, "wholesale": true, "workshop": true, "industrial": true,
	"restaurant": true, "services": true, "contracting": true, "other": true}
var signupPlans = map[string]bool{"starter": true, "professional": true, "enterprise": true}

func passwordStrength(p string) int {
	s := 0
	if len(p) >= 8 {
		s++
	}
	if regexp.MustCompile(`[A-Z]`).MatchString(p) && regexp.MustCompile(`[a-z]`).MatchString(p) {
		s++
	}
	if regexp.MustCompile(`\d`).MatchString(p) {
		s++
	}
	if regexp.MustCompile(`[^A-Za-z0-9]`).MatchString(p) {
		s++
	}
	return s
}

// ValidateSignup mirrors the UI rules (L29129–29150) server-side.
func ValidateSignup(body M) map[string]string {
	e := map[string]string{}
	o := sub(body, "owner")
	c := sub(body, "company")
	a := sub(c, "address")
	if strings.TrimSpace(str(o["name"])) == "" {
		e["owner.name"] = "required"
	}
	if em := strings.TrimSpace(str(o["email"])); em == "" {
		e["owner.email"] = "required"
	} else if !validEmail(em) {
		e["owner.email"] = "invalid email"
	}
	if m := str(o["mobile"]); m == "" {
		e["owner.mobile"] = "required"
	} else if !ValidSaudiMobile(m) {
		e["owner.mobile"] = "Saudi mobile number (05XXXXXXXX)"
	}
	if p := str(o["password"]); len(p) < 8 {
		e["owner.password"] = "at least 8 characters"
	} else if passwordStrength(p) < 2 {
		e["owner.password"] = "too weak"
	}
	if strings.TrimSpace(str(c["nameEn"])) == "" {
		e["company.nameEn"] = "Registered company name is required"
	}
	if n := strings.TrimSpace(str(c["nameAr"])); n == "" {
		e["company.nameAr"] = "required"
	} else if !hasArabic(n) {
		e["company.nameAr"] = "must contain Arabic letters"
	}
	if v := str(c["vatNo"]); v == "" {
		e["company.vatNo"] = "required"
	} else if !ValidVAT(v) {
		e["company.vatNo"] = "15 digits, starting and ending with 3"
	}
	if v := str(c["crNo"]); v == "" {
		e["company.crNo"] = "required"
	} else if !ValidCR(v) {
		e["company.crNo"] = "10 digits"
	}
	if m := str(c["mobile"]); m == "" {
		e["company.mobile"] = "required"
	} else if !ValidSaudiPhone(m) {
		e["company.mobile"] = "Saudi phone number"
	}
	if v := str(a["buildingNo"]); !re4.MatchString(v) {
		e["company.address.buildingNo"] = "4 digits"
	}
	if strings.TrimSpace(str(a["streetEn"])) == "" {
		e["company.address.streetEn"] = "required"
	}
	if v := str(a["streetAr"]); v == "" || !hasArabic(v) {
		e["company.address.streetAr"] = "Arabic street name required"
	}
	if strings.TrimSpace(str(a["districtEn"])) == "" {
		e["company.address.districtEn"] = "required"
	}
	if v := str(a["districtAr"]); v == "" || !hasArabic(v) {
		e["company.address.districtAr"] = "Arabic district name required"
	}
	if strings.TrimSpace(str(a["cityEn"])) == "" {
		e["company.address.cityEn"] = "required"
	}
	if v := str(a["postalCode"]); !re5.MatchString(v) {
		e["company.address.postalCode"] = "5 digits"
	}
	if v := str(a["additionalNo"]); !re4.MatchString(v) {
		e["company.address.additionalNo"] = "4 digits"
	}
	if v := str(a["shortAddress"]); v != "" && !regexp.MustCompile(`^[A-Za-z]{4}\d{4}$`).MatchString(v) {
		e["company.address.shortAddress"] = "format AAAA9999"
	}
	if t := str(c["type"]); !signupTypes[t] && CategoryTerminal(t) == "" {
		e["company.type"] = "invalid business type"
	}
	if p := str(c["plan"]); !signupPlans[p] {
		e["company.plan"] = "invalid plan"
	}
	return e
}

func handleSignup(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if errs := ValidateSignup(body); len(errs) > 0 {
		writeErr(w, errBadRequest("", errs))
		return
	}
	o := sub(body, "owner")
	c := sub(body, "company")
	a := sub(c, "address")
	email := strings.ToLower(strings.TrimSpace(str(o["email"])))
	category := signupCategory(str(c["type"]))
	if findUserByEmailCI(email) != nil {
		writeErr(w, errf(http.StatusConflict, "email_taken", "An account with this e-mail already exists.", map[string]string{"owner.email": "already registered"}))
		return
	}
	// Delegate to the existing guest-registration flow (store + store DB +
	// indexes + user), so the new store is created exactly like the old app's.
	legacyReq := M{
		"name": str(o["name"]), "email": email, "mob": cleanPhone(str(o["mobile"])), "password": str(o["password"]),
		"store_name": str(c["nameEn"]), "store_name_in_arabic": str(c["nameAr"]),
		"business_category": category, "registration_number": str(c["crNo"]), "vat_no": str(c["vatNo"]),
		"phone": cleanPhone(str(c["mobile"])), "country_code": "SA", "country_name": "Saudi Arabia", "zatca_phase": "2",
		"national_address": addressToLegacy(a),
	}
	fake := &Ctx{R: r}
	res, _ := callV1(fake, controller.GuestRegister, "POST", "/v1/guest-register", nil, "", legacyReq)
	if !res.ok() {
		writeErr(w, legacyErr(res, map[string]string{"email": "owner.email", "name": "owner.name", "mob": "owner.mobile",
			"password": "owner.password", "vat_no": "company.vatNo", "registration_number": "company.crNo",
			"business_category": "company.type", "national_address_building_no": "company.address.buildingNo",
			"national_address_street_name": "company.address.streetEn", "national_address_district_name": "company.address.districtEn",
			"national_address_city_name": "company.address.cityEn", "national_address_zipcode": "company.address.postalCode"}, nil, ""))
		return
	}
	u := findUserByEmailCI(email)
	if u == nil {
		writeErr(w, errInternal("Account created but could not be loaded."))
		return
	}
	uid, _ := oidOf(u["_id"])
	storeHex := ""
	for _, s := range arr(u["store_ids"]) {
		storeHex = hexOf(s)
		break
	}
	sid, _ := oidOf(storeHex)
	now := nowFn().In(riyadh).Format(layoutDT)
	ctx, cancel := dbctx()
	defer cancel()
	// additive StartERP fields only
	_, _ = mainDB().Collection("user").UpdateOne(ctx, bson.M{"_id": uid}, bson.M{"$set": bson.M{
		"erp.role": "r_admin", "erp.v": int64(1), "erp.x.status": "active", "erp.x.lastLogin": now,
		"erp.h": bson.A{historyEntry(str(o["name"]), "created", []interface{}{})},
	}})
	short := deriveShort(str(c["nameEn"]))
	sloc := storeLocation(legacyReq) // the new store's zone (SA)
	_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": sid}, bson.M{"$set": bson.M{
		"erp.v": int64(1), "erp.x.short": short, "erp.x.plan": str(c["plan"]), "erp.x.branchAr": "الفرع الرئيسي",
		"erp.x.phone2": cleanPhone(str(c["mobile"])), "erp.x.trialEndsAt": time.Now().AddDate(0, 0, 14).In(sloc).Format(layoutDay),
		"erp.x.businessType": category, "erp.x.address": M{"countryAr": "المملكة العربية السعودية", "shortAddress": str(a["shortAddress"])},
		"erp.h": bson.A{historyEntryIn(sloc, str(o["name"]), "created", []interface{}{})},
	}})
	tok, err := models.GenerateAccesstoken(str(u["email"]))
	if err != nil {
		writeErr(w, errInternal("Unable to issue tokens."))
		return
	}
	u, _ = loadUser(uid)
	stores, _ := accessibleStores(u)
	var store M
	sb := newStoresResource().Backend.(*storesBackend)
	for _, s := range stores {
		if hexOf(s["_id"]) == storeHex {
			store = sb.render(newMapCtx(nil, ""), s, false)
		}
	}
	writeJSON(w, http.StatusCreated, M{"accessToken": tok.Token, "refreshToken": tok.RefreshToken,
		"user": userToContractFull(u, stores), "store": store})
}

// signupCategory is the business_category saved for a sign-up: the canonical
// spelling of a POS business category (ZATCA-safe), else the legacy type.
func signupCategory(t string) string {
	if v, ok := CanonicalCategory(t); ok {
		return v
	}
	return strings.TrimSpace(t)
}

func deriveShort(name string) string {
	words := strings.Fields(strings.ToUpper(name))
	ini := ""
	for _, w := range words {
		w = reNonUpper.ReplaceAllString(w, "")
		if w != "" {
			ini += w[:1]
		}
	}
	if len(ini) < 2 {
		ini = reNonUpper.ReplaceAllString(strings.ToUpper(name), "")
	}
	if len(ini) > 5 {
		ini = ini[:5]
	}
	if len(ini) < 2 {
		return "MAIN"
	}
	return ini
}
