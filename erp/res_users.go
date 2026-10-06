package erp

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/sirinibin/startpos/backend/controller"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func userRoleID(u M) string {
	if r := str(get(u, "erp.role")); r != "" {
		return r
	}
	return legacyRoleToContract(str(u["role"]), boolv(u["admin"]))
}

func userToContract(x *mapCtx, u M) M {
	storeIDs := ids(u["store_ids"])
	rec := M{
		"name": str(u["name"]), "email": strings.ToLower(str(u["email"])), "phone": str(u["mob"]),
		"role": userRoleID(u), "storeIds": storeIDs, "status": userStatus(u),
		"lastLogin": str(get(u, "erp.x.lastLogin")), "apiRole": str(u["role"]), "admin": boolv(u["admin"]),
		"avatar": nilIfEmpty(str(u["photo"])), "apiTokens": []interface{}{},
	}
	if rec["lastLogin"] == "" {
		rec["lastLogin"] = fmtDT(u["last_online_at"])
	}
	return rec
}

// userToContractFull renders a user for /auth/me and /auth/login.
func userToContractFull(u M, stores []M) M {
	rec := applyEnvelope(userToContract(nil, u), u, boolv(u["deleted"]))
	rec["id"] = hexOf(u["_id"])
	if ids, ok := rec["storeIds"].([]string); ok && len(ids) == 0 && isLegacyAdmin(u) {
		all := []string{}
		for _, s := range stores {
			all = append(all, hexOf(s["_id"]))
		}
		rec["storeIds"] = all
	}
	delete(rec, "password")
	return rec
}

var userKnown = knownSet("name", "email", "phone", "role", "storeIds", "avatar", "apiRole", "admin", "password")

func userValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if strings.TrimSpace(str(rec["name"])) == "" {
		e["name"] = "required"
	}
	em := strings.ToLower(strings.TrimSpace(str(rec["email"])))
	if em == "" {
		e["email"] = "required"
	} else if !validEmail(em) {
		e["email"] = "invalid email"
	} else if other := findUserByEmailCI(em); other != nil && !boolv(other["deleted"]) && (prev == nil || hexOf(other["_id"]) != hexOf(prev["_id"])) {
		e["email"] = "already in use"
	}
	if p := str(rec["phone"]); p != "" && !ValidSaudiMobile(p) && !ValidSaudiPhone(p) {
		e["phone"] = "invalid phone"
	}
	role := str(rec["role"])
	if role == "" {
		e["role"] = "required"
	} else if systemRoleByID(role) == nil && loadCustomRole(role) == nil {
		e["role"] = "unknown role"
	}
	if sids := strs(rec["storeIds"]); len(sids) == 0 && !boolv(rec["admin"]) {
		e["storeIds"] = "at least one store"
	}
	if s := str(rec["status"]); s != "" && s != "active" && s != "inactive" {
		e["status"] = "active or inactive"
	}
	if pw, ok := rec["password"]; ok && pw != nil && len(str(pw)) < 8 {
		e["password"] = "at least 8 characters"
	}
	return e
}

func randomPassword() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "Tmp!" + hex.EncodeToString(b)
}

func userToLegacy(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error) {
	c := x.c
	// store access: a user can only grant stores they can access
	sids := strs(rec["storeIds"])
	for _, s := range sids {
		if c.store(s) == nil {
			return nil, errForbidden("You cannot grant access to a store you do not manage.")
		}
	}
	if !create && ch["role"] && prev != nil && hexOf(prev["_id"]) == c.UserID.Hex() && str(rec["role"]) != userRoleID(prev) {
		return nil, errForbidden("You cannot change your own role.")
	}
	if ch["role"] && str(rec["role"]) == "r_admin" && c.RoleID != "r_admin" {
		return nil, errForbidden("Only an administrator can assign the Admin role.")
	}
	p := M{
		"name":  str(rec["name"]),
		"email": strings.ToLower(strings.TrimSpace(str(rec["email"]))),
		"mob":   cleanPhone(str(rec["phone"])),
	}
	if create || ch["role"] {
		if prev != nil && strings.EqualFold(str(prev["role"]), "Admin") {
			p["role"] = "Admin" // never demote a legacy platform admin from StartERP
		} else {
			p["role"] = contractRoleToLegacy(str(rec["role"]))
		}
	} else if prev != nil {
		p["role"] = str(prev["role"])
	}
	oids := []string{}
	for _, s := range sids {
		oids = append(oids, s)
	}
	if create || ch["storeIds"] {
		p["store_ids"] = oids
	} else if prev != nil {
		p["store_ids"] = ids(prev["store_ids"])
	}
	if prev != nil {
		p["role_ids"] = ids(prev["role_ids"])
		p["admin"] = boolv(prev["admin"])
	} else {
		p["role_ids"] = []string{}
	}
	if create {
		pw := str(rec["password"])
		if pw == "" {
			pw = randomPassword()
		}
		p["password"] = pw
	} else if ch["password"] && str(rec["password"]) != "" {
		p["password"] = str(rec["password"])
	}
	if ch["avatar"] {
		if a := str(rec["avatar"]); strings.HasPrefix(a, "data:") {
			p["photo_content"] = a
		}
	}
	return p, nil
}

func newUsersResource() *Resource {
	b := &legacyBackend{
		coll: "user", inMain: true, mainOrg: true, deletedKey: "deleted",
		toC: userToContract, toL: userToLegacy, known: userKnown, validate: userValidate,
		access: func(c *Ctx) bson.M {
			if c.Admin {
				return bson.M{}
			}
			oids := []primitive.ObjectID{c.UserID}
			for _, s := range c.Stores {
				if id, ok := oidOf(s["_id"]); ok {
					oids = append(oids, id)
				}
			}
			return bson.M{"$or": bson.A{bson.M{"_id": c.UserID}, bson.M{"store_ids": bson.M{"$in": oids}}}}
		},
		v1: v1Ops{path: "/v1/user", create: controller.CreateUser, update: controller.UpdateUser, delete: controller.DeleteUser, noStoreParam: true},
		fieldErr: map[string]string{"name": "name", "email": "email", "mob": "phone", "password": "password",
			"role": "role", "store_ids": "storeIds", "photo_content": "avatar"},
		afterWrite: func(x *mapCtx, hexID string, rec, prevDoc M, ch map[string]bool, create bool) error {
			oid, _ := oidOf(hexID)
			set := bson.M{}
			if create || ch["role"] {
				set["erp.role"] = str(rec["role"])
			}
			if create && str(rec["password"]) == "" {
				set["erp.x.mustChangePassword"] = true
			}
			if len(set) == 0 {
				return nil
			}
			ctx, cancel := dbctx()
			defer cancel()
			_, err := mainDB().Collection("user").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": set})
			if err != nil {
				return errInternal("db: " + err.Error())
			}
			return nil
		},
		noRestore: "Restoring users is not supported by the existing system; create the user again.",
	}
	return &Resource{Name: "users", Path: "users", Scope: "org", Module: "settings", Legacy: "main DB `user`", Backend: &usersBackend{b}}
}

// usersBackend keeps the password out of erp.x and blocks self-deletion.
type usersBackend struct{ *legacyBackend }

func (u *usersBackend) Create(c *Ctx, storeHex string, body M, meta WriteMeta) (M, error) {
	if !c.can("settings", "create") {
		return nil, errForbidden("")
	}
	return u.legacyBackend.Create(c, storeHex, body, meta)
}

func (u *usersBackend) Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	if id == c.UserID.Hex() {
		return nil, errForbidden("You cannot delete your own account.")
	}
	return u.legacyBackend.Delete(c, storeHex, id, meta)
}
