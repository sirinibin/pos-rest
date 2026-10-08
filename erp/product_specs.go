package erp

import (
	"strings"
)

// Product option lists a store manages (resource productSpecs, NEW collection
// erp_product_spec): a product type for every store, and class, size and
// material for Industrial Supplies stores. The product keeps the option ids in
// `specs` (erp.x): {"type": id, "class": id, "size": id, "material": id}.
// POS terminals filter by them and show only the ones in use.

// ProductSpecKinds are the option lists a store manages.
var ProductSpecKinds = []string{"type", "class", "size", "material"}

func validSpecKind(k string) bool {
	for _, s := range ProductSpecKinds {
		if s == k {
			return true
		}
	}
	return false
}

const maxSpecName = 60

// productSpecValidate checks one option: a known kind and a name of 1–60 characters.
func productSpecValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if !validSpecKind(str(rec["kind"])) {
		e["kind"] = "one of type, class, size, material"
	}
	name := strings.TrimSpace(str(rec["name"]))
	if name == "" {
		e["name"] = "required"
	} else if len([]rune(name)) > maxSpecName {
		e["name"] = "at most 60 characters"
	}
	if n := str(rec["nameAr"]); len([]rune(n)) > maxSpecName {
		e["nameAr"] = "at most 60 characters"
	}
	return e
}

// validateProductSpecs checks a product's `specs`: an object whose keys are spec
// kinds and whose values are option ids (or empty to clear).
func validateProductSpecs(rec M, e map[string]string) {
	v, ok := rec["specs"]
	if !ok || v == nil {
		return
	}
	m, isMap := v.(M)
	if !isMap {
		if mm, ok2 := v.(map[string]interface{}); ok2 {
			m, isMap = M(mm), true
		}
	}
	if !isMap {
		e["specs"] = "must be an object of type, class, size and material"
		return
	}
	for k, id := range m {
		if !validSpecKind(k) {
			e["specs."+k] = "unknown specification"
			continue
		}
		if id == nil {
			continue
		}
		s, isStr := id.(string)
		if !isStr || (s != "" && !rePosToken.MatchString(s)) {
			e["specs."+k] = "must be an option id"
		}
	}
}
