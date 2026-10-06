package erp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Subscription billing by manual bank transfer.
//
// There is no payment gateway: the customer pays the company account from
// their banking app, then submits the transfer reference and a receipt (image
// or PDF). A platform admin (legacy Admin, who sees every store) checks the
// bank statement and accepts or rejects the submission. Accepting extends the
// store's paid period; rejecting records a reason the customer can read.
//
// Collections (main DB, additive):
//
//	erp_billing_settings  {_id: "bank_account", …}   company bank details shown to customers
//	erp_billing_payment   one submission per document (no receipt bytes)
//	erp_billing_receipt   {_id: payment _id, name, type, size, data}  the uploaded receipt
//
// The store keeps its subscription in erp.x.subscription (server-owned, see
// storesBackend.Update), next to erp.x.plan and erp.x.trialEndsAt from sign-up.
const (
	collBillingSettings = "erp_billing_settings"
	collBillingPayment  = "erp_billing_payment"
	collBillingReceipt  = "erp_billing_receipt"
	bankAccountDocID    = "bank_account"
)

// BillingVATRate is added on top of the plan price (prices exclude VAT).
const BillingVATRate = 0.15

// MaxReceiptBytes caps an uploaded receipt (decoded size).
const MaxReceiptBytes = 5 << 20

// BillingPlans are the self-serve plans payable by bank transfer (monthly
// price in SAR, excluding VAT). Enterprise is priced by sales and is not
// payable here. Keep in sync with the pricing on the landing page.
var BillingPlans = []M{
	{"id": "starter", "name": "Starter", "nameAr": "الأساسية", "monthly": 99.0},
	{"id": "professional", "name": "Professional", "nameAr": "الاحترافية", "monthly": 299.0},
}

var billingPeriods = map[string]int{"monthly": 1, "yearly": 12}

var billingStatuses = map[string]bool{"pending": true, "accepted": true, "rejected": true, "cancelled": true}

var receiptTypes = map[string]string{
	"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "application/pdf": "pdf",
}

func billingPlan(id string) M {
	for _, p := range BillingPlans {
		if p["id"] == id {
			return p
		}
	}
	return nil
}

// PlanPrice returns subtotal (excl. VAT), VAT and total for a plan and
// period. Yearly is ten months' price ("2 months free"). ok=false for an
// unknown plan or period.
func PlanPrice(plan, period string) (subtotal, vat, total float64, ok bool) {
	p := billingPlan(plan)
	months, okp := billingPeriods[period]
	if p == nil || !okp {
		return 0, 0, 0, false
	}
	m := num(p["monthly"])
	if months == 12 {
		subtotal = m * 10
	} else {
		subtotal = m
	}
	vat = round2(subtotal * BillingVATRate)
	return round2(subtotal), vat, round2(subtotal + vat), true
}

// ---- pure helpers (unit-tested) ----

var reIBANChars = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{10,30}$`)

// NormalizeIBAN removes spaces and upper-cases.
func NormalizeIBAN(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

// ValidIBAN checks the ISO 13616 shape and the mod-97 checksum. Saudi IBANs
// (SA) must be exactly 24 characters.
func ValidIBAN(s string) bool {
	s = NormalizeIBAN(s)
	if !reIBANChars.MatchString(s) {
		return false
	}
	if strings.HasPrefix(s, "SA") && len(s) != 24 {
		return false
	}
	r := s[4:] + s[:4]
	var b strings.Builder
	for _, ch := range r {
		if ch >= 'A' && ch <= 'Z' {
			b.WriteString(strconv.Itoa(int(ch-'A') + 10))
		} else {
			b.WriteRune(ch)
		}
	}
	n, ok := new(big.Int).SetString(b.String(), 10)
	if !ok {
		return false
	}
	return new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

var reTransferRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ./_-]{3,63}$`)

// ValidTransferReference: 4–64 characters, letters, digits, space . / _ -
func ValidTransferReference(s string) bool { return reTransferRef.MatchString(strings.TrimSpace(s)) }

// DecodeReceipt parses a data URL ("data:<type>;base64,<data>"), checks the
// declared type is an allowed image/PDF type, that the bytes really are that
// type, and the size limit. It returns the content type and decoded bytes.
func DecodeReceipt(dataURL string) (string, []byte, string) {
	if !strings.HasPrefix(dataURL, "data:") {
		return "", nil, "must be a data URL"
	}
	comma := strings.IndexByte(dataURL, ',')
	if comma < 0 {
		return "", nil, "must be a data URL"
	}
	meta := dataURL[5:comma]
	if !strings.HasSuffix(meta, ";base64") {
		return "", nil, "must be base64 encoded"
	}
	ctype := strings.ToLower(strings.TrimSuffix(meta, ";base64"))
	if _, ok := receiptTypes[ctype]; !ok {
		return "", nil, "only JPG, PNG, WEBP images or PDF files"
	}
	payload := dataURL[comma+1:]
	if base64.StdEncoding.DecodedLen(len(payload)) > MaxReceiptBytes+3 {
		return "", nil, fmt.Sprintf("file is larger than %d MB", MaxReceiptBytes>>20)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, "invalid base64 data"
	}
	if len(data) == 0 {
		return "", nil, "file is empty"
	}
	if len(data) > MaxReceiptBytes {
		return "", nil, fmt.Sprintf("file is larger than %d MB", MaxReceiptBytes>>20)
	}
	if !sniffMatches(ctype, data) {
		return "", nil, "file content does not match its type"
	}
	return ctype, data, ""
}

func sniffMatches(ctype string, b []byte) bool {
	switch ctype {
	case "application/pdf":
		return bytes.HasPrefix(b, []byte("%PDF-"))
	case "image/png":
		return bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n"))
	case "image/jpeg":
		return bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF})
	case "image/webp":
		return len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
	}
	return false
}

var reUnsafeFileChars = regexp.MustCompile(`[^\p{L}\p{N} ._()-]+`)

// SafeFileName keeps a display name for the receipt (no paths or odd characters).
func SafeFileName(name, ctype string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(reUnsafeFileChars.ReplaceAllString(name, "_"))
	if r := []rune(name); len(r) > 120 {
		name = string(r[len(r)-120:])
	}
	if name == "" || strings.Trim(name, "._ ") == "" {
		name = "receipt." + receiptTypes[ctype]
	}
	return name
}

func parseDay(s string) (time.Time, bool) {
	t, err := time.ParseInLocation(layoutDay, strings.TrimSpace(s), riyadh)
	return t, err == nil
}

func todayRiyadh() time.Time {
	n := nowFn().In(riyadh)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, riyadh)
}

// SubscriptionStatus derives the customer-facing status from the stored dates
// (YYYY-MM-DD, inclusive): active (paid), trial, expired, or none (a store
// with no billing data, e.g. created by the old app).
func SubscriptionStatus(paidUntil, trialEndsAt string, today time.Time) string {
	if p, ok := parseDay(paidUntil); ok && !p.Before(today) {
		return "active"
	}
	if t, ok := parseDay(trialEndsAt); ok && !t.Before(today) {
		return "trial"
	}
	if paidUntil != "" || trialEndsAt != "" {
		return "expired"
	}
	return "none"
}

// NextPaidPeriod returns the period an accepted payment covers. It starts the
// day after whatever the store already has (paid period or free trial), or
// today when both have ended, and runs for the period's months (inclusive).
func NextPaidPeriod(paidUntil, trialEndsAt, period string, today time.Time) (start, end time.Time) {
	start = today
	for _, s := range []string{paidUntil, trialEndsAt} {
		if d, ok := parseDay(s); ok && !d.Before(start) {
			start = d.AddDate(0, 0, 1)
		}
	}
	months := billingPeriods[period]
	if months == 0 {
		months = 1
	}
	end = start.AddDate(0, months, -1)
	return start, end
}

// ValidatePaymentSubmission checks a customer's submission body (receipt
// content is checked separately by DecodeReceipt).
func ValidatePaymentSubmission(body M, today time.Time) map[string]string {
	e := map[string]string{}
	if str(body["storeId"]) == "" {
		e["storeId"] = "required"
	}
	plan := str(body["plan"])
	if plan == "enterprise" {
		e["plan"] = "Enterprise is billed through sales"
	} else if billingPlan(plan) == nil {
		e["plan"] = "invalid plan"
	}
	if _, ok := billingPeriods[str(body["period"])]; !ok {
		e["period"] = "monthly or yearly"
	}
	if ref := strings.TrimSpace(str(body["reference"])); ref == "" {
		e["reference"] = "required"
	} else if !ValidTransferReference(ref) {
		e["reference"] = "4–64 letters, digits, spaces or . / _ -"
	}
	if d := str(body["transferDate"]); d == "" {
		e["transferDate"] = "required"
	} else if t, ok := parseDay(d); !ok {
		e["transferDate"] = "date YYYY-MM-DD"
	} else if t.After(today) {
		e["transferDate"] = "cannot be in the future"
	} else if t.Before(today.AddDate(0, 0, -90)) {
		e["transferDate"] = "older than 90 days"
	}
	if n := strings.TrimSpace(str(body["payerName"])); n == "" {
		e["payerName"] = "required"
	} else if len([]rune(n)) > 120 {
		e["payerName"] = "at most 120 characters"
	}
	if len([]rune(str(body["payerBank"]))) > 80 {
		e["payerBank"] = "at most 80 characters"
	}
	if len([]rune(str(body["note"]))) > 500 {
		e["note"] = "at most 500 characters"
	}
	rc := sub(body, "receipt")
	if str(rc["data"]) == "" {
		e["receipt"] = "attach the bank receipt (image or PDF)"
	}
	return e
}

// ValidateBankAccount checks the admin's bank account settings.
func ValidateBankAccount(body M) map[string]string {
	e := map[string]string{}
	if strings.TrimSpace(str(body["bankName"])) == "" {
		e["bankName"] = "required"
	}
	if strings.TrimSpace(str(body["accountName"])) == "" {
		e["accountName"] = "required"
	}
	if iban := str(body["iban"]); strings.TrimSpace(iban) == "" {
		e["iban"] = "required"
	} else if !ValidIBAN(iban) {
		e["iban"] = "invalid IBAN"
	}
	for k, max := range map[string]int{"bankName": 80, "bankNameAr": 80, "accountName": 120, "accountNameAr": 120,
		"accountNumber": 34, "swift": 11, "branch": 80, "instructions": 1000, "instructionsAr": 1000} {
		if len([]rune(str(body[k]))) > max {
			e[k] = fmt.Sprintf("at most %d characters", max)
		}
	}
	if s := strings.TrimSpace(str(body["swift"])); s != "" && !regexp.MustCompile(`^[A-Za-z]{6}[A-Za-z0-9]{2}([A-Za-z0-9]{3})?$`).MatchString(s) {
		e["swift"] = "8 or 11 characters"
	}
	if s := strings.TrimSpace(str(body["accountNumber"])); s != "" && !regexp.MustCompile(`^[0-9 -]{4,34}$`).MatchString(s) {
		e["accountNumber"] = "digits only"
	}
	return e
}

// ---- storage ----

func billingSettings() *mongo.Collection { return mainDB().Collection(collBillingSettings) }
func billingPayments() *mongo.Collection { return mainDB().Collection(collBillingPayment) }
func billingReceipts() *mongo.Collection { return mainDB().Collection(collBillingReceipt) }

var bankFields = []string{"bankName", "bankNameAr", "accountName", "accountNameAr", "iban", "accountNumber",
	"swift", "branch", "instructions", "instructionsAr"}

func loadBankAccount() M {
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	out := M{"configured": false}
	for _, k := range bankFields {
		out[k] = ""
	}
	if err := billingSettings().FindOne(ctx, bson.M{"_id": bankAccountDocID}).Decode(&raw); err != nil {
		return out
	}
	d := normDoc(raw)
	for _, k := range bankFields {
		out[k] = str(d[k])
	}
	out["updatedAt"] = str(d["updatedAt"])
	out["updatedBy"] = str(d["updatedBy"])
	out["configured"] = true
	return out
}

// subscriptionOf renders a store's subscription (customer view).
func subscriptionOf(store M, pending M) M {
	x := sub(sub(store, envKey), "x")
	s := sub(x, "subscription")
	paidUntil := str(s["paidUntil"])
	trial := str(x["trialEndsAt"])
	out := M{
		"storeId": hexOf(store["_id"]), "plan": str(x["plan"]), "period": str(s["period"]),
		"trialEndsAt": trial, "paidUntil": paidUntil, "status": SubscriptionStatus(paidUntil, trial, todayRiyadh()),
		"lastPaymentId": str(s["lastPaymentId"]), "activatedAt": str(s["activatedAt"]), "pendingPaymentId": "",
	}
	if out["plan"] == "" {
		out["plan"] = str(s["plan"])
	}
	if pending != nil {
		out["pendingPaymentId"] = str(pending["id"])
	}
	return out
}

func paymentOut(d M) M {
	out := M{}
	for k, v := range d {
		if k == "_id" || strings.HasPrefix(k, "_") {
			continue
		}
		out[k] = v
	}
	out["id"] = hexOf(d["_id"])
	if _, ok := out["history"]; !ok {
		out["history"] = []interface{}{}
	}
	return out
}

func findPayment(id string) (M, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := billingPayments().FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errNotFound()
		}
		return nil, errInternal("Unable to load the payment.")
	}
	return normDoc(raw), nil
}

func pendingPaymentFor(storeHex string) M {
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := billingPayments().FindOne(ctx, bson.M{"storeId": storeHex, "status": "pending"}).Decode(&raw); err != nil {
		return nil
	}
	return paymentOut(normDoc(raw))
}

var billingIndexOnce sync.Once

// ensureBillingIndexes: at most one pending payment per store (a partial
// unique index, so two simultaneous submissions cannot both get in).
func ensureBillingIndexes() {
	billingIndexOnce.Do(func() {
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = billingPayments().Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "storeId", Value: 1}}, Options: options.Index().SetName("one_pending_per_store").SetUnique(true).
				SetPartialFilterExpression(bson.M{"status": "pending"})},
			{Keys: bson.D{{Key: "status", Value: 1}, {Key: "submittedAt", Value: -1}}},
			{Keys: bson.D{{Key: "storeId", Value: 1}, {Key: "submittedAt", Value: -1}}},
		})
	})
}

// ---- access ----

// billingStore resolves a store the caller may see billing for (verb "view")
// or pay for (verb "edit": settings edit, i.e. the store's owner/admin).
// Platform admins pass for every store.
func billingStore(c *Ctx, storeHex, verb string) (M, error) {
	if storeHex == "" {
		return nil, errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	s := c.store(storeHex)
	if s == nil {
		return nil, errNotFound()
	}
	if !c.Admin && !c.can("settings", verb) {
		if verb == "edit" {
			return nil, errForbidden("Only the store's administrator can pay for the subscription.")
		}
		return nil, errForbidden("")
	}
	return s, nil
}

func requirePlatformAdmin(c *Ctx) error {
	if !c.Admin {
		return errForbidden("Only StartERP administrators can do this.")
	}
	return nil
}

// authed wraps a billing handler with authentication and error writing.
func authed(fn func(c *Ctx, w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := authenticate(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := fn(c, w, r); err != nil {
			writeErr(w, err)
		}
	}
}

func registerBilling(s *mux.Router) {
	s.HandleFunc("/billing/plans", authed(handleBillingPlans)).Methods("GET")
	s.HandleFunc("/billing/bank-account", authed(handleGetBankAccount)).Methods("GET")
	s.HandleFunc("/billing/bank-account", authed(handlePutBankAccount)).Methods("PUT")
	s.HandleFunc("/billing/subscription", authed(handleGetSubscription)).Methods("GET")
	s.HandleFunc("/billing/payments/summary", authed(handlePaymentSummary)).Methods("GET")
	s.HandleFunc("/billing/payments", authed(handleListPayments)).Methods("GET")
	s.HandleFunc("/billing/payments", authed(handleSubmitPayment)).Methods("POST")
	s.HandleFunc("/billing/payments/{id}", authed(handleGetPayment)).Methods("GET")
	s.HandleFunc("/billing/payments/{id}/receipt", authed(handleGetReceipt)).Methods("GET")
	s.HandleFunc("/billing/payments/{id}/accept", authed(handleAcceptPayment)).Methods("POST")
	s.HandleFunc("/billing/payments/{id}/reject", authed(handleRejectPayment)).Methods("POST")
	s.HandleFunc("/billing/payments/{id}/cancel", authed(handleCancelPayment)).Methods("POST")
}

// ---- handlers ----

func handleBillingPlans(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	plans := []M{}
	for _, p := range BillingPlans {
		row := cloneM(p)
		for _, per := range []string{"monthly", "yearly"} {
			st, vat, total, _ := PlanPrice(str(p["id"]), per)
			row[per] = M{"subtotal": st, "vat": vat, "total": total}
		}
		plans = append(plans, row)
	}
	writeJSON(w, http.StatusOK, M{"plans": plans, "vatRate": BillingVATRate, "currency": "SAR"})
	return nil
}

func handleGetBankAccount(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	fs, err := parseSelect(r.URL.Query().Get("select"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, fs.apply(loadBankAccount()))
	return nil
}

func handlePutBankAccount(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	if errs := ValidateBankAccount(body); len(errs) > 0 {
		return errBadRequest("", errs)
	}
	set := bson.M{}
	for _, k := range bankFields {
		v := strings.TrimSpace(str(body[k]))
		if k == "iban" {
			v = NormalizeIBAN(v)
		}
		if k == "swift" {
			v = strings.ToUpper(v)
		}
		set[k] = v
	}
	set["updatedAt"] = nowFn().In(riyadh).Format(layoutDT)
	set["updatedBy"] = c.UserName
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := billingSettings().UpdateOne(ctx, bson.M{"_id": bankAccountDocID}, bson.M{"$set": set}, options.Update().SetUpsert(true)); err != nil {
		return errInternal("Unable to save the bank account.")
	}
	writeJSON(w, http.StatusOK, loadBankAccount())
	return nil
}

func handleGetSubscription(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex := r.URL.Query().Get("storeId")
	s, err := billingStore(c, storeHex, "view")
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, subscriptionOf(s, pendingPaymentFor(storeHex)))
	return nil
}

// paymentFilter builds the list filter: platform admins see every store (or
// one with storeId); everyone else must name a store they can view.
func paymentFilter(c *Ctx, r *http.Request) (bson.M, error) {
	q := r.URL.Query()
	f := bson.M{}
	storeHex := q.Get("storeId")
	if !c.Admin || storeHex != "" {
		if _, err := billingStore(c, storeHex, "view"); err != nil {
			return nil, err
		}
		f["storeId"] = storeHex
	}
	if st := q.Get("status"); st != "" {
		list := []string{}
		for _, s := range strings.Split(st, ",") {
			if !billingStatuses[s] {
				return nil, errBadRequest("Invalid status.", map[string]string{"status": "pending, accepted, rejected or cancelled"})
			}
			list = append(list, s)
		}
		f["status"] = bson.M{"$in": list}
	}
	if s := strings.TrimSpace(q.Get("q")); s != "" {
		re := primitive.Regex{Pattern: regexp.QuoteMeta(s), Options: "i"}
		f["$or"] = bson.A{bson.M{"reference": re}, bson.M{"storeName": re}, bson.M{"payerName": re}, bson.M{"number": re}}
	}
	return f, nil
}

func handleListPayments(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	f, err := paymentFilter(c, r)
	if err != nil {
		return err
	}
	fs, err := parseSelect(r.URL.Query().Get("select"))
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 500 {
		limit = 100
	}
	ctx, cancel := dbctx()
	defer cancel()
	total, err := billingPayments().CountDocuments(ctx, f)
	if err != nil {
		return errInternal("Unable to list payments.")
	}
	opts := options.Find().SetSort(bson.D{{Key: "submittedAt", Value: -1}, {Key: "_id", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).SetLimit(int64(limit))
	if !fs.wants("history") {
		opts.SetProjection(bson.M{"history": 0})
	}
	cur, err := billingPayments().Find(ctx, f, opts)
	if err != nil {
		return errInternal("Unable to list payments.")
	}
	defer cur.Close(ctx)
	rows := []M{}
	for cur.Next(ctx) {
		rows = append(rows, paymentOut(bsonToM(cur.Current)))
	}
	writeJSON(w, http.StatusOK, M{"data": fs.applyAll(rows), "total": total, "page": page, "limit": limit})
	return nil
}

func handlePaymentSummary(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	f, err := paymentFilter(c, r)
	if err != nil {
		return err
	}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := billingPayments().Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: f}},
		{{Key: "$group", Value: bson.M{"_id": "$status", "count": bson.M{"$sum": 1}, "amount": bson.M{"$sum": "$amount"}}}},
	})
	if err != nil {
		return errInternal("Unable to summarise payments.")
	}
	defer cur.Close(ctx)
	out := M{}
	for s := range billingStatuses {
		out[s] = M{"count": int64(0), "amount": 0.0}
	}
	for cur.Next(ctx) {
		d := bsonToM(cur.Current)
		out[str(d["_id"])] = M{"count": intv(d["count"]), "amount": round2(num(d["amount"]))}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// visiblePayment loads a payment the caller may see.
func visiblePayment(c *Ctx, id string) (M, error) {
	d, err := findPayment(id)
	if err != nil {
		return nil, err
	}
	if !c.Admin {
		if _, err := billingStore(c, str(d["storeId"]), "view"); err != nil {
			if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
				return nil, errNotFound()
			}
			return nil, err
		}
	}
	return d, nil
}

func handleGetPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	fs, err := parseSelect(r.URL.Query().Get("select"))
	if err != nil {
		return err
	}
	d, err := visiblePayment(c, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, fs.apply(paymentOut(d)))
	return nil
}

func handleGetReceipt(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	d, err := visiblePayment(c, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	oid, _ := oidOf(d["_id"])
	var rc struct {
		Name string `bson:"name"`
		Type string `bson:"type"`
		Size int64  `bson:"size"`
		Data []byte `bson:"data"`
	}
	ctx, cancel := dbctx()
	defer cancel()
	if err := billingReceipts().FindOne(ctx, bson.M{"_id": oid}).Decode(&rc); err != nil {
		return errNotFound()
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, M{"name": rc.Name, "type": rc.Type, "size": rc.Size,
		"data": "data:" + rc.Type + ";base64," + base64.StdEncoding.EncodeToString(rc.Data)})
	return nil
}

func nextPaymentNumber() string {
	ctx, cancel := dbctx()
	defer cancel()
	var res struct {
		Seq int64 `bson:"seq"`
	}
	err := billingSettings().FindOneAndUpdate(ctx, bson.M{"_id": "payment_seq"}, bson.M{"$inc": bson.M{"seq": 1}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)).Decode(&res)
	if err != nil {
		return "PAY-" + strconv.FormatInt(nowFn().Unix(), 36)
	}
	return fmt.Sprintf("PAY-%06d", res.Seq)
}

func handleSubmitPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	today := todayRiyadh()
	errs := ValidatePaymentSubmission(body, today)
	rc := sub(body, "receipt")
	var ctype string
	var data []byte
	if _, missing := errs["receipt"]; !missing {
		var msg string
		ctype, data, msg = DecodeReceipt(str(rc["data"]))
		if msg != "" {
			errs["receipt"] = msg
		}
	}
	if len(errs) > 0 {
		return errBadRequest("", errs)
	}
	storeHex := str(body["storeId"])
	store, err := billingStore(c, storeHex, "edit")
	if err != nil {
		return err
	}
	ensureBillingIndexes()
	ref := strings.TrimSpace(str(body["reference"]))
	ctx, cancel := dbctx()
	defer cancel()
	if pendingPaymentFor(storeHex) != nil {
		return errf(http.StatusConflict, "pending_exists",
			"A payment for this store is already waiting for verification. Cancel it to submit a new one.", nil)
	}
	refRe := primitive.Regex{Pattern: "^" + regexp.QuoteMeta(ref) + "$", Options: "i"}
	if n, _ := billingPayments().CountDocuments(ctx, bson.M{"reference": refRe, "status": bson.M{"$in": bson.A{"pending", "accepted"}}}); n > 0 {
		return errf(http.StatusConflict, "duplicate_reference", "This transfer reference was already submitted.",
			map[string]string{"reference": "already submitted"})
	}
	plan, period := str(body["plan"]), str(body["period"])
	subtotal, vat, total, _ := PlanPrice(plan, period)
	oid := primitive.NewObjectID()
	now := nowFn().In(riyadh).Format(layoutDT)
	doc := bson.M{
		"_id": oid, "number": nextPaymentNumber(), "storeId": storeHex, "storeName": str(store["name"]),
		"storeNameAr": str(store["name_in_arabic"]), "storeVatNo": str(store["vat_no"]),
		"plan": plan, "period": period, "currency": "SAR", "subtotal": subtotal, "vat": vat, "amount": total,
		"reference": ref, "transferDate": str(body["transferDate"]), "payerName": strings.TrimSpace(str(body["payerName"])),
		"payerBank": strings.TrimSpace(str(body["payerBank"])), "note": strings.TrimSpace(str(body["note"])),
		"receipt": M{"name": SafeFileName(str(rc["name"]), ctype), "type": ctype, "size": int64(len(data))},
		"status":  "pending", "submittedAt": now, "submittedBy": c.UserName, "submittedById": c.UserID.Hex(),
		"submittedByEmail": str(c.User["email"]), "reviewedAt": "", "reviewedBy": "", "rejectionReason": "", "adminNote": "",
		"periodStart": "", "periodEnd": "", "version": int64(1),
		"history": bson.A{historyEntry(c.UserName, "submitted", []interface{}{})},
	}
	if _, err := billingReceipts().InsertOne(ctx, bson.M{"_id": oid, "name": doc["receipt"].(M)["name"], "type": ctype,
		"size": int64(len(data)), "data": data, "storeId": storeHex}); err != nil {
		return errInternal("Unable to save the receipt.")
	}
	if _, err := billingPayments().InsertOne(ctx, doc); err != nil {
		_, _ = billingReceipts().DeleteOne(ctx, bson.M{"_id": oid})
		if mongo.IsDuplicateKeyError(err) {
			return errf(http.StatusConflict, "pending_exists",
				"A payment for this store is already waiting for verification. Cancel it to submit a new one.", nil)
		}
		return errInternal("Unable to save the payment.")
	}
	d, err := findPayment(oid.Hex())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, paymentOut(d))
	return nil
}

// transition atomically moves a pending payment to a new status; a payment
// that is no longer pending (someone else reviewed it) is a 409.
func transition(id string, set bson.M, h M) (M, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	err = billingPayments().FindOneAndUpdate(ctx, bson.M{"_id": oid, "status": "pending"},
		bson.M{"$set": set, "$inc": bson.M{"version": 1}, "$push": bson.M{"history": h}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		if _, ferr := findPayment(id); ferr != nil {
			return nil, ferr
		}
		return nil, errf(http.StatusConflict, "not_pending", "This payment was already reviewed or cancelled.", nil)
	}
	if err != nil {
		return nil, errInternal("Unable to update the payment.")
	}
	return normDoc(raw), nil
}

func handleAcceptPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	note := strings.TrimSpace(str(body["note"]))
	if len([]rune(note)) > 500 {
		return errBadRequest("", map[string]string{"note": "at most 500 characters"})
	}
	id := mux.Vars(r)["id"]
	cur, err := findPayment(id)
	if err != nil {
		return err
	}
	if str(cur["status"]) != "pending" {
		return errf(http.StatusConflict, "not_pending", "This payment was already reviewed or cancelled.", nil)
	}
	storeHex := str(cur["storeId"])
	sid, ok := oidOf(storeHex)
	if !ok {
		return errInternal("Payment has no store.")
	}
	var storeRaw bson.M
	ctx, cancel := dbctx()
	defer cancel()
	if err := mainDB().Collection("store").FindOne(ctx, bson.M{"_id": sid}).Decode(&storeRaw); err != nil {
		return errf(http.StatusConflict, "store_missing", "The store for this payment no longer exists.", nil)
	}
	store := normDoc(storeRaw)
	x := sub(sub(store, envKey), "x")
	start, end := NextPaidPeriod(str(get(x, "subscription.paidUntil")), str(x["trialEndsAt"]), str(cur["period"]), todayRiyadh())
	now := nowFn().In(riyadh).Format(layoutDT)
	d, err := transition(id, bson.M{"status": "accepted", "reviewedAt": now, "reviewedBy": c.UserName,
		"reviewedById": c.UserID.Hex(), "adminNote": note, "periodStart": start.Format(layoutDay), "periodEnd": end.Format(layoutDay)},
		historyEntry(c.UserName, "accepted", []interface{}{}))
	if err != nil {
		return err
	}
	_, err = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": sid}, bson.M{
		"$set": bson.M{
			"erp.x.plan": str(cur["plan"]),
			"erp.x.subscription": bson.M{"plan": str(cur["plan"]), "period": str(cur["period"]), "status": "active",
				"paidUntil": end.Format(layoutDay), "lastPaymentId": id, "activatedAt": now},
		},
		"$inc": bson.M{"erp.v": int64(1)},
		"$push": bson.M{"erp.h": bson.M{"$each": bson.A{historyEntry(c.UserName, "subscription paid", []interface{}{
			M{"field": "paidUntil", "from": str(get(x, "subscription.paidUntil")), "to": end.Format(layoutDay)}})}, "$slice": -maxHistory}},
	})
	if err != nil {
		// keep payment and store consistent: put the payment back to pending
		_, _ = billingPayments().UpdateOne(ctx, bson.M{"_id": d["_id"]}, bson.M{"$set": bson.M{"status": "pending", "reviewedAt": "",
			"reviewedBy": "", "periodStart": "", "periodEnd": ""}})
		return errInternal("Unable to activate the subscription.")
	}
	writeJSON(w, http.StatusOK, paymentOut(d))
	return nil
}

func handleRejectPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	reason := strings.TrimSpace(str(body["reason"]))
	if n := len([]rune(reason)); n < 5 {
		return errBadRequest("", map[string]string{"reason": "tell the customer why (at least 5 characters)"})
	} else if n > 500 {
		return errBadRequest("", map[string]string{"reason": "at most 500 characters"})
	}
	now := nowFn().In(riyadh).Format(layoutDT)
	d, err := transition(mux.Vars(r)["id"], bson.M{"status": "rejected", "reviewedAt": now, "reviewedBy": c.UserName,
		"reviewedById": c.UserID.Hex(), "rejectionReason": reason}, historyEntry(c.UserName, "rejected", []interface{}{}))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, paymentOut(d))
	return nil
}

func handleCancelPayment(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	id := mux.Vars(r)["id"]
	cur, err := visiblePayment(c, id)
	if err != nil {
		return err
	}
	if !c.Admin {
		if _, err := billingStore(c, str(cur["storeId"]), "edit"); err != nil {
			return err
		}
	}
	now := nowFn().In(riyadh).Format(layoutDT)
	d, err := transition(id, bson.M{"status": "cancelled", "reviewedAt": now, "reviewedBy": c.UserName},
		historyEntry(c.UserName, "cancelled", []interface{}{}))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, paymentOut(d))
	return nil
}
