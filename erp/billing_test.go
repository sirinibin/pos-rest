package erp

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Pure-function and no-DB tests for billing.go (bank-transfer subscriptions).

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, ok := parseDay(s)
	if !ok {
		t.Fatalf("bad day %q", s)
	}
	return d
}

func dataURL(ctype string, b []byte) string {
	return "data:" + ctype + ";base64," + base64.StdEncoding.EncodeToString(b)
}

var (
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	pdfBytes  = []byte("%PDF-1.4\n%âãÏÓ\n1 0 obj\n<<>>\nendobj\ntrailer\n%%EOF")
	jpgBytes  = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 32)...)
	webpBytes = append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
)

func TestPlanPrice(t *testing.T) {
	cases := []struct {
		plan, period         string
		subtotal, vat, total float64
		ok                   bool
	}{
		{"starter", "monthly", 99, 14.85, 113.85, true},
		{"starter", "yearly", 990, 148.5, 1138.5, true},
		{"professional", "monthly", 299, 44.85, 343.85, true},
		{"professional", "yearly", 2990, 448.5, 3438.5, true},
		{"enterprise", "monthly", 0, 0, 0, false},
		{"starter", "weekly", 0, 0, 0, false},
		{"", "", 0, 0, 0, false},
	}
	for _, c := range cases {
		s, v, tot, ok := PlanPrice(c.plan, c.period)
		if ok != c.ok || s != c.subtotal || v != c.vat || tot != c.total {
			t.Errorf("PlanPrice(%s,%s) = %v %v %v %v, want %v %v %v %v", c.plan, c.period, s, v, tot, ok, c.subtotal, c.vat, c.total, c.ok)
		}
	}
}

func TestValidIBAN(t *testing.T) {
	good := []string{
		"SA0380000000608010167519",      // published Saudi example
		"sa03 8000 0000 6080 1016 7519", // spaces + lower case
		"GB82WEST12345698765432",
		"DE89370400440532013000",
	}
	for _, s := range good {
		if !ValidIBAN(s) {
			t.Errorf("ValidIBAN(%q) = false", s)
		}
	}
	bad := []string{
		"", "SA", "SA0380000000608010167518", // checksum
		"SA03800000006080101675191", // SA must be 24 chars
		"SA038000000060801016751",   // too short for SA
		"GB82WEST1234569876543!",    // bad char
		"1234567890123456",          // no country
	}
	for _, s := range bad {
		if ValidIBAN(s) {
			t.Errorf("ValidIBAN(%q) = true", s)
		}
	}
	if NormalizeIBAN(" sa03 8000\t0000 ") != "SA0380000000" {
		t.Error("NormalizeIBAN")
	}
}

func TestValidTransferReference(t *testing.T) {
	for _, s := range []string{"FT24123ABC", "1234", "TRX-2026/10/06_01", "  REF 0001  ", strings.Repeat("A", 64)} {
		if !ValidTransferReference(s) {
			t.Errorf("want valid %q", s)
		}
	}
	for _, s := range []string{"", "abc", "-ABCD", "REF#1", "<script>", strings.Repeat("A", 65), "مرجع1234"} {
		if ValidTransferReference(s) {
			t.Errorf("want invalid %q", s)
		}
	}
}

func TestDecodeReceipt(t *testing.T) {
	for ctype, b := range map[string][]byte{"image/png": pngBytes, "application/pdf": pdfBytes, "image/jpeg": jpgBytes, "image/webp": webpBytes} {
		got, data, msg := DecodeReceipt(dataURL(ctype, b))
		if msg != "" || got != ctype || len(data) != len(b) {
			t.Errorf("%s: %q %q %d", ctype, msg, got, len(data))
		}
	}
	// declared type is matched case-insensitively
	if _, _, msg := DecodeReceipt(dataURL("IMAGE/PNG", pngBytes)); msg != "" {
		t.Errorf("upper-case type: %s", msg)
	}
	bad := map[string]string{
		"not a data url":                                  "data URL",
		"data:image/png,abc":                              "base64",
		"data:image/png;base64":                           "data URL",
		dataURL("image/gif", []byte("GIF89a")):            "only JPG",
		dataURL("text/html", []byte("<html>")):            "only JPG",
		dataURL("image/svg+xml", []byte("<svg/>")):        "only JPG",
		"data:image/png;base64,@@@@":                      "invalid base64",
		"data:image/png;base64,":                          "empty",
		dataURL("image/png", pdfBytes):                    "does not match",
		dataURL("application/pdf", pngBytes):              "does not match",
		dataURL("image/webp", []byte("RIFF0000WAVEfmt ")): "does not match",
	}
	for in, want := range bad {
		if _, _, msg := DecodeReceipt(in); !strings.Contains(msg, want) {
			t.Errorf("DecodeReceipt(%.40q) = %q, want %q", in, msg, want)
		}
	}
	big := append(append([]byte{}, pdfBytes...), make([]byte, MaxReceiptBytes)...)
	if _, _, msg := DecodeReceipt(dataURL("application/pdf", big)); !strings.Contains(msg, "larger than 5 MB") {
		t.Errorf("oversize: %q", msg)
	}
	exact := append(append([]byte{}, pdfBytes...), make([]byte, MaxReceiptBytes-len(pdfBytes))...)
	if _, _, msg := DecodeReceipt(dataURL("application/pdf", exact)); msg != "" {
		t.Errorf("exactly 5 MB must pass: %q", msg)
	}
}

func TestSafeFileName(t *testing.T) {
	cases := map[[2]string]string{
		{"receipt.pdf", "application/pdf"}:           "receipt.pdf",
		{"C:\\fakepath\\bank slip.png", "image/png"}: "bank slip.png",
		{"../../etc/passwd", "image/png"}:            "passwd",
		{"<img onerror=x>.jpg", "image/jpeg"}:        "_img onerror_x_.jpg",
		{"", "image/webp"}:                           "receipt.webp",
		{"...", "application/pdf"}:                   "receipt.pdf",
		{"إيصال التحويل.pdf", "application/pdf"}:     "إيصال التحويل.pdf",
	}
	for in, want := range cases {
		if got := SafeFileName(in[0], in[1]); got != want {
			t.Errorf("SafeFileName(%q) = %q, want %q", in[0], got, want)
		}
	}
	if got := SafeFileName(strings.Repeat("a", 200)+".pdf", "application/pdf"); len([]rune(got)) != 120 || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("long name: %d %q", len(got), got[len(got)-8:])
	}
}

func TestSubscriptionStatus(t *testing.T) {
	today := day(t, "2026-10-06")
	cases := []struct{ paid, trial, want string }{
		{"2026-10-06", "", "active"}, // last paid day is inclusive
		{"2027-01-01", "2026-09-01", "active"},
		{"2026-10-05", "2026-10-20", "trial"},
		{"", "2026-10-06", "trial"},
		{"2026-10-05", "", "expired"},
		{"", "2026-10-05", "expired"},
		{"", "", "none"},
		{"garbage", "", "expired"},
	}
	for _, c := range cases {
		if got := SubscriptionStatus(c.paid, c.trial, today); got != c.want {
			t.Errorf("SubscriptionStatus(%q,%q) = %s, want %s", c.paid, c.trial, got, c.want)
		}
	}
}

func TestNextPaidPeriod(t *testing.T) {
	today := day(t, "2026-10-06")
	cases := []struct{ paid, trial, period, start, end string }{
		{"", "", "monthly", "2026-10-06", "2026-11-05"},
		{"", "", "yearly", "2026-10-06", "2027-10-05"},
		{"", "2026-10-16", "monthly", "2026-10-17", "2026-11-16"},           // paying during the trial keeps the trial days
		{"", "2026-09-01", "monthly", "2026-10-06", "2026-11-05"},           // ended trial: from today
		{"2026-12-31", "2026-10-16", "monthly", "2027-01-01", "2027-01-31"}, // renew early: extends the paid period
		{"2026-10-06", "", "monthly", "2026-10-07", "2026-11-06"},           // last day today
		{"2026-01-31", "", "monthly", "2026-10-06", "2026-11-05"},           // lapsed
		{"", "", "bogus", "2026-10-06", "2026-11-05"},
	}
	for _, c := range cases {
		s, e := NextPaidPeriod(c.paid, c.trial, c.period, today)
		if s.Format(layoutDay) != c.start || e.Format(layoutDay) != c.end {
			t.Errorf("NextPaidPeriod(%q,%q,%s) = %s..%s, want %s..%s", c.paid, c.trial, c.period,
				s.Format(layoutDay), e.Format(layoutDay), c.start, c.end)
		}
	}
	// month-end start: Jan 31 + 1 month - 1 day follows Go's normalisation (Mar 2/3), never before start
	s, e := NextPaidPeriod("2027-01-30", "", "monthly", today)
	if s.Format(layoutDay) != "2027-01-31" || !e.After(s) {
		t.Errorf("month end: %s..%s", s.Format(layoutDay), e.Format(layoutDay))
	}
}

func validPayment() M {
	return M{"storeId": "s1", "plan": "professional", "period": "monthly", "reference": "FT2610060001",
		"transferDate": "2026-10-05", "payerName": "Al Noor Trading", "payerBank": "Al Rajhi",
		"receipt": M{"name": "slip.pdf", "data": dataURL("application/pdf", pdfBytes)}}
}

func TestValidatePaymentSubmission(t *testing.T) {
	today := day(t, "2026-10-06")
	if e := ValidatePaymentSubmission(validPayment(), today); len(e) != 0 {
		t.Fatalf("valid payment: %v", e)
	}
	rows := []struct {
		name  string
		edit  func(M)
		field string
	}{
		{"no store", func(m M) { delete(m, "storeId") }, "storeId"},
		{"enterprise", func(m M) { m["plan"] = "enterprise" }, "plan"},
		{"bad plan", func(m M) { m["plan"] = "gold" }, "plan"},
		{"bad period", func(m M) { m["period"] = "weekly" }, "period"},
		{"no ref", func(m M) { m["reference"] = "  " }, "reference"},
		{"bad ref", func(m M) { m["reference"] = "ab" }, "reference"},
		{"no date", func(m M) { delete(m, "transferDate") }, "transferDate"},
		{"bad date", func(m M) { m["transferDate"] = "05/10/2026" }, "transferDate"},
		{"future date", func(m M) { m["transferDate"] = "2026-10-07" }, "transferDate"},
		{"old date", func(m M) { m["transferDate"] = "2026-07-07" }, "transferDate"},
		{"no payer", func(m M) { m["payerName"] = "" }, "payerName"},
		{"long payer", func(m M) { m["payerName"] = strings.Repeat("x", 121) }, "payerName"},
		{"long bank", func(m M) { m["payerBank"] = strings.Repeat("x", 81) }, "payerBank"},
		{"long note", func(m M) { m["note"] = strings.Repeat("x", 501) }, "note"},
		{"no receipt", func(m M) { delete(m, "receipt") }, "receipt"},
		{"empty receipt", func(m M) { m["receipt"] = M{"name": "x.pdf"} }, "receipt"},
	}
	for _, r := range rows {
		m := validPayment()
		r.edit(m)
		e := ValidatePaymentSubmission(m, today)
		if e[r.field] == "" || len(e) != 1 {
			t.Errorf("%s: %v", r.name, e)
		}
	}
	// boundaries: today and 90 days ago are accepted
	for _, d := range []string{"2026-10-06", "2026-07-08"} {
		m := validPayment()
		m["transferDate"] = d
		if e := ValidatePaymentSubmission(m, today); len(e) != 0 {
			t.Errorf("date %s: %v", d, e)
		}
	}
}

func TestValidateBankAccount(t *testing.T) {
	ok := M{"bankName": "Al Rajhi Bank", "accountName": "StartERP Co.", "iban": "SA03 8000 0000 6080 1016 7519",
		"swift": "RJHISARI", "accountNumber": "608010167519"}
	if e := ValidateBankAccount(ok); len(e) != 0 {
		t.Fatalf("valid: %v", e)
	}
	rows := []struct {
		edit  func(M)
		field string
	}{
		{func(m M) { m["bankName"] = "" }, "bankName"},
		{func(m M) { m["accountName"] = " " }, "accountName"},
		{func(m M) { m["iban"] = "" }, "iban"},
		{func(m M) { m["iban"] = "SA0380000000608010167518" }, "iban"},
		{func(m M) { m["swift"] = "RJHI" }, "swift"},
		{func(m M) { m["accountNumber"] = "ABC123" }, "accountNumber"},
		{func(m M) { m["instructions"] = strings.Repeat("x", 1001) }, "instructions"},
		{func(m M) { m["bankNameAr"] = strings.Repeat("x", 81) }, "bankNameAr"},
	}
	for i, r := range rows {
		m := cloneM(ok)
		r.edit(m)
		if e := ValidateBankAccount(m); e[r.field] == "" || len(e) != 1 {
			t.Errorf("row %d: %v", i, e)
		}
	}
}

func TestBillingEndpoints_Unauthenticated(t *testing.T) {
	eps := [][2]string{
		{"GET", "/billing/plans"}, {"GET", "/billing/bank-account"}, {"PUT", "/billing/bank-account"},
		{"GET", "/billing/subscription?storeId=x"}, {"GET", "/billing/payments"}, {"POST", "/billing/payments"},
		{"GET", "/billing/payments/summary"}, {"GET", "/billing/payments/x"}, {"GET", "/billing/payments/x/receipt"},
		{"POST", "/billing/payments/x/accept"}, {"POST", "/billing/payments/x/reject"}, {"POST", "/billing/payments/x/cancel"},
	}
	for _, e := range eps {
		for _, tok := range []string{"", "not-a-jwt"} {
			r := call(t, e[0], e[1], tok, M{})
			if r.Code != http.StatusUnauthorized || r.errCode() != "unauthorized" {
				t.Errorf("%s %s (token %q): %d %s", e[0], e[1], tok, r.Code, r.Raw)
			}
		}
	}
}

func TestStoreBillingFieldsAreServerOwned(t *testing.T) {
	want := map[string]bool{"plan": true, "trialEndsAt": true, "subscription": true}
	for _, k := range storeBillingFields {
		delete(want, k)
	}
	if len(want) != 0 {
		t.Fatalf("missing server-owned store fields: %v", want)
	}
}
