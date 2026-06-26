package models

import (
	"encoding/json"
	"strings"
)

// buildImageURL constructs a full image URL from components.
// If filename already starts with "/" it is returned as-is (backward compat with old full-path records).
func buildImageURL(storeID, category, entityID, filename string) string {
	if filename == "" || strings.HasPrefix(filename, "/") {
		return filename
	}
	if entityID != "" {
		return "/images/" + storeID + "/" + category + "/" + entityID + "/" + filename
	}
	return "/images/" + storeID + "/" + category + "/" + filename
}

func buildImageURLs(storeID, category, entityID string, images []string) []string {
	if len(images) == 0 {
		return images
	}
	out := make([]string, len(images))
	for i, img := range images {
		out[i] = buildImageURL(storeID, category, entityID, img)
	}
	return out
}

func (p Product) MarshalJSON() ([]byte, error) {
	type Alias Product
	a := Alias(p)
	storeID := ""
	if p.StoreID != nil {
		storeID = p.StoreID.Hex()
	} else {
		// Products are often global (no store_id); use first store from ProductStores map
		for sid := range p.ProductStores {
			storeID = sid
			break
		}
	}
	if storeID != "" {
		a.Images = buildImageURLs(storeID, "products", p.ID.Hex(), p.Images)
	}
	return json.Marshal(a)
}

func (v Vendor) MarshalJSON() ([]byte, error) {
	type Alias Vendor
	a := Alias(v)
	if v.StoreID != nil {
		a.Images = buildImageURLs(v.StoreID.Hex(), "vendors", v.ID.Hex(), v.Images)
		a.Logo = buildImageURL(v.StoreID.Hex(), "vendors", "", v.Logo)
	}
	return json.Marshal(a)
}

func (c Customer) MarshalJSON() ([]byte, error) {
	type Alias Customer
	a := Alias(c)
	if c.StoreID != nil {
		a.Images = buildImageURLs(c.StoreID.Hex(), "customers", c.ID.Hex(), c.Images)
	}
	return json.Marshal(a)
}

func (e Expense) MarshalJSON() ([]byte, error) {
	type Alias Expense
	a := Alias(e)
	if e.StoreID != nil {
		a.Images = buildImageURLs(e.StoreID.Hex(), "expenses", "", e.Images)
	}
	return json.Marshal(a)
}

func (c Capital) MarshalJSON() ([]byte, error) {
	type Alias Capital
	a := Alias(c)
	if c.StoreID != nil {
		a.Images = buildImageURLs(c.StoreID.Hex(), "capitals", "", c.Images)
	}
	return json.Marshal(a)
}

func (c CapitalWithdrawal) MarshalJSON() ([]byte, error) {
	type Alias CapitalWithdrawal
	a := Alias(c)
	if c.StoreID != nil {
		a.Images = buildImageURLs(c.StoreID.Hex(), "capital_withdrawals", "", c.Images)
	}
	return json.Marshal(a)
}

func (c CustomerDeposit) MarshalJSON() ([]byte, error) {
	type Alias CustomerDeposit
	a := Alias(c)
	if c.StoreID != nil {
		a.Images = buildImageURLs(c.StoreID.Hex(), "customer_deposits", "", c.Images)
	}
	return json.Marshal(a)
}

func (c CustomerWithdrawal) MarshalJSON() ([]byte, error) {
	type Alias CustomerWithdrawal
	a := Alias(c)
	if c.StoreID != nil {
		a.Images = buildImageURLs(c.StoreID.Hex(), "customer_withdrawals", "", c.Images)
	}
	return json.Marshal(a)
}

func (d Divident) MarshalJSON() ([]byte, error) {
	type Alias Divident
	a := Alias(d)
	if d.StoreID != nil {
		a.Images = buildImageURLs(d.StoreID.Hex(), "dividents", "", d.Images)
	}
	return json.Marshal(a)
}

func (s UserSignature) MarshalJSON() ([]byte, error) {
	type Alias UserSignature
	a := Alias(s)
	if s.StoreID != nil {
		a.Signature = buildImageURL(s.StoreID.Hex(), "signatures", "", s.Signature)
	}
	return json.Marshal(a)
}

func (u User) MarshalJSON() ([]byte, error) {
	type Alias User
	a := Alias(u)
	if u.Photo != "" && !strings.HasPrefix(u.Photo, "/") {
		storeFolder := "global"
		if len(u.StoreIDs) > 0 && u.StoreIDs[0] != nil {
			storeFolder = u.StoreIDs[0].Hex()
		}
		a.Photo = buildImageURL(storeFolder, "users", "", u.Photo)
	}
	return json.Marshal(a)
}

func (s Store) MarshalJSON() ([]byte, error) {
	type Alias Store
	a := Alias(s)
	a.Logo = buildImageURL(s.ID.Hex(), "store", "", s.Logo)
	return json.Marshal(a)
}
