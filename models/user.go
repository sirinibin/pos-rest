package models

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jameskeane/bcrypt"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
) //import "encoding/json"

type User struct {
	ID                 primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name               string                `bson:"name,omitempty" json:"name,omitempty"`
	Email              string                `bson:"email,omitempty" json:"email,omitempty"`
	Mob                string                `bson:"mob,omitempty" json:"mob,omitempty"`
	Password           string                `bson:"password,omitempty" json:"password,omitempty"`
	Photo              string                `bson:"photo,omitempty" json:"photo,omitempty"`
	PhotoContent       string                `json:"photo_content,omitempty"`
	Deleted            bool                  `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy          *primitive.ObjectID   `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser      *User                 `json:"deleted_by_user,omitempty"`
	DeletedAt          *time.Time            `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt          *time.Time            `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time            `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy          *primitive.ObjectID   `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy          *primitive.ObjectID   `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser      *User                 `json:"created_by_user,omitempty"`
	UpdatedByUser      *User                 `json:"updated_by_user,omitempty"`
	CreatedByName      string                `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName      string                `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName      string                `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Admin              bool                  `bson:"admin" json:"admin"`
	StoreIDs           []*primitive.ObjectID `json:"store_ids" bson:"store_ids"`
	StoreNames         []string              `json:"store_names" bson:"store_names"`
	Role               string                `json:"role,omitempty" bson:"role,omitempty"` //Admin | Manager | SalesMen
	Online             bool                  `bson:"online" json:"online"`
	LastOnlineAt       *time.Time            `bson:"last_online_at,omitempty" json:"last_online_at,omitempty"`
	LastOfflineAt      *time.Time            `bson:"last_offline_at,omitempty" json:"last_offline_at,omitempty"`
	ConnectedMobiles   int                   `json:"connected_mobiles" bson:"connected_mobiles"`
	ConnectedTabs      int                   `json:"connected_tabs" bson:"connected_tabs"`
	ConnectedComputers int                   `json:"connected_computers" bson:"connected_computers"`
	Devices            map[string]*Device    `bson:"devices" json:"devices"`
}

// Device represents detailed information about a user's device
type Device struct {
	DeviceID           string     `json:"device_id" bson:"device_id"`         // Unique device identifier (UUID stored in localStorage)
	Fingerprint        string     `json:"fingerprint" bson:"fingerprint"`     // FingerprintJS-generated visitor ID
	UserAgent          string     `json:"user_agent" bson:"user_agent"`       // Browser user agent string
	Platform           string     `json:"platform" bson:"platform"`           // OS platform (e.g., Windows, macOS, Linux, iOS, Android)
	DeviceType         string     `json:"device_type" bson:"device_type"`     // Device type (Mobile, Tablet, Computer)
	ScreenWidth        string     `json:"screen_width" bson:"screen_width"`   // Screen width in pixels
	ScreenHeight       string     `json:"screen_height" bson:"screen_height"` // Screen height in pixels
	CPUCores           string     `json:"cpu_cores" bson:"cpu_cores"`         // Number of CPU cores
	RAM                string     `json:"ram" bson:"ram"`                     // Estimated RAM size in GB (if available)
	Timezone           string     `json:"timezone" bson:"timezone"`           // User's time zone
	Touch              bool       `json:"touch" bson:"touch"`                 // Whether the device has touch capabilities
	Connected          bool       `json:"connected" bson:"connected"`         // Whether the device is currently online
	Battery            string     `json:"battery" bson:"battery"`             // Battery level (0 to 1), -1 if unknown
	IPAddress          string     `json:"ip_address" bson:"ip_address"`       // Device's IP address (optional)
	TabsOpen           int        `json:"tabs_open" bson:"tabs_open"`
	FirstConnectedAt   *time.Time `bson:"first_connected_at,omitempty" json:"first_connected_at,omitempty"`
	LastConnectedAt    *time.Time `bson:"last_connected_at,omitempty" json:"last_connected_at,omitempty"`
	LastDisConnectedAt *time.Time `bson:"last_disconnected_at,omitempty" json:"last_disconnected_at,omitempty"`
	Location           Location   `json:"location" bson:"location"`
}

type Location struct {
	Latitude      string     `json:"latitude" bson:"latitude"`
	Longitude     string     `json:"longitude" bson:"longitude"`
	City          string     `json:"city" bson:"city"`
	Country       string     `json:"country" bson:"country"`
	LastUpdatedAt *time.Time `bson:"last_updated_at,omitempty" json:"last_updated_at,omitempty"`
}

type UserForm struct {
	ID           primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name         string                `bson:"name,omitempty" json:"name,omitempty"`
	Email        string                `bson:"email,omitempty" json:"email,omitempty"`
	Mob          string                `bson:"mob,omitempty" json:"mob,omitempty"`
	Password     string                `bson:"password,omitempty" json:"password,omitempty"`
	Photo        string                `bson:"photo,omitempty" json:"photo,omitempty"`
	PhotoContent string                `json:"photo_content,omitempty"`
	Role         string                `bson:"role,omitempty" json:"role,omitempty"`
	StoreIDs     []*primitive.ObjectID `json:"store_ids" bson:"store_ids"`
	StoreNames   []string              `json:"store_names" bson:"store_names"`
	Admin        bool                  `bson:"admin" json:"admin"`
}

func (user *User) SetOnlineStatus() error {
	connectedDevicesCount := 0
	for _, device := range user.Devices {
		if device.Connected {
			connectedDevicesCount++
		}
	}

	now := time.Now()
	if connectedDevicesCount > 0 && !user.Online {
		user.Online = true
		user.LastOfflineAt = &now

	} else if connectedDevicesCount == 0 && user.Online {
		user.Online = false
		user.LastOnlineAt = &now
	}

	return nil
}

func (user *User) SetDeviceCounts() error {
	connectedMobiles := 0
	connectedTabs := 0
	connectedComputers := 0
	for _, device := range user.Devices {
		if device.Connected {
			if device.DeviceType == "Computer" {
				connectedComputers++
			} else if device.DeviceType == "Tablet" {
				connectedTabs++
			} else if device.DeviceType == "Mobile" {
				connectedMobiles++
			}
		}
	}

	user.ConnectedMobiles = connectedMobiles
	user.ConnectedTabs = connectedTabs
	user.ConnectedComputers = connectedComputers

	return nil
}

func GetOnlineAdminUsers() (users []User, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)
	filter := map[string]interface{}{
		"role":   "Admin",
		"online": true,
	}
	cur, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return users, errors.New("Error fetching users:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return users, errors.New("Cursor error:" + err.Error())
		}
		user := User{}
		err = cur.Decode(&user)
		if err != nil {
			return users, errors.New("Cursor decode error:" + err.Error())
		}
		users = append(users, user)
	} //end for loop

	return users, nil
}

func GetOnlineUsersByStoreID(storeID *primitive.ObjectID) (users []User, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)
	/*
		criterias.SearchBy["$or"] = []bson.M{
			{"store_id": storeID},
			{"store_id": bson.M{"$in": store.UseProductsFromStoreID}},
		}*/
	filter := map[string]interface{}{
		"$or": []bson.M{
			{"store_ids": storeID},
			{"role": "Admin"},
		},
		"online": true,
	}
	cur, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return users, errors.New("Error fetching users:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return users, errors.New("Cursor error:" + err.Error())
		}
		user := User{}
		err = cur.Decode(&user)
		if err != nil {
			return users, errors.New("Cursor decode error:" + err.Error())
		}
		users = append(users, user)
	} //end for loop

	return users, nil
}
func (user *User) AttributesValueChangeEvent(userOld *User) error {

	if user.Name != userOld.Name {
		/*
			err := store.UpdateManyByCollectionName(
				"quotation",
				bson.M{"delivered_by": user.ID},
				bson.M{"delivered_by_name": user.Name},
			)
			if err != nil {
				return nil
			}

			err = store.UpdateManyByCollectionName(
				"purchase",
				bson.M{"order_placed_by": user.ID},
				bson.M{"order_placed_by_name": user.Name},
			)
			if err != nil {
				return nil
			}

			usedInCollections := []string{
				"order",
				"customer",
				"purchase",
				"product_category",
				"product",
				"quotation",
				"signature",
				"store",
				"vendor",
			}

			for _, collectionName := range usedInCollections {

				err := store.UpdateManyByCollectionName(
					collectionName,
					bson.M{"created_by": user.ID},
					bson.M{"created_by_name": user.Name},
				)
				if err != nil {
					return nil
				}

				err = store.UpdateManyByCollectionName(
					collectionName,
					bson.M{"updated_by": user.ID},
					bson.M{"updated_by_name": user.Name},
				)
				if err != nil {
					return nil
				}

				err = store.UpdateManyByCollectionName(
					collectionName,
					bson.M{"deleted_by": user.ID},
					bson.M{"deleted_by_name": user.Name},
				)
				if err != nil {
					return nil
				}

				err = store.UpdateManyByCollectionName(
					collectionName,
					bson.M{"change_logs.created_by": user.ID},
					bson.M{"change_logs.$.created_by_name": user.Name},
				)
				if err != nil {
					return nil
				}

			}*/

	}

	return nil
}

func (user *User) UpdateForeignLabelFields() error {

	user.StoreNames = []string{}

	for _, storeID := range user.StoreIDs {
		storeTemp, err := FindStoreByID(storeID, bson.M{"id": 1, "name": 1, "branch_name": 1})
		if err != nil {
			return errors.New("Error Finding store id:" + storeID.Hex() + ",error:" + err.Error())
		}
		user.StoreNames = append(user.StoreNames, storeTemp.Name+" - "+storeTemp.BranchName)
	}

	if user.CreatedBy != nil {
		createdByUser, err := FindUserByID(user.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		user.CreatedByName = createdByUser.Name
	}

	if user.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(user.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		user.UpdatedByName = updatedByUser.Name
	}

	if user.DeletedBy != nil && !user.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(user.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		user.DeletedByName = deletedByUser.Name
	}

	return nil
}

func FindUserByEmail(email string) (user *User, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"email": email}).
		Decode(&user)
	if err != nil {
		return nil, err
	}
	return user, err
}

func (user *User) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if user.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsUserExists(&user.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid User:" + user.ID.Hex()
		}

	}

	if govalidator.IsNull(user.Name) {
		errs["name"] = "Name is required"
	}

	if govalidator.IsNull(user.Email) {
		errs["email"] = "E-mail is required"
	}

	if govalidator.IsNull(user.Mob) {
		errs["mob"] = "Mob is required"
	}

	if user.ID.IsZero() && govalidator.IsNull(user.Password) {
		errs["password"] = "Password is required"
	}

	if !govalidator.IsNull(user.PhotoContent) {
		splits := strings.Split(user.PhotoContent, ",")

		if len(splits) == 2 {
			user.PhotoContent = splits[1]
		} else if len(splits) == 1 {
			user.PhotoContent = splits[0]
		}

		valid, err := IsStringBase64(user.PhotoContent)
		if err != nil {
			errs["photo_content"] = err.Error()
		}

		if !valid {
			errs["photo_content"] = "Invalid base64 string"
		}
	}

	emailExists, err := user.IsEmailExists()
	if err != nil {
		errs["email"] = err.Error()
	}

	if emailExists {
		errs["email"] = "E-mail is Already in use"
	}

	if emailExists {
		w.WriteHeader(http.StatusConflict)
	} else if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func SearchUser(w http.ResponseWriter, r *http.Request) (users []User, criterias SearchCriterias, err error) {
	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	tokenClaims, _ := AuthenticateByAccessToken(r)
	accessingUserID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	accessingUser, _ := FindUserByID(&accessingUserID, bson.M{})

	if accessingUser.Role != "Admin" {
		criterias.SearchBy["store_ids"] = bson.M{"$in": accessingUser.StoreIDs}
	}

	timeZoneOffset := 0.0
	keys, ok := r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	keys, ok = r.URL.Query()["search[online]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return users, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["online"] = bson.M{"$eq": true}
		} else if value == 0 {
			criterias.SearchBy["online"] = bson.M{"$ne": true}
		}
	}

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[email]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["email"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[mob]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["mob"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return users, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return users, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["created_at"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	keys, ok = r.URL.Query()["search[created_at_from]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtStartDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return users, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return users, criterias, err
		}

		if timeZoneOffset != 0 {
			createdAtEndDate = ConvertTimeZoneToUTC(timeZoneOffset, createdAtEndDate)
		}

		createdAtEndDate = createdAtEndDate.Add(time.Hour * time.Duration(24))
		createdAtEndDate = createdAtEndDate.Add(-time.Second * time.Duration(1))
	}

	if !createdAtStartDate.IsZero() && !createdAtEndDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate, "$lte": createdAtEndDate}
	} else if !createdAtStartDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate}
	} else if !createdAtEndDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$lte": createdAtEndDate}
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
		}

	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	//Fetch all device documents with (garbage:true AND (gc_processed:false if exist OR gc_processed not exist ))
	/* Note: Actual Record fetching will not happen here
	as it is using mongodb cursor and record fetching will
	start with we call cur.Next()
	*/
	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return users, criterias, errors.New("Error fetching Users:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return users, criterias, errors.New("Cursor error:" + err.Error())
		}
		user := User{}
		err = cur.Decode(&user)
		if err != nil {
			return users, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			user.CreatedByUser, _ = FindUserByID(user.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			user.UpdatedByUser, _ = FindUserByID(user.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			user.DeletedByUser, _ = FindUserByID(user.DeletedBy, deletedByUserSelectFields)
		}

		users = append(users, user)
	} //end for loop

	return users, criterias, nil

}

func (user *User) Insert() error {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, &user)
	if err != nil {
		return err
	}
	return nil
}

func (user *User) SavePhoto() error {
	content, err := base64.StdEncoding.DecodeString(user.PhotoContent)
	if err != nil {
		return err
	}

	extension, err := GetFileExtensionFromBase64(content)
	if err != nil {
		return err
	}

	filename := "images/users/user_" + user.ID.Hex() + extension
	err = SaveBase64File(filename, content)
	if err != nil {
		return err
	}
	user.Photo = "/" + filename
	user.PhotoContent = ""
	return nil
}

func (user *User) Update() error {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": user},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (user *User) DeleteUser(tokenClaims TokenClaims) (err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = user.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	user.Deleted = true
	user.DeletedBy = &userID
	now := time.Now()
	user.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": user},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindUserByID(
	ID *primitive.ObjectID,
	selectFields bson.M,
) (user *User, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&user)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		user.CreatedByUser, _ = FindUserByID(user.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		user.UpdatedByUser, _ = FindUserByID(user.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		user.DeletedByUser, _ = FindUserByID(user.DeletedBy, fields)
	}

	return user, err
}

func (user *User) IsEmailExists() (exists bool, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if user.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": user.Email,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": user.Email,
			"_id":   bson.M{"$ne": user.ID},
		})
	}

	return (count > 0), err
}

func (user *User) IsPhoneExists() (exists bool, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if user.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"mob": user.Mob,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"mob": user.Mob,
			"_id": bson.M{"$ne": user.ID},
		})
	}

	return (count > 0), err
}

func IsUserExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func HashPassword(password string) string {
	salt, _ := bcrypt.Salt(10)
	hash, _ := bcrypt.Hash(password, salt)
	return hash
}
