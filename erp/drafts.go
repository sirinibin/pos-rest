package erp

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Drafts (§2.1b): work-in-progress documents live in SEPARATE store-DB
// collections `<legacy collection>_draft`. These routes never call a create
// path, never allocate numbers/ICV/UUID and never touch stock, ledger, stats
// or ZATCA. Finalize runs the normal create of the resource and only then
// deletes the draft.
var draftTypes = map[string]struct{ coll, resource, legacyColl string }{
	"sales":             {"order_draft", "sales", "order"},
	"quotations":        {"quotation_draft", "quotations", "quotation"},
	"purchases":         {"purchase_draft", "purchases", "purchase"},
	"sales-returns":     {"salesreturn_draft", "sales-returns", ""},
	"purchase-returns":  {"purchasereturn_draft", "purchase-returns", ""},
	"delivery-notes":    {"delivery_note_draft", "delivery-notes", ""},
	"purchase-orders":   {"purchase_order_draft", "purchase-orders", ""},
	"quotation-returns": {"quotation_sales_return_draft", "quotation-returns", ""},
	"deposits":          {"customerdeposit_draft", "deposits", ""},
	"withdrawals":       {"customerwithdrawal_draft", "withdrawals", ""},
	"stock-transfers":   {"stocktransfer_draft", "stock-transfers", ""},
	"pos":               {"pos_cart_draft", "sales", ""},
}

var draftAliases = map[string]string{
	"order": "sales", "quotation": "quotations", "purchase": "purchases", "salesreturn": "sales-returns",
	"purchasereturn": "purchase-returns", "delivery_note": "delivery-notes", "purchase_order": "purchase-orders",
	"quotation_sales_return": "quotation-returns", "customerdeposit": "deposits", "customerwithdrawal": "withdrawals",
	"stocktransfer": "stock-transfers", "pos_cart": "pos",
}

func draftType(r *http.Request) (string, error) {
	t := mux.Vars(r)["docType"]
	if a, ok := draftAliases[t]; ok {
		t = a
	}
	if _, ok := draftTypes[t]; !ok {
		return "", errNotFound()
	}
	return t, nil
}

func draftStore(c *Ctx, r *http.Request, body M) (string, error) {
	s := strings.TrimSpace(r.URL.Query().Get("storeId"))
	if s == "" && body != nil {
		s = str(body["storeId"])
		if s == "" {
			s = str(sub(body, "payload")["storeId"])
		}
	}
	if s == "" {
		return "", errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	if c.store(s) == nil {
		return "", errForbidden("You do not have access to this store.")
	}
	return s, nil
}

func draftToContract(loc *time.Location, d M) M {
	out := M{"id": hexOf(d["_id"]), "storeId": hexOf(d["store_id"]), "docType": str(d["doc_type"]),
		"payload": d["payload"], "title": str(d["title"]), "createdBy": str(d["created_by"]), "updatedBy": str(d["updated_by"]),
		"createdAt": fmtDTIn(loc, d["created_at"]), "updatedAt": fmtDTIn(loc, d["updated_at"]), "deviceId": str(d["device_id"]), "legacyDraft": false}
	if e := fmtDTIn(loc, d["expires_at"]); e != "" {
		out["expiresAt"] = e
	}
	return out
}

func draftTitle(payload M) string {
	name := str(payload["customerName"])
	if name == "" {
		name = str(payload["vendorName"])
	}
	n := len(arr(payload["items"]))
	if name == "" {
		name = "Draft"
	}
	return name + " · " + itoa(n) + " line(s)"
}

func draftModule(t string) string {
	if res := resourceByPath(draftTypes[t].resource); res != nil {
		return res.Module
	}
	return "sales"
}

func handleDraftList(w http.ResponseWriter, r *http.Request) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	t, err := draftType(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !c.can(draftModule(t), "view") {
		writeErr(w, errForbidden(""))
		return
	}
	s, err := draftStore(c, r, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := dbctx()
	defer cancel()
	out := []M{}
	cur, err := storeDB(s).Collection(draftTypes[t].coll).Find(ctx, bson.M{"doc_type": t}, options.Find().SetSort(bson.M{"updated_at": -1}))
	if err == nil {
		for cur.Next(ctx) {
			out = append(out, draftToContract(storeLocation(c.store(s)), bsonToM(cur.Current)))
		}
		cur.Close(ctx)
	}
	// legacy drafts (status:"draft" in the real collection) are shown read-only
	if lc := draftTypes[t].legacyColl; lc != "" {
		cur2, err := storeDB(s).Collection(lc).Find(ctx, bson.M{"status": "draft", "deleted": bson.M{"$ne": true}}, options.Find().SetLimit(500))
		if err == nil {
			res := resourceByPath(draftTypes[t].resource)
			lb, _ := res.Backend.(*legacyBackend)
			for cur2.Next(ctx) {
				d := bsonToM(cur2.Current)
				var payload M
				if lb != nil {
					payload = lb.render(newMapCtx(c, s), d, true)
				}
				out = append(out, M{"id": hexOf(d["_id"]), "storeId": s, "docType": t, "payload": payload,
					"title": "Legacy draft " + str(d["code"]), "legacyDraft": true, "createdAt": fmtDTIn(storeLocation(c.store(s)), d["created_at"]),
					"updatedAt": fmtDTIn(storeLocation(c.store(s)), d["updated_at"])})
			}
			cur2.Close(ctx)
		}
	}
	writeJSON(w, http.StatusOK, M{"data": out, "total": len(out)})
}

func loadDraft(s, t, id string) (M, error) {
	oid, ok := oidOf(id)
	if !ok {
		return nil, errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	err := storeDB(s).Collection(draftTypes[t].coll).FindOne(ctx, bson.M{"_id": oid}).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		return nil, errNotFound()
	}
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	return normDoc(raw), nil
}

func handleDraftGet(w http.ResponseWriter, r *http.Request) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	t, err := draftType(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	s, err := draftStore(c, r, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	d, err := loadDraft(s, t, mux.Vars(r)["id"])
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, draftToContract(storeLocation(c.store(s)), d))
}

func handleDraftCreate(w http.ResponseWriter, r *http.Request) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		t, err := draftType(r)
		if err != nil {
			return 0, nil, err
		}
		if !c.can(draftModule(t), "create") {
			return 0, nil, errForbidden("")
		}
		body, err := readBody(r)
		if err != nil {
			return 0, nil, err
		}
		s, err := draftStore(c, r, body)
		if err != nil {
			return 0, nil, err
		}
		payload, _ := body["payload"].(M)
		if payload == nil {
			return 0, nil, errBadRequest("", map[string]string{"payload": "required"})
		}
		now := time.Now()
		title := str(body["title"])
		if title == "" {
			title = draftTitle(payload)
		}
		doc := bson.M{"_id": primitive.NewObjectID(), "store_id": s, "doc_type": t, "payload": payload, "title": title,
			"created_by": c.UserName, "updated_by": c.UserName, "created_at": now, "updated_at": now, "device_id": str(body["deviceId"])}
		if e := str(body["expiresAt"]); e != "" {
			if et, err := parseClientTimeIn(storeLocation(c.store(s)), e); err == nil {
				doc["expires_at"] = et
			}
		}
		ctx, cancel := dbctx()
		defer cancel()
		if _, err := storeDB(s).Collection(draftTypes[t].coll).InsertOne(ctx, doc); err != nil {
			return 0, nil, errInternal("db: " + err.Error())
		}
		d, _ := loadDraft(s, t, doc["_id"].(primitive.ObjectID).Hex())
		return http.StatusCreated, draftToContract(storeLocation(c.store(s)), d), nil
	})
}

func handleDraftPut(w http.ResponseWriter, r *http.Request) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		t, err := draftType(r)
		if err != nil {
			return 0, nil, err
		}
		if !c.can(draftModule(t), "create") {
			return 0, nil, errForbidden("")
		}
		body, err := readBody(r)
		if err != nil {
			return 0, nil, err
		}
		s, err := draftStore(c, r, body)
		if err != nil {
			return 0, nil, err
		}
		id := mux.Vars(r)["id"]
		if _, err := loadDraft(s, t, id); err != nil {
			return 0, nil, err
		}
		payload, _ := body["payload"].(M)
		if payload == nil {
			return 0, nil, errBadRequest("", map[string]string{"payload": "required"})
		}
		title := str(body["title"])
		if title == "" {
			title = draftTitle(payload)
		}
		oid, _ := oidOf(id)
		ctx, cancel := dbctx()
		defer cancel()
		_, err = storeDB(s).Collection(draftTypes[t].coll).UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
			"payload": payload, "title": title, "updated_by": c.UserName, "updated_at": time.Now(), "device_id": str(body["deviceId"])}})
		if err != nil {
			return 0, nil, errInternal("db: " + err.Error())
		}
		d, _ := loadDraft(s, t, id)
		return http.StatusOK, draftToContract(storeLocation(c.store(s)), d), nil
	})
}

func handleDraftDelete(w http.ResponseWriter, r *http.Request) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		t, err := draftType(r)
		if err != nil {
			return 0, nil, err
		}
		s, err := draftStore(c, r, nil)
		if err != nil {
			return 0, nil, err
		}
		id := mux.Vars(r)["id"]
		if _, err := loadDraft(s, t, id); err != nil {
			return 0, nil, err
		}
		oid, _ := oidOf(id)
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = storeDB(s).Collection(draftTypes[t].coll).DeleteOne(ctx, bson.M{"_id": oid})
		return http.StatusNoContent, nil, nil
	})
}

func handleDraftFinalize(w http.ResponseWriter, r *http.Request) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		t, err := draftType(r)
		if err != nil {
			return 0, nil, err
		}
		res := resourceByPath(draftTypes[t].resource)
		if !c.can(res.Module, "create") {
			return 0, nil, errForbidden("")
		}
		s, err := draftStore(c, r, nil)
		if err != nil {
			return 0, nil, err
		}
		id := mux.Vars(r)["id"]
		d, err := loadDraft(s, t, id)
		if err != nil {
			return 0, nil, err
		}
		payload := cloneM(sub(d, "payload"))
		delete(payload, "id")
		payload["storeId"] = s
		rec, err := res.Backend.Create(c, s, payload, WriteMeta{Action: "created"})
		if err != nil {
			return 0, nil, err // draft stays untouched
		}
		dashboardTouched(res, s, str(rec["id"]))
		oid, _ := oidOf(id)
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = storeDB(s).Collection(draftTypes[t].coll).DeleteOne(ctx, bson.M{"_id": oid})
		return http.StatusCreated, rec, nil
	})
}
