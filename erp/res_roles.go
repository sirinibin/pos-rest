package erp

import (
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// collCustomRole (main DB, NEW): StartERP custom roles (org-scoped).
const collCustomRole = "erp_role"

// rolesBackend = system roles (synthesized, immutable) + custom roles
// (erp_role, native) + legacy user_role documents (read-only, mapped).
type rolesBackend struct {
	custom *nativeBackend
}

func legacyRoleToContractRole(storeHex string, d M) M {
	rec := M{"id": hexOf(d["_id"]), "name": str(d["name"]), "system": false, "legacy": true,
		"description": "Legacy role (" + str(d["store_name"]) + ") — edit in the existing app",
		"perms":       permsToM(legacyPermissionsToPerms(arr(d["permissions"]))), "maxDiscount": 100,
		"flags": M{"manageUsers": false, "viewReports": true, "deleteRecords": false}}
	return applyEnvelope(rec, d, boolv(d["deleted"]))
}

func (b *rolesBackend) legacyRoles(c *Ctx, includeDeleted bool) []M {
	out := []M{}
	for _, s := range c.storeHexes() {
		f := bson.M{}
		if !includeDeleted {
			f["deleted"] = bson.M{"$ne": true}
		}
		ctx, cancel := dbctx()
		cur, err := storeDB(s).Collection("user_role").Find(ctx, f, options.Find().SetSort(bson.M{"_id": 1}))
		if err == nil {
			for cur.Next(ctx) {
				out = append(out, legacyRoleToContractRole(s, bsonToM(cur.Current)))
			}
			cur.Close(ctx)
		}
		cancel()
	}
	return out
}

func (b *rolesBackend) List(c *Ctx, storeHex string, q ListQuery) ([]M, int64, error) {
	all := systemRoles()
	custom, _, err := b.custom.List(c, "", ListQuery{Limit: 10000, Page: 1, IncludeDeleted: q.IncludeDeleted})
	if err != nil {
		return nil, 0, err
	}
	all = append(all, custom...)
	all = append(all, b.legacyRoles(c, q.IncludeDeleted)...)
	total := int64(len(all))
	start := (q.Page - 1) * q.Limit
	if start > len(all) {
		start = len(all)
	}
	end := start + q.Limit
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}

func (b *rolesBackend) Get(c *Ctx, storeHex, id string, includeHidden bool) (M, error) {
	if r := systemRoleByID(id); r != nil {
		return r, nil
	}
	if r, err := b.custom.Get(c, "", id, includeHidden); err != nil || r != nil {
		return r, err
	}
	for _, r := range b.legacyRoles(c, true) {
		if str(r["id"]) == id {
			return r, nil
		}
	}
	return nil, nil
}

func (b *rolesBackend) Locate(c *Ctx, id string) (string, bool) { return "", true }

func roleValidate(c *Ctx, rec M, selfID string) map[string]string {
	e := map[string]string{}
	name := strings.TrimSpace(str(rec["name"]))
	if name == "" {
		e["name"] = "required"
	} else {
		for _, r := range systemRoles() {
			if strings.EqualFold(str(r["name"]), name) {
				e["name"] = "already exists"
			}
		}
		ctx, cancel := dbctx()
		defer cancel()
		cur, err := mainDB().Collection(collCustomRole).Find(ctx, bson.M{"deleted": bson.M{"$ne": true}})
		if err == nil {
			for cur.Next(ctx) {
				d := bsonToM(cur.Current)
				if str(d["_id"]) != selfID && strings.EqualFold(str(d["name"]), name) {
					e["name"] = "already exists"
				}
			}
			cur.Close(ctx)
		}
	}
	if md, ok := rec["maxDiscount"]; ok && md != nil && (num(md) < 0 || num(md) > 100) {
		e["maxDiscount"] = "0..100"
	}
	return e
}

func (b *rolesBackend) guard(c *Ctx, id string) error {
	if systemRoleByID(id) != nil {
		return errForbidden("System roles cannot be changed.")
	}
	for _, r := range b.legacyRoles(c, true) {
		if str(r["id"]) == id {
			return errUnsupported("Legacy roles are managed in the existing app (User Roles).")
		}
	}
	return nil
}

func (b *rolesBackend) Create(c *Ctx, storeHex string, body M, meta WriteMeta) (M, error) {
	if systemRoleByID(str(body["id"])) != nil {
		return nil, errConflict("id_conflict", "A record with this id already exists.")
	}
	if errs := roleValidate(c, body, ""); len(errs) > 0 {
		return nil, errBadRequest("", errs)
	}
	body = cloneM(body)
	body["system"] = false
	if _, ok := body["perms"]; ok {
		body["perms"] = permsToM(permsFromM(body["perms"]))
	}
	return b.custom.Create(c, "", body, meta)
}

func (b *rolesBackend) Update(c *Ctx, storeHex, id string, prev, next M, changed []string, meta WriteMeta) (M, error) {
	if err := b.guard(c, id); err != nil {
		return nil, err
	}
	if errs := roleValidate(c, next, id); len(errs) > 0 {
		return nil, errBadRequest("", errs)
	}
	next["system"] = false
	return b.custom.Update(c, "", id, prev, next, changed, meta)
}

func (b *rolesBackend) Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	if err := b.guard(c, id); err != nil {
		return nil, err
	}
	return b.custom.Delete(c, "", id, meta)
}

func (b *rolesBackend) Restore(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	if err := b.guard(c, id); err != nil {
		return nil, err
	}
	return b.custom.Restore(c, "", id, meta)
}

func (b *rolesBackend) HardDelete(c *Ctx, storeHex, id string, meta WriteMeta) error {
	if err := b.guard(c, id); err != nil {
		return err
	}
	return b.custom.HardDelete(c, "", id, meta)
}

func newRolesResource() *Resource {
	return &Resource{Name: "roles", Path: "roles", Scope: "org", Module: "settings",
		Legacy:  "system roles (synth) + main DB `erp_role` (NEW) + store DB `user_role` (read-only)",
		Backend: &rolesBackend{custom: &nativeBackend{coll: collCustomRole, org: true, idPrefix: "rol"}}}
}
