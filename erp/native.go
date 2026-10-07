package erp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// nativeBackend stores resources that have NO legacy equivalent in NEW
// additive collections (erp_*). Records are stored in contract shape
// (schemaless) with _id = contract id. Store-scoped native collections live in
// the store DB (store_<hex>), org-scoped ones in the main DB.
type nativeBackend struct {
	coll      string
	org       bool
	dateField string
	serialKey string // numbering (serverNumbers) key, "" = no code
	idPrefix  string
	validate  func(x *mapCtx, rec M, prev M) map[string]string
}

func (b *nativeBackend) col(storeHex string) *mongo.Collection {
	if b.org {
		return mainDB().Collection(b.coll)
	}
	return storeDB(storeHex).Collection(b.coll)
}

const nativeDateKey = "_erp_date"

func (b *nativeBackend) clean(d M) M {
	out := M{}
	for k, v := range d {
		if strings.HasPrefix(k, "_") {
			continue
		}
		out[k] = v
	}
	out["id"] = str(d["_id"])
	if _, ok := out["history"]; !ok {
		out["history"] = []interface{}{}
	}
	return out
}

func (b *nativeBackend) List(c *Ctx, storeHex string, q ListQuery) ([]M, int64, error) {
	f := bson.M{"_erp_hd": bson.M{"$ne": true}}
	if !q.IncludeDeleted {
		f["deleted"] = bson.M{"$ne": true}
	}
	if from := q.fromIn(c.storeLoc(storeHex)); from != nil && b.dateField != "" {
		f[nativeDateKey] = bson.M{"$gte": *from}
	}
	if sf := searchFilter(q.Search, nativeSearchKeys); sf != nil {
		f["$or"] = sf["$or"]
	}
	if len(q.IDs) > 0 {
		f["_id"] = bson.M{"$in": q.IDs}
	}
	if len(q.Where) > 0 || len(q.Range) > 0 {
		return nil, 0, errBadRequest("This list cannot be filtered.", map[string]string{"where": "not supported for this resource"})
	}
	if to := q.toIn(c.storeLoc(storeHex)); to != nil && b.dateField != "" {
		if cur, ok := f[nativeDateKey].(bson.M); ok {
			cur["$lt"] = *to
		} else {
			f[nativeDateKey] = bson.M{"$lt": *to}
		}
	}
	dir := 1
	switch q.Sort {
	case "":
	case "date":
		if q.Desc {
			dir = -1
		}
	default:
		return nil, 0, errBadRequest("This list cannot be sorted by "+q.Sort+".", map[string]string{"sort": "not supported for this resource"})
	}
	ctx, cancel := dbctx()
	defer cancel()
	col := b.col(storeHex)
	total, err := col.CountDocuments(ctx, f)
	if err != nil {
		return nil, 0, errInternal("db: " + err.Error())
	}
	opts := options.Find().SetSort(bson.D{{Key: nativeDateKey, Value: dir}, {Key: "_id", Value: dir}}).
		SetSkip(int64((q.Page - 1) * q.Limit)).SetLimit(int64(q.Limit))
	if p := q.Select.dbProjection("history"); p != nil {
		opts.SetProjection(p)
	}
	cur, err := col.Find(ctx, f, opts)
	if err != nil {
		return nil, 0, errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	rows := []M{}
	for cur.Next(ctx) {
		rows = append(rows, b.clean(bsonToM(cur.Current)))
	}
	return rows, total, cur.Err()
}

func (b *nativeBackend) load(storeHex, id string, includeHidden bool) (M, error) {
	f := bson.M{"_id": id}
	if !includeHidden {
		f["_erp_hd"] = bson.M{"$ne": true}
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	err := b.col(storeHex).FindOne(ctx, f).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	return normDoc(raw), nil
}

func (b *nativeBackend) Get(c *Ctx, storeHex, id string, includeHidden bool) (M, error) {
	d, err := b.load(storeHex, id, includeHidden)
	if err != nil || d == nil {
		return nil, err
	}
	return b.clean(d), nil
}

func (b *nativeBackend) Locate(c *Ctx, id string) (string, bool) {
	if b.org {
		return "", true
	}
	for _, s := range c.storeHexes() {
		if d, _ := b.load(s, id, true); d != nil {
			return s, true
		}
	}
	return "", false
}

func (b *nativeBackend) dateOf(loc *time.Location, rec M) interface{} {
	if b.dateField == "" {
		return nil
	}
	if t, err := parseClientTimeIn(loc, str(rec[b.dateField])); err == nil {
		return t
	}
	return nil
}

func randomID(prefix string) string {
	return prefix + "_" + primitive.NewObjectID().Hex()
}

func (b *nativeBackend) persist(loc *time.Location, storeHex string, rec M, upsert bool) error {
	doc := bson.M{}
	for k, v := range rec {
		if k == "id" {
			continue
		}
		doc[k] = v
	}
	doc["_id"] = str(rec["id"])
	doc[nativeDateKey] = b.dateOf(loc, rec)
	ctx, cancel := dbctx()
	defer cancel()
	var err error
	if upsert {
		_, err = b.col(storeHex).ReplaceOne(ctx, bson.M{"_id": doc["_id"]}, doc, options.Replace().SetUpsert(true))
	} else {
		_, err = b.col(storeHex).InsertOne(ctx, doc)
	}
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return errConflict("id_conflict", "A record with this id already exists.")
		}
		return errInternal("db: " + err.Error())
	}
	return nil
}

func (b *nativeBackend) Create(c *Ctx, storeHex string, body M, meta WriteMeta) (M, error) {
	rec := stripServerOwned(body)
	id := strings.TrimSpace(str(body["id"]))
	if id == "" || strings.HasPrefix(id, "tmp_") {
		id = randomID(b.idPrefix)
	} else if d, _ := b.load(storeHex, id, true); d != nil {
		return nil, errConflict("id_conflict", "A record with this id already exists.")
	}
	x := newMapCtx(c, storeHex)
	if b.validate != nil {
		if errs := b.validate(x, rec, nil); len(errs) > 0 {
			return nil, errBadRequest("", errs)
		}
	}
	if b.serialKey != "" {
		code, err := b.assignCode(x, storeHex, str(rec["code"]), "")
		if err != nil {
			return nil, err
		}
		rec["code"] = code
	}
	loc := c.storeLoc(storeHex)
	now := nowFn().In(loc).Format(layoutDT)
	rec["id"] = id
	if b.org {
		delete(rec, "storeId")
	} else {
		rec["storeId"] = storeHex
	}
	rec["version"] = int64(1)
	rec["deleted"] = false
	ca := str(body["createdAt"])
	if ca == "" {
		ca = now
	}
	rec["createdAt"] = ca
	rec["createdBy"] = c.UserName
	rec["updatedAt"] = now
	rec["updatedBy"] = c.UserName
	rec["history"] = []interface{}{historyEntryIn(loc, c.UserName, "created", []interface{}{})}
	if err := b.persist(loc, storeHex, rec, false); err != nil {
		return nil, err
	}
	return b.Get(c, storeHex, id, true)
}

func (b *nativeBackend) write(c *Ctx, storeHex string, prev, next M, action string) (M, error) {
	next["version"] = intv(prev["version"]) + 1
	loc := c.storeLoc(storeHex)
	next["updatedAt"] = nowFn().In(loc).Format(layoutDT)
	next["updatedBy"] = c.UserName
	next["history"] = appendHistory(arr(prev["history"]), historyEntryIn(loc, c.UserName, action, diff(prev, next)))
	if err := b.persist(loc, storeHex, next, true); err != nil {
		return nil, err
	}
	return b.Get(c, storeHex, str(next["id"]), true)
}

func (b *nativeBackend) Update(c *Ctx, storeHex, id string, prev, next M, changed []string, meta WriteMeta) (M, error) {
	x := newMapCtx(c, storeHex)
	if b.validate != nil {
		if errs := b.validate(x, next, prev); len(errs) > 0 {
			return nil, errBadRequest("", errs)
		}
	}
	if b.serialKey != "" && changedSet(changed)["code"] {
		code, err := b.assignCode(x, storeHex, str(next["code"]), id)
		if err != nil {
			return nil, err
		}
		next["code"] = code
	}
	next["id"] = prev["id"]
	next["createdAt"] = prev["createdAt"]
	next["createdBy"] = prev["createdBy"]
	next["deleted"] = prev["deleted"]
	return b.write(c, storeHex, prev, next, meta.Action)
}

func (b *nativeBackend) Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	prev, err := b.Get(c, storeHex, id, false)
	if err != nil || prev == nil {
		return nil, errNotFound()
	}
	next := cloneM(prev)
	next["deleted"] = true
	return b.write(c, storeHex, prev, next, "deleted")
}

func (b *nativeBackend) Restore(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	prev, err := b.Get(c, storeHex, id, false)
	if err != nil || prev == nil {
		return nil, errNotFound()
	}
	next := cloneM(prev)
	next["deleted"] = false
	return b.write(c, storeHex, prev, next, "restored")
}

func (b *nativeBackend) HardDelete(c *Ctx, storeHex, id string, meta WriteMeta) error {
	ctx, cancel := dbctx()
	defer cancel()
	_, err := b.col(storeHex).DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	return nil
}

// ---- numbering for native resources (contract §5) ----

var reYearPrefix = regexp.MustCompile(`\d{4}-$`)

// assignCode keeps the client code when unused in (store, collection),
// otherwise assigns short-prefix+next number.
func (b *nativeBackend) assignCode(x *mapCtx, storeHex, code, selfID string) (string, error) {
	code = strings.TrimSpace(code)
	ctx, cancel := dbctx()
	defer cancel()
	col := b.col(storeHex)
	if code != "" {
		f := bson.M{"code": code}
		if selfID != "" {
			f["_id"] = bson.M{"$ne": selfID}
		}
		n, err := col.CountDocuments(ctx, f)
		if err != nil {
			return "", errInternal("db: " + err.Error())
		}
		if n == 0 {
			return code, nil
		}
	}
	short := storeShort(x.store)
	prefix, start := serialCfg(x.store, b.serialKey)
	base := short + "-" + prefix
	width := 4
	if reYearPrefix.MatchString(prefix) {
		width = 6
	}
	max := start - 1
	cur, err := col.Find(ctx, bson.M{"code": bson.M{"$regex": "^" + regexp.QuoteMeta(base)}}, options.Find().SetProjection(bson.M{"code": 1}))
	if err == nil {
		for cur.Next(ctx) {
			d := bsonToM(cur.Current)
			if n, err := strconv.Atoi(strings.TrimPrefix(str(d["code"]), base)); err == nil && n > max {
				max = n
			}
		}
		cur.Close(ctx)
	}
	return fmt.Sprintf("%s%0*d", base, width, max+1), nil
}

var _ Backend = (*nativeBackend)(nil)
