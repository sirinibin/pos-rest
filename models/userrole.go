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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Permission struct {
	Resource string `json:"resource" bson:"resource"`
	Read     bool   `json:"read" bson:"read"`
	Create   bool   `json:"create" bson:"create"`
	Update   bool   `json:"update" bson:"update"`
	Delete   bool   `json:"delete" bson:"delete"`
}

type UserRole struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string              `json:"name" bson:"name"`
	StoreID       *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName     string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	Permissions   []Permission        `json:"permissions" bson:"permissions"`
	Deleted       bool                `bson:"deleted" json:"deleted"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser *User               `json:"deleted_by_user,omitempty" bson:"-"`
	DeletedAt     *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser *User               `json:"created_by_user,omitempty" bson:"-"`
	UpdatedByUser *User               `json:"updated_by_user,omitempty" bson:"-"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
}

func getUserRoleCollection(storeID *primitive.ObjectID) *mongo.Collection {
	return db.GetDB("store_" + storeID.Hex()).Collection("user_role")
}

func (userRole *UserRole) UpdateForeignLabelFields() error {
	if userRole.StoreID != nil && !userRole.StoreID.IsZero() {
		store, err := FindStoreByID(userRole.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding store: " + err.Error())
		}
		userRole.StoreName = store.Name
	}

	if userRole.CreatedBy != nil {
		createdByUser, err := FindUserByID(userRole.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding created_by user: " + err.Error())
		}
		userRole.CreatedByName = createdByUser.Name
	}

	if userRole.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(userRole.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding updated_by user: " + err.Error())
		}
		userRole.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func (userRole *UserRole) Validate(scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	if userRole.Name == "" {
		errs["name"] = "Role name is required"
	}

	if userRole.StoreID == nil || userRole.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	}

	return errs
}

func (userRole *UserRole) Insert() error {
	if userRole.StoreID == nil || userRole.StoreID.IsZero() {
		return errors.New("store_id is required to insert user role")
	}
	collection := getUserRoleCollection(userRole.StoreID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := userRole.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userRole.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, userRole)
	return err
}

func (userRole *UserRole) Update() error {
	if userRole.StoreID == nil || userRole.StoreID.IsZero() {
		return errors.New("store_id is required to update user role")
	}
	collection := getUserRoleCollection(userRole.StoreID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := userRole.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	updateOptions := options.Update()
	updateOptions.SetUpsert(false)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": userRole.ID},
		bson.M{"$set": userRole},
		updateOptions,
	)
	return err
}

func (userRole *UserRole) Delete(tokenClaims TokenClaims) error {
	if userRole.StoreID == nil || userRole.StoreID.IsZero() {
		return errors.New("store_id is required to delete user role")
	}
	collection := getUserRoleCollection(userRole.StoreID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	now := time.Now()
	userRole.Deleted = true
	userRole.DeletedBy = &userID
	userRole.DeletedAt = &now

	updateOptions := options.Update()
	updateOptions.SetUpsert(false)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": userRole.ID},
		bson.M{"$set": userRole},
		updateOptions,
	)
	return err
}

func FindUserRoleByID(storeID *primitive.ObjectID, id *primitive.ObjectID, selectFields bson.M) (*UserRole, error) {
	collection := getUserRoleCollection(storeID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	var userRole UserRole
	err := collection.FindOne(ctx, bson.M{"_id": id, "deleted": bson.M{"$ne": true}}, findOneOptions).Decode(&userRole)
	if err != nil {
		return nil, err
	}
	return &userRole, nil
}

func SearchUserRole(w http.ResponseWriter, r *http.Request) (userRoles []UserRole, criterias SearchCriterias, err error) {
	criterias = SearchCriterias{
		Page: 1,
		Size: 10,
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	if name := r.URL.Query().Get("search[name]"); name != "" {
		criterias.SearchBy["name"] = bson.M{"$regex": name, "$options": "i"}
	}

	keys, ok := r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		p, _ := strconv.Atoi(keys[0])
		criterias.Page = p
	}

	keys, ok = r.URL.Query()["page_size"]
	if ok && len(keys[0]) >= 1 {
		s, _ := strconv.Atoi(keys[0])
		criterias.Size = s
	}

	if criterias.Page < 1 {
		criterias.Page = 1
	}
	if criterias.Size < 1 {
		criterias.Size = 10
	}

	criterias.SortBy = map[string]interface{}{"created_at": -1}

	// Multi-store search: search[store_ids] = comma-separated IDs (used by user form role suggestions)
	if storeIDsStr := r.URL.Query().Get("search[store_ids]"); storeIDsStr != "" {
		parts := strings.Split(storeIDsStr, ",")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)
		findOptions.SetSort(criterias.SortBy)
		for _, p := range parts {
			oid, e := primitive.ObjectIDFromHex(strings.TrimSpace(p))
			if e != nil {
				continue
			}
			collection := getUserRoleCollection(&oid)
			cur, e := collection.Find(ctx, criterias.SearchBy, findOptions)
			if e != nil || cur == nil {
				continue
			}
			for cur.Next(ctx) {
				var ur UserRole
				if decErr := cur.Decode(&ur); decErr == nil {
					userRoles = append(userRoles, ur)
				}
			}
			cur.Close(ctx)
		}
		return userRoles, criterias, nil
	}

	// Single-store search: search[store_id] required for the list/view pages
	var storeID *primitive.ObjectID
	if storeIDStr := r.URL.Query().Get("search[store_id]"); storeIDStr != "" {
		oid, e := primitive.ObjectIDFromHex(storeIDStr)
		if e != nil {
			return userRoles, criterias, errors.New("invalid store_id: " + e.Error())
		}
		storeID = &oid
	} else {
		return userRoles, criterias, errors.New("search[store_id] is required")
	}

	offset := int64((criterias.Page - 1) * criterias.Size)
	collection := getUserRoleCollection(storeID)
	ctx := context.Background()

	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)
	findOptions.SetSkip(offset)
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return userRoles, criterias, errors.New("Error fetching user roles: " + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for cur != nil && cur.Next(ctx) {
		if err := cur.Err(); err != nil {
			return userRoles, criterias, errors.New("Cursor error: " + err.Error())
		}
		var ur UserRole
		if err := cur.Decode(&ur); err != nil {
			return userRoles, criterias, errors.New("Cursor decode error: " + err.Error())
		}
		userRoles = append(userRoles, ur)
	}

	return userRoles, criterias, nil
}

func GetUserRolesTotalCount(storeID *primitive.ObjectID, searchBy map[string]interface{}) (int64, error) {
	collection := getUserRoleCollection(storeID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := collection.CountDocuments(ctx, searchBy)
	return count, err
}

// UserHasPermission checks whether the current UserObject has the given action
// on the given resource via their effective RBAC permissions.
func UserHasPermission(resource, action string) bool {
	if UserObject == nil || len(UserObject.RoleIDs) == 0 {
		return false
	}
	perms, err := GetEffectivePermissions(UserObject.StoreIDs, UserObject.RoleIDs)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if p.Resource == resource {
			switch action {
			case "read":
				return p.Read
			case "create":
				return p.Create
			case "update":
				return p.Update
			case "delete":
				return p.Delete
			}
		}
	}
	return false
}

// IsUserRoleInUse returns true if any non-deleted user has this role assigned.
func IsUserRoleInUse(roleID *primitive.ObjectID) (bool, error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := collection.CountDocuments(ctx, bson.M{
		"role_ids": roleID,
		"deleted":  bson.M{"$ne": true},
	})
	return count > 0, err
}

// GetEffectivePermissions unions permissions across all roles a user holds,
// searching each store's DB for the matching role IDs.
func GetEffectivePermissions(storeIDs []*primitive.ObjectID, roleIDs []*primitive.ObjectID) ([]Permission, error) {
	merged := map[string]*Permission{}

	for _, storeID := range storeIDs {
		if storeID == nil || storeID.IsZero() {
			continue
		}
		collection := getUserRoleCollection(storeID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for _, rid := range roleIDs {
			var ur UserRole
			err := collection.FindOne(ctx, bson.M{"_id": rid, "deleted": bson.M{"$ne": true}}).Decode(&ur)
			if err != nil {
				continue
			}
			for _, p := range ur.Permissions {
				if existing, ok := merged[p.Resource]; ok {
					if p.Read {
						existing.Read = true
					}
					if p.Create {
						existing.Create = true
					}
					if p.Update {
						existing.Update = true
					}
					if p.Delete {
						existing.Delete = true
					}
				} else {
					cp := p
					merged[p.Resource] = &cp
				}
			}
		}
	}

	result := make([]Permission, 0, len(merged))
	for _, p := range merged {
		result = append(result, *p)
	}
	return result, nil
}
