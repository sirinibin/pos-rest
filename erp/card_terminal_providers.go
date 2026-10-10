package erp

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Card terminal providers: the payment companies whose card machines
// StartERP can drive. Each provider has an adapter that pushes an amount to
// one terminal, reads the result back and cancels a payment still waiting on
// the terminal. Everything the browser needs to know (names, countries,
// credential fields, where the result comes from) is in the catalog below;
// the step-by-step guides live in the frontend (src/payments/terminalGuides.js).
//
// Two levels of configuration:
//   - platform (StartERP admin): which providers are offered in which
//     countries, sandbox or live, and the partner (ISV) credentials a
//     provider issues to StartERP itself (erp_card_terminal_provider).
//   - store (settings): each card machine the store owns, its provider,
//     terminal / device id and the merchant credentials the provider issued
//     to the store (erp_card_terminal in the store DB).

// terminalField is one credential / setting a provider needs.
type terminalField struct {
	Key      string `json:"key"`
	LabelEn  string `json:"labelEn"`
	LabelAr  string `json:"labelAr"`
	HintEn   string `json:"hintEn,omitempty"`
	HintAr   string `json:"hintAr,omitempty"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
	// Max characters (0 = 200)
	Max int `json:"max,omitempty"`
}

// terminalProviderSpec is a catalog entry (seeddata/card_terminal_providers.json,
// the same file as starterp-frontend-v1 src/payments/terminalProviders.json,
// which also carries the step-by-step guides).
type terminalProviderSpec struct {
	ID     string `json:"id"`
	NameEn string `json:"nameEn"`
	NameAr string `json:"nameAr"`
	// Countries the provider can serve (store.countryCode values).
	Countries []string `json:"countries"`
	// Model: "cloud" (the provider's cloud pushes to the terminal over the
	// internet), "simulator" (StartERP test terminal, never real money) or
	// "manual" (no public connection: the payment is recorded by hand).
	Model string `json:"model"`
	// Integration: how the provider connects ("cloud-api", "partner-api",
	// "android-app", "lan-ecr", "contact-provider", "simulator").
	Integration string `json:"integration"`
	// Schemes the terminals accept, for the settings screen.
	Schemes []string `json:"schemes"`
	// Webhook: the provider can call us back with the result (polling is
	// always used as well).
	Webhook bool `json:"webhook"`
	// Refunds: the provider can refund on the terminal.
	Refunds bool `json:"refunds"`
	// Sandbox: the provider offers test credentials / a test environment.
	Sandbox bool `json:"sandbox"`
	// BridgeDrivers: Card Bridge drivers that reach this provider's machines
	// from the shop's computer (LAN / USB / serial ECR, see cardbridge/).
	BridgeDrivers []string `json:"bridgeDrivers,omitempty"`
	// Partner fields are kept by the StartERP admin, store fields by the store.
	PartnerFields []terminalField `json:"partnerFields"`
	StoreFields   []terminalField `json:"storeFields"`
	// Base URLs per environment ("sandbox", "live").
	BaseURL   map[string]string `json:"baseUrl"`
	Website   string            `json:"website"`
	DocsURL   string            `json:"docsUrl"`
	SignupURL string            `json:"signupUrl"`
}

//go:embed seeddata/card_terminal_providers.json
var terminalProvidersJSON []byte

// terminalAdapters: providers StartERP can drive, by id. Adapters register
// themselves in package variables (before init loads the catalog).
var terminalAdapters = map[string]terminalAdapter{}

func registerTerminalAdapter(id string, a terminalAdapter) bool {
	terminalAdapters[id] = a
	return true
}

func init() {
	var f struct {
		Providers []*terminalProviderSpec `json:"providers"`
	}
	if err := json.Unmarshal(terminalProvidersJSON, &f); err != nil {
		panic("card_terminal_providers.json: " + err.Error())
	}
	for _, p := range f.Providers {
		terminalProviders[p.ID] = p
	}
}

func (p *terminalProviderSpec) adapter() terminalAdapter {
	if a := terminalAdapters[p.ID]; a != nil {
		return a
	}
	if len(p.BridgeDrivers) > 0 {
		return bridgeAdapter{}
	}
	return nil
}

// terminalCfg is everything an adapter call gets: the environment, the base
// URL and the (opened) partner + store credentials.
type terminalCfg struct {
	Env      string
	BaseURL  string
	Partner  map[string]string
	Store    map[string]string
	Terminal string // provider's terminal / device id (store field "terminalId")
	HTTP     *http.Client
	// Card Bridge machines: the store, the machine's record, the paired
	// bridge (shop computer) and the bridge driver that talks to it.
	StoreHex   string
	TerminalID string
	BridgeID   string
	Driver     string
}

func (c terminalCfg) get(k string) string {
	if v := strings.TrimSpace(c.Store[k]); v != "" {
		return v
	}
	return strings.TrimSpace(c.Partner[k])
}

// terminalPayReq is one payment pushed to a terminal.
type terminalPayReq struct {
	PaymentID string  // our id, sent as the provider's order / reference id
	Reference string  // the sale's reference (bill number / client ref)
	Amount    float64 // in currency units
	Currency  string  // ISO 4217
	Decimals  int     // minor units of the currency
	Kind      string  // "sale" or "refund"
	// RefundOf is the provider reference of the original payment (refunds).
	RefundOf string
	// WebhookURL: our call-back for this payment ("" when not configured).
	WebhookURL string
}

// minorUnits converts an amount to the currency's minor units (halalas, fils, baisa).
func (p terminalPayReq) minorUnits() int64 {
	f := 1.0
	for i := 0; i < p.Decimals; i++ {
		f *= 10
	}
	v := p.Amount * f
	if v < 0 {
		return int64(v - 0.5)
	}
	return int64(v + 0.5)
}

// Terminal payment statuses (the contract).
const (
	tpPending   = "pending"   // on the terminal, waiting for the card
	tpApproved  = "approved"  // money taken
	tpDeclined  = "declined"  // card / issuer said no
	tpCancelled = "cancelled" // cancelled on the till or the terminal
	tpFailed    = "failed"    // could not reach the terminal / provider error
	tpTimeout   = "timeout"   // nobody presented a card in time
)

var tpFinal = map[string]bool{tpApproved: true, tpDeclined: true, tpCancelled: true, tpFailed: true, tpTimeout: true}

// terminalResult is what an adapter reports.
type terminalResult struct {
	Status      string
	ProviderRef string // provider's id for this payment
	AuthCode    string
	RRN         string
	MaskedPan   string
	Scheme      string
	Message     string
}

// terminalAdapter talks to one provider. Start returns quickly with
// tpPending (or a final status for synchronous APIs); Status is polled until
// the payment is final.
type terminalAdapter interface {
	Check(ctx context.Context, cfg terminalCfg) error
	Start(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error)
	Status(ctx context.Context, cfg terminalCfg, req terminalPayReq, providerRef string) (terminalResult, error)
	Cancel(ctx context.Context, cfg terminalCfg, req terminalPayReq, providerRef string) (terminalResult, error)
}

// terminalRunner is an adapter whose payment call blocks until the card is
// done (NearPay, Adyen sync). StartERP runs it in the background and writes the
// result onto the payment when it returns; the till keeps polling our server.
// Cancel on such an adapter asks the terminal to abort, which makes Run return.
type terminalRunner interface {
	Run(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error)
}

// terminalTimeout: a payment still pending this long after it was sent is
// asked to cancel and reported as timed out.
var terminalTimeout = 3 * time.Minute

var terminalProviders = map[string]*terminalProviderSpec{}

func terminalProviderIDs() []string {
	ids := make([]string, 0, len(terminalProviders))
	for id := range terminalProviders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (p *terminalProviderSpec) servesCountry(cc string) bool {
	for _, c := range p.Countries {
		if c == cc {
			return true
		}
	}
	return false
}

// catalogRow is the public description of a provider (no secrets).
func (p *terminalProviderSpec) catalogRow() M {
	return M{
		"id": p.ID, "nameEn": p.NameEn, "nameAr": p.NameAr, "countries": p.Countries,
		"model": p.Model, "schemes": p.Schemes, "webhook": p.Webhook, "refunds": p.Refunds,
		"storeFields": p.StoreFields, "partnerFields": p.PartnerFields,
		"website": p.Website, "docsUrl": p.DocsURL, "signupUrl": p.SignupURL, "connect": p.connect(),
		"sandbox": p.Sandbox, "integration": p.Integration, "bridgeDrivers": nonNilStrings(p.BridgeDrivers),
	}
}

// connect is how StartERP reaches the provider's machines: "api" (StartERP
// pushes the amount and reads the result) or "manual" (no public connection
// yet: the store adds the machine for its records, takes the payment on it
// and records it in the POS; the guide explains how to get connected).
func (p *terminalProviderSpec) connect() string {
	if terminalAdapters[p.ID] != nil {
		return "api"
	}
	if len(p.BridgeDrivers) > 0 {
		return "bridge"
	}
	return "manual"
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
