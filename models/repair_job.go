package models

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RepairJobPart : a part used in a repair job
type RepairJobPart struct {
	ProductID          *primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name               string              `json:"name" bson:"name"`
	Qty                float64             `json:"qty" bson:"qty"`
	UnitPrice          float64             `json:"unit_price" bson:"unit_price"`
	UnitPriceWithVat   float64             `json:"unit_price_with_vat" bson:"unit_price_with_vat"`
	TotalPrice         float64             `json:"total_price" bson:"total_price"`
	TotalPriceWithVat  float64             `json:"total_price_with_vat" bson:"total_price_with_vat"`
}

// RepairJob : structure for AutoMobile Workshop repair job
type RepairJob struct {
	ID                primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID           *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	JobNumber         string              `json:"job_number" bson:"job_number"`
	Title             string              `json:"title" bson:"title,omitempty"`
	Date              *time.Time          `json:"date" bson:"date"`
	VehicleID         *primitive.ObjectID `json:"vehicle_id,omitempty" bson:"vehicle_id"`
	CustomerID        *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id"`
	VehicleNumber     string              `json:"vehicle_number" bson:"vehicle_number"`
	Brand             string              `json:"brand" bson:"brand"`
	Model             string              `json:"model" bson:"model"`
	KM                float64             `json:"km" bson:"km"`
	Complaint         string              `json:"complaint" bson:"complaint"`
	Inspection        string              `json:"inspection" bson:"inspection"`
	WorkDone          string              `json:"work_done" bson:"work_done"`
	TechnicianID      *primitive.ObjectID `json:"technician_id,omitempty" bson:"technician_id,omitempty"`
	TechnicianName    string              `json:"technician_name" bson:"technician_name"`
	LabourCharge      float64             `json:"labour_charge" bson:"labour_charge"`
	Parts             []RepairJobPart     `json:"parts" bson:"parts"`
	PartsTotal        float64             `json:"parts_total" bson:"parts_total"`
	PartsTotalWithVat float64             `json:"parts_total_with_vat" bson:"parts_total_with_vat"`
	Total             float64             `json:"total" bson:"total"`
	TotalWithVat      float64             `json:"total_with_vat" bson:"total_with_vat"`
	EstimatedDelivery *time.Time          `json:"estimated_delivery,omitempty" bson:"estimated_delivery,omitempty"`
	Status            string              `json:"status" bson:"status"` // open, in_progress, completed, delivered, cancelled
	Deleted           bool                `bson:"deleted" json:"deleted"`
	DeletedBy         *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt         *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt         *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt         *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy         *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy         *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName     string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName     string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName     string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
}

func (job *RepairJob) CalculateTotals() {
	job.PartsTotal = 0
	job.PartsTotalWithVat = 0
	for i := range job.Parts {
		job.Parts[i].TotalPrice = job.Parts[i].Qty * job.Parts[i].UnitPrice
		unitWithVat := job.Parts[i].UnitPriceWithVat
		if unitWithVat == 0 {
			unitWithVat = job.Parts[i].UnitPrice
		}
		job.Parts[i].TotalPriceWithVat = job.Parts[i].Qty * unitWithVat
		job.PartsTotal += job.Parts[i].TotalPrice
		job.PartsTotalWithVat += job.Parts[i].TotalPriceWithVat
	}
	job.Total = job.LabourCharge + job.PartsTotal
	job.TotalWithVat = job.LabourCharge + job.PartsTotalWithVat
}

func (job *RepairJob) UpdateForeignLabelFields() error {
	if job.CreatedBy != nil {
		createdByUser, err := FindUserByID(job.CreatedBy, bson.M{"id": 1, "name": 1})
		if err == nil {
			job.CreatedByName = createdByUser.Name
		}
	}
	if job.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(job.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err == nil {
			job.UpdatedByName = updatedByUser.Name
		}
	}

	// Populate vehicle & customer info
	if job.VehicleID != nil && !job.VehicleID.IsZero() {
		store, err := FindStoreByID(job.StoreID, bson.M{})
		if err == nil && store != nil {
			vehicle, err := store.FindVehicleByID(job.VehicleID, bson.M{})
			if err == nil && vehicle != nil {
				job.VehicleNumber = vehicle.VehicleNumber
				job.Brand = vehicle.Brand
				job.Model = vehicle.Model
				if vehicle.CustomerID != nil && !vehicle.CustomerID.IsZero() {
					job.CustomerID = vehicle.CustomerID
				}
			}
		}
	}

	// Technician name
	if job.TechnicianID != nil && !job.TechnicianID.IsZero() {
		store, err := FindStoreByID(job.StoreID, bson.M{})
		if err == nil && store != nil {
			emp, err := store.FindEmployeeByID(job.TechnicianID, bson.M{"name": 1})
			if err == nil && emp != nil {
				job.TechnicianName = emp.Name
			}
		}
	}

	return nil
}

func (job *RepairJob) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	if job.StoreID == nil || job.StoreID.IsZero() {
		errs["store_id"] = "Store ID is required"
	}

	if job.Title == "" {
		errs["title"] = "Title is required"
	}

	return errs
}

// GenerateJobNumber creates an auto-incrementing job number like RJ-0001
func (store *Store) GenerateRepairJobNumber() (string, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("repair_job")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := collection.CountDocuments(ctx, bson.M{"deleted": bson.M{"$ne": true}})
	if err != nil {
		return "", err
	}
	return "RJ-" + strconv.FormatInt(count+1, 10), nil
}

func (job *RepairJob) Insert() error {
	collection := db.GetDB("store_" + job.StoreID.Hex()).Collection("repair_job")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	job.ID = primitive.NewObjectID()
	job.CalculateTotals()
	_, err := collection.InsertOne(ctx, &job)
	return err
}

func (job *RepairJob) Update() error {
	collection := db.GetDB("store_" + job.StoreID.Hex()).Collection("repair_job")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	job.UpdatedAt = &now
	job.CalculateTotals()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": job.ID},
		bson.M{"$set": job},
		options.Update().SetUpsert(false),
	)
	return err
}

func (store *Store) FindRepairJobByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (job *RepairJob, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("repair_job")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx, bson.M{
		"_id":      ID,
		"store_id": store.ID,
	}, findOneOptions).Decode(&job)
	if err != nil {
		return nil, err
	}

	return job, nil
}

func (store *Store) SearchRepairJob(w http.ResponseWriter, r *http.Request) (jobs []RepairJob, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var keys []string
	var ok bool

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return jobs, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {
		customerID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return jobs, criterias, err
		}
		criterias.SearchBy["customer_id"] = customerID
	}

	keys, ok = r.URL.Query()["search[vehicle_id]"]
	if ok && len(keys[0]) >= 1 {
		vehicleID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return jobs, criterias, err
		}
		criterias.SearchBy["vehicle_id"] = vehicleID
	}

	keys, ok = r.URL.Query()["search[status]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["status"] = keys[0]
	}

	keys, ok = r.URL.Query()["search[search]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = []bson.M{
			{"job_number": bson.M{"$regex": keys[0], "$options": "i"}},
			{"vehicle_number": bson.M{"$regex": keys[0], "$options": "i"}},
			{"brand": bson.M{"$regex": keys[0], "$options": "i"}},
			{"model": bson.M{"$regex": keys[0], "$options": "i"}},
			{"technician_name": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	ParseTextSearch(r, &criterias, "search[job_number]", "job_number")

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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("repair_job")
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
		return jobs, criterias, errors.New("Error fetching repair jobs:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		if err := cur.Err(); err != nil {
			return jobs, criterias, errors.New("Cursor error:" + err.Error())
		}
		job := RepairJob{}
		if err := cur.Decode(&job); err != nil {
			return jobs, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		jobs = append(jobs, job)
	}

	return jobs, criterias, nil
}

func (job *RepairJob) Delete(tokenClaims TokenClaims) error {
	collection := db.GetDB("store_" + job.StoreID.Hex()).Collection("repair_job")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	now := time.Now()
	job.Deleted = true
	job.DeletedBy = &userID
	job.DeletedAt = &now

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": job.ID},
		bson.M{"$set": job},
		options.Update().SetUpsert(false),
	)
	return err
}
