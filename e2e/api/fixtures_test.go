//go:build e2e

package api

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Store is a company signed up for one test: its store, owner and token.
type Store struct {
	ID       string
	Country  string
	Token    string // owner (r_admin)
	Email    string
	Password string
	MS       string // the virtual main-store warehouse id (ms_<id>)
	Rec      M      // the store record returned by sign-up
	Loc      *time.Location
}

const ownerPassword = "Str0ng!Pass"

// SignupBody returns a valid sign-up body for a supported country
// (SA, AE, OM, QA, BH, KW, IN) with a unique owner e-mail.
func SignupBody(country string) M {
	b := M{
		"owner": M{"name": "E2E Owner", "email": "e2e+" + strings.ToLower(country) + Uniq() + Digits(4) + "@e2e.example",
			"mobile": "0512345678", "password": ownerPassword},
		"company": M{
			"nameEn": "E2E Trading " + Uniq(), "nameAr": "شركة الاختبار", "vatNo": "310122393500003", "crNo": "1010101010",
			"mobile": "0112345678", "type": "retail", "plan": "professional",
			"address": M{"buildingNo": "1234", "streetEn": "Olaya", "streetAr": "العليا", "districtEn": "Olaya", "districtAr": "العليا",
				"cityEn": "Riyadh", "cityAr": "الرياض", "postalCode": "12345", "additionalNo": "6789", "shortAddress": "RRRD1234"},
		},
	}
	c := b["company"].(M)
	o := b["owner"].(M)
	if country == "" || country == "SA" {
		return b
	}
	c["countryCode"] = country
	addr := M{"streetEn": "Sheikh Zayed Rd", "cityEn": "Dubai"}
	switch country {
	case "AE":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "0501234567", "042345678", "100123456700003", "CN-1234567"
	case "OM":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "92123456", "24123456", "OM1100012345", "1234567"
		addr["postalCode"] = "112"
	case "QA":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "55123456", "44123456", "", "123456"
	case "BH":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "36123456", "17123456", "200000898300002", "12345-1"
	case "KW":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "99123456", "22123456", "", "123456"
	case "IN":
		c["nameAr"] = ""
		c["vatNo"], c["crNo"], c["mobile"] = "27AAPFU0939F1ZV", "AAPFU0939F", "02226543210"
		o["mobile"] = "9876543210"
		addr = M{"streetEn": "MG Road", "cityEn": "Mumbai", "stateCode": "27", "postalCode": "400001"}
	}
	c["address"] = addr
	return b
}

// Signup signs up a new company in the given country ("" = Saudi Arabia).
func Signup(t testing.TB, country string) *Store {
	t.Helper()
	return SignupWith(t, SignupBody(country))
}

// SignupWith signs up with a prepared body.
func SignupWith(t testing.TB, body M) *Store {
	t.Helper()
	r := Must(t, Call(t, "POST", "/auth/signup", "", body), 201, "sign-up")
	st := r.Body["store"].(M)
	cc := S(st["countryCode"])
	if cc == "" {
		cc = "SA"
	}
	loc, err := time.LoadLocation(models.TimezoneMap[cc])
	if err != nil || models.TimezoneMap[cc] == "" {
		loc, _ = time.LoadLocation("Asia/Riyadh")
	}
	dropStoreDBAfter(t, S(st["id"]))
	return &Store{ID: S(st["id"]), Country: cc, Token: S(r.Body["accessToken"]), Email: S(Get(body, "owner.email")),
		Password: S(Get(body, "owner.password")), MS: "ms_" + S(st["id"]), Rec: st, Loc: loc}
}

// Now is the store's local wall-clock time as a contract datetime.
func (s *Store) Now() string { return time.Now().In(s.Loc).Format("2006-01-02T15:04") }

// Today is the store's local date.
func (s *Store) Today() string { return time.Now().In(s.Loc).Format("2006-01-02") }

// Login signs in with e-mail and password and returns the access token.
func Login(t testing.TB, email, password string) string {
	t.Helper()
	r := Must(t, Call(t, "POST", "/auth/login", "", M{"email": email, "password": password}), 200, "login "+email)
	return S(r.Body["accessToken"])
}

// User creates a staff user with a role in the store and returns its token.
func (s *Store) User(t testing.TB, role string) (id, token string) {
	t.Helper()
	email := strings.TrimPrefix(role, "r_") + Uniq() + Digits(4) + "@e2e.example"
	pw := "Staff@" + Digits(6)
	u := Create(t, s.Token, "users", M{"name": "E2E " + role, "email": email, "phone": "05" + Digits(8), "role": role,
		"storeIds": []string{s.ID}, "password": pw})
	return S(u["id"]), Login(t, email, pw)
}

// Product creates a product with purchase/retail prices and stock in the
// main store.
func (s *Store) Product(t testing.TB, purchase, retail, qty float64) M {
	t.Helper()
	return Create(t, s.Token, "products", M{"storeId": s.ID, "nameEn": "Product " + Uniq(), "nameAr": "منتج",
		"pricing": M{"purchase": purchase, "retail": retail}, "stock": M{s.MS: M{"qty": qty}}})
}

// Customer creates a customer (vatNo "" = a consumer, B2C).
func (s *Store) Customer(t testing.TB, vatNo string) M {
	t.Helper()
	b := M{"storeId": s.ID, "nameEn": "Customer " + Uniq(), "nameAr": "عميل", "phone": "05" + Digits(8)}
	if vatNo != "" {
		b["vatNo"] = vatNo
	}
	return Create(t, s.Token, "customers", b)
}

// Vendor creates a vendor.
func (s *Store) Vendor(t testing.TB) M {
	t.Helper()
	return Create(t, s.Token, "vendors", M{"storeId": s.ID, "nameEn": "Vendor " + Uniq(), "nameAr": "مورد", "phone": "05" + Digits(8)})
}

// Line is a document line in the main store.
func (s *Store) Line(productID string, qty, unitPrice float64) M {
	return M{"productId": productID, "qty": qty, "unitPrice": unitPrice, "unitDiscount": 0, "warehouseId": s.MS, "vatPercent": F(s.Rec, "vatPercent")}
}

// Stock is the product's current quantity in the main store.
func (s *Store) Stock(t testing.TB, productID string) float64 {
	t.Helper()
	return F(Read(t, s.Token, "products", productID), "stock."+s.MS+".qty")
}

// Every sign-up creates a store database with ~100 collections. With
// E2E_MONGO_URI set (CI and e2e/run.sh) the test drops it when it ends, so a
// long run doesn't exhaust MongoDB's open files. E2E_KEEP_DATA=1 keeps it.
var (
	mongoOnce   sync.Once
	mongoClient *mongo.Client
)

func dropStoreDBAfter(t testing.TB, storeID string) {
	uri := os.Getenv("E2E_MONGO_URI")
	if uri == "" || os.Getenv("E2E_KEEP_DATA") == "1" || storeID == "" {
		return
	}
	mongoOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if c, err := mongo.Connect(ctx, options.Client().ApplyURI(uri)); err == nil {
			mongoClient = c
		}
	})
	if mongoClient == nil {
		return
	}
	t.Cleanup(func() {
		// background work after the last request (stock, ledger) finishes first
		time.Sleep(2 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = mongoClient.Database("store_" + storeID).Drop(ctx)
	})
}
