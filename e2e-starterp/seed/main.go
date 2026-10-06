// Command seed writes the legacy-shaped StartERP fixture into a TEST database
// (MONGO_DB must start with t1_/test/erp_test) and prints the ids as JSON.
//
//	MONGO_DB=t1_pos REDIS_DSN=127.0.0.1:6380 go run ./e2e-starterp/seed > fixture.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/erp/erpfixture"
)

func main() {
	reset := flag.Bool("reset", false, "drop the test main DB and every store_<id> DB it references first")
	flag.Parse()
	name := db.GetPosDB()
	db.Client("")
	if *reset {
		if !(strings.HasPrefix(name, "t1_") || strings.HasPrefix(name, "test") || strings.HasPrefix(name, "erp_test")) {
			fmt.Fprintln(os.Stderr, "refusing to reset", name)
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		cur, err := db.GetDB("").Collection("store").Find(ctx, bson.M{})
		if err == nil {
			for cur.Next(ctx) {
				var s struct {
					ID interface{} `bson:"_id"`
				}
				if cur.Decode(&s) == nil {
					if oid, ok := s.ID.(interface{ Hex() string }); ok {
						_ = db.GetDB("store_" + oid.Hex()).Drop(ctx)
					}
				}
			}
			cur.Close(ctx)
		}
		_ = db.GetDB("").Drop(ctx)
		cancel()
	}
	f, err := erpfixture.Seed(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
	out := map[string]interface{}{"mainDB": f.MainDB, "password": erpfixture.Password, "adminEmail": f.AdminEmail,
		"managerEmail": f.ManagerEmail, "salesEmail": f.SalesEmail, "userBEmail": f.UserBEmail,
		"storeA": f.StoreA.Hex(), "storeB": f.StoreB.Hex(), "customerA1": f.CustomerA1.Hex(), "customerA2": f.CustomerA2.Hex(),
		"productA1": f.ProductA1.Hex(), "productA2": f.ProductA2.Hex(), "orderA1": f.OrderA1.Hex(), "orderA2": f.OrderA2.Hex(),
		"warehouseA": f.WarehouseA.Hex(), "vendorA1": f.VendorA1.Hex()}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}
