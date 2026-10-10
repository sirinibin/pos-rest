package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// rbacResources maps a /v1/<segment> route to the RBAC resource the app's
// role editor and sidebar use for it.
var rbacResources = map[string]string{
	"order":                   "sales",
	"sales-return":            "sales_return",
	"purchase":                "purchases",
	"purchase-order":          "purchase_orders",
	"purchase-request":        "purchase_requests",
	"purchase-return":         "purchase_return",
	"delivery-note":           "delivery_notes",
	"quotation":               "quotations",
	"quotation-sales-return":  "qtn_sales_return",
	"non-vat-sales":           "non_vat_sales",
	"non-vat-sales-return":    "non_vat_sales_return",
	"vendor":                  "vendors",
	"warehouse":               "warehouses",
	"stock-transfer":          "stock_transfers",
	"customer":                "customers",
	"product":                 "products",
	"product-category":        "product_category",
	"service-category":        "service_category",
	"product-brand":           "product_brand",
	"expense-category":        "expense_category",
	"expense":                 "expenses",
	"customer-deposit":        "receivables",
	"customer-withdrawal":     "payables",
	"capital":                 "capitals",
	"capital-withdrawal":      "capitals",
	"divident":                "dividents",
	"customer-package":        "customer_packages",
	"employee":                "employees",
	"employee-salary-payment": "salaries",
	"vehicle":                 "vehicles",
	"repair-job":              "repair_jobs",
}

// rbacTarget returns the resource and action a request writes, or "" when
// the route is not a plain create (POST /v1/x), update (PUT/PATCH
// /v1/x/{id}) or delete (DELETE /v1/x/{id}). Helper routes such as
// POST /v1/order/calculate-net-total are not writes and are left alone.
func rbacTarget(method, pathTemplate string) (resource, action string) {
	rest := strings.TrimPrefix(pathTemplate, "/v1/")
	if rest == pathTemplate {
		return "", ""
	}
	parts := strings.Split(rest, "/")
	resource = rbacResources[parts[0]]
	if resource == "" {
		return "", ""
	}
	switch {
	case len(parts) == 1 && method == http.MethodPost:
		return resource, "create"
	case len(parts) == 2 && parts[1] == "{id}" && (method == http.MethodPut || method == http.MethodPatch):
		return resource, "update"
	case len(parts) == 2 && parts[1] == "{id}" && method == http.MethodDelete:
		return resource, "delete"
	}
	return "", ""
}

// rbacDenies applies the app's rule: a role grant is checked only when the
// resource appears in the user's effective permissions; a resource no role
// mentions stays allowed, as the sidebar treats it.
func rbacDenies(perms []models.Permission, resource, action string) bool {
	for _, p := range perms {
		if p.Resource != resource {
			continue
		}
		switch action {
		case "create":
			return !p.Create
		case "update":
			return !p.Update
		case "delete":
			return !p.Delete
		}
	}
	return false
}

// RBACWriteMiddleware refuses (403) a create, update or delete that the
// user's roles don't grant, in stores that turned the RBAC module on.
// Before, the app only hid the buttons: a SalesMan whose role lacks
// "create" on products could still POST /v1/product. Admins, users with no
// roles and stores with RBAC off are unaffected.
func RBACWriteMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tpl := ""
		if route := mux.CurrentRoute(r); route != nil {
			tpl, _ = route.GetPathTemplate()
		}
		resource, action := rbacTarget(r.Method, tpl)
		if resource == "" {
			next.ServeHTTP(w, r)
			return
		}
		if msg := rbacRefusal(r, resource, action); msg != "" {
			var response models.Response
			response.Status = false
			response.Errors = map[string]string{"permission": msg}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func rbacRefusal(r *http.Request, resource, action string) string {
	user := rbacUser(r)
	if user == nil || isAdminUser(user) || len(user.RoleIDs) == 0 {
		return ""
	}
	storeID := rbacStoreID(r)
	if storeID == nil {
		return ""
	}
	store, err := models.FindStoreByID(storeID, bson.M{"settings.enable_rbac_module": 1})
	if err != nil || !store.Settings.EnableRBACModule {
		return ""
	}
	perms, err := models.GetEffectivePermissions([]*primitive.ObjectID{storeID}, user.RoleIDs)
	if err != nil {
		return ""
	}
	if rbacDenies(perms, resource, action) {
		return "Your role does not allow you to " + action + " " + strings.ReplaceAll(resource, "_", " ")
	}
	return ""
}

// rbacStoreID is the store a write names (see requestedStoreIDs), or the
// store of the record behind /v1/x/{id} given as ?store_id.
func rbacStoreID(r *http.Request) *primitive.ObjectID {
	ids := requestedStoreIDs(r)
	if len(ids) == 0 {
		return nil
	}
	return &ids[0]
}

func rbacUser(r *http.Request) *models.User {
	tokenStr, err := models.ParseAccessTokenFromRequest(r)
	if err != nil {
		return nil
	}
	claims, err := models.AuthenticateByJWTToken(tokenStr)
	if err != nil || claims.Type != "access_token" {
		return nil
	}
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil
	}
	user, err := models.FindUserByID(&userID, bson.M{"role": 1, "admin": 1, "store_ids": 1, "role_ids": 1})
	if err != nil {
		return nil
	}
	return user
}
