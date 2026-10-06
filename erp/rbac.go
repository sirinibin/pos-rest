package erp

import (
	"strings"
)

// Modules of the StartERP role matrix (roles.perms keys).
var modules = []string{"sales", "purchases", "inventory", "customers", "vendors", "finance", "hr", "workshop", "reports", "settings"}

var verbs = []string{"view", "create", "edit", "delete", "print", "export"}

// Perm is one module's permission set.
type Perm map[string]bool

// Perms maps module -> verb -> allowed.
type Perms map[string]Perm

func fullPerm() Perm {
	return Perm{"view": true, "create": true, "edit": true, "delete": true, "print": true, "export": true}
}
func viewPerm() Perm {
	return Perm{"view": true, "create": false, "edit": false, "delete": false, "print": true, "export": true}
}
func nonePerm() Perm {
	return Perm{"view": false, "create": false, "edit": false, "delete": false, "print": false, "export": false}
}

func permsFor(fn func(module string) Perm) Perms {
	p := Perms{}
	for _, m := range modules {
		p[m] = fn(m)
	}
	return p
}

// systemRoles reproduces the prototype's built-in roles (gA(), L12521).
func systemRoles() []M {
	mk := func(id, name, desc string, perms Perms, maxDiscount float64, flags M) M {
		return M{"id": id, "name": name, "system": true, "description": desc, "perms": permsToM(perms),
			"maxDiscount": maxDiscount, "flags": flags, "version": int64(1), "deleted": false, "history": []interface{}{}}
	}
	return []M{
		mk("r_admin", "Admin", "Full access to every module and setting",
			permsFor(func(string) Perm { return fullPerm() }), 100,
			M{"manageUsers": true, "viewReports": true, "deleteRecords": true}),
		mk("r_manager", "Manager", "Everything except store settings and users",
			permsFor(func(m string) Perm {
				if m == "settings" {
					return viewPerm()
				}
				return fullPerm()
			}), 25, M{"manageUsers": false, "viewReports": true, "deleteRecords": true}),
		mk("r_salesman", "Salesman", "Sales, quotations and customers",
			permsFor(func(m string) Perm {
				switch m {
				case "sales", "customers":
					p := fullPerm()
					p["delete"] = false
					return p
				case "inventory":
					return viewPerm()
				}
				return nonePerm()
			}), 10, M{"manageUsers": false, "viewReports": false, "deleteRecords": false}),
		mk("r_cashier", "Cashier", "Point of sale and payments",
			permsFor(func(m string) Perm {
				if m == "sales" {
					p := fullPerm()
					p["delete"] = false
					p["edit"] = false
					return p
				}
				p := nonePerm()
				p["view"] = m == "customers" || m == "inventory"
				return p
			}), 5, M{"manageUsers": false, "viewReports": false, "deleteRecords": false}),
		mk("r_accountant", "Accountant", "Finance, accounting and reports",
			permsFor(func(m string) Perm {
				if m == "finance" || m == "reports" || m == "hr" {
					return fullPerm()
				}
				return viewPerm()
			}), 0, M{"manageUsers": false, "viewReports": true, "deleteRecords": false}),
		mk("r_viewer", "Viewer", "Read-only access",
			permsFor(func(string) Perm { return viewPerm() }), 0,
			M{"manageUsers": false, "viewReports": true, "deleteRecords": false}),
	}
}

func systemRoleByID(id string) M {
	for _, r := range systemRoles() {
		if r["id"] == id {
			return r
		}
	}
	return nil
}

func permsToM(p Perms) M {
	out := M{}
	for m, pp := range p {
		mm := M{}
		for _, v := range verbs {
			mm[v] = pp[v]
		}
		out[m] = mm
	}
	return out
}

func permsFromM(v interface{}) Perms {
	p := permsFor(func(string) Perm { return nonePerm() })
	mm, _ := v.(M)
	for _, m := range modules {
		vm, _ := mm[m].(M)
		for _, verb := range verbs {
			if vm != nil {
				p[m][verb] = boolv(vm[verb])
			}
		}
	}
	return p
}

func unionPerms(a, b Perms) Perms {
	out := permsFor(func(string) Perm { return nonePerm() })
	for _, m := range modules {
		for _, v := range verbs {
			out[m][v] = (a != nil && a[m][v]) || (b != nil && b[m][v])
		}
	}
	return out
}

// legacyRoleToContract maps the legacy user.role string to a system role id.
func legacyRoleToContract(role string, admin bool) string {
	if admin {
		return "r_admin"
	}
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return "r_admin"
	case "manager":
		return "r_manager"
	case "salesman", "salesmen", "sales man":
		return "r_salesman"
	case "cashier":
		return "r_cashier"
	case "accountant":
		return "r_accountant"
	case "viewer":
		return "r_viewer"
	}
	return "r_salesman"
}

// contractRoleToLegacy maps a contract role id back onto the legacy role
// string written to user.role (legacy only knows Admin/Manager/SalesMan).
// The legacy "Admin" is a PLATFORM super-admin (every store of every tenant),
// so StartERP's tenant admin (r_admin) is written as legacy "Manager"; the
// contract role itself is kept in the additive user.erp.role.
func contractRoleToLegacy(roleID string) string {
	switch roleID {
	case "r_admin", "r_manager", "r_accountant":
		return "Manager"
	}
	return "SalesMan"
}

// legacyResourceModule maps legacy RBAC resource keys (user_role.permissions,
// sidebar_menu_config.js) to contract modules.
var legacyResourceModule = map[string]string{
	"sales": "sales", "sales_return": "sales", "quotations": "sales", "qtn_sales_return": "sales",
	"non_vat_sales": "sales", "non_vat_sales_return": "sales", "delivery_notes": "sales", "customer_packages": "sales",
	"purchases": "purchases", "purchase_orders": "purchases", "purchase_requests": "purchases", "rfq_received": "purchases",
	"rfq_suppliers": "purchases", "procurement_emails": "purchases", "procurement_whatsapp": "purchases",
	"purchase_bill_images": "purchases", "purchase_return": "purchases",
	"products": "inventory", "services": "inventory", "product_category": "inventory", "service_category": "inventory",
	"product_brand": "inventory", "warehouses": "inventory", "stock_transfers": "inventory",
	"customers": "customers", "vendors": "vendors",
	"expenses": "finance", "expense_category": "finance", "receivables": "finance", "payables": "finance",
	"capitals": "finance", "dividents": "finance", "ledger": "finance", "accounts": "finance",
	"employees": "hr", "salaries": "hr",
	"vehicles": "workshop", "repair_jobs": "workshop", "automobile_dashboard": "workshop",
	"stats": "reports", "analytics": "reports", "dashboard": "reports",
	"stores": "settings", "users": "settings", "user_roles": "settings",
}

// legacyPermissionsToPerms converts legacy user_role.permissions[] into
// contract perms (read→view/print/export, update→edit).
func legacyPermissionsToPerms(perms []interface{}) Perms {
	p := permsFor(func(string) Perm { return nonePerm() })
	for _, e := range perms {
		pm, _ := e.(M)
		if pm == nil {
			continue
		}
		mod := legacyResourceModule[str(pm["resource"])]
		if mod == "" {
			continue
		}
		if boolv(pm["read"]) {
			p[mod]["view"], p[mod]["print"], p[mod]["export"] = true, true, true
		}
		if boolv(pm["create"]) {
			p[mod]["create"] = true
		}
		if boolv(pm["update"]) {
			p[mod]["edit"] = true
		}
		if boolv(pm["delete"]) {
			p[mod]["delete"] = true
		}
	}
	return p
}

// verbForMethod maps HTTP verbs onto permission verbs (contract §8).
func verbForMethod(method string, restore bool) string {
	switch method {
	case "GET":
		return "view"
	case "POST":
		if restore {
			return "edit"
		}
		return "create"
	case "PATCH", "PUT":
		return "edit"
	case "DELETE":
		return "delete"
	}
	return "view"
}
