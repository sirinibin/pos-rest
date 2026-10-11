package erp

import "github.com/sirinibin/startpos/backend/models"

// roundD rounds money half away from zero at d decimals (the store
// currency's: 3 for OMR, BHD and KWD, else 2), like the legacy save
// (models.RoundMoney); round2 is roundD(x, 2).
func roundD(x float64, d int) float64 {
	if d == 2 {
		return round2(x)
	}
	return models.RoundMoney(x, d)
}

// decimals is the store currency's number of decimals (2 or 3).
func (x *mapCtx) decimals() int {
	if x != nil && storeProfile(x.store).Decimals == 3 {
		return 3
	}
	return 2
}

// ksTotals reproduces the client's ks() taxable/VAT/net for rounding.
func ksTotals(rec M, vat float64) (beforeRounding float64) { return ksTotalsD(rec, vat, 2) }

// ksTotalsD is ksTotals at the currency's decimals.
func ksTotalsD(rec M, vat float64, d int) float64 {
	gross, itemDisc := 0.0, 0.0
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		q := num(im["qty"])
		gross += q * num(im["unitPrice"])
		itemDisc += q * num(im["unitDiscount"])
	}
	taxable := roundD(gross-itemDisc-num(rec["discount"])+num(rec["shipping"]), d)
	v := roundD(taxable*vat/100, d)
	return roundD(taxable+v, d)
}

// legacyBeforeRounding reproduces the legacy save's net before rounding
// (models.Order FindTotal / FindNetTotal and their twins on the other
// documents): it rounds the running line sum to the currency's decimals after
// every line, so with VAT-inclusive prices (4-decimal unit prices) it can land
// a cent away from ksTotals.  The legacy net is legacyBeforeRounding +
// rounding_amount.
func legacyBeforeRounding(rec M, vat float64) float64 { return legacyBeforeRoundingD(rec, vat, 2) }

func legacyBeforeRoundingD(rec M, vat float64, d int) float64 {
	total := 0.0
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		total = roundD(total+num(im["qty"])*(num(im["unitPrice"])-num(im["unitDiscount"])), d)
	}
	base := roundD(total+roundD(num(rec["shipping"]), d)-roundD(num(rec["discount"]), d), d)
	return roundD(base+roundD(base*vat/100, d), d)
}

// roundingCentFix is what the legacy rounding_amount carries on top of the
// client's rounding so that the legacy net equals the client's net (else the
// legacy save rejects a payment of the full total: "Total payment should not
// exceed …").  Read back, it is taken off again.
func roundingCentFix(rec M, vat float64) float64 { return roundingCentFixD(rec, vat, 2) }

func roundingCentFixD(rec M, vat float64, d int) float64 {
	return roundD(ksTotalsD(rec, vat, d)-legacyBeforeRoundingD(rec, vat, d), d)
}
