package erp

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/controller"
	"go.mongodb.org/mongo-driver/bson"
)

// zatcaReporters are the EXISTING legacy ZATCA report handlers (Go UBL
// builder + Python signer + per-collection hash chain). They are variables
// only so tests can stub the external ZATCA call.
var zatcaReporters = map[string]http.HandlerFunc{
	"sales":         controller.ReportOrderToZatca,
	"sales-returns": controller.ReportSalesReturnToZatca,
	"deposits":      controller.ReportCustomerDepositToZatca,
	"withdrawals":   controller.ReportCustomerWithdrawalToZatca,
}

var zatcaReportPaths = map[string]string{
	"sales": "/v1/order/zatca/report/", "sales-returns": "/v1/sales-return/zatca/report/",
	"deposits": "/v1/customer-deposit/zatca/report/", "withdrawals": "/v1/customer-withdrawal/zatca/report/",
}

var (
	zatcaConnectHandler    http.HandlerFunc = controller.ConnectStoreToZatca
	zatcaDisconnectHandler http.HandlerFunc = controller.DisconnectStoreFromZatca
)

func handleZatcaReport(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
			res := resourceByPath(path)
			if res == nil {
				return 0, nil, errNotFound()
			}
			if err := res.checkPerm(c, "edit"); err != nil {
				return 0, nil, err
			}
			id := mux.Vars(r)["id"]
			storeHex, err := resolveStore(c, res, id, nil)
			if err != nil {
				return 0, nil, err
			}
			doc, err := res.Backend.Get(c, storeHex, id, false)
			if err != nil {
				return 0, nil, err
			}
			if doc == nil {
				return 0, nil, errNotFound()
			}
			st := c.store(storeHex)
			if str(get(st, "zatca.phase")) != "2" || !boolv(get(st, "zatca.connected")) {
				return 0, nil, errf(http.StatusConflict, "zatca_not_connected",
					"This store is not connected to ZATCA (Phase 2). Connect it in Settings → ZATCA first.", nil)
			}
			if boolv(get(st, "zatca.zatca_reconnect_required")) {
				return 0, nil, errf(http.StatusConflict, "zatca_reconnect_required",
					"ZATCA re-connection is required because company details changed. Reconnect before reporting.", nil)
			}
			if z, _ := doc["zatca"].(M); z != nil && (z["status"] == "reported" || z["status"] == "cleared") {
				return http.StatusOK, doc, nil // idempotent: never re-sign / re-chain
			}
			hex := id
			if _, ok := oidOf(id); !ok {
				hex = str(doc["id"])
			}
			lr, _ := callV1(c, zatcaReporters[path], "POST", zatcaReportPaths[path]+hex, map[string]string{"id": hex}, storeHex, M{})
			out, gerr := res.Backend.Get(c, storeHex, hex, true)
			if gerr != nil || out == nil {
				return 0, nil, errInternal("reported document could not be reloaded")
			}
			if !lr.ok() {
				if lr.HTTP == http.StatusUnauthorized {
					return 0, nil, errUnauthorized("")
				}
				z, _ := out["zatca"].(M)
				if z == nil {
					z = M{}
				}
				z["status"] = "failed"
				z["error"] = firstLegacyMsg(lr.Errors)
				z["attemptAt"] = nowFn().In(riyadh).Format(layoutDT)
				out["zatca"] = z
			}
			return http.StatusOK, out, nil
		})
	}
}

func handleZatcaConnect(w http.ResponseWriter, r *http.Request) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		if !c.can("settings", "edit") {
			return 0, nil, errForbidden("")
		}
		id := mux.Vars(r)["id"]
		if c.store(id) == nil {
			return 0, nil, errNotFound()
		}
		body, err := readBody(r)
		if err != nil {
			return 0, nil, err
		}
		otp := str(body["otp"])
		if !reOTP.MatchString(otp) {
			return 0, nil, errf(http.StatusBadRequest, "invalid_otp", "Invalid-OTP: the OTP is invalid or has expired",
				map[string]string{"otp": "invalid"})
		}
		lr, _ := callV1(c, zatcaConnectHandler, "POST", "/v1/store/zatca/connect", nil, "", M{"id": id, "otp": otp})
		if !lr.ok() {
			if lr.HTTP == http.StatusUnauthorized {
				return 0, nil, errUnauthorized("")
			}
			return 0, nil, errf(http.StatusBadRequest, "invalid_otp", "ZATCA onboarding failed: "+firstLegacyMsg(lr.Errors),
				map[string]string{"otp": "invalid"})
		}
		sb := resourceByPath("stores").Backend.(*storesBackend)
		cur, _ := sb.Get(c, "", id, true)
		snap, _ := json.Marshal(M{"nameEn": cur["nameEn"], "nameAr": cur["nameAr"], "vatNo": cur["vatNo"], "crNo": cur["crNo"], "address": cur["address"]})
		_ = sb.setEnv("", id, M{"x.zatca.snapshot": string(snap), "x.zatca.certExpires": time.Now().AddDate(1, 0, 0).In(riyadh).Format(layoutDay),
			"x.zatca.connectedAt": nowFn().In(riyadh).Format(layoutDT)})
		out, _ := sb.Get(c, "", id, true)
		return http.StatusOK, out, nil
	})
}

func handleZatcaDisconnect(w http.ResponseWriter, r *http.Request) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		if !c.can("settings", "edit") {
			return 0, nil, errForbidden("")
		}
		id := mux.Vars(r)["id"]
		if c.store(id) == nil {
			return 0, nil, errNotFound()
		}
		lr, _ := callV1(c, zatcaDisconnectHandler, "POST", "/v1/store/zatca/disconnect", nil, "", M{"id": id})
		if !lr.ok() {
			return 0, nil, legacyErr(lr, nil, nil, "")
		}
		sb := resourceByPath("stores").Backend.(*storesBackend)
		oid, _ := oidOf(id)
		ctx, cancel := dbctx()
		_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
			"erp.x.zatca.disconnectedAt": nowFn().In(riyadh).Format(layoutDT)}, "$unset": bson.M{"erp.x.zatca.snapshot": ""}})
		cancel()
		out, _ := sb.Get(c, "", id, true)
		if z, ok := out["zatca"].(M); ok {
			z["pcsid"] = nil
			z["reconnectNeeded"] = false
		}
		return http.StatusOK, out, nil
	})
}
