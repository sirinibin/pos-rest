package models

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestStockIn(t *testing.T) {
	ps := ProductStore{Stock: 10, WarehouseStocks: map[string]float64{"WH1": 3, "WH2": 2}}
	cases := []struct {
		name string
		ps   ProductStore
		code string
		want float64
	}{
		{"warehouse", ps, "WH1", 3},
		{"unknown warehouse", ps, "WH9", 0},
		{"main store not computed yet", ps, "main_store", 5},
		{"main store computed", ProductStore{Stock: 10, WarehouseStocks: map[string]float64{"main_store": 4, "WH1": 6}}, "main_store", 4},
		{"no stock at all", ProductStore{}, "main_store", 0},
		{"negative main store", ProductStore{Stock: -2}, "main_store", -2},
	}
	for _, c := range cases {
		if got := stockIn(c.ps, c.code); got != c.want {
			t.Errorf("%s: stockIn = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTransferSourceCode(t *testing.T) {
	if got := transferSourceCode(nil); got != "main_store" {
		t.Fatalf("nil warehouse = %q, want main_store", got)
	}
	if got := transferSourceCode(&Warehouse{Code: "WH7"}); got != "WH7" {
		t.Fatalf("got %q, want WH7", got)
	}
}

func TestTransferShortfalls(t *testing.T) {
	a, b, svc := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	line := func(id primitive.ObjectID, q float64) StockTransferProduct {
		return StockTransferProduct{ProductID: id, Quantity: q}
	}
	cases := []struct {
		name    string
		lines   []StockTransferProduct
		avail   map[string]float64
		already map[string]float64
		want    map[int]float64
	}{
		{"within stock", []StockTransferProduct{line(a, 3)}, map[string]float64{a.Hex(): 5}, nil, map[int]float64{}},
		{"exactly the stock", []StockTransferProduct{line(a, 5)}, map[string]float64{a.Hex(): 5}, nil, map[int]float64{}},
		{"over by one", []StockTransferProduct{line(a, 6)}, map[string]float64{a.Hex(): 5}, nil, map[int]float64{0: 1}},
		{"nothing in stock", []StockTransferProduct{line(a, 1)}, map[string]float64{a.Hex(): 0}, nil, map[int]float64{0: 1}},
		{"negative stock", []StockTransferProduct{line(a, 1)}, map[string]float64{a.Hex(): -2}, nil, map[int]float64{0: 3}},
		{"two lines share a product", []StockTransferProduct{line(a, 3), line(a, 3)}, map[string]float64{a.Hex(): 5}, nil, map[int]float64{1: 1}},
		{"products are separate", []StockTransferProduct{line(a, 5), line(b, 2)}, map[string]float64{a.Hex(): 5, b.Hex(): 1}, nil, map[int]float64{1: 1}},
		{"untracked product (service or set)", []StockTransferProduct{line(svc, 100)}, map[string]float64{}, nil, map[int]float64{}},
		{"edit re-uses what it moved", []StockTransferProduct{line(a, 5)}, map[string]float64{a.Hex(): 2}, map[string]float64{a.Hex(): 3}, map[int]float64{}},
		{"edit can't go beyond that", []StockTransferProduct{line(a, 6)}, map[string]float64{a.Hex(): 2}, map[string]float64{a.Hex(): 3}, map[int]float64{0: 1}},
		{"fractions", []StockTransferProduct{line(a, 0.3)}, map[string]float64{a.Hex(): 0.1 + 0.2}, nil, map[int]float64{}},
	}
	for _, c := range cases {
		got := transferShortfalls(c.lines, c.avail, c.already)
		if len(got) != len(c.want) {
			t.Errorf("%s: shortfalls = %v, want %v", c.name, got, c.want)
			continue
		}
		for i, v := range c.want {
			if got[i] != v {
				t.Errorf("%s: line %d short %v, want %v", c.name, i, got[i], v)
			}
		}
	}
}

func TestZatcaSecretsAreHiddenAndKept(t *testing.T) {
	stored := &Store{}
	stored.Zatca.PrivateKey, stored.Zatca.Secret, stored.Zatca.BinarySecurityToken = "key", "sec", "tok"
	stored.Zatca.ProductionSecret, stored.Zatca.ProductionBinarySecurityToken = "psec", "ptok"
	stored.Zatca.Csr = "csr"

	out := *stored
	out.HideZatcaSecrets()
	z := out.Zatca
	if z.PrivateKey != "" || z.Secret != "" || z.BinarySecurityToken != "" || z.ProductionSecret != "" || z.ProductionBinarySecurityToken != "" {
		t.Fatalf("secrets left in the response: %+v", z)
	}
	if z.Csr != "csr" {
		t.Fatalf("the CSR (public) was dropped")
	}

	// The form sends them back blank, or planted; the stored ones win.
	form := out
	form.Zatca.Secret = "planted"
	form.KeepZatcaSecretsFrom(stored)
	if form.Zatca.PrivateKey != "key" || form.Zatca.Secret != "sec" || form.Zatca.BinarySecurityToken != "tok" ||
		form.Zatca.ProductionSecret != "psec" || form.Zatca.ProductionBinarySecurityToken != "ptok" {
		t.Fatalf("stored credentials not kept: %+v", form.Zatca)
	}
}

func TestSameWarehouse(t *testing.T) {
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	zero := primitive.NilObjectID
	cases := []struct {
		name string
		x, y *primitive.ObjectID
		want bool
	}{
		{"both main (nil)", nil, nil, true},
		{"nil and zero are the main store", nil, &zero, true},
		{"same warehouse", &a, &a, true},
		{"different warehouses", &a, &b, false},
		{"warehouse and main store", &a, nil, false},
		{"main store and warehouse", &zero, &b, false},
	}
	for _, c := range cases {
		if got := sameWarehouse(c.x, c.y); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
