package controller

import (
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

// ── pure-logic helpers that mirror the controller code ─────────────────────

// resolveCreateCode mirrors the create-path logic in CreateOrder:
// use supplied code when setting is on and code is non-empty,
// otherwise signal "auto-generate" by returning "".
// Returns (codeToUse, shouldAutoGenerate).
func resolveCreateCode(setting bool, suppliedCode string) (string, bool) {
	trimmed := strings.TrimSpace(suppliedCode)
	if setting && trimmed != "" {
		return trimmed, false // custom code
	}
	return "", true // auto-generate
}

// resolveUpdateCode mirrors the update-path logic in UpdateOrder.
// originalCode is the code fetched from DB; decodedCode is what the client sent.
// Returns the code that should be persisted.
func resolveUpdateCode(setting bool, originalCode, decodedCode string) (string, bool, string) {
	if !setting {
		return originalCode, false, "" // always preserve
	}
	trimmed := strings.TrimSpace(decodedCode)
	if trimmed == "" {
		return originalCode, false, "" // can't clear on update — restore
	}
	if trimmed == originalCode {
		return originalCode, false, "" // unchanged
	}
	// Changed — caller must check uniqueness; signal that with isDuplicate check needed.
	return trimmed, true, trimmed // (newCode, needsUniquenessCheck, codeToCheck)
}

// ── create-path tests ─────────────────────────────────────────────────────

func TestResolveCreateCode_SettingDisabled_AnyCode_AutoGenerates(t *testing.T) {
	code, autoGen := resolveCreateCode(false, "INV-001")
	if !autoGen {
		t.Error("expected auto-generate when setting is disabled")
	}
	if code != "" {
		t.Errorf("expected empty code, got %q", code)
	}
}

func TestResolveCreateCode_SettingEnabled_EmptyCode_AutoGenerates(t *testing.T) {
	code, autoGen := resolveCreateCode(true, "")
	if !autoGen {
		t.Error("expected auto-generate when code is empty")
	}
	if code != "" {
		t.Errorf("expected empty code, got %q", code)
	}
}

func TestResolveCreateCode_SettingEnabled_WhitespaceOnly_AutoGenerates(t *testing.T) {
	code, autoGen := resolveCreateCode(true, "   ")
	if !autoGen {
		t.Error("expected auto-generate when code is whitespace only")
	}
	if code != "" {
		t.Errorf("expected empty code, got %q", code)
	}
}

func TestResolveCreateCode_SettingEnabled_NonEmptyCode_UsesCustom(t *testing.T) {
	code, autoGen := resolveCreateCode(true, "INV-001")
	if autoGen {
		t.Error("expected custom code, not auto-generate")
	}
	if code != "INV-001" {
		t.Errorf("expected INV-001, got %q", code)
	}
}

func TestResolveCreateCode_SettingEnabled_CodeTrimmed(t *testing.T) {
	code, autoGen := resolveCreateCode(true, "  INV-002  ")
	if autoGen {
		t.Error("expected custom code after trim")
	}
	if code != "INV-002" {
		t.Errorf("expected INV-002 (trimmed), got %q", code)
	}
}

func TestResolveCreateCode_SettingDisabled_EmptyCode_AutoGenerates(t *testing.T) {
	_, autoGen := resolveCreateCode(false, "")
	if !autoGen {
		t.Error("expected auto-generate when setting disabled and code empty")
	}
}

// ── update-path tests ─────────────────────────────────────────────────────

func TestResolveUpdateCode_SettingDisabled_AlwaysPreservesOriginal(t *testing.T) {
	got, needsCheck, _ := resolveUpdateCode(false, "INV-001", "INV-999")
	if got != "INV-001" {
		t.Errorf("expected original code INV-001, got %q", got)
	}
	if needsCheck {
		t.Error("no uniqueness check needed when setting is disabled")
	}
}

func TestResolveUpdateCode_SettingEnabled_EmptyDecoded_PreservesOriginal(t *testing.T) {
	got, needsCheck, _ := resolveUpdateCode(true, "INV-001", "")
	if got != "INV-001" {
		t.Errorf("expected original code INV-001, got %q", got)
	}
	if needsCheck {
		t.Error("no uniqueness check when clearing is attempted")
	}
}

func TestResolveUpdateCode_SettingEnabled_WhitespaceDecoded_PreservesOriginal(t *testing.T) {
	got, needsCheck, _ := resolveUpdateCode(true, "INV-001", "   ")
	if got != "INV-001" {
		t.Errorf("expected original code INV-001, got %q", got)
	}
	if needsCheck {
		t.Error("no uniqueness check when clearing is attempted with whitespace")
	}
}

func TestResolveUpdateCode_SettingEnabled_SameCode_NoUniquenessCheck(t *testing.T) {
	got, needsCheck, _ := resolveUpdateCode(true, "INV-001", "INV-001")
	if got != "INV-001" {
		t.Errorf("expected INV-001, got %q", got)
	}
	if needsCheck {
		t.Error("no uniqueness check needed when code is unchanged")
	}
}

func TestResolveUpdateCode_SettingEnabled_NewCode_NeedsUniquenessCheck(t *testing.T) {
	got, needsCheck, codeToCheck := resolveUpdateCode(true, "INV-001", "INV-002")
	if got != "INV-002" {
		t.Errorf("expected INV-002, got %q", got)
	}
	if !needsCheck {
		t.Error("uniqueness check required when code changes")
	}
	if codeToCheck != "INV-002" {
		t.Errorf("codeToCheck: expected INV-002, got %q", codeToCheck)
	}
}

func TestResolveUpdateCode_SettingEnabled_NewCodeTrimmed(t *testing.T) {
	got, needsCheck, codeToCheck := resolveUpdateCode(true, "INV-001", "  INV-003  ")
	if got != "INV-003" {
		t.Errorf("expected INV-003 (trimmed), got %q", got)
	}
	if !needsCheck {
		t.Error("uniqueness check required for new code")
	}
	if codeToCheck != "INV-003" {
		t.Errorf("codeToCheck should be trimmed, got %q", codeToCheck)
	}
}

// ── StoreSettings field presence ─────────────────────────────────────────

func TestStoreSettings_EnableCustomSalesInvoiceID_DefaultFalse(t *testing.T) {
	var s models.StoreSettings
	if s.EnableCustomSalesInvoiceID {
		t.Error("EnableCustomSalesInvoiceID should default to false (disabled)")
	}
}

func TestStoreSettings_EnableCustomSalesInvoiceID_CanBeEnabled(t *testing.T) {
	s := models.StoreSettings{EnableCustomSalesInvoiceID: true}
	if !s.EnableCustomSalesInvoiceID {
		t.Error("EnableCustomSalesInvoiceID should be true when set")
	}
}

// ── behavioural invariants ────────────────────────────────────────────────

func TestCustomInvoiceID_CreateInvariants(t *testing.T) {
	cases := []struct {
		name        string
		setting     bool
		code        string
		wantAutoGen bool
	}{
		{"setting_off_empty", false, "", true},
		{"setting_off_provided", false, "X-001", true},
		{"setting_on_empty", true, "", true},
		{"setting_on_provided", true, "X-001", false},
		{"setting_on_whitespace", true, "  ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, autoGen := resolveCreateCode(tc.setting, tc.code)
			if autoGen != tc.wantAutoGen {
				t.Errorf("autoGen: got %v, want %v", autoGen, tc.wantAutoGen)
			}
		})
	}
}

func TestCustomInvoiceID_UpdateNeverClearsCode(t *testing.T) {
	// On update, the code must never become empty regardless of what the client sends.
	originals := []string{"INV-001", "CUSTOM-XYZ", "2024-0001"}
	for _, orig := range originals {
		t.Run(orig, func(t *testing.T) {
			// Try to clear with empty
			got, _, _ := resolveUpdateCode(true, orig, "")
			if got == "" {
				t.Errorf("update must not clear invoice ID for original=%q", orig)
			}
			// Try to clear with whitespace
			got, _, _ = resolveUpdateCode(true, orig, "   ")
			if got == "" {
				t.Errorf("update must not clear invoice ID (whitespace) for original=%q", orig)
			}
		})
	}
}

// ── auto-generate retry logic ─────────────────────────────────────────────

// simulateAutoGen mimics CreateOrder's code-generation branch.
// When settingEnabled=false it calls generator exactly once (original path).
// When settingEnabled=true it retries up to maxRetries times, skipping taken codes.
// Returns the accepted code and number of MakeCode calls made.
func simulateAutoGen(settingEnabled bool, taken map[string]bool, generator func() string, maxRetries int) (string, int) {
	if !settingEnabled {
		// Original path — single call, no uniqueness check.
		return generator(), 1
	}
	// Retry path.
	for attempt := 0; attempt < maxRetries; attempt++ {
		code := generator()
		if !taken[code] {
			return code, attempt + 1
		}
	}
	return "", maxRetries // exhausted
}

// simulateAutoGenWithRetry is kept as an alias so existing tests compile.
func simulateAutoGenWithRetry(taken map[string]bool, generator func() string, maxRetries int) (string, int) {
	return simulateAutoGen(true, taken, generator, maxRetries)
}

func TestAutoGen_SettingDisabled_ExactlyOneCall_NoDuplicateCheck(t *testing.T) {
	// When the setting is off, MakeCode is called exactly once regardless of
	// what's in the DB — the uniqueness check must never run.
	calls := 0
	gen := func() string { calls++; return "INV-00005" }
	// "INV-00005" is in the DB — but with setting off we must never check.
	taken := map[string]bool{"INV-00005": true}

	code, attempts := simulateAutoGen(false, taken, gen, 10)
	if code != "INV-00005" {
		t.Errorf("expected INV-00005 (setting off = no check), got %q", code)
	}
	if attempts != 1 {
		t.Errorf("setting disabled must call MakeCode exactly once, got %d", attempts)
	}
	if calls != 1 {
		t.Errorf("generator must be called exactly once, got %d", calls)
	}
}

func TestAutoGen_SettingDisabled_NeverRetries(t *testing.T) {
	// Confirm: even if every generated code is "taken", setting=off means one call only.
	calls := 0
	gen := func() string { calls++; return "TAKEN" }
	taken := map[string]bool{"TAKEN": true}

	_, attempts := simulateAutoGen(false, taken, gen, 10)
	if attempts != 1 || calls != 1 {
		t.Errorf("setting disabled must never retry; attempts=%d calls=%d", attempts, calls)
	}
}

func TestAutoGenRetry_NoCollision_OneAttempt(t *testing.T) {
	seq := []string{"INV-00001"}
	i := 0
	gen := func() string { c := seq[i]; i++; return c }

	code, attempts := simulateAutoGenWithRetry(map[string]bool{}, gen, 10)
	if code != "INV-00001" {
		t.Errorf("expected INV-00001, got %q", code)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestAutoGenRetry_OneCollision_TwoAttempts(t *testing.T) {
	// Counter hits 5 which is taken by a custom ID; counter 6 is free.
	seq := []string{"INV-00005", "INV-00006"}
	i := 0
	gen := func() string { c := seq[i]; i++; return c }
	taken := map[string]bool{"INV-00005": true}

	code, attempts := simulateAutoGenWithRetry(taken, gen, 10)
	if code != "INV-00006" {
		t.Errorf("expected INV-00006, got %q", code)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestAutoGenRetry_MultipleCollisions_SkipsAll(t *testing.T) {
	// Three consecutive custom IDs happen to occupy slots 5, 6, 7.
	seq := []string{"INV-00005", "INV-00006", "INV-00007", "INV-00008"}
	i := 0
	gen := func() string { c := seq[i]; i++; return c }
	taken := map[string]bool{"INV-00005": true, "INV-00006": true, "INV-00007": true}

	code, attempts := simulateAutoGenWithRetry(taken, gen, 10)
	if code != "INV-00008" {
		t.Errorf("expected INV-00008, got %q", code)
	}
	if attempts != 4 {
		t.Errorf("expected 4 attempts, got %d", attempts)
	}
}

func TestAutoGenRetry_ExhaustedRetries_ReturnsEmpty(t *testing.T) {
	// All generated codes are taken — should exhaust maxRetries=3 and return "".
	seq := []string{"X", "X", "X"}
	i := 0
	gen := func() string { c := seq[i%len(seq)]; i++; return c }
	taken := map[string]bool{"X": true}

	code, attempts := simulateAutoGenWithRetry(taken, gen, 3)
	if code != "" {
		t.Errorf("expected empty (exhausted), got %q", code)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (maxRetries), got %d", attempts)
	}
}
