package erp

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Platform-admin tool for the one-off ledger repair (models/ledger_repair.go):
// deleted deposits, withdrawals, capitals and dividends that still have
// their postings, and capital withdrawals that were never posted.
//
//	POST /admin/repairs/ledger {storeId?, apply?}
//
// storeId omitted = every store. apply omitted / false = dry run (reads
// only, returns the same report). The same repair runs from the command line:
// `<binary> repair-ledgers [--apply] [--store=<id>]`.

// ledgerRepairRequest validates the body: storeId (optional store id) and
// apply (optional boolean).
func ledgerRepairRequest(body M) (storeID *primitive.ObjectID, apply bool, err error) {
	errs := map[string]string{}
	if v, ok := body["apply"]; ok && v != nil {
		b, isBool := v.(bool)
		if !isBool {
			errs["apply"] = "must be true or false"
		}
		apply = b
	}
	if v, ok := body["storeId"]; ok && v != nil && v != "" {
		s, isStr := v.(string)
		oid, valid := oidOf(s)
		if !isStr || !valid {
			errs["storeId"] = "invalid store id"
		} else {
			storeID = &oid
		}
	}
	if len(errs) > 0 {
		return nil, false, errBadRequest("", errs)
	}
	return storeID, apply, nil
}

// POST /admin/repairs/ledger — platform admins only.
func handleLedgerRepair(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeID, apply, err := ledgerRepairRequest(body)
	if err != nil {
		return err
	}
	if storeID != nil {
		ctx, cancel := dbctx()
		n, err := mainDB().Collection("store").CountDocuments(ctx, bson.M{"_id": *storeID})
		cancel()
		if err != nil {
			return errInternal("db: " + err.Error())
		}
		if n == 0 {
			return errBadRequest("", map[string]string{"storeId": "unknown store"})
		}
	}
	run, err := models.RunLedgerRepair(storeID, apply)
	if err != nil {
		return errInternal("ledger repair: " + err.Error())
	}
	writeJSON(w, http.StatusOK, run)
	return nil
}

func registerLedgerRepair(s *mux.Router) {
	s.HandleFunc("/admin/repairs/ledger", authed(handleLedgerRepair)).Methods("POST")
}
