package erp

import (
	"fmt"
	"strconv"
)

// discountLimited are the selling documents a role's maxDiscount applies to.
var discountLimited = map[string]bool{"sales": true, "quotations": true, "nonvat-sales": true}

// roleMaxDiscount is the caller's role's maxDiscount in percent (100 = no
// limit). Platform admins have no limit; a custom role without one has none.
func roleMaxDiscount(c *Ctx) float64 {
	if c == nil || c.Admin {
		return 100
	}
	r := systemRoleByID(c.RoleID)
	if r == nil {
		r = loadCustomRole(c.RoleID)
	}
	if r == nil {
		return 100
	}
	if v, ok := r["maxDiscount"]; ok && v != nil {
		return num(v)
	}
	return 100
}

// discountLimitErrors refuses (400, on the field) line and document
// discounts above the caller's role's maxDiscount on sales, quotations and
// non-VAT sales. A line's discount is unitDiscount against its unit price;
// the document discount is against the lines' total after their discounts.
// It used to be stored on the role and never enforced. On an edit only the
// discounts being changed are checked, so a document a manager discounted
// can still be edited by a cashier for something else.
func discountLimitErrors(c *Ctx, res *Resource, rec M, changed []string) map[string]string {
	if res == nil || !discountLimited[res.Path] {
		return nil
	}
	max := roleMaxDiscount(c)
	if max >= 100 {
		return nil
	}
	check := map[string]bool{"items": true, "discount": true}
	if changed != nil {
		check = changedSet(changed)
	}
	e := map[string]string{}
	msg := "exceeds the maximum discount of your role (" + strconv.FormatFloat(max, 'f', -1, 64) + "%)"
	net := 0.0
	for i, it := range arr(rec["items"]) {
		im, _ := it.(M)
		if im == nil {
			continue
		}
		q, up, ud := num(im["qty"]), num(im["unitPrice"]), num(im["unitDiscount"])
		net += q * (up - ud)
		if check["items"] && ud > 0 && up > 0 && ud/up*100 > max+1e-9 {
			e[fmt.Sprintf("items.%d.unitDiscount", i)] = msg
		}
	}
	if d := num(rec["discount"]); check["discount"] && d > 0 && (net <= 0 || d/net*100 > max+1e-9) {
		e["discount"] = msg
	}
	if len(e) == 0 {
		return nil
	}
	return e
}
