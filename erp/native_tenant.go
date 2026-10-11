package erp

import (
	"log"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Org-scoped native records (custom roles, customer categories) live in one
// main-DB collection shared by every company. Each record now carries the
// stores of the company that owns it (_erp_stores, hidden from the API), and
// a non-admin only lists, reads and changes records owned by one of their
// stores: the rule users already follow (people who share a store see each
// other). Platform admins (legacy Admin) see every record.
const nativeOwnerKey = "_erp_stores"

// orgOwnerStores is what a new org record is stamped with: the creator's own
// stores (store_ids) that they can use, or every store they can use.
func orgOwnerStores(c *Ctx) []string {
	out := []string{}
	for _, s := range arr(c.User["store_ids"]) {
		if h := hexOf(s); h != "" && c.store(h) != nil {
			out = append(out, h)
		}
	}
	if len(out) == 0 && !c.Admin {
		out = c.storeHexes()
	}
	return out
}

// tenantFilter narrows an org collection query to the caller's company; nil
// for platform admins.
func tenantFilter(c *Ctx) bson.M {
	if c == nil || c.Admin {
		return nil
	}
	return bson.M{nativeOwnerKey: bson.M{"$in": c.storeHexes()}}
}

// ownedBy reports whether raw record d belongs to the caller's company.
func ownedBy(c *Ctx, d M) bool {
	if c == nil || c.Admin {
		return true
	}
	for _, s := range strs(d[nativeOwnerKey]) {
		if c.store(s) != nil {
			return true
		}
	}
	return false
}

var claimOnce sync.Map // collection -> *sync.Once

// claimUnowned gives every record written before owners were stored an
// owner, once per collection and process (idempotent: it only touches
// records without one). The owner is derived from what the record has:
//   - a custom role: the stores of the users who hold it (user.erp.role),
//   - otherwise the stores of the one user named as its creator (createdBy);
//     when several users share that name the record is left to platform
//     admins (owner []) rather than shown to several companies.
func (b *nativeBackend) claimUnowned() {
	if !b.org {
		return
	}
	o, _ := claimOnce.LoadOrStore(b.coll, &sync.Once{})
	o.(*sync.Once).Do(func() {
		ctx, cancel := dbctx()
		defer cancel()
		cur, err := mainDB().Collection(b.coll).Find(ctx, bson.M{nativeOwnerKey: bson.M{"$exists": false}},
			options.Find().SetProjection(bson.M{"_id": 1, "createdBy": 1}))
		if err != nil {
			log.Printf("erp: claim owners of %s: %v", b.coll, err)
			claimOnce.Delete(b.coll) // try again on the next request
			return
		}
		type rec struct{ id, by string }
		recs := []rec{}
		for cur.Next(ctx) {
			d := bsonToM(cur.Current)
			recs = append(recs, rec{str(d["_id"]), str(d["createdBy"])})
		}
		cur.Close(ctx)
		for _, r := range recs {
			owner := []string{}
			if b.coll == collCustomRole {
				owner = userStores(bson.M{"erp.role": r.id, "deleted": bson.M{"$ne": true}}, false)
			}
			if len(owner) == 0 && strings.TrimSpace(r.by) != "" {
				owner = userStores(bson.M{"name": r.by}, true)
			}
			_, err := mainDB().Collection(b.coll).UpdateOne(ctx, bson.M{"_id": r.id, nativeOwnerKey: bson.M{"$exists": false}},
				bson.M{"$set": bson.M{nativeOwnerKey: owner}})
			if err != nil {
				log.Printf("erp: claim owner of %s %s: %v", b.coll, r.id, err)
			}
			if len(owner) == 0 {
				log.Printf("erp: %s %s has no clear owner; only platform admins see it", b.coll, r.id)
			}
		}
	})
}

// userStores is the union of store_ids of the users matching f; with single,
// nothing unless exactly one user matches.
func userStores(f bson.M, single bool) []string {
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := mainDB().Collection("user").Find(ctx, f, options.Find().SetProjection(bson.M{"store_ids": 1}).SetLimit(1000))
	if err != nil {
		return []string{}
	}
	defer cur.Close(ctx)
	seen := map[string]bool{}
	out := []string{}
	n := 0
	for cur.Next(ctx) {
		n++
		d := bsonToM(cur.Current)
		for _, s := range arr(d["store_ids"]) {
			if id, ok := oidOf(s); ok && id != primitive.NilObjectID && !seen[id.Hex()] {
				seen[id.Hex()] = true
				out = append(out, id.Hex())
			}
		}
	}
	if single && n != 1 {
		return []string{}
	}
	return out
}
