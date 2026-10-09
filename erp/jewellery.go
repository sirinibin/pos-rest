package erp

import (
	"math"
	"regexp"
	"strings"
)

// Jewellery details of a product (kept in erp.x as `jewel`), used by the
// Jewellery POS terminal to price a piece by weight at the day's metal rate:
//
//	{"metal":"gold","purity":"22K","pricing":"weight","gross":12.35,"stone":0.4,
//	 "net":11.95,"makingType":"gram","making":25,"wastage":8,"stoneValue":350,
//	 "stoneNote":"CZ","huid":"AB12C3","hallmark":true,"certificate":"IGI 123"}
//
// price (before tax) = net × rate(purity) × (1 + wastage%) + making + stoneValue
// where making is per gram of net weight, a percent of the metal value or a
// fixed amount. "fixed" pricing items sell at their retail price instead.

// JewelPurities lists the purities of each metal (karat for gold, fineness for
// silver and platinum).
var JewelPurities = map[string][]string{
	"gold":     {"24K", "22K", "21K", "18K", "14K"},
	"silver":   {"999", "925"},
	"platinum": {"950"},
}

var jewelMakingTypes = map[string]bool{"gram": true, "percent": true, "fixed": true}

// reHUID: India's 6-character hallmark unique ID (BIS), letters and digits.
var reHUID = regexp.MustCompile(`^[A-Z0-9]{6}$`)

const maxJewelWeight = 100000.0 // grams

func validJewelPurity(metal, purity string) bool {
	for _, p := range JewelPurities[metal] {
		if p == purity {
			return true
		}
	}
	return false
}

// validateJewel checks a product's `jewel` object (absent or null: nothing).
func validateJewel(rec M, e map[string]string) {
	v, ok := rec["jewel"]
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
		e["jewel"] = "must be an object"
		return
	}
	metal := str(m["metal"])
	if _, known := JewelPurities[metal]; !known {
		e["jewel.metal"] = "one of gold, silver, platinum"
	} else if !validJewelPurity(metal, str(m["purity"])) {
		e["jewel.purity"] = "one of " + strings.Join(JewelPurities[metal], ", ")
	}
	pricing := str(m["pricing"])
	if pricing != "" && pricing != "weight" && pricing != "fixed" {
		e["jewel.pricing"] = "weight or fixed"
	}
	weight := func(k string) (float64, bool) {
		x, present := m[k]
		if !present || x == nil {
			return 0, false
		}
		n, isNum := x.(float64)
		if !isNum {
			if i, isInt := x.(int); isInt {
				n, isNum = float64(i), true
			} else if i64, isInt64 := x.(int64); isInt64 {
				n, isNum = float64(i64), true
			}
		}
		if !isNum || math.IsNaN(n) || n < 0 || n > maxJewelWeight {
			e["jewel."+k] = "grams between 0 and 100000"
			return 0, false
		}
		return n, true
	}
	gross, hasGross := weight("gross")
	stone, _ := weight("stone")
	net, hasNet := weight("net")
	if pricing != "fixed" && (!hasGross || gross <= 0) && e["jewel.gross"] == "" {
		e["jewel.gross"] = "required: the gross weight in grams"
	}
	if hasGross && stone > gross {
		e["jewel.stone"] = "can't exceed the gross weight"
	}
	if hasNet && hasGross && net > gross+0.0005 {
		e["jewel.net"] = "can't exceed the gross weight"
	}
	if mt := str(m["makingType"]); mt != "" && !jewelMakingTypes[mt] {
		e["jewel.makingType"] = "gram, percent or fixed"
	}
	if mk, present := m["making"]; present && mk != nil {
		n := num(mk)
		if n < 0 || n > 1e7 {
			e["jewel.making"] = "must be 0 or more"
		} else if str(m["makingType"]) == "percent" && n > 100 {
			e["jewel.making"] = "a percent up to 100"
		}
	}
	if w, present := m["wastage"]; present && w != nil {
		if n := num(w); n < 0 || n > 50 {
			e["jewel.wastage"] = "a percent from 0 to 50"
		}
	}
	if sv, present := m["stoneValue"]; present && sv != nil && num(sv) < 0 {
		e["jewel.stoneValue"] = "must be 0 or more"
	}
	if len([]rune(str(m["stoneNote"]))) > 80 {
		e["jewel.stoneNote"] = "at most 80 characters"
	}
	if len([]rune(str(m["certificate"]))) > 40 {
		e["jewel.certificate"] = "at most 40 characters"
	}
	if h := str(m["huid"]); h != "" && !reHUID.MatchString(h) {
		e["jewel.huid"] = "HUID: 6 capital letters and digits"
	}
	if hm, present := m["hallmark"]; present && hm != nil {
		if _, isBool := hm.(bool); !isBool {
			e["jewel.hallmark"] = "true or false"
		}
	}
}
