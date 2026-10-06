package erp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Ctx is the per-request adapter context. Unlike the legacy handlers it never
// touches the process-global models.UserObject.
type Ctx struct {
	R        *http.Request
	Token    string // raw access token
	Claims   models.TokenClaims
	User     M
	UserID   primitive.ObjectID
	UserName string
	Admin    bool // legacy Admin (sees every store)
	RoleID   string
	Perms    Perms
	Stores   []M // accessible, non-deleted legacy store documents
	storeIdx map[string]M
}

func mainDB() *mongo.Database { return db.Client("").Database(db.GetPosDB()) }

func storeDB(storeHex string) *mongo.Database { return db.GetDB("store_" + storeHex) }

func dbctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}

func bearer(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

// authenticate validates the bearer token exactly like the legacy middleware
// (JWT signature + Redis session) and loads the caller.
func authenticate(r *http.Request) (*Ctx, error) {
	tok := bearer(r)
	if tok == "" {
		return nil, errUnauthorized("Authentication required.")
	}
	claims, err := models.AuthenticateByJWTToken(tok)
	if err != nil || claims.Type != "access_token" {
		return nil, errUnauthorized("Your session has expired. Please sign in again.")
	}
	uid, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return nil, errUnauthorized("Invalid session.")
	}
	user, err := loadUser(uid)
	if err != nil || user == nil || boolv(user["deleted"]) {
		return nil, errUnauthorized("Account not found.")
	}
	if userStatus(user) == "inactive" {
		return nil, errUnauthorized("This account is inactive.")
	}
	c := &Ctx{R: r, Token: tok, Claims: claims, User: user, UserID: uid}
	if err := c.loadAccess(); err != nil {
		return nil, err
	}
	return c, nil
}

func loadUser(id primitive.ObjectID) (M, error) {
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	err := mainDB().Collection("user").FindOne(ctx, bson.M{"_id": id}).Decode(&raw)
	if err != nil {
		return nil, err
	}
	return normDoc(raw), nil
}

func userStatus(u M) string {
	if s := str(get(u, "erp.x.status")); s != "" {
		return s
	}
	return "active"
}

func isLegacyAdmin(u M) bool {
	return boolv(u["admin"]) || strings.EqualFold(str(u["role"]), "Admin")
}

func (c *Ctx) loadAccess() error {
	c.UserName = str(c.User["name"])
	c.Admin = isLegacyAdmin(c.User)
	stores, err := accessibleStores(c.User)
	if err != nil {
		return errInternal("Unable to load stores.")
	}
	c.Stores = stores
	c.storeIdx = map[string]M{}
	for _, s := range stores {
		c.storeIdx[hexOf(s["_id"])] = s
	}
	c.RoleID, c.Perms = effectivePerms(c.User, stores)
	return nil
}

func accessibleStores(u M) ([]M, error) {
	filter := bson.M{"deleted": bson.M{"$ne": true}}
	if !isLegacyAdmin(u) {
		oids := []primitive.ObjectID{}
		for _, s := range arr(u["store_ids"]) {
			if id, ok := oidOf(s); ok {
				oids = append(oids, id)
			}
		}
		if len(oids) == 0 {
			return []M{}, nil
		}
		filter["_id"] = bson.M{"$in": oids}
	}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := mainDB().Collection("store").Find(ctx, filter, options.Find().SetSort(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []M{}
	for cur.Next(ctx) {
		out = append(out, bsonToM(cur.Current))
	}
	return out, cur.Err()
}

// effectivePerms resolves the caller's contract role and permissions:
//   - an additive user.erp.role (set through the adapter) wins,
//   - otherwise the legacy user.role string maps to a system role,
//   - legacy user_role documents referenced by user.role_ids are unioned in
//     (so legacy RBAC grants keep working).
func effectivePerms(u M, stores []M) (string, Perms) {
	roleID := str(get(u, "erp.role"))
	if roleID == "" {
		roleID = legacyRoleToContract(str(u["role"]), boolv(u["admin"]))
	}
	if isLegacyAdmin(u) {
		// Legacy admins keep full access whatever erp.role says.
		return "r_admin", permsFromM(systemRoleByID("r_admin")["perms"])
	}
	var base Perms
	if r := systemRoleByID(roleID); r != nil {
		base = permsFromM(r["perms"])
	} else if r := loadCustomRole(roleID); r != nil {
		base = permsFromM(r["perms"])
	} else {
		roleID = legacyRoleToContract(str(u["role"]), false)
		base = permsFromM(systemRoleByID(roleID)["perms"])
	}
	roleIDs := []primitive.ObjectID{}
	for _, r := range arr(u["role_ids"]) {
		if id, ok := oidOf(r); ok {
			roleIDs = append(roleIDs, id)
		}
	}
	if len(roleIDs) > 0 {
		for _, s := range stores {
			ctx, cancel := dbctx()
			cur, err := storeDB(hexOf(s["_id"])).Collection("user_role").Find(ctx, bson.M{"_id": bson.M{"$in": roleIDs}, "deleted": bson.M{"$ne": true}})
			if err == nil {
				for cur.Next(ctx) {
					d := bsonToM(cur.Current)
					base = unionPerms(base, legacyPermissionsToPerms(arr(d["permissions"])))
				}
				cur.Close(ctx)
			}
			cancel()
		}
	}
	return roleID, base
}

func loadCustomRole(id string) M {
	if id == "" {
		return nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := mainDB().Collection(collCustomRole).FindOne(ctx, bson.M{"_id": id, "deleted": bson.M{"$ne": true}}).Decode(&raw); err != nil {
		return nil
	}
	return normDoc(raw)
}

func (c *Ctx) can(module, verb string) bool {
	if module == "" {
		return true
	}
	if c.Perms == nil {
		return false
	}
	return c.Perms[module][verb]
}

func (c *Ctx) store(hex string) M { return c.storeIdx[hex] }

func (c *Ctx) storeHexes() []string {
	out := make([]string, 0, len(c.Stores))
	for _, s := range c.Stores {
		out = append(out, hexOf(s["_id"]))
	}
	return out
}

// primaryStore is where org-scoped records backed by per-store legacy
// collections (categories, brands, …) are created.
func (c *Ctx) primaryStore() string {
	if h := hexOf(c.User["store_id"]); h != "" && c.storeIdx[h] != nil {
		return h
	}
	for _, s := range arr(c.User["store_ids"]) {
		if h := hexOf(s); h != "" && c.storeIdx[h] != nil {
			return h
		}
	}
	if len(c.Stores) > 0 {
		return hexOf(c.Stores[0]["_id"])
	}
	return ""
}
