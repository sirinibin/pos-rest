package erp

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Pure-function tests for card terminals (no MongoDB).

func TestSealSecret_RoundTripAndHint(t *testing.T) {
	for _, plain := range []string{"", "k", "sk_test_123456", "مفتاح-سري-١٢٣٤", strings.Repeat("x", 500)} {
		s, err := sealSecret(plain)
		if err != nil {
			t.Fatalf("seal %q: %v", plain, err)
		}
		if plain == "" {
			if s != "" {
				t.Fatalf("empty must stay empty, got %q", s)
			}
			continue
		}
		if !strings.HasPrefix(s, sealedPrefix) || (len(plain) > 6 && strings.Contains(s, plain)) {
			t.Fatalf("not sealed: %q", s)
		}
		s2, _ := sealSecret(plain)
		if s2 == s {
			t.Fatalf("same plain text must seal differently each time (random nonce)")
		}
		back, err := openSecret(s)
		if err != nil || back != plain {
			t.Fatalf("open %q: %q %v", plain, back, err)
		}
	}
	if _, err := openSecret("plain"); err == nil {
		t.Fatal("unsealed text must not open")
	}
	if _, err := openSecret(sealedPrefix + "AAAA"); err == nil {
		t.Fatal("short / tampered secret must not open")
	}
	s, _ := sealSecret("abcdef")
	tampered := s[:len(s)-2] + "AA"
	if v, err := openSecret(tampered); err == nil && v == "abcdef" {
		t.Fatal("tampered secret must not open to the plain text")
	}
	cases := map[string]string{"": "", "abc": "••••", "abcd": "••••", "abcde": "••••bcde", "sk_live_9876": "••••9876"}
	for in, want := range cases {
		if got := secretHint(in); got != want {
			t.Errorf("secretHint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSealSecret_KeyMatters(t *testing.T) {
	s, _ := sealSecret("merchant-secret")
	old := secretKeyFn
	defer func() { secretKeyFn = old }()
	secretKeyFn = func() []byte { k := make([]byte, 32); k[0] = 9; return k }
	if _, err := openSecret(s); err == nil {
		t.Fatal("a different key must not open the secret")
	}
}

func TestMergeFields(t *testing.T) {
	defs := []terminalField{
		{Key: "terminalId", Required: true, Max: 10},
		{Key: "apiKey", Secret: true, Required: true},
		{Key: "note"},
	}
	cur := map[string]string{"terminalId": "T1", "apiKey": "old-secret", "note": "n"}
	tests := []struct {
		name    string
		body    M
		require bool
		want    map[string]string
		errKeys []string
	}{
		{"nothing given keeps all", M{}, true, cur, nil},
		{"secret empty string keeps it", M{"apiKey": ""}, true, cur, nil},
		{"secret null clears (then required)", M{"apiKey": nil}, true, map[string]string{"terminalId": "T1", "apiKey": "", "note": "n"}, []string{"f.apiKey"}},
		{"secret replaced", M{"apiKey": " new "}, true, map[string]string{"terminalId": "T1", "apiKey": "new", "note": "n"}, nil},
		{"plain emptied", M{"note": ""}, true, map[string]string{"terminalId": "T1", "apiKey": "old-secret", "note": ""}, nil},
		{"too long", M{"terminalId": "12345678901"}, true, nil, []string{"f.terminalId"}},
		{"not text", M{"terminalId": 5.0}, true, nil, []string{"f.terminalId"}},
		{"newline", M{"note": "a\nb"}, true, nil, []string{"f.note"}},
		{"required off", M{"terminalId": ""}, false, map[string]string{"terminalId": "", "apiKey": "old-secret", "note": "n"}, nil},
		{"required on", M{"terminalId": ""}, true, nil, []string{"f.terminalId"}},
	}
	for _, tc := range tests {
		got, errs := mergeFields(defs, cur, tc.body, "f.", tc.require)
		for _, k := range tc.errKeys {
			if errs[k] == "" {
				t.Errorf("%s: expected error on %s, got %v", tc.name, k, errs)
			}
		}
		if len(tc.errKeys) == 0 && len(errs) > 0 {
			t.Errorf("%s: unexpected errors %v", tc.name, errs)
		}
		if tc.want != nil {
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s: %s = %q, want %q", tc.name, k, got[k], v)
				}
			}
		}
	}
	if cur["apiKey"] != "old-secret" {
		t.Fatal("mergeFields must not change its input")
	}
}

func TestFieldsOut_NeverShowsSecrets(t *testing.T) {
	defs := []terminalField{{Key: "terminalId"}, {Key: "apiKey", Secret: true}}
	out := fieldsOut(defs, map[string]string{"terminalId": "T-9", "apiKey": "sk_live_abcdef"})
	if out["terminalId"] != "T-9" {
		t.Fatalf("plain field: %v", out)
	}
	sec := out["apiKey"].(M)
	if sec["set"] != true || sec["hint"] != "••••cdef" {
		t.Fatalf("secret: %v", sec)
	}
	if strings.Contains(strings.Join([]string{str(sec["hint"])}, ""), "sk_live") {
		t.Fatal("secret leaked")
	}
	if fieldsOut(defs, map[string]string{})["apiKey"].(M)["set"] != false {
		t.Fatal("unset secret must say set=false")
	}
}

func TestValidAmount(t *testing.T) {
	cases := []struct {
		v    interface{}
		dec  int
		want float64
		bad  bool
	}{
		{10.5, 2, 10.5, false}, {0.01, 2, 0.01, false}, {1.234, 3, 1.234, false}, {int64(7), 2, 7, false},
		{1.234, 2, 0, true}, {0.0, 2, 0, true}, {-5.0, 2, 0, true}, {10000000.0, 2, 10000000, false},
		{10000000.01, 2, 0, true}, {"5", 2, 0, true}, {nil, 2, 0, true}, {0.1 + 0.2, 2, 0.3, false},
		{12.5, 0, 0, true}, {12.0, 0, 12, false},
	}
	for _, c := range cases {
		got, msg := validAmount(c.v, c.dec)
		if c.bad != (msg != "") || (!c.bad && got != c.want) {
			t.Errorf("validAmount(%v, %d) = %v %q", c.v, c.dec, got, msg)
		}
	}
}

func TestMinorUnits(t *testing.T) {
	cases := []struct {
		amt  float64
		dec  int
		want int64
	}{{10.05, 2, 1005}, {0.29, 2, 29}, {1.005, 3, 1005}, {12.345, 3, 12345}, {100, 0, 100}, {19.99, 2, 1999}, {0.1 + 0.2, 2, 30}}
	for _, c := range cases {
		if got := (terminalPayReq{Amount: c.amt, Decimals: c.dec}).minorUnits(); got != c.want {
			t.Errorf("minorUnits(%v,%d) = %d, want %d", c.amt, c.dec, got, c.want)
		}
	}
}

func TestSimulatorOutcomes(t *testing.T) {
	old := nowFn
	defer func() { nowFn = old }()
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return t0 }
	a := simulatorAdapter{}
	cfg := terminalCfg{Terminal: "TEST-1"}
	if err := a.Check(context.TODO(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.Check(context.TODO(), terminalCfg{}); err == nil {
		t.Fatal("check without a terminal name must fail")
	}
	cases := []struct {
		amount float64
		dec    int
		final  string
	}{{25, 2, tpApproved}, {10.05, 2, tpDeclined}, {10.06, 2, tpPending}, {1.105, 3, tpDeclined}, {99.99, 2, tpApproved}}
	for _, c := range cases {
		req := terminalPayReq{PaymentID: "p1", Amount: c.amount, Decimals: c.dec}
		r, err := a.Start(context.TODO(), cfg, req)
		if err != nil || r.Status != tpPending || r.ProviderRef == "" {
			t.Fatalf("start %v: %+v %v", c.amount, r, err)
		}
		nowFn = func() time.Time { return t0.Add(time.Second) }
		if s, _ := a.Status(context.TODO(), cfg, req, r.ProviderRef); s.Status != tpPending {
			t.Fatalf("%v: before the delay: %s", c.amount, s.Status)
		}
		nowFn = func() time.Time { return t0.Add(simulatorApproveAfter) }
		s, _ := a.Status(context.TODO(), cfg, req, r.ProviderRef)
		if s.Status != c.final {
			t.Fatalf("%v: got %s, want %s", c.amount, s.Status, c.final)
		}
		if s.Status == tpApproved && (len(s.AuthCode) != 6 || len(s.RRN) != 12 || !strings.Contains(s.Message, "TEST")) {
			t.Fatalf("approved details: %+v", s)
		}
		nowFn = func() time.Time { return t0 }
	}
	if r, _ := a.Start(context.TODO(), cfg, terminalPayReq{PaymentID: "p", Amount: 3.07, Decimals: 2}); r.Status != tpFailed {
		t.Fatalf(".07 must fail: %+v", r)
	}
	if r, _ := a.Cancel(context.TODO(), cfg, terminalPayReq{}, "sim_x_1"); r.Status != tpCancelled {
		t.Fatalf("cancel: %+v", r)
	}
	if _, err := a.Status(context.TODO(), cfg, terminalPayReq{}, "other"); err == nil {
		t.Fatal("unknown ref must error")
	}
}

func TestWebhookSig(t *testing.T) {
	a := webhookSig("simulator", "s1", "p1")
	if len(a) != 32 || a != webhookSig("simulator", "s1", "p1") {
		t.Fatalf("sig: %q", a)
	}
	for _, other := range []string{webhookSig("simulator", "s1", "p2"), webhookSig("simulator", "s2", "p1"), webhookSig("geidea", "s1", "p1")} {
		if other == a {
			t.Fatal("signature must depend on provider, store and payment")
		}
	}
	t.Setenv("API_PUBLIC_URL", "")
	if terminalWebhookURL("simulator", "s1", "p1") != "" {
		t.Fatal("no public URL → no webhook")
	}
	t.Setenv("API_PUBLIC_URL", "https://api.example/")
	if u := terminalWebhookURL("simulator", "s1", "p1"); u != "https://api.example/v1/erp/card-terminal-webhooks/simulator/s1/p1?sig="+a {
		t.Fatalf("url: %s", u)
	}
}

func TestTerminalCatalog_Consistent(t *testing.T) {
	if len(terminalProviders) < 2 {
		t.Fatalf("catalog too small: %d", len(terminalProviders))
	}
	gcc := map[string]bool{"SA": true, "AE": true, "OM": true, "QA": true, "BH": true, "KW": true, "IN": true}
	for id, p := range terminalProviders {
		if p.ID != id || p.NameEn == "" || len(p.Countries) == 0 {
			t.Errorf("%s: incomplete entry", id)
		}
		for _, c := range p.Countries {
			if !gcc[c] {
				t.Errorf("%s: unknown country %s", id, c)
			}
		}
		keys := map[string]bool{}
		for _, f := range append(append([]terminalField{}, p.StoreFields...), p.PartnerFields...) {
			if f.Key == "" || f.LabelEn == "" || keys[f.Key] {
				t.Errorf("%s: bad or duplicate field %q", id, f.Key)
			}
			keys[f.Key] = true
		}
		if p.connect() == "api" {
			if p.Model != "simulator" {
				if p.BaseURL["live"] == "" || !strings.HasPrefix(p.BaseURL["live"], "https://") {
					t.Errorf("%s: api provider needs an https live base URL", id)
				}
			}
			hasTid := false
			for _, f := range p.StoreFields {
				if f.Key == "terminalId" {
					hasTid = true
				}
			}
			if !hasTid {
				t.Errorf("%s: api provider needs a terminalId store field", id)
			}
		}
		row := p.catalogRow()
		if row["connect"] != p.connect() {
			t.Errorf("%s: catalog row connect", id)
		}
	}
	d := defaultProviderConfig(terminalProviders["simulator"])
	if d.Enabled {
		t.Error("the test terminal must be off by default")
	}
}

func TestPartnerReady(t *testing.T) {
	p := &terminalProviderSpec{PartnerFields: []terminalField{{Key: "a", Required: true}, {Key: "b"}}}
	if partnerReady(p, providerConfig{Partner: map[string]string{}}) {
		t.Fatal("missing required partner field")
	}
	if !partnerReady(p, providerConfig{Partner: map[string]string{"a": "x"}}) {
		t.Fatal("required set → ready")
	}
	c := providerConfig{Enabled: true, Countries: []string{"SA"}}
	if !c.offeredIn("SA") || c.offeredIn("AE") {
		t.Fatal("offeredIn")
	}
	c.Enabled = false
	if c.offeredIn("SA") {
		t.Fatal("disabled provider is not offered")
	}
}
