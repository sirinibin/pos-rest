package erp

import (
	"strings"
	"testing"
)

func TestProductSpecValidate(t *testing.T) {
	long := strings.Repeat("x", 61)
	cases := []struct {
		name string
		rec  M
		errs []string
	}{
		{"class ok", M{"kind": "class", "name": "150"}, nil},
		{"size ok", M{"kind": "size", "name": `2"`}, nil},
		{"material ok + arabic", M{"kind": "material", "name": "SS316", "nameAr": "ستانلس 316"}, nil},
		{"60 chars ok", M{"kind": "class", "name": strings.Repeat("x", 60)}, nil},
		{"type ok", M{"kind": "type", "name": "Valve"}, nil},
		{"unknown kind", M{"kind": "colour", "name": "Red"}, []string{"kind"}},
		{"missing kind", M{"name": "Valve"}, []string{"kind"}},
		{"blank name", M{"kind": "size", "name": "   "}, []string{"name"}},
		{"missing name", M{"kind": "size"}, []string{"name"}},
		{"long name", M{"kind": "size", "name": long}, []string{"name"}},
		{"long arabic", M{"kind": "size", "name": "1", "nameAr": long}, []string{"nameAr"}},
		{"both bad", M{"kind": "", "name": ""}, []string{"kind", "name"}},
	}
	for _, c := range cases {
		e := productSpecValidate(nil, c.rec, nil)
		if len(e) != len(c.errs) {
			t.Errorf("%s: errors %v, want keys %v", c.name, e, c.errs)
			continue
		}
		for _, k := range c.errs {
			if _, ok := e[k]; !ok {
				t.Errorf("%s: missing error %s in %v", c.name, k, e)
			}
		}
	}
}

func TestValidateProductSpecs(t *testing.T) {
	cases := []struct {
		name string
		rec  M
		errs []string
	}{
		{"absent", M{}, nil},
		{"nil", M{"specs": nil}, nil},
		{"all set", M{"specs": M{"class": "psp_ab12", "size": "psp_cd34", "material": "psp_ef56"}}, nil},
		{"plain map", M{"specs": map[string]interface{}{"class": "psp_1"}}, nil},
		{"cleared", M{"specs": M{"class": "", "size": nil}}, nil},
		{"not an object", M{"specs": "150"}, []string{"specs"}},
		{"type set", M{"specs": M{"type": "psp_1"}}, nil},
		{"unknown kind", M{"specs": M{"colour": "psp_1"}}, []string{"specs.colour"}},
		{"bad id", M{"specs": M{"size": "2 inch!"}}, []string{"specs.size"}},
		{"number id", M{"specs": M{"material": 5}}, []string{"specs.material"}},
		{"long id", M{"specs": M{"class": strings.Repeat("a", 41)}}, []string{"specs.class"}},
	}
	for _, c := range cases {
		e := map[string]string{}
		validateProductSpecs(c.rec, e)
		if len(e) != len(c.errs) {
			t.Errorf("%s: errors %v, want keys %v", c.name, e, c.errs)
			continue
		}
		for _, k := range c.errs {
			if _, ok := e[k]; !ok {
				t.Errorf("%s: missing error %s in %v", c.name, k, e)
			}
		}
	}
	// products run it through validatePosFields
	e := map[string]string{}
	validatePosFields(M{"specs": M{"grade": "x"}}, e)
	if e["specs.grade"] == "" {
		t.Errorf("validatePosFields should check specs: %v", e)
	}
}

func TestProductSpecsResourceRegistered(t *testing.T) {
	for _, r := range allResources() {
		if r.Path == "product-specs" {
			if r.Name != "productSpecs" || r.Scope != "store" || r.Module != "inventory" {
				t.Fatalf("product-specs resource: %+v", r)
			}
			return
		}
	}
	t.Fatal("product-specs resource missing")
}
