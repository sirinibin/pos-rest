package erp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Card Bridge pure functions and public endpoints (no database).

func TestPairingCode_Format(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c := newPairingCode()
		if len(c) != 9 || c[4] != '-' || normPairingCode(c) != c {
			t.Fatalf("code %q", c)
		}
		for _, r := range strings.ReplaceAll(c, "-", "") {
			if !strings.ContainsRune(bridgeCodeAlphabet, r) {
				t.Fatalf("look-alike %q in %q", r, c)
			}
		}
		seen[c] = true
	}
	if len(seen) < 190 {
		t.Fatalf("codes repeat too often: %d unique", len(seen))
	}
	for in, want := range map[string]string{"abcd-efgh": "ABCD-EFGH", " ab cd ef gh ": "ABCD-EFGH", "ABCDEFGH": "ABCD-EFGH",
		"ABCD-EFG": "", "ABCD-EFGO": "", "ABCD-EFG1": "", "": "", "ABCD-EFGHJ": ""} {
		if got := normPairingCode(in); got != want {
			t.Errorf("normPairingCode(%q) = %q want %q", in, got, want)
		}
	}
}

func TestBridgeToken_Shape(t *testing.T) {
	a, b := newBridgeToken(), newBridgeToken()
	if a == b || !strings.HasPrefix(a, "cb1_") || len(a) < 40 || len(a) > 100 {
		t.Fatalf("tokens %q %q", a, b)
	}
	if sha256Hex(a) == a || len(sha256Hex(a)) != 64 {
		t.Fatal("token hash")
	}
}

func TestTerminalProviders_BridgeConnect(t *testing.T) {
	for id, want := range map[string]string{"neoleap": "bridge", "geidea": "bridge", "knet": "bridge", "alhamrani": "bridge",
		"bridge-simulator": "bridge", "nearpay": "api", "adyen": "api", "simulator": "api", "snb": "manual"} {
		p := terminalProviders[id]
		if p == nil {
			t.Fatalf("missing provider %s", id)
		}
		if p.connect() != want {
			t.Errorf("%s connect = %s want %s", id, p.connect(), want)
		}
		if want == "bridge" {
			if _, ok := p.adapter().(bridgeAdapter); !ok {
				t.Errorf("%s should use the bridge adapter", id)
			}
		}
	}
	if terminalProviders["snb"].adapter() != nil {
		t.Error("manual providers have no adapter")
	}
	// every driver the catalog names exists in cardbridge/
	known := map[string]bool{"neoleap-ws": true, "geidea-webecr": true, "knet-esocket": true, "alhamrani-signalr": true, "simulator": true}
	for _, id := range terminalProviderIDs() {
		for _, d := range terminalProviders[id].BridgeDrivers {
			if !known[d] {
				t.Errorf("%s names unknown driver %s", id, d)
			}
		}
	}
	if defaultProviderConfig(terminalProviders["bridge-simulator"]).Enabled && os.Getenv("CARD_TERMINAL_SIMULATOR") != "1" {
		t.Error("the bridge test machine must be off by default")
	}
}

func TestTerminalDriver(t *testing.T) {
	p := terminalProviders["neoleap"]
	if terminalDriver(p, M{}) != "neoleap-ws" || terminalDriver(p, M{"driver": "x"}) != "x" || terminalDriver(terminalProviders["snb"], M{}) != "" {
		t.Fatal("terminalDriver")
	}
	set, errs := bson_M(), map[string]string{}
	bridgeTerminalFields("000000000000000000000000", p, M{"driver": "nope"}, M{"provider": "neoleap"}, set, errs)
	if errs["driver"] == "" {
		t.Fatal("unknown driver accepted")
	}
}

func bson_M() map[string]interface{} { return map[string]interface{}{} }

func TestBridgeDownloads_Public(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CARD_BRIDGE_DIST", dir)
	r := call(t, "GET", "/card-bridge/downloads", "", nil)
	if r.Code != 200 || r.Body["available"] != false {
		t.Fatalf("no builds: %d %s", r.Code, r.Raw)
	}
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":"1.0.0","files":[{"os":"windows","arch":"amd64","file":"StartERP-CardBridge-windows-x64.exe","size":3}]}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "StartERP-CardBridge-windows-x64.exe"), []byte("exe"), 0o644)
	r = call(t, "GET", "/card-bridge/downloads", "", nil)
	if r.Code != 200 || r.Body["available"] != true || r.Body["version"] != "1.0.0" {
		t.Fatalf("manifest: %d %s", r.Code, r.Raw)
	}
	d := call(t, "GET", "/card-bridge/download/StartERP-CardBridge-windows-x64.exe", "", nil)
	if d.Code != 200 || d.Raw != "exe" || !strings.Contains(d.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %q %v", d.Code, d.Raw, d.Header)
	}
	for _, bad := range []string{"manifest.json", "..%2Fsecret", "nope.exe", ".hidden"} {
		if r := call(t, "GET", "/card-bridge/download/"+bad, "", nil); r.Code == 200 {
			t.Errorf("%s: %d", bad, r.Code)
		}
	}
}

func TestBridgeEndpoints_NoToken(t *testing.T) {
	for _, tok := range []string{"", "nope", "cb1_" + strings.Repeat("x", 120)} {
		r := call(t, "GET", "/card-bridge/jobs?wait=0", "", nil, "X-Card-Bridge-Token", tok)
		if r.Code != 401 || r.errCode() != "unauthorized" {
			t.Errorf("token %q: %d %s", tok, r.Code, r.Raw)
		}
	}
	if r := call(t, "POST", "/card-bridge/pair", "", M{"code": "12"}); r.Code != 400 || r.errField("code") == "" {
		t.Fatalf("pair validation: %d %s", r.Code, r.Raw)
	}
}
