package erp

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Platform-admin tools for the ZATCA re-connection mark.
//
// A store holding ZATCA credentials is marked zatca.zatca_reconnect_required
// when a ZATCA-sensitive detail changes (controller.zatcaSensitiveFieldsChanged).
// StartERP administrators can list the marked stores and clear the mark when a
// re-connection is not needed. Clearing it never touches the store's ZATCA
// credentials, phase or environment: it only lets the store report again.

const zatcaReconnectListMax = 100

// zatcaReconnectStoreFields are the store fields the admin list returns.
var zatcaReconnectStoreFields = bson.M{
	"name": 1, "name_in_arabic": 1, "branch_name": 1, "code": 1, "vat_no": 1, "registration_number": 1,
	"business_category": 1, "country_code": 1, "zatca.phase": 1, "zatca.env": 1, "zatca.connected": 1,
}

// zatcaReconnectFilter matches non-deleted stores that are marked for
// re-connection, narrowed by q (name, Arabic name, code, VAT or CR number).
func zatcaReconnectFilter(q string) bson.M {
	f := bson.M{"zatca.zatca_reconnect_required": true, "deleted": bson.M{"$ne": true}}
	if sf := searchFilter(strings.TrimSpace(q), []string{"name", "name_in_arabic", "code", "vat_no", "registration_number"}); sf != nil {
		f = bson.M{"$and": bson.A{f, sf}}
	}
	return f
}

// zatcaReconnectRow is one store of the admin list (contract shape).
func zatcaReconnectRow(d M) M {
	return M{
		"id": hexOf(d["_id"]), "nameEn": str(d["name"]), "nameAr": str(d["name_in_arabic"]),
		"branchEn": str(d["branch_name"]), "code": str(d["code"]), "vatNo": str(d["vat_no"]),
		"crNo": str(d["registration_number"]), "category": str(d["business_category"]),
		"countryCode": storeCountryOrSA(d),
		"zatca": M{"phase": str(get(d, "zatca.phase")), "env": str(get(d, "zatca.env")),
			"connected": boolv(get(d, "zatca.connected")), "reconnectNeeded": true},
	}
}

// zatcaReconnectLimit parses ?limit= (default 50, 1..100).
func zatcaReconnectLimit(v string) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return 50, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 || n > zatcaReconnectListMax {
		return 0, errBadRequest("", map[string]string{"limit": "1 to 100"})
	}
	return int64(n), nil
}

// GET /admin/zatca-reconnects?q=&limit= — stores marked for ZATCA re-connection.
func handleZatcaReconnectList(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	limit, err := zatcaReconnectLimit(r.URL.Query().Get("limit"))
	if err != nil {
		return err
	}
	filter := zatcaReconnectFilter(r.URL.Query().Get("q"))
	ctx, cancel := dbctx()
	defer cancel()
	col := mainDB().Collection("store")
	total, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	cur, err := col.Find(ctx, filter, options.Find().SetProjection(zatcaReconnectStoreFields).
		SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(limit))
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	items := []M{}
	for cur.Next(ctx) {
		var raw bson.M
		if err := cur.Decode(&raw); err != nil {
			return errInternal("db: " + err.Error())
		}
		items = append(items, zatcaReconnectRow(normDoc(raw)))
	}
	writeJSON(w, http.StatusOK, M{"items": items, "total": total})
	return nil
}

// POST /stores/{id}/zatca/unmark — clears the re-connection mark (platform
// admins only). Credentials, phase and environment are left untouched; who
// cleared it and when are kept in erp.x.zatca.
func handleZatcaUnmark(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	id := mux.Vars(r)["id"]
	oid, ok := oidOf(id)
	if !ok {
		return errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	col := mainDB().Collection("store")
	var raw bson.M
	if err := col.FindOne(ctx, bson.M{"_id": oid, "deleted": bson.M{"$ne": true}},
		options.FindOne().SetProjection(bson.M{"zatca.zatca_reconnect_required": 1, "country_code": 1})).Decode(&raw); err != nil {
		return errNotFound()
	}
	d := normDoc(raw)
	if !boolv(get(d, "zatca.zatca_reconnect_required")) {
		return errf(http.StatusConflict, "zatca_not_marked", "This store is not marked for ZATCA re-connection.", nil)
	}
	at := nowFn().In(storeLocation(d)).Format(layoutDT)
	if _, err := col.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"zatca.zatca_reconnect_required":       false,
		envKey + ".x.zatca.reconnectClearedAt": at,
		envKey + ".x.zatca.reconnectClearedBy": c.UserName,
	}}); err != nil {
		return errInternal("db: " + err.Error())
	}
	writeJSON(w, http.StatusOK, M{"id": id, "zatca": M{"reconnectNeeded": false, "reconnectClearedAt": at, "reconnectClearedBy": c.UserName}})
	return nil
}

func registerZatcaAdmin(s *mux.Router) {
	s.HandleFunc("/admin/zatca-reconnects", authed(handleZatcaReconnectList)).Methods("GET")
	s.HandleFunc("/stores/{id}/zatca/unmark", authed(handleZatcaUnmark)).Methods("POST")
}
