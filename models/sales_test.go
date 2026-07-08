package models

import (
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func vatPct(v float64) *float64 { return &v }

func makeOrder(products []OrderProduct, vatPct float64, discount, shipping float64) Order {
	return Order{
		Products:               products,
		VatPercent:             vatPct2(vatPct),
		Discount:               discount,
		ShippingOrHandlingFees: shipping,
		AutoRoundingAmount:     false,
	}
}

func vatPct2(v float64) *float64 { return &v }

func makeProduct(qty, price, discount float64) OrderProduct {
	return OrderProduct{
		Quantity:    qty,
		UnitPrice:   price,
		UnitDiscount: discount,
	}
}

func makeB2BCustomer() Customer {
	return Customer{
		VATNo: "123456789012345",
		NationalAddress: NationalAddress{
			ZipCode:      "12345",
			BuildingNo:   "1234",
			StreetName:   "King Fahd Road",
			DistrictName: "Al-Malaz",
			CityName:     "Riyadh",
		},
	}
}

// ── FindTotal ─────────────────────────────────────────────────────────────────

func TestOrder_FindTotal_SingleProduct_NoDiscount(t *testing.T) {
	o := makeOrder([]OrderProduct{makeProduct(2, 50.00, 0)}, 15, 0, 0)
	o.FindTotal()
	if o.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", o.Total)
	}
}

func TestOrder_FindTotal_SingleProduct_WithDiscount(t *testing.T) {
	o := makeOrder([]OrderProduct{makeProduct(1, 100.00, 10.00)}, 15, 0, 0)
	o.FindTotal()
	if o.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", o.Total)
	}
}

func TestOrder_FindTotal_MultipleProducts(t *testing.T) {
	products := []OrderProduct{
		makeProduct(1, 50.00, 0),    // 50.00
		makeProduct(2, 25.00, 5.00), // 2*(25-5) = 40.00
		makeProduct(3, 10.00, 0),    // 30.00
	}
	o := makeOrder(products, 15, 0, 0)
	o.FindTotal()
	if o.Total != 120.00 {
		t.Errorf("Total = %v, want 120.00", o.Total)
	}
}

func TestOrder_FindTotal_ZeroQuantity(t *testing.T) {
	o := makeOrder([]OrderProduct{makeProduct(0, 100.00, 0)}, 15, 0, 0)
	o.FindTotal()
	if o.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", o.Total)
	}
}

func TestOrder_FindTotal_EmptyProducts(t *testing.T) {
	o := makeOrder([]OrderProduct{}, 15, 0, 0)
	o.FindTotal()
	if o.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", o.Total)
	}
}

func TestOrder_FindTotal_FractionalPrices(t *testing.T) {
	// 3 * 33.333 = 99.999 → rounds to 100.00
	o := makeOrder([]OrderProduct{makeProduct(3, 33.333, 0)}, 15, 0, 0)
	o.FindTotal()
	if o.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", o.Total)
	}
}

// ── FindNetTotal ──────────────────────────────────────────────────────────────

func TestOrder_FindNetTotal_BasicVAT15(t *testing.T) {
	// Total=100, VAT=15% → NetTotal=115
	o := makeOrder([]OrderProduct{makeProduct(1, 100.00, 0)}, 15, 0, 0)
	o.FindNetTotal()

	if o.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", o.Total)
	}
	if o.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00", o.VatPrice)
	}
	if o.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", o.NetTotal)
	}
}

func TestOrder_FindNetTotal_WithOrderDiscount(t *testing.T) {
	// Total=100, Discount=10, VAT=15% → base=90, VatPrice=13.50, NetTotal=103.50
	o := makeOrder([]OrderProduct{makeProduct(1, 100.00, 0)}, 15, 10.00, 0)
	o.FindNetTotal()

	if o.VatPrice != 13.50 {
		t.Errorf("VatPrice = %v, want 13.50", o.VatPrice)
	}
	if o.NetTotal != 103.50 {
		t.Errorf("NetTotal = %v, want 103.50", o.NetTotal)
	}
}

func TestOrder_FindNetTotal_WithShippingFees(t *testing.T) {
	// Total=100, Shipping=20, VAT=15% → base=120, VatPrice=18, NetTotal=138
	o := makeOrder([]OrderProduct{makeProduct(1, 100.00, 0)}, 15, 0, 20.00)
	o.FindNetTotal()

	if o.VatPrice != 18.00 {
		t.Errorf("VatPrice = %v, want 18.00", o.VatPrice)
	}
	if o.NetTotal != 138.00 {
		t.Errorf("NetTotal = %v, want 138.00", o.NetTotal)
	}
}

func TestOrder_FindNetTotal_WithDiscountAndShipping(t *testing.T) {
	// Total=100, Discount=10, Shipping=20, VAT=15% → base=110, VatPrice=16.50, NetTotal=126.50
	o := makeOrder([]OrderProduct{makeProduct(1, 100.00, 0)}, 15, 10.00, 20.00)
	o.FindNetTotal()

	if o.VatPrice != 16.50 {
		t.Errorf("VatPrice = %v, want 16.50", o.VatPrice)
	}
	if o.NetTotal != 126.50 {
		t.Errorf("NetTotal = %v, want 126.50", o.NetTotal)
	}
}

func TestOrder_FindNetTotal_ZeroVAT(t *testing.T) {
	// Total=100, VAT=0% → VatPrice=0, NetTotal=100
	o := makeOrder([]OrderProduct{makeProduct(1, 100.00, 0)}, 0, 0, 0)
	o.FindNetTotal()

	if o.VatPrice != 0.00 {
		t.Errorf("VatPrice = %v, want 0.00", o.VatPrice)
	}
	if o.NetTotal != 100.00 {
		t.Errorf("NetTotal = %v, want 100.00", o.NetTotal)
	}
}

func TestOrder_FindNetTotal_RoundingEdgeCase(t *testing.T) {
	// qty=1, price=99.99, VAT=15% → base=99.99, VatPrice=15.00 (rounded), NetTotal=114.99
	o := makeOrder([]OrderProduct{makeProduct(1, 99.99, 0)}, 15, 0, 0)
	o.FindNetTotal()
	want := 99.99 + RoundTo2Decimals(99.99*0.15)
	want = RoundTo2Decimals(want)
	if o.NetTotal != want {
		t.Errorf("NetTotal = %v, want %v", o.NetTotal, want)
	}
}

func TestOrder_FindNetTotal_DiscountExceedsTotal(t *testing.T) {
	// Discount larger than Total: base becomes negative; VAT computed on negative base
	o := makeOrder([]OrderProduct{makeProduct(1, 50.00, 0)}, 15, 100.00, 0)
	o.FindNetTotal()
	// base = 50 - 100 = -50; VatPrice = -7.50; NetTotal = -57.50
	if o.VatPrice != -7.50 {
		t.Errorf("VatPrice = %v, want -7.50", o.VatPrice)
	}
	if o.NetTotal != -57.50 {
		t.Errorf("NetTotal = %v, want -57.50", o.NetTotal)
	}
}

// ── CalculateDiscountPercentage ───────────────────────────────────────────────

func TestOrder_CalculateDiscountPercentage_ZeroDiscount(t *testing.T) {
	o := Order{Discount: 0, NetTotal: 115.00}
	o.CalculateDiscountPercentage()
	if o.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00", o.DiscountPercent)
	}
}

func TestOrder_CalculateDiscountPercentage_NegativeDiscount(t *testing.T) {
	o := Order{Discount: -5.00, NetTotal: 115.00}
	o.CalculateDiscountPercentage()
	if o.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00 (negative discount treated as zero)", o.DiscountPercent)
	}
}

func TestOrder_CalculateDiscountPercentage_WithDiscount(t *testing.T) {
	// NetTotal=103.50 (after discount=10), so base=103.50+10=113.50
	// percent = 10/113.50*100 ≈ 8.81%
	o := Order{Discount: 10.00, NetTotal: 103.50}
	o.CalculateDiscountPercentage()
	want := RoundTo2Decimals((10.00 / 113.50) * 100)
	if o.DiscountPercent != want {
		t.Errorf("DiscountPercent = %v, want %v", o.DiscountPercent, want)
	}
}

func TestOrder_CalculateDiscountPercentage_ZeroBase(t *testing.T) {
	// NetTotal=0, Discount>0 but NetTotal+Discount=0 only when discount=0 is not the case here.
	// If NetTotal=-10 and Discount=10 then base=0 → divide-by-zero guard
	o := Order{Discount: 10.00, NetTotal: -10.00}
	o.CalculateDiscountPercentage()
	if o.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00 (zero base guard)", o.DiscountPercent)
	}
}

// ── FindTotalQuantity ─────────────────────────────────────────────────────────

func TestOrder_FindTotalQuantity_SumAllProducts(t *testing.T) {
	products := []OrderProduct{
		makeProduct(3, 10, 0),
		makeProduct(5, 20, 0),
		makeProduct(2, 30, 0),
	}
	o := Order{Products: products}
	o.FindTotalQuantity()
	if o.TotalQuantity != 10 {
		t.Errorf("TotalQuantity = %v, want 10", o.TotalQuantity)
	}
}

func TestOrder_FindTotalQuantity_EmptyProducts(t *testing.T) {
	o := Order{Products: []OrderProduct{}}
	o.FindTotalQuantity()
	if o.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", o.TotalQuantity)
	}
}

func TestOrder_FindTotalQuantity_FractionalQuantity(t *testing.T) {
	products := []OrderProduct{
		makeProduct(1.5, 10, 0),
		makeProduct(2.5, 10, 0),
	}
	o := Order{Products: products}
	o.FindTotalQuantity()
	if o.TotalQuantity != 4.0 {
		t.Errorf("TotalQuantity = %v, want 4.0", o.TotalQuantity)
	}
}

// ── IsCreditLimitExceeded ─────────────────────────────────────────────────────

func makeAssetCustomer(creditBalance, creditLimit float64) Customer {
	return Customer{
		CreditBalance: creditBalance,
		CreditLimit:   creditLimit,
		Account:       &Account{Type: "asset"},
	}
}

func makeLiabilityCustomer(creditBalance, creditLimit float64) Customer {
	return Customer{
		CreditBalance: creditBalance,
		CreditLimit:   creditLimit,
		Account:       &Account{Type: "liability"},
	}
}

func TestCustomer_IsCreditLimitExceeded_Asset_NotExceeded(t *testing.T) {
	c := makeAssetCustomer(100, 200)
	if c.IsCreditLimitExceeded(50, false) {
		t.Error("expected false: 100+50=150 does not exceed limit 200")
	}
}

func TestCustomer_IsCreditLimitExceeded_Asset_Exceeded(t *testing.T) {
	c := makeAssetCustomer(100, 200)
	if !c.IsCreditLimitExceeded(150, false) {
		t.Error("expected true: 100+150=250 exceeds limit 200")
	}
}

func TestCustomer_IsCreditLimitExceeded_Asset_ExactlyAtLimit(t *testing.T) {
	c := makeAssetCustomer(100, 200)
	// 100+100=200; 200>200 is false
	if c.IsCreditLimitExceeded(100, false) {
		t.Error("expected false: exactly at limit (not strictly over)")
	}
}

func TestCustomer_IsCreditLimitExceeded_Asset_Return_ReducesBalance(t *testing.T) {
	c := makeAssetCustomer(100, 200)
	// isReturn=true: newBalance=100-50=50 < 200
	if c.IsCreditLimitExceeded(50, true) {
		t.Error("expected false: return reduces balance to 50, under limit 200")
	}
}

func TestCustomer_IsCreditLimitExceeded_Asset_NegativeCreditBalance(t *testing.T) {
	// Negative creditBalance is flipped to positive
	c := makeAssetCustomer(-100, 200)
	// abs=100; 100+50=150 < 200
	if c.IsCreditLimitExceeded(50, false) {
		t.Error("expected false: abs(balance)=100, 100+50=150 under limit 200")
	}
}

func TestCustomer_IsCreditLimitExceeded_Liability_NotExceeded(t *testing.T) {
	c := makeLiabilityCustomer(100, 50)
	// newBalance=100-30=70; -70>50? No
	if c.IsCreditLimitExceeded(30, false) {
		t.Error("expected false: 100-30=70, -70 does not exceed limit 50")
	}
}

func TestCustomer_IsCreditLimitExceeded_Liability_Exceeded(t *testing.T) {
	c := makeLiabilityCustomer(100, 50)
	// newBalance=100-200=-100; -(-100)=100>50? Yes
	if !c.IsCreditLimitExceeded(200, false) {
		t.Error("expected true: 100-200=-100 → 100 exceeds limit 50")
	}
}

func TestCustomer_IsCreditLimitExceeded_Liability_Return_IncreasesBalance(t *testing.T) {
	c := makeLiabilityCustomer(100, 200)
	// isReturn=true: newBalance=100+50=150; -150>200? No
	if c.IsCreditLimitExceeded(50, true) {
		t.Error("expected false: return increases balance to 150, -150 under limit 200")
	}
}

func TestCustomer_IsCreditLimitExceeded_UnknownType_AlwaysFalse(t *testing.T) {
	c := Customer{
		CreditBalance: 9999,
		CreditLimit:   1,
		Account:       &Account{Type: "equity"},
	}
	if c.IsCreditLimitExceeded(10000, false) {
		t.Error("expected false: unknown account type always returns false")
	}
}

// ── WillEditExceedCreditLimit ─────────────────────────────────────────────────

func TestCustomer_WillEditExceedCreditLimit_Asset_Increase_NotExceeded(t *testing.T) {
	c := makeAssetCustomer(200, 300)
	// delta=150-100=50; newBalance=200+50=250 < 300
	if c.WillEditExceedCreditLimit(100, 150, false) {
		t.Error("expected false: balance 200+50=250 under limit 300")
	}
}

func TestCustomer_WillEditExceedCreditLimit_Asset_Increase_Exceeded(t *testing.T) {
	c := makeAssetCustomer(200, 300)
	// delta=400-100=300; newBalance=200+300=500 > 300
	if !c.WillEditExceedCreditLimit(100, 400, false) {
		t.Error("expected true: balance 200+300=500 exceeds limit 300")
	}
}

func TestCustomer_WillEditExceedCreditLimit_Asset_Decrease_NeverExceeds(t *testing.T) {
	c := makeAssetCustomer(200, 300)
	// newAmount < oldAmount; delta=50-100=-50; newBalance=200-50=150 < 300
	if c.WillEditExceedCreditLimit(100, 50, false) {
		t.Error("expected false: decreasing amount cannot exceed credit limit")
	}
}

func TestCustomer_WillEditExceedCreditLimit_Liability_Increase_Exceeded(t *testing.T) {
	c := makeLiabilityCustomer(100, 50)
	// delta=oldAmount-newAmount=100-300=-200; newBalance=100+(-200)=-100; -(-100)=100>50? Yes
	if !c.WillEditExceedCreditLimit(100, 300, false) {
		t.Error("expected true: increasing amount on liability exceeds limit")
	}
}

func TestCustomer_WillEditExceedCreditLimit_UnknownType(t *testing.T) {
	c := Customer{
		CreditBalance: 9999,
		CreditLimit:   1,
		Account:       &Account{Type: "revenue"},
	}
	if c.WillEditExceedCreditLimit(100, 500, false) {
		t.Error("expected false: unknown account type always returns false")
	}
}

// ── IsB2B ─────────────────────────────────────────────────────────────────────

func TestCustomer_IsB2B_Valid(t *testing.T) {
	c := makeB2BCustomer()
	if !c.IsB2B() {
		t.Error("expected true: fully populated B2B customer")
	}
}

func TestCustomer_IsB2B_EmptyVATNo(t *testing.T) {
	c := makeB2BCustomer()
	c.VATNo = ""
	if c.IsB2B() {
		t.Error("expected false: empty VATNo")
	}
}

func TestCustomer_IsB2B_ShortVATNo_14Digits(t *testing.T) {
	c := makeB2BCustomer()
	c.VATNo = "12345678901234" // 14 digits, not 15
	if c.IsB2B() {
		t.Error("expected false: 14-digit VATNo is not valid")
	}
}

func TestCustomer_IsB2B_LongVATNo_16Digits(t *testing.T) {
	c := makeB2BCustomer()
	c.VATNo = "1234567890123456" // 16 digits
	if c.IsB2B() {
		t.Error("expected false: 16-digit VATNo is not valid")
	}
}

func TestCustomer_IsB2B_AlphaVATNo(t *testing.T) {
	c := makeB2BCustomer()
	c.VATNo = "12345678901234A" // letters mixed in
	if c.IsB2B() {
		t.Error("expected false: non-numeric VATNo")
	}
}

func TestCustomer_IsB2B_MissingZipCode(t *testing.T) {
	c := makeB2BCustomer()
	c.NationalAddress.ZipCode = ""
	if c.IsB2B() {
		t.Error("expected false: missing ZipCode")
	}
}

func TestCustomer_IsB2B_InvalidZipCode_4Digits(t *testing.T) {
	c := makeB2BCustomer()
	c.NationalAddress.ZipCode = "1234" // must be exactly 5 digits
	if c.IsB2B() {
		t.Error("expected false: 4-digit ZipCode is not valid")
	}
}

func TestCustomer_IsB2B_MissingBuildingNo(t *testing.T) {
	c := makeB2BCustomer()
	c.NationalAddress.BuildingNo = ""
	if c.IsB2B() {
		t.Error("expected false: missing BuildingNo")
	}
}

func TestCustomer_IsB2B_InvalidBuildingNo_3Digits(t *testing.T) {
	c := makeB2BCustomer()
	c.NationalAddress.BuildingNo = "123" // must be exactly 4 digits
	if c.IsB2B() {
		t.Error("expected false: 3-digit BuildingNo is not valid")
	}
}

func TestCustomer_IsB2B_MissingStreetName(t *testing.T) {
	c := makeB2BCustomer()
	c.NationalAddress.StreetName = ""
	if c.IsB2B() {
		t.Error("expected false: missing StreetName")
	}
}

func TestCustomer_IsB2B_MissingDistrictName(t *testing.T) {
	c := makeB2BCustomer()
	c.NationalAddress.DistrictName = ""
	if c.IsB2B() {
		t.Error("expected false: missing DistrictName")
	}
}

func TestCustomer_IsB2B_MissingCityName(t *testing.T) {
	c := makeB2BCustomer()
	c.NationalAddress.CityName = ""
	if c.IsB2B() {
		t.Error("expected false: missing CityName")
	}
}

func TestCustomer_IsB2B_InvalidRegistrationNumber(t *testing.T) {
	c := makeB2BCustomer()
	c.RegistrationNumber = "CR-1234!@#" // non-alphanumeric characters
	if c.IsB2B() {
		t.Error("expected false: non-alphanumeric RegistrationNumber")
	}
}

func TestCustomer_IsB2B_ValidAlphanumericRegistrationNumber(t *testing.T) {
	c := makeB2BCustomer()
	c.RegistrationNumber = "CR1234567"
	if !c.IsB2B() {
		t.Error("expected true: alphanumeric RegistrationNumber is valid")
	}
}

// ── resolveDateKeyword ────────────────────────────────────────────────────────

func TestResolveDateKeyword_UnknownKeyword(t *testing.T) {
	_, _, ok := resolveDateKeyword("foobar", 0)
	if ok {
		t.Error("expected ok=false for unknown keyword")
	}
}

func TestResolveDateKeyword_EmptyString(t *testing.T) {
	_, _, ok := resolveDateKeyword("", 0)
	if ok {
		t.Error("expected ok=false for empty keyword")
	}
}

func TestResolveDateKeyword_Today_ReturnsOkAndStartBeforeEnd(t *testing.T) {
	start, end, ok := resolveDateKeyword("today", 0)
	if !ok {
		t.Fatal("expected ok=true for 'today'")
	}
	if !start.Before(end) {
		t.Errorf("start %v should be before end %v", start, end)
	}
}

func TestResolveDateKeyword_Now_SameAsToday(t *testing.T) {
	s1, e1, ok1 := resolveDateKeyword("today", 0)
	s2, e2, ok2 := resolveDateKeyword("now", 0)
	if !ok1 || !ok2 {
		t.Fatal("both 'today' and 'now' should return ok=true")
	}
	if !s1.Equal(s2) || !e1.Equal(e2) {
		t.Error("'today' and 'now' should resolve to the same range")
	}
}

func TestResolveDateKeyword_CaseInsensitive(t *testing.T) {
	_, _, ok := resolveDateKeyword("TODAY", 0)
	if !ok {
		t.Error("expected ok=true: keywords should be case-insensitive")
	}
}

func TestResolveDateKeyword_Yesterday_ReturnsOkAndStartBeforeEnd(t *testing.T) {
	start, end, ok := resolveDateKeyword("yesterday", 0)
	if !ok {
		t.Fatal("expected ok=true for 'yesterday'")
	}
	if !start.Before(end) {
		t.Errorf("start %v should be before end %v", start, end)
	}
}

func TestResolveDateKeyword_Yesterday_IsBeforeToday(t *testing.T) {
	todayStart, _, _ := resolveDateKeyword("today", 0)
	_, yesterdayEnd, _ := resolveDateKeyword("yesterday", 0)
	if !yesterdayEnd.Before(todayStart) {
		t.Errorf("yesterday end %v should be before today start %v", yesterdayEnd, todayStart)
	}
}

func TestResolveDateKeyword_ThisWeek_ReturnsOkAndStartBeforeEnd(t *testing.T) {
	start, end, ok := resolveDateKeyword("this week", 0)
	if !ok {
		t.Fatal("expected ok=true for 'this week'")
	}
	if !start.Before(end) {
		t.Errorf("start %v should be before end %v", start, end)
	}
}

func TestResolveDateKeyword_CurrentWeek_SameAsThisWeek(t *testing.T) {
	s1, e1, _ := resolveDateKeyword("this week", 0)
	s2, e2, ok := resolveDateKeyword("current week", 0)
	if !ok {
		t.Fatal("expected ok=true for 'current week'")
	}
	if !s1.Equal(s2) || !e1.Equal(e2) {
		t.Error("'this week' and 'current week' should resolve to the same range")
	}
}

func TestResolveDateKeyword_LastWeek_IsBeforeThisWeek(t *testing.T) {
	thisStart, _, _ := resolveDateKeyword("this week", 0)
	_, lastEnd, ok := resolveDateKeyword("last week", 0)
	if !ok {
		t.Fatal("expected ok=true for 'last week'")
	}
	if !lastEnd.Before(thisStart) {
		t.Errorf("last week end %v should be before this week start %v", lastEnd, thisStart)
	}
}

func TestResolveDateKeyword_ThisMonth_ReturnsOkAndStartBeforeEnd(t *testing.T) {
	start, end, ok := resolveDateKeyword("this month", 0)
	if !ok {
		t.Fatal("expected ok=true for 'this month'")
	}
	if !start.Before(end) {
		t.Errorf("start %v should be before end %v", start, end)
	}
}

func TestResolveDateKeyword_LastMonth_IsBeforeThisMonth(t *testing.T) {
	thisStart, _, _ := resolveDateKeyword("this month", 0)
	_, lastEnd, ok := resolveDateKeyword("last month", 0)
	if !ok {
		t.Fatal("expected ok=true for 'last month'")
	}
	if !lastEnd.Before(thisStart) {
		t.Errorf("last month end %v should be before this month start %v", lastEnd, thisStart)
	}
}

func TestResolveDateKeyword_ThisYear_ReturnsOkAndStartBeforeEnd(t *testing.T) {
	start, end, ok := resolveDateKeyword("this year", 0)
	if !ok {
		t.Fatal("expected ok=true for 'this year'")
	}
	if !start.Before(end) {
		t.Errorf("start %v should be before end %v", start, end)
	}
}

func TestResolveDateKeyword_LastYear_IsBeforeThisYear(t *testing.T) {
	thisStart, _, _ := resolveDateKeyword("this year", 0)
	_, lastEnd, ok := resolveDateKeyword("last year", 0)
	if !ok {
		t.Fatal("expected ok=true for 'last year'")
	}
	if !lastEnd.Before(thisStart) {
		t.Errorf("last year end %v should be before this year start %v", lastEnd, thisStart)
	}
}

func TestResolveDateKeyword_LastNDays_Valid(t *testing.T) {
	start, end, ok := resolveDateKeyword("last 7 days", 0)
	if !ok {
		t.Fatal("expected ok=true for 'last 7 days'")
	}
	if !start.Before(end) {
		t.Errorf("start %v should be before end %v", start, end)
	}
}

func TestResolveDateKeyword_LastNDays_SpanIsNDays(t *testing.T) {
	start, end, ok := resolveDateKeyword("last 30 days", 0)
	if !ok {
		t.Fatal("expected ok=true for 'last 30 days'")
	}
	// span is (end+1s - start) ≈ 30 * 24h
	span := end.Sub(start)
	days := span.Hours() / 24
	if days < 29.9 || days > 30.1 {
		t.Errorf("'last 30 days' span = %.2f days, want ~30", days)
	}
}

func TestResolveDateKeyword_PastNDays_SameAsLastNDays(t *testing.T) {
	s1, e1, _ := resolveDateKeyword("last 7 days", 0)
	s2, e2, ok := resolveDateKeyword("past 7 days", 0)
	if !ok {
		t.Fatal("expected ok=true for 'past 7 days'")
	}
	if !s1.Equal(s2) || !e1.Equal(e2) {
		t.Error("'last 7 days' and 'past 7 days' should resolve identically")
	}
}

func TestResolveDateKeyword_LastNDays_ZeroIsInvalid(t *testing.T) {
	_, _, ok := resolveDateKeyword("last 0 days", 0)
	if ok {
		t.Error("expected ok=false for 'last 0 days' (n must be > 0)")
	}
}

func TestResolveDateKeyword_LastNDays_NegativeIsInvalid(t *testing.T) {
	_, _, ok := resolveDateKeyword("last -5 days", 0)
	if ok {
		t.Error("expected ok=false for 'last -5 days'")
	}
}

func TestResolveDateKeyword_TimezoneOffset_SA(t *testing.T) {
	// Saudi Arabia is UTC+3, represented as tzOffset=-3 in this codebase
	start0, end0, _ := resolveDateKeyword("today", 0)
	startSA, endSA, ok := resolveDateKeyword("today", -3)
	if !ok {
		t.Fatal("expected ok=true")
	}
	// SA today starts 3h earlier in UTC than UTC+0 today
	diff := start0.Sub(startSA).Hours()
	if diff < 2.9 || diff > 3.1 {
		t.Errorf("SA start should be ~3h earlier than UTC+0 start, got diff=%.2fh", diff)
	}
	_ = endSA
	_ = end0
}
