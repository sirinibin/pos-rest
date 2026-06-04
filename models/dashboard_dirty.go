package models

// dashboard_dirty.go — lightweight dirty-month queue for incremental dashboard updates.
//
// Strategy:
//  1. Startup: full history backfill (one-time, in main.go).
//  2. On every Create/Update/Delete of a transactional document (order,
//     sales-return, purchase, expense, quotation, etc.), call
//     MarkDashboardDirty(storeID, txDate) — fire-and-forget.
//     The async worker drains the queue continuously.

import (
	"context"
	"sync"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// dirtyJob is queued when a transaction is created/updated/deleted.
type dirtyJob struct {
	StoreID  primitive.ObjectID
	MonthStr string // "2025-01" in the store's local timezone
	TZOffset float64
}

var (
	dirtyQueue chan dirtyJob
	startOnce  sync.Once
)

// StartDashboardDirtyWorker launches the background goroutine that drains the
// dirty queue and triggers recomputation. Call once from main() at startup.
func StartDashboardDirtyWorker() {
	startOnce.Do(func() {
		dirtyQueue = make(chan dirtyJob, 2000)
		go func() {
			pending := map[string]dirtyJob{}
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case job := <-dirtyQueue:
					key := job.StoreID.Hex() + "|" + job.MonthStr
					pending[key] = job

				case <-ticker.C:
					if len(pending) == 0 {
						continue
					}
					snapshot := pending
					pending = map[string]dirtyJob{}
					for _, job := range snapshot {
						_ = ComputeAndUpsertDashboardMonthly(job.StoreID, job.MonthStr, job.TZOffset)
					}
				}
			}
		}()
	})
}

// MarkDashboardDirty queues a dashboard recomputation for the given store and
// UTC transaction time. Non-blocking.
func MarkDashboardDirty(storeID primitive.ObjectID, txTimeUTC *time.Time) {
	if txTimeUTC == nil || dirtyQueue == nil {
		return
	}
	tzOffset := cachedTZOffset(storeID)
	localTime := txTimeUTC.Add(time.Duration(-tzOffset * float64(time.Hour)))
	monthStr := localTime.Format("2006-01")
	job := dirtyJob{StoreID: storeID, MonthStr: monthStr, TZOffset: tzOffset}
	select {
	case dirtyQueue <- job:
	default:
	}
}

// MarkDashboardDirtyMonth queues a recomputation for a pre-formatted local month string ("2025-01").
func MarkDashboardDirtyMonth(storeID primitive.ObjectID, localMonthStr string) {
	if localMonthStr == "" || dirtyQueue == nil {
		return
	}
	tzOffset := cachedTZOffset(storeID)
	job := dirtyJob{StoreID: storeID, MonthStr: localMonthStr, TZOffset: tzOffset}
	select {
	case dirtyQueue <- job:
	default:
	}
}

// ─── Backfill ─────────────────────────────────────────────────────────────────

// BackfillDashboardForStore recomputes monthly records strictly within the
// store's actual data range: earliest transaction month → latest transaction month.
// Pass months > 0 to limit the lookback window; pass 0 for a full backfill.
func BackfillDashboardForStore(storeID primitive.ObjectID, months int) {
	store, err := FindStoreByID(&storeID, nil)
	if err != nil || store == nil {
		return
	}
	tzOffset := CountryTimezoneOffset(store.CountryCode)

	EnsureDashboardMonthlyIndexes(storeID)

	latest := latestTransactionDate(storeID)
	if latest.IsZero() {
		return
	}
	localLatest := latest.Add(time.Duration(-tzOffset * float64(time.Hour)))
	endMonth := time.Date(localLatest.Year(), localLatest.Month(), 1, 0, 0, 0, 0, time.UTC)

	var startMonth time.Time
	if months > 0 {
		startMonth = endMonth.AddDate(0, -(months - 1), 0)
	} else {
		earliest := earliestTransactionDate(storeID)
		if earliest.IsZero() {
			return
		}
		localEarliest := earliest.Add(time.Duration(-tzOffset * float64(time.Hour)))
		startMonth = time.Date(localEarliest.Year(), localEarliest.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	cur := startMonth
	for !cur.After(endMonth) {
		_ = ComputeAndUpsertDashboardMonthly(storeID, cur.Format("2006-01"), tzOffset)
		cur = cur.AddDate(0, 1, 0)
	}
}

// BackfillDashboardForAllStores runs a full backfill for every active store.
func BackfillDashboardForAllStores() {
	stores, err := GetAllStores()
	if err != nil {
		return
	}
	for _, store := range stores {
		if !store.Deleted {
			BackfillDashboardForStore(store.ID, 0)
		}
	}
}

// ─── Per-store TZ offset cache ────────────────────────────────────────────────

var tzCache sync.Map // storeID.Hex() → float64

func cachedTZOffset(storeID primitive.ObjectID) float64 {
	if v, ok := tzCache.Load(storeID.Hex()); ok {
		return v.(float64)
	}
	store, err := FindStoreByID(&storeID, bson.M{"country_code": 1})
	var offset float64
	if err == nil && store != nil {
		offset = CountryTimezoneOffset(store.CountryCode)
	}
	tzCache.Store(storeID.Hex(), offset)
	return offset
}

// ─── Persistent dirty-months collection (for crash recovery) ──────────────────

type persistedDirtyMonth struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	StoreID  primitive.ObjectID `bson:"store_id"`
	MonthStr string             `bson:"month_str"`
	QueuedAt time.Time          `bson:"queued_at"`
}

func PersistDirtyMonth(storeID primitive.ObjectID, monthStr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coll := db.GetDB("store_" + storeID.Hex()).Collection("dashboard_dirty_months")
	_, _ = coll.UpdateOne(ctx,
		bson.M{"store_id": storeID, "month_str": monthStr},
		bson.M{"$setOnInsert": persistedDirtyMonth{
			StoreID: storeID, MonthStr: monthStr, QueuedAt: time.Now(),
		}},
		options.Update().SetUpsert(true),
	)
}

func ClearDirtyMonth(storeID primitive.ObjectID, monthStr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coll := db.GetDB("store_" + storeID.Hex()).Collection("dashboard_dirty_months")
	_, _ = coll.DeleteOne(ctx, bson.M{"store_id": storeID, "month_str": monthStr})
}

// DrainPersistedDirtyDates re-queues any months left from a previous crash.
// Call once at startup (after StartDashboardDirtyWorker).
func DrainPersistedDirtyDates() {
	stores, err := GetAllStores()
	if err != nil {
		return
	}
	for _, store := range stores {
		if store.Deleted {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		cur, err := db.GetDB("store_"+store.ID.Hex()).Collection("dashboard_dirty_months").
			Find(ctx, bson.M{}, options.Find())
		if err != nil {
			cancel()
			continue
		}
		var rows []persistedDirtyMonth
		_ = cur.All(ctx, &rows)
		cur.Close(ctx)
		cancel()
		for _, row := range rows {
			MarkDashboardDirtyMonth(store.ID, row.MonthStr)
		}
	}
}

// EnsureDirtyDatesIndex creates an index on the dashboard_dirty_months collection.
func EnsureDirtyDatesIndex(storeID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	coll := db.GetDB("store_" + storeID.Hex()).Collection("dashboard_dirty_months")
	_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{bson.E{Key: "store_id", Value: 1}, bson.E{Key: "month_str", Value: 1}},
		Options: options.Index().SetUnique(true).SetBackground(true),
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func monthsBetween(a, b time.Time) int {
	years := b.Year() - a.Year()
	months := int(b.Month()) - int(a.Month())
	return years*12 + months
}

// latestTransactionDate scans key collections and returns the most recent date.
func latestTransactionDate(storeID primitive.ObjectID) time.Time {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sdb := db.GetDB("store_" + storeID.Hex())
	colls := []string{
		"order", "salesreturn", "sales_payment",
		"purchase", "purchasereturn",
		"expense", "quotation", "quotation_sales_return",
		"customerdeposit",
	}
	var latest time.Time
	for _, name := range colls {
		var row struct {
			Date time.Time `bson:"date"`
		}
		err := sdb.Collection(name).FindOne(ctx, bson.M{},
			options.FindOne().
				SetSort(bson.D{bson.E{Key: "date", Value: -1}}).
				SetProjection(bson.M{"date": 1}),
		).Decode(&row)
		if err != nil || row.Date.IsZero() {
			continue
		}
		if row.Date.After(latest) {
			latest = row.Date
		}
	}
	return latest
}

// earliestTransactionDate scans key collections and returns the earliest date.
func earliestTransactionDate(storeID primitive.ObjectID) time.Time {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sdb := db.GetDB("store_" + storeID.Hex())
	colls := []string{
		"order", "salesreturn", "sales_payment",
		"purchase", "purchasereturn",
		"expense", "quotation", "quotation_sales_return",
		"customerdeposit",
	}
	var earliest time.Time
	for _, name := range colls {
		var row struct {
			Date time.Time `bson:"date"`
		}
		err := sdb.Collection(name).FindOne(ctx, bson.M{},
			options.FindOne().
				SetSort(bson.D{bson.E{Key: "date", Value: 1}}).
				SetProjection(bson.M{"date": 1}),
		).Decode(&row)
		if err != nil || row.Date.IsZero() {
			continue
		}
		if earliest.IsZero() || row.Date.Before(earliest) {
			earliest = row.Date
		}
	}
	return earliest
}
