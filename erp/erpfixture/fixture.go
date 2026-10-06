// Package erpfixture builds a LEGACY-SHAPED fixture database for the StartERP
// adapter tests and the e2e harness: a main DB plus two store_<id> DBs whose
// documents are written exactly like the existing models/bson tags produce
// them (including an "old" store with missing/legacy fields, documents
// without uuid/zatca/payments[]/product_stores, numbers stored as int32,
// misspelled keys such as `produc_id`, collections such as `salesreturn`).
//
// It never runs against production: Seed refuses DB names that do not start
// with a test prefix.
package erpfixture

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Password used for every fixture user.
const Password = "Demo@2026"

// Fixture holds the ids of the seeded documents.
type Fixture struct {
	MainDB                                  string
	StoreA, StoreB                          primitive.ObjectID
	Admin, ManagerA, SalesmanA, UserB       primitive.ObjectID
	AdminEmail, ManagerEmail, SalesEmail    string
	UserBEmail                              string
	WarehouseA                              primitive.ObjectID
	CategoryA, BrandA, ExpenseCatA, VendCat primitive.ObjectID
	ProductA1, ProductA2, ProductA3         primitive.ObjectID
	ServiceA                                primitive.ObjectID
	CustomerA1, CustomerA2                  primitive.ObjectID
	VendorA1                                primitive.ObjectID
	OrderA1, OrderA2                        primitive.ObjectID
	SalesReturnA1                           primitive.ObjectID
	QuotationA1, PurchaseA1, ExpenseA1      primitive.ObjectID
	DepositA1, WithdrawalA1, CapitalA1      primitive.ObjectID
	DividentA1, EmployeeA1, SalaryA1        primitive.ObjectID
	VehicleA1, RepairJobA1, TransferA1      primitive.ObjectID
	DeliveryNoteA1, LegacyRoleA, AccountA1  primitive.ObjectID
	PackageA1                               primitive.ObjectID
	ProductB1, CustomerB1, OrderB1          primitive.ObjectID
	Now                                     time.Time
}

func ins(dbName, coll string, docs ...bson.M) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := db.GetDB(dbName).Collection(coll)
	for _, d := range docs {
		if _, err := c.InsertOne(ctx, d); err != nil {
			return fmt.Errorf("insert %s.%s: %w", dbName, coll, err)
		}
	}
	return nil
}

func ptrF(f float64) *float64 { return &f }

// Seed inserts the fixture. mainDB must start with "t" + digit + "_" or
// "test" (safety guard).
func Seed(mainDB string) (*Fixture, error) {
	if !(strings.HasPrefix(mainDB, "t1_") || strings.HasPrefix(mainDB, "test") || strings.HasPrefix(mainDB, "erp_test")) {
		return nil, fmt.Errorf("refusing to seed non-test database %q", mainDB)
	}
	f := &Fixture{MainDB: mainDB, Now: time.Now().UTC().Truncate(time.Millisecond)}
	n := f.Now
	day := func(d int) time.Time { return n.AddDate(0, 0, -d) }
	oid := primitive.NewObjectID
	f.StoreA, f.StoreB = oid(), oid()
	f.Admin, f.ManagerA, f.SalesmanA, f.UserB = oid(), oid(), oid(), oid()
	suffix := strings.ToLower(f.StoreA.Hex()[18:])
	f.AdminEmail = "admin+" + suffix + "@t1.example"
	f.ManagerEmail = "Manager+" + suffix + "@T1.example" // mixed case on purpose
	f.SalesEmail = "sales+" + suffix + "@t1.example"
	f.UserBEmail = "userb+" + suffix + "@t1.example"
	hash := models.HashPassword(Password)
	sA, sB := f.StoreA.Hex(), f.StoreB.Hex()
	dbA, dbB := "store_"+sA, "store_"+sB

	serial := func(prefix string, pad int64) bson.M {
		return bson.M{"prefix": prefix, "start_from_count": int64(1), "padding_count": pad}
	}
	// ---- main DB: stores ----
	storeA := bson.M{"_id": f.StoreA, "name": "Al Noor Trading", "name_in_arabic": "شركة النور التجارية", "code": "ANT",
		"branch_name": "Olaya", "business_category": "Retail", "registration_number": "1010101010", "vat_no": "310122393500003",
		"registration_number_arabic": "١٠١٠١٠١٠١٠", "vat_no_in_arabic": "٣١٠١٢٢٣٩٣٥٠٠٠٠٣", "phone_in_arabic": "٠١١٢٣٤٥٦٧٨",
		"vat_percent": 15.0, "email": "store@t1.example", "phone": "0112345678", "country_name": "Saudi Arabia", "country_code": "SA",
		"national_address": bson.M{"building_no": "1234", "street_name": "Olaya St", "street_name_arabic": "شارع العليا",
			"district_name": "Olaya", "district_name_arabic": "العليا", "city_name": "Riyadh", "city_name_arabic": "الرياض",
			"zipcode": "12345", "additional_no": "6789"},
		"zatca":               bson.M{"phase": "2", "env": "NonProduction", "connected": false, "compliance_check": bson.M{}},
		"sales_serial_number": serial("S-INV", 3), "sales_return_serial_number": serial("SR-INV", 3),
		"purchase_serial_number": serial("P-INV", 3), "purchase_return_serial_number": serial("PR-INV", 3),
		"purchase_order_serial_number": serial("PO", 4), "purchase_request_serial_number": serial("PRQ", 4),
		"quotation_serial_number": serial("QTN", 3), "quotation_sales_return_serial_number": serial("QTN-SR", 3),
		"customer_serial_number": serial("CUST", 4), "vendor_serial_number": serial("VND", 4),
		"expense_serial_number": serial("EXP", 4), "delivery_note_serial_number": serial("DN", 4),
		"customer_deposit_serial_number": serial("CUST-RCVBLE", 4), "customer_withdrawal_serial_number": serial("CUST-PAYBLE", 4),
		"capital_deposit_serial_number": serial("CAP-DPST", 4), "divident_serial_number": serial("CAP-DRWNG", 4),
		"stock_transfer_serial_number": serial("ST-TR", 3), "non_vat_sales_serial_number": serial("NVS", 3),
		"non_vat_sales_return_serial_number": serial("NVS-R", 3),
		"bank_account":                       bson.M{"bank_name": "SNB", "iban": "SA0380000000608010167519", "account_name": "Al Noor", "account_no": "608010167519"},
		"settings": bson.M{"enable_warehouse_module": true, "enable_rbac_module": false, "enable_monthly_serial_number": false,
			"invoice": bson.M{"quotation_title": "Quotation", "receivabale_title": "", "payable_title": ""}},
		"title": "Tax Invoice", "title_in_arabic": "فاتورة ضريبية",
		"created_at": day(400), "updated_at": day(10), "created_by_name": "Seeder"}
	// old store: no settings sub-doc, legacy top-level flags, no national address, no serial numbers
	storeB := bson.M{"_id": f.StoreB, "name": "Old Branch", "code": "OLD1", "branch_name": "Jeddah", "vat_no": "",
		"vat_percent": int32(15), "country_name": "", "show_address_in_invoice_footer": true,
		"enable_monthly_serial_number": false, "zatca_qr_on_left_bottom": true,
		"sales_serial_number": serial("INV", 4), "created_at": day(2000)}
	if err := ins("", "store", storeA, storeB); err != nil {
		return nil, err
	}
	// ---- users ----
	if err := ins("", "user",
		bson.M{"_id": f.Admin, "name": "Admin T1", "email": f.AdminEmail, "mob": "0500000001", "password": hash,
			"admin": true, "role": "Admin", "store_ids": bson.A{f.StoreA, f.StoreB}, "created_at": day(500), "updated_at": day(500)},
		bson.M{"_id": f.ManagerA, "name": "Manager A", "email": f.ManagerEmail, "mob": "0500000002", "password": hash,
			"admin": false, "role": "Manager", "store_ids": bson.A{f.StoreA}, "store_names": bson.A{"Al Noor Trading"}, "created_at": day(300)},
		bson.M{"_id": f.SalesmanA, "name": "Sales A", "email": f.SalesEmail, "mob": "0500000003", "password": hash,
			"role": "SalesMan", "store_ids": bson.A{f.StoreA}, "created_at": day(200)},
		bson.M{"_id": f.UserB, "name": "User B", "email": f.UserBEmail, "mob": "0500000004", "password": hash,
			"role": "Manager", "store_ids": bson.A{f.StoreB}},
	); err != nil {
		return nil, err
	}
	// ---- store A master data ----
	f.WarehouseA = oid()
	f.CategoryA, f.BrandA, f.ExpenseCatA, f.VendCat = oid(), oid(), oid(), oid()
	if err := ins(dbA, "warehouse", bson.M{"_id": f.WarehouseA, "name": "Warehouse 1", "code": "WH1", "store_id": f.StoreA,
		"created_at": day(300), "updated_at": day(300)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "product_category", bson.M{"_id": f.CategoryA, "name": "Engine Oil", "store_id": f.StoreA, "deleted": false, "created_at": day(300)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "product_brand", bson.M{"_id": f.BrandA, "name": "Mobil", "code": "MOB", "store_id": f.StoreA, "deleted": false}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "expense_category", bson.M{"_id": f.ExpenseCatA, "name": "Rent", "store_id": f.StoreA}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "vendor_category", bson.M{"_id": f.VendCat, "name": "Lubricants", "store_id": f.StoreA}); err != nil {
		return nil, err
	}
	f.ProductA1, f.ProductA2, f.ProductA3, f.ServiceA = oid(), oid(), oid(), oid()
	opening := func(q float64, wh *primitive.ObjectID, code *string) bson.M {
		return bson.M{"date": day(365), "type": "adding", "quantity": q, "reason": "opening", "warehouse_id": wh, "warehouse_code": code, "created_at": day(365)}
	}
	whCode := "WH1"
	ps := func(purchase, retail, stock float64, adj bson.A, wh bson.M) bson.M {
		return bson.M{sA: bson.M{"store_id": f.StoreA, "store_name": "Al Noor Trading", "purchase_unit_price": purchase,
			"purchase_unit_price_with_vat": purchase * 1.15, "retail_unit_price": retail, "retail_unit_price_with_vat": retail * 1.15,
			"wholesale_unit_price": retail * 0.9, "stock": stock, "warehouse_stocks": wh, "stock_adjustments": adj,
			"sales_count": int64(0), "sales_quantity": 0.0}}
	}
	if err := ins(dbA, "product",
		bson.M{"_id": f.ProductA1, "name": "Engine Oil 5W30 4L", "name_in_arabic": "زيت محرك", "item_code": "EO-5W30-4L", "ean_12": "100000000001",
			"part_number": "PN-001", "prefix_part_number": "", "unit": "pcs", "store_id": f.StoreA, "category_id": bson.A{f.CategoryA},
			"category_name": bson.A{"Engine Oil"}, "brand_id": f.BrandA, "brand_name": "Mobil", "is_service": false, "deleted": false,
			"product_stores": ps(80, 120, 100, bson.A{opening(90, nil, nil), opening(10, &f.WarehouseA, &whCode)}, bson.M{"main_store": 90.0, "WH1": 10.0}),
			"set":            bson.M{"products": bson.A{}}, "created_at": day(365), "updated_at": day(30)},
		bson.M{"_id": f.ProductA2, "name": "Oil Filter", "item_code": "OF-1", "ean_12": "100000000002", "part_number": "PN-002", "unit": "pcs",
			"store_id": f.StoreA, "is_service": false, "deleted": false,
			"product_stores": ps(10, 25, 50, bson.A{opening(50, nil, nil)}, bson.M{"main_store": 50.0}),
			"created_at":     day(365)},
		// OLD shape: no product_stores, no name_in_arabic, top-level rack, set with misspelled produc_id
		bson.M{"_id": f.ProductA3, "name": "Legacy Brake Pad", "item_code": "BP-OLD", "ean_12": "100000000003", "part_number": "PN-003", "store_id": f.StoreA,
			"rack": "R-9", "set": bson.M{"products": bson.A{bson.M{"produc_id": f.ProductA2, "quantity": int32(2)}}}, "is_set": true,
			"created_at": day(1500)},
		bson.M{"_id": f.ServiceA, "name": "Labour charge", "item_code": "SRV-LAB", "ean_12": "100000000004", "store_id": f.StoreA, "is_service": true,
			"product_stores": ps(0, 100, 0, bson.A{}, bson.M{}), "created_at": day(300)},
	); err != nil {
		return nil, err
	}
	f.CustomerA1, f.CustomerA2, f.VendorA1 = oid(), oid(), oid()
	if err := ins(dbA, "customer",
		bson.M{"_id": f.CustomerA1, "code": "CUST-0001", "name": "Riyadh Motors", "name_in_arabic": "الرياض موتورز",
			"vat_no": "300000000000003", "phone": "0512345678", "credit_limit": 5000.0, "store_id": f.StoreA, "deleted": false,
			"national_address": bson.M{"building_no": "1111", "street_name": "King Fahd Rd", "city_name": "Riyadh", "zipcode": "11564"},
			"stores":           bson.M{sA: bson.M{"sales_count": int64(2)}}, "created_at": day(300), "updated_at": day(20)},
		// old customer: missing fields, int32 credit limit, phone with spaces, no national address
		bson.M{"_id": f.CustomerA2, "code": "CUST-0002", "name": "Walk In Old", "phone": "05 1234 5679", "credit_limit": int32(0),
			"store_id": f.StoreA, "created_at": day(1800)},
	); err != nil {
		return nil, err
	}
	if err := ins(dbA, "vendor", bson.M{"_id": f.VendorA1, "code": "VND-0001", "name": "Gulf Lubricants", "vat_no": "311111111100003",
		"store_id": f.StoreA, "category_id": bson.A{f.VendCat}, "category_name": bson.A{"Lubricants"}, "deleted": false, "created_at": day(300)}); err != nil {
		return nil, err
	}
	// ---- store A transactions ----
	f.OrderA1, f.OrderA2, f.SalesReturnA1 = oid(), oid(), oid()
	pay1 := oid()
	line := func(p primitive.ObjectID, name string, q, price, cost float64, wh *primitive.ObjectID, whc *string) bson.M {
		return bson.M{"product_id": p, "warehouse_id": wh, "warehouse_code": whc, "name": name, "quantity": q, "quantity_returned": 0.0,
			"unit_price": price, "unit_price_with_vat": price * 1.15, "purchase_unit_price": cost, "unit": "pcs",
			"unit_discount": 0.0, "unit_discount_with_vat": 0.0, "unit_discount_percent": 0.0, "profit": (price - cost) * q}
	}
	if err := ins(dbA, "order",
		bson.M{"_id": f.OrderA1, "code": "S-INV-001", "date": day(30), "store_id": f.StoreA, "customer_id": f.CustomerA1,
			"customer_name": "Riyadh Motors", "vat_no": "300000000000003", "vat_percent": 15.0, "invoice_count_value": int64(1),
			"uuid": "11111111-2222-3333-4444-555555555555", "hash": "abc=", "prev_hash": "NWZlY2ViNjZmZmM4NmYzOGQ5NTI3ODZjNmQ2OTZjNzljMmRiYzIzOWRkNGU5MWI0NjcyOWQ3M2EyN2ZiNTdlOQ==",
			"products": bson.A{line(f.ProductA1, "Engine Oil 5W30 4L", 2, 120, 80, nil, nil)},
			"discount": 0.0, "shipping_handling_fees": 0.0, "total": 240.0, "total_with_vat": 276.0, "vat_price": 36.0, "net_total": 276.0,
			"rounding_amount": 0.0, "auto_rounding_amount": false, "cash_discount": 0.0, "total_payment_received": 276.0, "balance_amount": 0.0,
			"payment_status": "paid", "payment_methods": bson.A{"cash"}, "payments_count": int64(1),
			"payments": bson.A{bson.M{"_id": pay1, "date": day(30), "order_id": f.OrderA1, "order_code": "S-INV-001", "amount": 276.0, "method": "cash", "store_id": f.StoreA}},
			"zatca":    bson.M{"is_simplified": false, "reporting_passed": true, "reporting_passed_at": day(30), "qr_code": "QRDATA"},
			"profit":   80.0, "created_at": day(30), "updated_at": day(30), "created_by_name": "Manager A", "status": "delivered"},
		// OLD order: no payments[] embedded, no uuid/zatca, vat_percent int32, warehouse line
		bson.M{"_id": f.OrderA2, "code": "S-INV-000", "date": day(700), "store_id": f.StoreA, "customer_id": f.CustomerA2,
			"customer_name": "Walk In Old", "vat_percent": int32(15),
			"products": bson.A{line(f.ProductA2, "Oil Filter", 1, 20, 10, &f.WarehouseA, &whCode)},
			"total":    20.0, "vat_price": 3.0, "net_total": 23.0, "total_payment_received": 23.0, "balance_amount": 0.0,
			"payment_status": "paid", "created_at": day(700)},
	); err != nil {
		return nil, err
	}
	if err := ins(dbA, "sales_payment",
		bson.M{"_id": pay1, "date": day(30), "order_id": f.OrderA1, "order_code": "S-INV-001", "amount": 276.0, "method": "cash", "store_id": f.StoreA, "deleted": false},
		bson.M{"_id": oid(), "date": day(700), "order_id": f.OrderA2, "order_code": "S-INV-000", "amount": 23.0, "method": "bank_card", "store_id": f.StoreA, "deleted": false},
	); err != nil {
		return nil, err
	}
	srl := line(f.ProductA1, "Engine Oil 5W30 4L", 1, 120, 80, nil, nil)
	srl["selected"] = true
	delete(srl, "quantity_returned")
	if err := ins(dbA, "salesreturn", bson.M{"_id": f.SalesReturnA1, "code": "SR-INV-001", "order_id": f.OrderA1, "order_code": "S-INV-001",
		"date": day(20), "store_id": f.StoreA, "customer_id": f.CustomerA1, "customer_name": "Riyadh Motors", "vat_percent": 15.0,
		"products": bson.A{srl}, "total": 120.0, "vat_price": 18.0, "net_total": 138.0, "payment_status": "not_paid", "deleted": false,
		"created_at": day(20), "updated_at": day(20)}); err != nil {
		return nil, err
	}
	f.QuotationA1, f.PurchaseA1, f.ExpenseA1 = oid(), oid(), oid()
	if err := ins(dbA, "quotation", bson.M{"_id": f.QuotationA1, "code": "QTN-001", "date": day(40), "store_id": f.StoreA,
		"customer_id": f.CustomerA1, "customer_name": "Riyadh Motors", "type": "quotation", "status": "created", "vat_percent": 15.0,
		"validity_days": int64(7), "delivery_days": int64(3), "products": bson.A{line(f.ProductA1, "Engine Oil 5W30 4L", 5, 118, 80, nil, nil)},
		"net_total": 678.5, "created_at": day(40)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "purchase", bson.M{"_id": f.PurchaseA1, "code": "P-INV-001", "date": day(60), "store_id": f.StoreA,
		"vendor_id": f.VendorA1, "vendor_name": "Gulf Lubricants", "vendor_invoice_no": "GL-77", "vat_percent": 15.0,
		"products": bson.A{bson.M{"product_id": f.ProductA1, "name": "Engine Oil 5W30 4L", "quantity": 10.0, "purchase_unit_price": 80.0,
			"retail_unit_price": 120.0, "unit": "pcs", "unit_discount": 0.0}},
		"net_total": 920.0, "total_payment_paid": 0.0, "payment_status": "not_paid", "created_at": day(60)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "expense", bson.M{"_id": f.ExpenseA1, "code": "EXP-0001", "amount": 1150.0, "vat_price": 150.0, "description": "Shop rent",
		"date": day(15), "payment_method": "bank_transfer", "store_id": f.StoreA, "category_id": bson.A{f.ExpenseCatA},
		"category_name": bson.A{"Rent"}, "vendor_id": f.VendorA1, "deleted": false, "created_at": day(15)}); err != nil {
		return nil, err
	}
	f.DepositA1, f.WithdrawalA1, f.CapitalA1, f.DividentA1 = oid(), oid(), oid(), oid()
	if err := ins(dbA, "customerdeposit", bson.M{"_id": f.DepositA1, "code": "CUST-RCVBLE-0001", "date": day(10), "store_id": f.StoreA,
		"customer_id": f.CustomerA1, "customer_name": "Riyadh Motors", "type": "customer", "payment_method": "cash",
		"payments": bson.A{bson.M{"_id": oid(), "date": day(10), "amount": 500.0, "method": "cash"}}, "total": 500.0, "net_total": 500.0,
		"remarks": "Advance", "created_at": day(10)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "customerwithdrawal", bson.M{"_id": f.WithdrawalA1, "code": "CUST-PAYBLE-0001", "date": day(9), "store_id": f.StoreA,
		"customer_id": f.CustomerA1, "customer_name": "Riyadh Motors", "type": "customer", "payment_method": "cash",
		"payments": bson.A{bson.M{"_id": oid(), "date": day(9), "amount": 50.0, "method": "cash"}}, "amount": 50.0, "net_total": 50.0,
		"created_at": day(9)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "capital", bson.M{"_id": f.CapitalA1, "code": "CAP-DPST-0001", "amount": 100000.0, "description": "Initial capital",
		"date": day(400), "invested_by_user_id": f.Admin, "invested_by_user_name": "Admin T1", "payment_method": "bank_transfer", "store_id": f.StoreA}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "divident", bson.M{"_id": f.DividentA1, "code": "CAP-DRWNG-0001", "amount": 2000.0, "description": "Drawing",
		"date": day(50), "withdrawn_by_user_id": f.Admin, "withdrawn_by_user_name": "Admin T1", "payment_method": "cash", "store_id": f.StoreA}); err != nil {
		return nil, err
	}
	f.EmployeeA1, f.SalaryA1, f.VehicleA1, f.RepairJobA1 = oid(), oid(), oid(), oid()
	if err := ins(dbA, "employee", bson.M{"_id": f.EmployeeA1, "code": "EMP-0001", "name": "Ahmed Tech", "iqama_no": "2123456789",
		"position": "Technician", "salary": 4000.0, "salary_day": int32(1), "joining_date": day(900), "is_active": true, "store_id": f.StoreA,
		"mob1": "0555555555", "deleted": false}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "employee_salary_payment", bson.M{"_id": f.SalaryA1, "code": "SAL-0001", "employee_id": f.EmployeeA1,
		"employee_name": "Ahmed Tech", "store_id": f.StoreA, "date": day(5), "amount": 4000.0, "payment_method": "bank_transfer",
		"month": int32(n.Month()), "year": int32(n.Year()), "deleted": false}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "vehicle", bson.M{"_id": f.VehicleA1, "store_id": f.StoreA, "customer_id": f.CustomerA1, "customer_name": "Riyadh Motors",
		"vehicle_number": "ABJ 1234", "brand": "Toyota", "model": "Camry", "year": int32(2020), "chassis_number": "JTDBR32E720123456", "deleted": false}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "repair_job", bson.M{"_id": f.RepairJobA1, "store_id": f.StoreA, "job_number": "JOB-0001", "title": "Brake noise",
		"date": day(3), "vehicle_id": f.VehicleA1, "customer_id": f.CustomerA1, "customer_name": "Riyadh Motors", "vehicle_number": "ABJ 1234",
		"brand": "Toyota", "model": "Camry", "km": 45000.0, "complaint": "Brake noise when stopping", "status": "open", "labour_charge": 100.0,
		"vat_percent": 15.0, "parts": bson.A{bson.M{"product_id": f.ProductA2, "name": "Oil Filter", "qty": 1.0, "unit_price": 25.0}}, "deleted": false}); err != nil {
		return nil, err
	}
	f.TransferA1, f.DeliveryNoteA1, f.LegacyRoleA, f.AccountA1 = oid(), oid(), oid(), oid()
	if err := ins(dbA, "stocktransfer", bson.M{"_id": f.TransferA1, "code": "ST-TR-001", "date": day(25), "store_id": f.StoreA,
		"from_warehouse_id": nil, "from_warehouse_code": nil, "to_warehouse_id": f.WarehouseA, "to_warehouse_code": "WH1",
		"products": bson.A{bson.M{"product_id": f.ProductA2, "name": "Oil Filter", "quantity": 1.0, "unit_price": 10.0}}, "created_at": day(25)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "delivery_note", bson.M{"_id": f.DeliveryNoteA1, "code": "DN-0001", "date": day(12), "store_id": f.StoreA,
		"customer_id": f.CustomerA1, "customer_name": "Riyadh Motors", "products": bson.A{bson.M{"product_id": f.ProductA1, "name": "Engine Oil 5W30 4L", "quantity": 1.0}},
		"vat_percent": 15.0, "created_at": day(12)}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "user_role", bson.M{"_id": f.LegacyRoleA, "name": "Legacy Stock Clerk", "store_id": f.StoreA, "store_name": "Al Noor Trading",
		"permissions": bson.A{bson.M{"resource": "products", "read": true, "create": true, "update": true, "delete": false},
			bson.M{"resource": "purchases", "read": true, "create": false, "update": false, "delete": false}}, "deleted": false}); err != nil {
		return nil, err
	}
	if err := ins(dbA, "account", bson.M{"_id": f.AccountA1, "store_id": f.StoreA, "type": "asset", "number": "1000", "name": "CASH",
		"balance": 1234.5, "debit_or_credit_balance": "debit_balance", "open": true}); err != nil {
		return nil, err
	}
	f.PackageA1 = oid()
	if err := ins("", "customer_package", bson.M{"_id": f.PackageA1, "store_id": f.StoreA, "code": "ANT-PKG-1", "name": "Oil change x5",
		"customer_id": f.CustomerA1, "customer_name": "Riyadh Motors", "price": 500.0, "visits": int32(5), "used": int32(1),
		"valid_from": n.Format("2006-01-02"), "valid_days": int32(365), "status": "active"}); err != nil {
		return nil, err
	}
	// ---- store B (old store) ----
	f.ProductB1, f.CustomerB1, f.OrderB1 = oid(), oid(), oid()
	if err := ins(dbB, "product", bson.M{"_id": f.ProductB1, "name": "Old Store Item", "ean_12": "100000000001", "store_id": f.StoreB, "created_at": day(1900)}); err != nil {
		return nil, err
	}
	if err := ins(dbB, "customer", bson.M{"_id": f.CustomerB1, "name": "Jeddah Customer", "store_id": f.StoreB}); err != nil {
		return nil, err
	}
	if err := ins(dbB, "order", bson.M{"_id": f.OrderB1, "code": "INV-0001", "date": day(1900), "store_id": f.StoreB, "customer_id": f.CustomerB1,
		"products":  bson.A{bson.M{"product_id": f.ProductB1, "name": "Old Store Item", "quantity": int32(3), "unit_price": "10"}},
		"net_total": 34.5}); err != nil {
		return nil, err
	}
	return f, nil
}

// Drop removes every database created by the fixture.
func (f *Fixture) Drop() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, name := range []string{"store_" + f.StoreA.Hex(), "store_" + f.StoreB.Hex()} {
		_ = db.GetDB(name).Drop(ctx)
	}
	main := db.GetDB("")
	_, _ = main.Collection("store").DeleteMany(ctx, bson.M{"_id": bson.M{"$in": bson.A{f.StoreA, f.StoreB}}})
	_, _ = main.Collection("user").DeleteMany(ctx, bson.M{"store_ids": bson.M{"$in": bson.A{f.StoreA, f.StoreB}}})
	_, _ = main.Collection("customer_package").DeleteMany(ctx, bson.M{"store_id": bson.M{"$in": bson.A{f.StoreA, f.StoreB}}})
}

var _ = ptrF
