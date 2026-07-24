package models

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Vehicle : Vehicle structure
type Vehicle struct {
	ID                 primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID            *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID         *primitive.ObjectID `json:"customer_id" bson:"customer_id"`
	CustomerName       string              `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic string              `json:"customer_name_arabic" bson:"customer_name_arabic"`
	VehicleNumber      string              `bson:"vehicle_number" json:"vehicle_number"`
	ChassisNumber      string              `bson:"chassis_number" json:"chassis_number"`
	Brand              string              `bson:"brand" json:"brand"`
	Model              string              `bson:"model" json:"model"`
	Variant            string              `bson:"variant" json:"variant"`
	Year               int                 `bson:"year" json:"year"`
	EngineNumber       string              `bson:"engine_number" json:"engine_number"`
	CurrentKM          float64             `bson:"current_km" json:"current_km"`
	IstimaraNo         string              `bson:"istimara_no" json:"istimara_no"`
	Color              string              `bson:"color" json:"color"`
	Remarks            string              `bson:"remarks" json:"remarks"`
	Deleted            bool                `bson:"deleted" json:"deleted"`
	DeletedBy          *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt          *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy          *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy          *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName      string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName      string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName      string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
}

// VehicleSnapshot is embedded in Order to capture vehicle state at sale time.
type VehicleSnapshot struct {
	VehicleNumber string  `bson:"vehicle_number" json:"vehicle_number"`
	ChassisNumber string  `bson:"chassis_number" json:"chassis_number"`
	Brand         string  `bson:"brand" json:"brand"`
	Model         string  `bson:"model" json:"model"`
	Variant       string  `bson:"variant" json:"variant"`
	Year          int     `bson:"year" json:"year"`
	EngineNumber  string  `bson:"engine_number" json:"engine_number"`
	CurrentKM     float64 `bson:"current_km" json:"current_km"`
	IstimaraNo    string  `bson:"istimara_no" json:"istimara_no"`
}

// VehicleBrand holds a brand and its available models for the KSA market.
type VehicleBrand struct {
	Brand  string   `json:"brand"`
	Models []string `json:"models"`
}

// GetKSAVehicleBrands returns the static list of vehicle brands available in Saudi Arabia.
func GetKSAVehicleBrands() []VehicleBrand {
	return []VehicleBrand{
		{Brand: "Toyota", Models: []string{"Camry", "Corolla", "Yaris", "Land Cruiser", "Land Cruiser Prado", "Hilux", "RAV4", "Fortuner", "Rush", "Avanza", "Innova", "Prius", "CHR", "Highlander", "Sequoia", "Tundra", "Tacoma", "4Runner", "Vios", "Crown", "FJ Cruiser"}},
		{Brand: "Nissan", Models: []string{"Patrol", "Pathfinder", "Altima", "Sentra", "Sunny", "Maxima", "Tiida", "X-Trail", "Murano", "Armada", "Frontier", "Navara", "Urvan", "NV350", "Kicks", "Juke", "Note", "Micra", "Terra", "360Z"}},
		{Brand: "Hyundai", Models: []string{"Sonata", "Elantra", "Accent", "Tucson", "Santa Fe", "Palisade", "Creta", "Venue", "Ioniq 5", "Ioniq 6", "Staria", "H-1", "H100", "Grand i10", "Azera", "Kona", "Nexo"}},
		{Brand: "Kia", Models: []string{"Sportage", "Sorento", "Telluride", "Carnival", "Stinger", "EV6", "K5", "K8", "Picanto", "Rio", "Cerato", "Mohave", "Seltos", "Sonet", "Niro"}},
		{Brand: "GMC", Models: []string{"Yukon", "Yukon XL", "Sierra", "Canyon", "Acadia", "Terrain", "Envoy"}},
		{Brand: "Chevrolet", Models: []string{"Tahoe", "Suburban", "Silverado", "Colorado", "Traverse", "Blazer", "Equinox", "Camaro", "Corvette", "Trailblazer", "Spark", "Captiva", "Malibu", "Impala"}},
		{Brand: "Ford", Models: []string{"F-150", "F-250", "F-350", "Ranger", "Explorer", "Expedition", "Edge", "Bronco", "Mustang", "Escape", "EcoSport", "Maverick", "Transit", "Galaxy"}},
		{Brand: "BMW", Models: []string{"3 Series", "5 Series", "7 Series", "X1", "X3", "X5", "X6", "X7", "M3", "M5", "iX", "i4", "Z4", "2 Series", "4 Series", "6 Series"}},
		{Brand: "Mercedes-Benz", Models: []string{"C-Class", "E-Class", "S-Class", "GLA", "GLC", "GLE", "GLS", "AMG GT", "EQS", "EQC", "A-Class", "B-Class", "CLA", "CLS", "G-Class", "Maybach S-Class", "Sprinter", "Vito"}},
		{Brand: "Lexus", Models: []string{"LX", "GX", "RX", "NX", "UX", "ES", "LS", "IS", "RC", "LC", "LM", "TX", "GX460", "LX600"}},
		{Brand: "Honda", Models: []string{"Accord", "Civic", "City", "HR-V", "CR-V", "Pilot", "Ridgeline", "Odyssey", "Passport", "Jazz", "BR-V", "WR-V", "ZR-V", "e:NS1"}},
		{Brand: "Mitsubishi", Models: []string{"Pajero", "Pajero Sport", "Outlander", "Eclipse Cross", "L200", "Lancer", "Galant", "ASX", "Grandis", "Attrage", "Xpander", "Delica"}},
		{Brand: "Suzuki", Models: []string{"Vitara", "Grand Vitara", "Jimny", "Swift", "Baleno", "Celerio", "Ertiga", "XL6", "Dzire", "Ciaz", "S-Presso", "Brezza"}},
		{Brand: "Mazda", Models: []string{"CX-5", "CX-9", "CX-30", "CX-3", "CX-50", "CX-60", "Mazda6", "Mazda3", "MX-5 Miata", "BT-50"}},
		{Brand: "Land Rover", Models: []string{"Range Rover", "Range Rover Sport", "Range Rover Velar", "Range Rover Evoque", "Discovery", "Discovery Sport", "Defender"}},
		{Brand: "Jeep", Models: []string{"Wrangler", "Grand Cherokee", "Grand Cherokee L", "Cherokee", "Compass", "Renegade", "Gladiator", "Wagoneer", "Grand Wagoneer"}},
		{Brand: "Ram", Models: []string{"1500", "2500", "3500", "ProMaster"}},
		{Brand: "Dodge", Models: []string{"Durango", "Challenger", "Charger", "Journey"}},
		{Brand: "Cadillac", Models: []string{"Escalade", "Escalade ESV", "XT5", "XT6", "CT5", "CT6", "Lyriq"}},
		{Brand: "Infiniti", Models: []string{"QX80", "QX60", "QX55", "QX50", "Q50", "Q60"}},
		{Brand: "Volkswagen", Models: []string{"Tiguan", "Touareg", "Passat", "Golf", "Polo", "Jetta", "T-Roc", "Teramont", "ID.4"}},
		{Brand: "Audi", Models: []string{"A3", "A4", "A6", "A8", "Q3", "Q5", "Q7", "Q8", "e-tron", "RS3", "RS6"}},
		{Brand: "Porsche", Models: []string{"Cayenne", "Macan", "Panamera", "Taycan", "911", "718"}},
		{Brand: "Volvo", Models: []string{"XC40", "XC60", "XC90", "S60", "S90", "V60", "C40"}},
		{Brand: "Subaru", Models: []string{"Outback", "Forester", "Crosstrek", "Impreza", "Legacy", "BRZ", "WRX"}},
		{Brand: "Isuzu", Models: []string{"D-Max", "MU-X", "Trooper"}},
		{Brand: "Peugeot", Models: []string{"2008", "3008", "5008", "508", "208", "308"}},
		{Brand: "Renault", Models: []string{"Duster", "Koleos", "Safrane", "Logan", "Megane", "Captur"}},
		{Brand: "BYD", Models: []string{"Atto 3", "Han", "Tang", "Seal", "Dolphin", "King"}},
		{Brand: "Chery", Models: []string{"Tiggo 4", "Tiggo 7", "Tiggo 8", "Arrizo 6"}},
		{Brand: "Geely", Models: []string{"Coolray", "Okavango", "Monjaro", "Emgrand"}},
		{Brand: "MG", Models: []string{"ZS", "HS", "RX5", "MG5", "MG6", "Cyberster", "4 EV"}},
	}
}

func (vehicle *Vehicle) UpdateForeignLabelFields() error {
	if vehicle.CreatedBy != nil {
		createdByUser, err := FindUserByID(vehicle.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding created_by user:" + err.Error())
		}
		vehicle.CreatedByName = createdByUser.Name
	}

	if vehicle.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(vehicle.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding updated_by user:" + err.Error())
		}
		vehicle.UpdatedByName = updatedByUser.Name
	}

	if vehicle.CustomerID != nil && !vehicle.CustomerID.IsZero() {
		store, err := FindStoreByID(vehicle.StoreID, bson.M{})
		if err != nil {
			return err
		}
		customer, err := store.FindCustomerByID(vehicle.CustomerID, bson.M{"name": 1, "name_in_arabic": 1})
		if err != nil {
			return errors.New("Error finding customer:" + err.Error())
		}
		vehicle.CustomerName = customer.Name
		vehicle.CustomerNameArabic = customer.NameInArabic
	}

	return nil
}

func (vehicle *Vehicle) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	if vehicle.StoreID == nil || vehicle.StoreID.IsZero() {
		errs["store_id"] = "Store ID is required"
	}

	if scenario == "create" {
		if vehicle.Brand == "" {
			errs["brand"] = "Brand is required"
		}
		if vehicle.Model == "" {
			errs["model"] = "Model is required"
		}
	}

	if vehicle.CustomerID == nil || vehicle.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	}

	return errs
}

func (vehicle *Vehicle) Insert() error {
	collection := db.GetDB("store_" + vehicle.StoreID.Hex()).Collection("vehicle")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vehicle.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &vehicle)
	if err != nil {
		return err
	}

	if vehicle.CustomerID != nil && !vehicle.CustomerID.IsZero() {
		store, _ := FindStoreByID(vehicle.StoreID, bson.M{})
		if store != nil {
			go store.UpdateCustomerVehicleCount(vehicle.CustomerID)
		}
	}

	return nil
}

func (vehicle *Vehicle) Update() error {
	collection := db.GetDB("store_" + vehicle.StoreID.Hex()).Collection("vehicle")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	now := time.Now()
	vehicle.UpdatedAt = &now

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": vehicle.ID},
		bson.M{"$set": vehicle},
		updateOptions,
	)
	return err
}

func (store *Store) FindVehicleByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (vehicle *Vehicle, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vehicle")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx, bson.M{
		"_id":      ID,
		"store_id": store.ID,
	}, findOneOptions).Decode(&vehicle)
	if err != nil {
		return nil, err
	}

	return vehicle, nil
}

func (store *Store) SearchVehicle(w http.ResponseWriter, r *http.Request) (vehicles []Vehicle, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var keys []string
	var ok bool

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return vehicles, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {
		customerID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return vehicles, criterias, err
		}
		criterias.SearchBy["customer_id"] = customerID
	}

	keys, ok = r.URL.Query()["search[brand]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["brand"] = bson.M{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[search]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = []bson.M{
			{"vehicle_number": bson.M{"$regex": keys[0], "$options": "i"}},
			{"chassis_number": bson.M{"$regex": keys[0], "$options": "i"}},
			{"brand": bson.M{"$regex": keys[0], "$options": "i"}},
			{"model": bson.M{"$regex": keys[0], "$options": "i"}},
			{"istimara_no": bson.M{"$regex": keys[0], "$options": "i"}},
			{"customer_name": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	ParseTextSearch(r, &criterias, "search[vehicle_number]", "vehicle_number")
	ParseTextSearch(r, &criterias, "search[chassis_number]", "chassis_number")
	ParseTextSearch(r, &criterias, "search[istimara_no]", "istimara_no")

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vehicle")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
	}
	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return vehicles, criterias, errors.New("Error fetching vehicles:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		if err := cur.Err(); err != nil {
			return vehicles, criterias, errors.New("Cursor error:" + err.Error())
		}
		vehicle := Vehicle{}
		if err := cur.Decode(&vehicle); err != nil {
			return vehicles, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		vehicles = append(vehicles, vehicle)
	}

	return vehicles, criterias, nil
}

func (vehicle *Vehicle) Delete(tokenClaims TokenClaims) error {
	collection := db.GetDB("store_" + vehicle.StoreID.Hex()).Collection("vehicle")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	now := time.Now()
	vehicle.Deleted = true
	vehicle.DeletedBy = &userID
	vehicle.DeletedAt = &now

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": vehicle.ID},
		bson.M{"$set": vehicle},
		options.Update().SetUpsert(false),
	)
	if err != nil {
		return err
	}

	if vehicle.CustomerID != nil && !vehicle.CustomerID.IsZero() {
		store, _ := FindStoreByID(vehicle.StoreID, bson.M{})
		if store != nil {
			go store.UpdateCustomerVehicleCount(vehicle.CustomerID)
		}
	}

	return nil
}

// UpdateCustomerVehicleCount recalculates and stores the vehicle count on the Customer document.
func (store *Store) UpdateCustomerVehicleCount(customerID *primitive.ObjectID) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vehicle")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collection.CountDocuments(ctx, bson.M{
		"customer_id": customerID,
		"deleted":     bson.M{"$ne": true},
	})
	if err != nil {
		return err
	}

	customerCollection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	_, err = customerCollection.UpdateOne(
		ctx,
		bson.M{"_id": customerID},
		bson.M{"$set": bson.M{"vehicle_count": count}},
		options.Update().SetUpsert(false),
	)
	return err
}

// GetVehicleSnapshot builds a VehicleSnapshot from a Vehicle.
func (v *Vehicle) GetVehicleSnapshot() VehicleSnapshot {
	return VehicleSnapshot{
		VehicleNumber: v.VehicleNumber,
		ChassisNumber: v.ChassisNumber,
		Brand:         v.Brand,
		Model:         v.Model,
		Variant:       v.Variant,
		Year:          v.Year,
		EngineNumber:  v.EngineNumber,
		CurrentKM:     v.CurrentKM,
		IstimaraNo:    v.IstimaraNo,
	}
}

// GetVehicleCountByCustomerID returns how many vehicles a customer has.
func (store *Store) GetVehicleCountByCustomerID(customerID primitive.ObjectID) (int64, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vehicle")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"customer_id": customerID,
		"deleted":     bson.M{"$ne": true},
	})
}

// IsVehicleExists checks whether a vehicle document exists by ID.
func (store *Store) IsVehicleExists(ID *primitive.ObjectID) (bool, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vehicle")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collection.CountDocuments(ctx, bson.M{"_id": ID})
	return count > 0, err
}

// VehicleDisplayLabel builds a human-readable label for dropdowns.
func (v *Vehicle) VehicleDisplayLabel() string {
	label := strings.TrimSpace(v.Brand + " " + v.Model)
	if v.VehicleNumber != "" {
		label += " - " + v.VehicleNumber
	} else if v.IstimaraNo != "" {
		label += " - " + v.IstimaraNo
	}
	return label
}
