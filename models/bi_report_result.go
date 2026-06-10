package models

import (
	"context"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// FindStoreByIDString is a convenience wrapper for POST handlers that receive
// store_id as a plain hex string in the request body rather than a query param.
func FindStoreByIDString(idHex string) (*Store, error) {
	idHex = strings.TrimPrefix(idHex, "store_")
	oid, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return nil, err
	}
	return FindStoreByID(&oid, bson.M{})
}

// BIReportResult stores the CSV and optional PDF output of a BI report run.
// One document per (store, report_key) — always overwritten on each cron run.
type BIReportResult struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	StoreID     primitive.ObjectID `bson:"store_id" json:"store_id"`
	ReportKey   string             `bson:"report_key" json:"report_key"`
	CSVContent  string             `bson:"csv_content" json:"csv_content"`
	PDFContent  []byte             `bson:"pdf_content,omitempty" json:"-"`
	HasPDF      bool               `bson:"has_pdf" json:"has_pdf"`
	RowCount    int                `bson:"row_count" json:"row_count"`
	GeneratedAt time.Time          `bson:"generated_at" json:"generated_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

func (store *Store) GetBIReportResult(reportKey string) (*BIReportResult, error) {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("bi_report_result")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := options.FindOne().SetProjection(bson.M{"pdf_content": 0})
	var result BIReportResult
	if err := col.FindOne(ctx, bson.M{"store_id": store.ID, "report_key": reportKey}, opts).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (store *Store) GetBIReportResultWithPDF(reportKey string) (*BIReportResult, error) {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("bi_report_result")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result BIReportResult
	if err := col.FindOne(ctx, bson.M{"store_id": store.ID, "report_key": reportKey}).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (store *Store) UpsertBIReportResult(reportKey, csvContent string, pdfContent []byte, rowCount int) error {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("bi_report_result")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	now := time.Now().UTC()
	doc := BIReportResult{
		StoreID:     store.ID,
		ReportKey:   reportKey,
		CSVContent:  csvContent,
		PDFContent:  pdfContent,
		HasPDF:      len(pdfContent) > 0,
		RowCount:    rowCount,
		GeneratedAt: now,
		UpdatedAt:   now,
	}
	opts := options.Replace().SetUpsert(true)
	_, err := col.ReplaceOne(ctx, bson.M{"store_id": store.ID, "report_key": reportKey}, doc, opts)
	return err
}
