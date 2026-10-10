package erp

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
)

// Property/fuzz tests of the money math every document total goes through,
// checked against exact decimal arithmetic (shopspring/decimal) instead of
// floats. `go test` runs the seed corpus; `go test -fuzz=FuzzX ./erp/` explores.

func dec2(d decimal.Decimal) float64 {
	f, _ := d.Round(2).Float64() // half away from zero, like the app
	return f
}

// round2 of an amount with 3 decimals is exact half-away-from-zero rounding.
func FuzzRound2(f *testing.F) {
	for _, k := range []int64{0, 5, 15, 125, 1005, 1015, 2675, -5, -1005, 999995, 123456785, 1 << 40} {
		f.Add(k)
	}
	f.Fuzz(func(t *testing.T, k int64) {
		if k > 1e14 || k < -1e14 { // amounts up to 100 billion
			return
		}
		x := float64(k) / 1000
		want := dec2(decimal.New(k, -3))
		if got := round2(x); got != want {
			t.Fatalf("round2(%v) = %v, want %v", x, got, want)
		}
		if round2(round2(x)) != round2(x) || round2(-x) != -round2(x) {
			t.Fatalf("round2 not idempotent/symmetric at %v", x)
		}
	})
}

// The cash rounding lands the total on the 0.05 grid and never moves it more
// than half a step.
func FuzzAutoRounding(f *testing.F) {
	for _, c := range []int64{0, 1, 2, 3, 4, 5, 7, 99, 101, 12345, 99999999} {
		f.Add(c)
	}
	f.Fuzz(func(t *testing.T, cents int64) {
		if cents < 0 || cents > 1e12 {
			return
		}
		before := float64(cents) / 100
		r := autoRounding(before, nil)
		after := decimal.NewFromInt(cents).Add(decimal.NewFromFloat(r).Mul(decimal.NewFromInt(100)).Round(0))
		if !after.Mod(decimal.NewFromInt(5)).IsZero() {
			t.Fatalf("%.2f + rounding %.2f is not a multiple of 0.05", before, r)
		}
		if math.Abs(r) > 0.025+1e-9 {
			t.Fatalf("rounding %.2f of %.2f is more than half a step", r, before)
		}
	})
}

// ksTotals (the document net before cash rounding) equals the exact decimal
// calculation, and the legacy per-line rounding is at most half a cent per
// line away from it (what rounding_amount absorbs, roundingCentFix).
func FuzzDocumentTotals(f *testing.F) {
	f.Add(int64(1000), int64(3333), int64(0), int64(1000), int64(9999), int64(150), int64(0), int64(0), uint8(15))
	f.Add(int64(3), int64(1), int64(0), int64(7), int64(13043), int64(0), int64(5), int64(250), uint8(15))
	f.Add(int64(2500), int64(869565), int64(1), int64(1), int64(0), int64(0), int64(0), int64(0), uint8(5))
	f.Fuzz(func(t *testing.T, q1, p1, d1, q2, p2, d2, disc, ship int64, vatPct uint8) {
		// qty in thousandths (0.001–10,000), prices in 1/10,000 (VAT-inclusive
		// entry gives 4-decimal unit prices), discounts and shipping in cents
		for _, v := range []int64{q1, q2} {
			if v <= 0 || v > 1e7 {
				return
			}
		}
		for _, v := range []int64{p1, p2, d1, d2} {
			if v < 0 || v > 1e10 {
				return
			}
		}
		if d1 > p1 || d2 > p2 || disc < 0 || ship < 0 || disc > 1e9 || ship > 1e9 {
			return
		}
		vat := float64(vatPct % 31)
		rec := M{"discount": float64(disc) / 100, "shipping": float64(ship) / 100, "items": []interface{}{
			M{"qty": float64(q1) / 1000, "unitPrice": float64(p1) / 10000, "unitDiscount": float64(d1) / 10000},
			M{"qty": float64(q2) / 1000, "unitPrice": float64(p2) / 10000, "unitDiscount": float64(d2) / 10000},
		}}
		line := func(q, p, d int64) decimal.Decimal {
			return decimal.New(q, -3).Mul(decimal.New(p, -4).Sub(decimal.New(d, -4)))
		}
		taxable := line(q1, p1, d1).Add(line(q2, p2, d2)).Sub(decimal.New(disc, -2)).Add(decimal.New(ship, -2)).Round(2)
		vatAmt := taxable.Mul(decimal.NewFromFloat(vat)).Div(decimal.NewFromInt(100)).Round(2)
		want := dec2(taxable.Add(vatAmt))
		got := ksTotals(rec, vat)
		if math.Abs(want) > 1e11 {
			return // beyond float64's cent precision
		}
		if math.Round(got*100) != math.Round(want*100) {
			t.Fatalf("ksTotals = %.2f, exact %.2f (rec %v, vat %v)", got, want, rec, vat)
		}
		if fix := roundingCentFix(rec, vat); math.Abs(fix) > 0.02*(1+vat/100)+1e-9 {
			t.Fatalf("legacy rounding drifts %.2f from the exact total (rec %v)", fix, rec)
		}
	})
}
