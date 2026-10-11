package models

import (
	"math"
	"strconv"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// RoundMoney rounds half away from zero at the given number of decimals, on
// the decimal value of x: x is first read back at 15 significant digits, so a
// binary artefact does not move a half below it (3.30 × 15% is 0.495, which
// float64 holds as 0.49499…: math.Round(x*100)/100 gave 0.49, this gives
// 0.50). It is the web app's rounding (erp round2) at any precision.
func RoundMoney(x float64, decimals int) float64 {
	if x == 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	p := math.Pow10(decimals)
	a, _ := strconv.ParseFloat(strconv.FormatFloat(math.Abs(x)*p, 'g', 15, 64), 64)
	r := math.Floor(a+0.5) / p
	if r == 0 {
		return 0
	}
	return math.Copysign(r, x)
}

// moneyDecimalsCache: store id → currency decimals (a store's country rarely
// changes; a change is picked up within moneyDecimalsTTL).
var moneyDecimalsCache sync.Map

type moneyDecimalsEntry struct {
	decimals int
	at       time.Time
}

const moneyDecimalsTTL = time.Minute

// StoreMoneyDecimals is the number of decimals of the store's currency: 3 for
// OMR, BHD and KWD, else 2 (also for an unknown store).
func StoreMoneyDecimals(storeID *primitive.ObjectID) int {
	if storeID == nil || storeID.IsZero() {
		return 2
	}
	if e, ok := moneyDecimalsCache.Load(*storeID); ok {
		if en := e.(moneyDecimalsEntry); time.Since(en.at) < moneyDecimalsTTL {
			return en.decimals
		}
	}
	store, err := FindStoreByID(storeID, bson.M{"country_code": 1})
	if err != nil || store == nil {
		return 2
	}
	d := StoreCountryProfile(store).Decimals
	if d != 3 {
		d = 2
	}
	moneyDecimalsCache.Store(*storeID, moneyDecimalsEntry{decimals: d, at: time.Now()})
	return d
}

// money rounds an amount of the document at its store's currency decimals.
func (order *Order) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(order.StoreID))
}

func (salesReturn *SalesReturn) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(salesReturn.StoreID))
}

func (quotation *Quotation) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(quotation.StoreID))
}

func (quotationsalesReturn *QuotationSalesReturn) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(quotationsalesReturn.StoreID))
}

func (s *NonVATSales) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(s.StoreID))
}

func (r *NonVATSalesReturn) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(r.StoreID))
}

func (purchase *Purchase) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(purchase.StoreID))
}

func (purchaseReturn *PurchaseReturn) money(x float64) float64 {
	return RoundMoney(x, StoreMoneyDecimals(purchaseReturn.StoreID))
}
