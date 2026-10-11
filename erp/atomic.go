package erp

import (
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// collSerialCounters holds one document per numbering series:
// {_id: "<collection>|<code base>", n: <last number handed out>}.
const collSerialCounters = "erp_serial_counters"

// nextSerial hands out the next number of a series, never the same number
// twice, even to requests that arrive together. highest is the highest number
// already used by the stored records (scanned by the caller): the counter
// first moves up to it ($max, so it continues from existing data, including
// codes typed by hand or imported) and then counts on ($inc). Both steps are
// atomic single-document updates (no transaction needed); a number taken by
// a create that then fails is skipped, not reused.
func nextSerial(db *mongo.Database, key string, highest int) (int, error) {
	ctx, cancel := dbctx()
	defer cancel()
	col := db.Collection(collSerialCounters)
	_, err := col.UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$max": bson.M{"n": highest}}, options.Update().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		// two first-time upserts at once: the other one created the counter
		_, err = col.UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$max": bson.M{"n": highest}})
	}
	if err != nil {
		return 0, err
	}
	var d struct {
		N int `bson:"n"`
	}
	err = col.FindOneAndUpdate(ctx, bson.M{"_id": key}, bson.M{"$inc": bson.M{"n": 1}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&d)
	if err != nil {
		return 0, err
	}
	return d.N, nil
}

// claimField marks a record that one request is turning into something else
// (a draft being finalized, a pending transfer being completed). Fields that
// start with "_" never reach API clients.
const claimField = "_erp_claim"

// claimTTL: a claim left behind by a crash stops blocking after this long.
const claimTTL = 10 * time.Minute

// claimRecord takes the record for this request with one conditional,
// atomic update: of several requests (from this process or another one) only
// one gets true; the others get false and must answer 409 (still there) or
// 404 (gone). The winner calls releaseClaim when its work fails.
func claimRecord(col *mongo.Collection, id interface{}) (bool, error) {
	ctx, cancel := dbctx()
	defer cancel()
	now := time.Now()
	res, err := col.UpdateOne(ctx, bson.M{"_id": id, "$or": []bson.M{
		{claimField: bson.M{"$exists": false}},
		{claimField: bson.M{"$lt": now.Add(-claimTTL)}},
	}}, bson.M{"$set": bson.M{claimField: now}})
	if err != nil {
		return false, err
	}
	return res.ModifiedCount == 1, nil
}

// releaseClaim gives a claimed record back (its work failed).
func releaseClaim(col *mongo.Collection, id interface{}) {
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$unset": bson.M{claimField: ""}})
}

// claimOrAnswer claims the record or returns the error the losing request
// answers with: 409 while the record is still there, 404 once it is gone.
func claimOrAnswer(col *mongo.Collection, id interface{}) error {
	ok, err := claimRecord(col, id)
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	if ok {
		return nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	if n, _ := col.CountDocuments(ctx, bson.M{"_id": id}); n == 0 {
		return errNotFound()
	}
	return errConflict("in_progress", "Another user is finishing this record right now.")
}
