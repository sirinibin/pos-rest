package models

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ClearStoreData drops every collection in the store's database (store_<id>),
// resets the store's Redis serial-number counters, recreates indexes and
// re-posts cash/bank opening balances. Store settings (in the main DB) are kept.
func (store *Store) ClearStoreData() (droppedCount int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	storeDB := db.GetDB("store_" + store.ID.Hex())
	colNames, err := storeDB.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return 0, errors.New("failed to list collections: " + err.Error())
	}

	for _, colName := range colNames {
		if err := storeDB.Collection(colName).Drop(ctx); err != nil {
			return droppedCount, errors.New("failed to drop collection " + colName + ": " + err.Error())
		}
		droppedCount++
	}

	// Reset all Redis serial-number counters scoped to this store
	// (keys look like "<storeIDHex>_customer_counter", "<storeIDHex>_invoice_counter", …)
	if db.RedisClient != nil {
		keys, kerr := db.RedisClient.Keys(store.ID.Hex() + "_*").Result()
		if kerr == nil && len(keys) > 0 {
			if derr := db.RedisClient.Del(keys...).Err(); derr != nil {
				log.Printf("ClearStoreData: failed to delete redis counters for store %s: %v", store.ID.Hex(), derr)
			}
		}
	}

	if err := store.CreateAllIndexes(); err != nil {
		log.Printf("ClearStoreData: failed to recreate indexes for store %s: %v", store.ID.Hex(), err)
	}

	if err := store.PostCashOpeningBalanceIfNeeded(); err != nil {
		log.Printf("ClearStoreData: failed to re-post cash opening balance for store %s: %v", store.ID.Hex(), err)
	}
	if err := store.PostBankOpeningBalanceIfNeeded(); err != nil {
		log.Printf("ClearStoreData: failed to re-post bank opening balance for store %s: %v", store.ID.Hex(), err)
	}

	return droppedCount, nil
}

// PopulateAutomobileTestData fills the store with realistic sample data for an
// automobile workshop: customers, technicians (employees), spare parts &
// services, vehicles and repair jobs in a variety of statuses.
func (store *Store) PopulateAutomobileTestData(userID *primitive.ObjectID) (summary map[string]int, err error) {
	summary = map[string]int{}
	now := time.Now()

	vatPercent := store.VatPercent
	if vatPercent == 0 {
		vatPercent = 15
	}

	// ---------- Customers ----------
	customerSeeds := []struct {
		Name       string
		NameArabic string
		Phone      string
	}{
		{"Ahmed Al-Qahtani", "أحمد القحطاني", "0501111101"},
		{"Mohammed Al-Otaibi", "محمد العتيبي", "0501111102"},
		{"Khalid Al-Harbi", "خالد الحربي", "0501111103"},
		{"Faisal Al-Ghamdi", "فيصل الغامدي", "0501111104"},
		{"Abdullah Al-Shehri", "عبدالله الشهري", "0501111105"},
		{"Al-Riyadh Transport Co.", "شركة الرياض للنقل", "0501111106"},
	}

	customers := []*Customer{}
	for _, seed := range customerSeeds {
		customer := Customer{
			Name:         seed.Name,
			NameInArabic: seed.NameArabic,
			Phone:        seed.Phone,
			StoreID:      &store.ID,
			CreatedAt:    &now,
			UpdatedAt:    &now,
			CreatedBy:    userID,
			UpdatedBy:    userID,
		}
		if err = customer.MakeCode(); err != nil {
			return summary, errors.New("error making customer code: " + err.Error())
		}
		customer.GenerateSearchWords()
		customer.SetSearchLabel()
		customer.SetAdditionalkeywords()
		if err = customer.Insert(); err != nil {
			return summary, errors.New("error inserting customer: " + err.Error())
		}
		if err = customer.UpdateForeignLabelFields(); err != nil {
			log.Printf("PopulateAutomobileTestData: customer UpdateForeignLabelFields: %v", err)
		}
		customers = append(customers, &customer)
		summary["customers"]++
	}

	// ---------- Employees (technicians) ----------
	employeeSeeds := []struct {
		Name       string
		NameArabic string
		Position   string
		Mob1       string
		Salary     float64
	}{
		{"Ramesh Kumar", "راميش كومار", "Senior Technician", "0502222201", 3500},
		{"Arun Nair", "أرون ناير", "Technician", "0502222202", 2800},
		{"Imran Khan", "عمران خان", "AC Specialist", "0502222203", 3000},
		{"Sameer Abbas", "سمير عباس", "Auto Electrician", "0502222204", 2600},
	}

	joining := now.AddDate(-1, -3, 0)
	employees := []*Employee{}
	for i, seed := range employeeSeeds {
		employee := Employee{
			Code:         "EMP-" + strconv.Itoa(i+1),
			Name:         seed.Name,
			NameInArabic: seed.NameArabic,
			Position:     seed.Position,
			Mob1:         seed.Mob1,
			Salary:       seed.Salary,
			SalaryDay:    5,
			JoiningDate:  &joining,
			IsActive:     true,
			StoreID:      &store.ID,
			CreatedAt:    &now,
			UpdatedAt:    &now,
			CreatedBy:    userID,
			UpdatedBy:    userID,
		}
		if err = employee.UpdateForeignLabelFields(); err != nil {
			log.Printf("PopulateAutomobileTestData: employee UpdateForeignLabelFields: %v", err)
		}
		if err = employee.Insert(); err != nil {
			return summary, errors.New("error inserting employee: " + err.Error())
		}
		employees = append(employees, &employee)
		summary["employees"]++
	}

	// ---------- Products: spare parts & services ----------
	type productSeed struct {
		Name          string
		NameArabic    string
		PartNumber    string
		Unit          string
		PurchasePrice float64
		RetailPrice   float64
		Stock         float64
		IsService     bool
	}
	productSeeds := []productSeed{
		{"Engine Oil 5W-30 Synthetic 4L", "زيت محرك 5W-30", "EO-5W30-4L", "PCS", 45, 75, 40, false},
		{"Oil Filter", "فلتر زيت", "OF-90915", "PCS", 12, 25, 60, false},
		{"Air Filter", "فلتر هواء", "AF-17801", "PCS", 18, 35, 50, false},
		{"Front Brake Pads Set", "طقم فحمات أمامية", "BP-F-04465", "SET", 80, 150, 30, false},
		{"Rear Brake Pads Set", "طقم فحمات خلفية", "BP-R-04466", "SET", 70, 130, 25, false},
		{"Car Battery 70Ah", "بطارية سيارة 70 أمبير", "BAT-70AH", "PCS", 180, 280, 15, false},
		{"Spark Plug Iridium", "بوجيه إيريديوم", "SP-IR-9081", "PCS", 25, 45, 80, false},
		{"Coolant 4L", "سائل تبريد 4 لتر", "CL-GRN-4L", "PCS", 20, 40, 35, false},
		{"Wiper Blade Set", "طقم مساحات", "WB-SET-24", "SET", 15, 35, 45, false},
		{"AC Cabin Filter", "فلتر مكيف", "ACF-87139", "PCS", 15, 30, 40, false},
		{"Labour Charge", "أجرة عمل", "", "", 0, 100, 0, true},
		{"Engine Oil Change Service", "خدمة تغيير زيت", "", "", 0, 50, 0, true},
		{"Wheel Alignment", "ضبط ميزان", "", "", 0, 120, 0, true},
		{"AC Gas Refill", "تعبئة فريون", "", "", 0, 150, 0, true},
	}

	products := []*Product{}
	for _, seed := range productSeeds {
		vatFactor := 1 + vatPercent/100
		product := Product{
			Name:         seed.Name,
			NameInArabic: seed.NameArabic,
			PartNumber:   seed.PartNumber,
			Unit:         seed.Unit,
			IsService:    seed.IsService,
			StoreID:      &store.ID,
			StoreName:    store.Name,
			CreatedAt:    &now,
			UpdatedAt:    &now,
			CreatedBy:    userID,
			UpdatedBy:    userID,
			ProductStores: map[string]ProductStore{
				store.ID.Hex(): {
					StoreID:                  store.ID,
					StoreName:                store.Name,
					PurchaseUnitPrice:        seed.PurchasePrice,
					PurchaseUnitPriceWithVAT: RoundTo2Decimals(seed.PurchasePrice * vatFactor),
					RetailUnitPrice:          seed.RetailPrice,
					RetailUnitPriceWithVAT:   RoundTo2Decimals(seed.RetailPrice * vatFactor),
					WholesaleUnitPrice:       seed.RetailPrice,
					WholesaleUnitPriceWithVAT: RoundTo2Decimals(seed.RetailPrice * vatFactor),
					Stock:                    seed.Stock,
				},
			},
		}
		if err = product.SetPartNumber(); err != nil {
			log.Printf("PopulateAutomobileTestData: product SetPartNumber: %v", err)
		}
		if err = product.SetBarcode(); err != nil {
			log.Printf("PopulateAutomobileTestData: product SetBarcode: %v", err)
		}
		if err = product.UpdateForeignLabelFields(); err != nil {
			log.Printf("PopulateAutomobileTestData: product UpdateForeignLabelFields: %v", err)
		}
		if err = product.CalculateUnitProfit(); err != nil {
			log.Printf("PopulateAutomobileTestData: product CalculateUnitProfit: %v", err)
		}
		product.GeneratePrefixes()
		product.SetAdditionalkeywords()
		product.SetSearchLabel()
		if err = product.Insert(); err != nil {
			return summary, errors.New("error inserting product: " + err.Error())
		}
		products = append(products, &product)
		summary["products"]++
	}
	partByName := map[string]*Product{}
	for _, p := range products {
		partByName[p.Name] = p
	}

	// ---------- Vehicles ----------
	type vehicleSeed struct {
		CustomerIdx int
		Brand       string
		Model       string
		Year        int
		Plate       string
		Color       string
		KM          float64
		Chassis     string
		Engine      string
		Istimara    string
	}
	vehicleSeeds := []vehicleSeed{
		{0, "Toyota", "Camry", 2020, "ABC 1234", "White", 85000, "JTNB11HK2L3001234", "2AR-1234567", "IST-100201"},
		{1, "Toyota", "Hilux", 2019, "DEF 5678", "Silver", 140000, "MR0FB8CD1K0567890", "2GD-7654321", "IST-100202"},
		{2, "Nissan", "Patrol", 2021, "GHJ 4321", "Black", 60000, "JN8AY2NY5M9004321", "VK56-1122334", "IST-100203"},
		{3, "Hyundai", "Sonata", 2018, "KLM 8765", "Grey", 110000, "KMHE341GBJA087650", "G4KN-5566778", "IST-100204"},
		{4, "Lexus", "LX570", 2022, "NPQ 2468", "White", 35000, "JTJHY7AX8N4024680", "3UR-9988776", "IST-100205"},
		{5, "Ford", "Explorer", 2017, "RST 1357", "Blue", 155000, "1FM5K8D84HGA13570", "ECO-3344556", "IST-100206"},
		{5, "GMC", "Yukon", 2020, "UVW 9753", "Black", 90000, "1GKS2BKC8LR097530", "L86-6677889", "IST-100207"},
		{0, "Kia", "Sportage", 2021, "XYZ 8642", "Red", 45000, "KNDPMCAC1M7086420", "G4FJ-2211334", "IST-100208"},
	}

	vehicles := []*Vehicle{}
	for _, seed := range vehicleSeeds {
		customer := customers[seed.CustomerIdx]
		vehicle := Vehicle{
			StoreID:       &store.ID,
			CustomerID:    &customer.ID,
			VehicleNumber: seed.Plate,
			ChassisNumber: seed.Chassis,
			Brand:         seed.Brand,
			Model:         seed.Model,
			Year:          seed.Year,
			EngineNumber:  seed.Engine,
			CurrentKM:     seed.KM,
			IstimaraNo:    seed.Istimara,
			Color:         seed.Color,
			CreatedAt:     &now,
			UpdatedAt:     &now,
			CreatedBy:     userID,
			UpdatedBy:     userID,
		}
		if err = vehicle.UpdateForeignLabelFields(); err != nil {
			log.Printf("PopulateAutomobileTestData: vehicle UpdateForeignLabelFields: %v", err)
		}
		if err = vehicle.Insert(); err != nil {
			return summary, errors.New("error inserting vehicle: " + err.Error())
		}
		vehicles = append(vehicles, &vehicle)
		summary["vehicles"]++
	}

	// ---------- Repair Jobs ----------
	makePart := func(productName string, qty float64) RepairJobPart {
		p := partByName[productName]
		if p == nil {
			return RepairJobPart{Name: productName, Qty: qty}
		}
		ps := p.ProductStores[store.ID.Hex()]
		return RepairJobPart{
			ProductID:         &p.ID,
			ItemCode:          p.ItemCode,
			PartNumber:        p.PartNumber,
			Name:              p.Name,
			Qty:               qty,
			PurchaseUnitPrice: ps.PurchaseUnitPrice,
			Stock:             ps.Stock,
			UnitPrice:         ps.RetailUnitPrice,
			UnitPriceWithVat:  ps.RetailUnitPriceWithVAT,
		}
	}
	daysAgo := func(d int) *time.Time {
		t := now.AddDate(0, 0, -d)
		return &t
	}
	daysAhead := func(d int) *time.Time {
		t := now.AddDate(0, 0, d)
		return &t
	}

	type jobSeed struct {
		Title        string
		VehicleIdx   int
		Status       string
		Date         *time.Time
		Delivery     *time.Time
		Complaint    string
		Inspection   string
		WorkDone     string
		LabourCharge float64
		TechIdxs     []int
		Parts        []RepairJobPart
	}
	jobSeeds := []jobSeed{
		{
			Title: "Engine oil & filter change", VehicleIdx: 0, Status: "completed",
			Date: daysAgo(12), Delivery: daysAgo(11),
			Complaint:  "Customer requested periodic oil change.",
			Inspection: "Oil level low and dark. Oil filter due for replacement.",
			WorkDone:   "Drained old oil, replaced oil filter, refilled with 5W-30 synthetic. Checked all fluid levels.",
			LabourCharge: 50, TechIdxs: []int{0},
			Parts: []RepairJobPart{makePart("Engine Oil 5W-30 Synthetic 4L", 1), makePart("Oil Filter", 1)},
		},
		{
			Title: "Front brake pads replacement", VehicleIdx: 1, Status: "in_progress",
			Date: daysAgo(2), Delivery: daysAhead(1),
			Complaint:  "Squealing noise when braking.",
			Inspection: "Front pads worn below 2mm. Discs within tolerance.",
			LabourCharge: 120, TechIdxs: []int{1},
			Parts: []RepairJobPart{makePart("Front Brake Pads Set", 1)},
		},
		{
			Title: "AC not cooling", VehicleIdx: 2, Status: "in_progress",
			Date: daysAgo(1), Delivery: daysAhead(2),
			Complaint:  "AC blows warm air even at max setting.",
			Inspection: "Refrigerant pressure low. Cabin filter clogged. No visible leaks.",
			LabourCharge: 150, TechIdxs: []int{2},
			Parts: []RepairJobPart{makePart("AC Cabin Filter", 1)},
		},
		{
			Title: "Battery replacement", VehicleIdx: 3, Status: "delivered",
			Date: daysAgo(8), Delivery: daysAgo(8),
			Complaint:  "Car struggles to start in the morning.",
			Inspection: "Battery voltage 11.4V under load — failed test.",
			WorkDone:   "Replaced battery with new 70Ah unit, checked charging system output 14.2V.",
			LabourCharge: 30, TechIdxs: []int{3},
			Parts: []RepairJobPart{makePart("Car Battery 70Ah", 1)},
		},
		{
			Title: "30,000 km periodic service", VehicleIdx: 4, Status: "open",
			Date: daysAgo(0), Delivery: daysAhead(2),
			Complaint:  "Scheduled periodic maintenance.",
			LabourCharge: 150, TechIdxs: []int{0, 1},
			Parts: []RepairJobPart{
				makePart("Engine Oil 5W-30 Synthetic 4L", 2),
				makePart("Oil Filter", 1),
				makePart("Air Filter", 1),
				makePart("Spark Plug Iridium", 8),
			},
		},
		{
			Title: "Suspension noise diagnosis", VehicleIdx: 5, Status: "open",
			Date: daysAgo(0), Delivery: daysAhead(0),
			Complaint: "Knocking sound from front suspension over bumps.",
			LabourCharge: 80, TechIdxs: []int{1},
		},
		{
			Title: "Coolant leak repair", VehicleIdx: 6, Status: "in_progress",
			Date: daysAgo(4), Delivery: daysAgo(2),
			Complaint:  "Coolant warning light on, coolant level dropping.",
			Inspection: "Leak from radiator lower hose clamp. Hose serviceable, clamp replaced.",
			LabourCharge: 200, TechIdxs: []int{0, 3},
			Parts: []RepairJobPart{makePart("Coolant 4L", 2)},
		},
		{
			Title: "Wiper replacement & general check", VehicleIdx: 7, Status: "completed",
			Date: daysAgo(6), Delivery: daysAgo(6),
			Complaint: "Wipers leaving streaks.",
			WorkDone:  "Replaced wiper blade set. Topped up washer fluid. General visual check OK.",
			LabourCharge: 20, TechIdxs: []int{2},
			Parts: []RepairJobPart{makePart("Wiper Blade Set", 1)},
		},
		{
			Title: "Full brake service", VehicleIdx: 0, Status: "open",
			Date: daysAgo(0), Delivery: daysAhead(5),
			Complaint:  "Brake pedal feels spongy, requested full brake check.",
			LabourCharge: 180, TechIdxs: []int{1, 3},
			Parts: []RepairJobPart{
				makePart("Front Brake Pads Set", 1),
				makePart("Rear Brake Pads Set", 1),
			},
		},
		{
			Title: "Dashboard warning lights fault", VehicleIdx: 1, Status: "closed",
			Date: daysAgo(15), Delivery: daysAgo(14),
			Complaint:  "Multiple warning lights flickering on dashboard.",
			Inspection: "Loose ground connection behind instrument cluster.",
			WorkDone:   "Cleaned and secured ground terminal, cleared fault codes, road tested.",
			LabourCharge: 100, TechIdxs: []int{3},
		},
	}

	for _, seed := range jobSeeds {
		vehicle := vehicles[seed.VehicleIdx]
		job := RepairJob{
			StoreID:      &store.ID,
			Title:        seed.Title,
			Date:         seed.Date,
			VehicleID:    &vehicle.ID,
			KM:           vehicle.CurrentKM,
			Complaint:    seed.Complaint,
			Inspection:   seed.Inspection,
			WorkDone:     seed.WorkDone,
			LabourCharge: seed.LabourCharge,
			VatPercent:   vatPercent,
			Parts:        seed.Parts,
			EstimatedDelivery: seed.Delivery,
			Status:       seed.Status,
			CreatedAt:    seed.Date,
			UpdatedAt:    seed.Date,
			CreatedBy:    userID,
			UpdatedBy:    userID,
		}
		job.JobNumber, err = store.GenerateRepairJobNumber()
		if err != nil {
			return summary, errors.New("error generating repair job number: " + err.Error())
		}
		for _, techIdx := range seed.TechIdxs {
			employee := employees[techIdx]
			job.TechnicianIDs = append(job.TechnicianIDs, employee.ID)
			job.TechnicianNames = append(job.TechnicianNames, employee.Name)
		}
		if len(seed.TechIdxs) > 0 {
			job.TechnicianID = &employees[seed.TechIdxs[0]].ID
			job.TechnicianName = employees[seed.TechIdxs[0]].Name
		}
		if err = job.UpdateForeignLabelFields(); err != nil {
			log.Printf("PopulateAutomobileTestData: repair job UpdateForeignLabelFields: %v", err)
		}
		if err = job.Insert(); err != nil {
			return summary, errors.New("error inserting repair job: " + err.Error())
		}
		summary["repair_jobs"]++
	}

	return summary, nil
}
