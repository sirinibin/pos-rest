package models

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RFQActivityLog is a single timestamped event in the RFQ processing timeline.
type RFQActivityLog struct {
	ID      primitive.ObjectID     `bson:"_id,omitempty"     json:"id,omitempty"`
	At      time.Time              `bson:"at"                json:"at"`
	Step    string                 `bson:"step"              json:"step"`
	Message string                 `bson:"message"           json:"message"`
	Icon    string                 `bson:"icon,omitempty"    json:"icon,omitempty"`
	Color   string                 `bson:"color,omitempty"   json:"color,omitempty"`
	Details map[string]interface{} `bson:"details,omitempty" json:"details,omitempty"`
}

// RFQProduct is a single line item extracted from an incoming RFQ message.
type RFQProduct struct {
	ProductID    *primitive.ObjectID `bson:"product_id,omitempty"    json:"product_id,omitempty"`
	PartNo       string              `bson:"part_no,omitempty"       json:"part_no,omitempty"`
	Name         string              `bson:"name"                    json:"name"`
	NameInArabic string              `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	Quantity     float64             `bson:"quantity,omitempty"      json:"quantity,omitempty"`
	Unit         string              `bson:"unit,omitempty"          json:"unit,omitempty"`
	Notes        string              `bson:"notes,omitempty"         json:"notes,omitempty"`
}

type RFQForwardRecord struct {
	SupplierID     primitive.ObjectID `bson:"supplier_id,omitempty" json:"supplier_id,omitempty"`
	SupplierName   string             `bson:"supplier_name" json:"supplier_name"`
	Phone          string             `bson:"phone" json:"phone"`
	SentFromPhone  string             `bson:"sent_from_phone,omitempty" json:"sent_from_phone,omitempty"`
	PurchaseMarket string             `bson:"purchase_market,omitempty" json:"purchase_market,omitempty"`
	Category       string             `bson:"category,omitempty" json:"category,omitempty"`
	GoogleMapsURL  string             `bson:"google_maps_url,omitempty" json:"google_maps_url,omitempty"`
	SentMessage    string             `bson:"sent_message,omitempty" json:"sent_message,omitempty"`
	SentAt         *time.Time         `bson:"sent_at,omitempty" json:"sent_at,omitempty"`
	// pending | sent | failed
	Status   string `bson:"status" json:"status"`
	ErrorMsg string `bson:"error_msg,omitempty" json:"error_msg,omitempty"`
}

// RFQDocument holds metadata about an attached file (PDF, Excel, etc.)
type RFQDocument struct {
	URL      string `bson:"url" json:"url"`
	FileName string `bson:"file_name" json:"file_name"`
	MimeType string `bson:"mime_type" json:"mime_type"`
}

// SupplierReplyPrice holds LLM-extracted price for a single product in a supplier reply.
type SupplierReplyPrice struct {
	ProductIndex int     `bson:"product_index"          json:"product_index"` // index into RFQReceived.Products
	PartNo       string  `bson:"part_no,omitempty"      json:"part_no,omitempty"`
	ProductName  string  `bson:"product_name"           json:"product_name"`
	Quantity     float64 `bson:"quantity,omitempty"     json:"quantity,omitempty"`
	UnitPrice    float64 `bson:"unit_price"             json:"unit_price"`
	Currency     string  `bson:"currency,omitempty"     json:"currency,omitempty"`
	// VATIncluded indicates whether the UnitPrice already includes VAT.
	VATIncluded  bool    `bson:"vat_included"           json:"vat_included"`
	Notes        string  `bson:"notes,omitempty"        json:"notes,omitempty"`
}

// SupplierReply stores a single reply from a supplier (WhatsApp, email, manual, etc.) including extracted prices.
type SupplierReply struct {
	ID               primitive.ObjectID   `bson:"_id,omitempty"              json:"id,omitempty"`
	SupplierID       *primitive.ObjectID  `bson:"supplier_id,omitempty"      json:"supplier_id,omitempty"`
	SupplierName     string               `bson:"supplier_name"              json:"supplier_name"`
	SupplierPhone    string               `bson:"supplier_phone,omitempty"   json:"supplier_phone,omitempty"`
	SupplierEmail    string               `bson:"supplier_email,omitempty"   json:"supplier_email,omitempty"`
	ReceivedAt       time.Time            `bson:"received_at"                json:"received_at"`
	RawText          string               `bson:"raw_text,omitempty"         json:"raw_text,omitempty"`
	MediaURLs        []string             `bson:"media_urls,omitempty"       json:"media_urls,omitempty"`
	// IsQuotation indicates the LLM detected this reply is a price quotation (not just an acknowledgement).
	IsQuotation      bool                 `bson:"is_quotation"               json:"is_quotation"`
	// GeneralNotes holds quotation-wide conditions (validity, delivery, payment terms) that are not product-specific.
	GeneralNotes     string               `bson:"general_notes,omitempty"    json:"general_notes,omitempty"`
	Prices           []SupplierReplyPrice `bson:"prices,omitempty"           json:"prices,omitempty"`
	// pending | done | failed
	ExtractionStatus string               `bson:"extraction_status,omitempty" json:"extraction_status,omitempty"`
	ExtractionError  string               `bson:"extraction_error,omitempty"  json:"extraction_error,omitempty"`
	// Source: whatsapp | email | manual
	Source           string               `bson:"source,omitempty"           json:"source,omitempty"`
	// ProcurementMessageID links back to the ProcurementMessage that was labelled as this quotation.
	ProcurementMessageID   *primitive.ObjectID `bson:"procurement_message_id,omitempty"   json:"procurement_message_id,omitempty"`
	ProcurementMessageCode string              `bson:"procurement_message_code,omitempty" json:"procurement_message_code,omitempty"`
}

// RFQReceived stores every incoming message delivered to the Bot WhatsApp number.
type RFQReceived struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"    json:"id,omitempty"`
	StoreID    primitive.ObjectID `bson:"store_id"         json:"store_id"`
	// Auto-generated serial code (e.g. RFQ-0001)
	Code       string             `bson:"code,omitempty"   json:"code,omitempty"`
	ReceivedAt time.Time          `bson:"received_at"      json:"received_at"`
	// Source: whatsapp | email | manual
	Source     string             `bson:"source,omitempty" json:"source,omitempty"`
	FromPhone  string             `bson:"from_phone"       json:"from_phone"`
	FromName   string             `bson:"from_name,omitempty" json:"from_name,omitempty"`
	// WhatsApp message ID of the original buyer message — used to quote it when relaying supplier replies
	BuyerMsgID string             `bson:"buyer_msg_id,omitempty" json:"buyer_msg_id,omitempty"`
	// text | image | document | mixed
	MessageType string        `bson:"message_type"           json:"message_type"`
	TextContent string        `bson:"text_content,omitempty" json:"text_content,omitempty"`
	MediaURLs   []string      `bson:"media_urls,omitempty"   json:"media_urls,omitempty"`
	// Meta WABA media IDs (from Meta Cloud API webhooks — referenced for re-download)
	MetaMediaIDs []string     `bson:"meta_media_ids,omitempty" json:"meta_media_ids,omitempty"`
	Documents    []RFQDocument `bson:"documents,omitempty"   json:"documents,omitempty"`
	// LLM-identified product categories
	Categories []string `bson:"categories,omitempty" json:"categories,omitempty"`
	// LLM-extracted product line items
	Products []RFQProduct `bson:"products,omitempty" json:"products,omitempty"`
	// ExtractedText holds text extracted from attached documents (XLSX, CSV) for LLM context only.
	ExtractedText string `bson:"extracted_text,omitempty" json:"extracted_text,omitempty"`
	// Linked customer (created/found by LLM-extracted contact info)
	CustomerID              *primitive.ObjectID `bson:"customer_id,omitempty"               json:"customer_id,omitempty"`
	CustomerName            string              `bson:"customer_name,omitempty"             json:"customer_name,omitempty"`
	CustomerContactPerson   string              `bson:"customer_contact_person,omitempty"   json:"customer_contact_person,omitempty"`
	CustomerPhone           string              `bson:"customer_phone,omitempty"            json:"customer_phone,omitempty"`
	CustomerEmail           string              `bson:"customer_email,omitempty"            json:"customer_email,omitempty"`
	CustomerCompany         string              `bson:"customer_company,omitempty"          json:"customer_company,omitempty"`
	CustomerAddress         string              `bson:"customer_address,omitempty"          json:"customer_address,omitempty"`
	CustomerVATNo           string              `bson:"customer_vat_no,omitempty"           json:"customer_vat_no,omitempty"`
	CustomerCRNo            string              `bson:"customer_cr_no,omitempty"            json:"customer_cr_no,omitempty"`
	CustomerNationalAddress string              `bson:"customer_national_address,omitempty" json:"customer_national_address,omitempty"`
	CustomerCity            string              `bson:"customer_city,omitempty"             json:"customer_city,omitempty"`
	// received | processing | ready_to_send | forwarded | failed | ignored
	Status      string             `bson:"status"                  json:"status"`
	ProcessedAt *time.Time         `bson:"processed_at,omitempty"  json:"processed_at,omitempty"`
	ForwardedTo        []RFQForwardRecord   `bson:"forwarded_to,omitempty"  json:"forwarded_to,omitempty"`
	MatchedSupplierIDs []primitive.ObjectID `bson:"matched_supplier_ids,omitempty" json:"matched_supplier_ids,omitempty"`
	ErrorMsg    string             `bson:"error_msg,omitempty"     json:"error_msg,omitempty"`
	// BuyerRelays tracks each message the bot sends to the buyer relaying a supplier reply.
	BuyerRelays []BuyerRelayRecord `bson:"buyer_relays,omitempty" json:"buyer_relays,omitempty"`
	// SupplierReplies holds replies from each supplier (raw text + LLM-extracted prices).
	SupplierReplies []SupplierReply `bson:"supplier_replies,omitempty" json:"supplier_replies,omitempty"`
	// ActivityLogs is the ordered timeline of processing events for this RFQ.
	ActivityLogs []RFQActivityLog `bson:"activity_logs,omitempty" json:"activity_logs,omitempty"`
	// CustomerRFQID is the RFQ reference number from the customer's own RFQ document.
	CustomerRFQID string `bson:"customer_rfq_id,omitempty" json:"customer_rfq_id,omitempty"`
	// PreparedBy and AuthorizedBy are set when the RFQ is sent to suppliers via WABA template.
	PreparedBy   string `bson:"prepared_by,omitempty"   json:"prepared_by,omitempty"`
	AuthorizedBy string `bson:"authorized_by,omitempty" json:"authorized_by,omitempty"`
	// Link to the originating procurement message (email or WhatsApp inbox record).
	ProcurementMessageID   *primitive.ObjectID `bson:"procurement_message_id,omitempty"   json:"procurement_message_id,omitempty"`
	ProcurementMessageCode string              `bson:"procurement_message_code,omitempty" json:"procurement_message_code,omitempty"`
	// AttachmentURLs holds /cdn/ URLs of product files (PDF/image/CSV) attached during manual RFQ creation.
	// Shown in place of the products table in the RFQ preview/PDF.
	AttachmentURLs []string `bson:"attachment_urls,omitempty" json:"attachment_urls,omitempty"`
	// AdditionalAttachmentURLs holds /cdn/ URLs of additional detail files.
	// Shown below the products table in the RFQ preview/PDF.
	AdditionalAttachmentURLs  []string `bson:"additional_attachment_urls,omitempty" json:"additional_attachment_urls,omitempty"`
	AdditionalAttachmentFilenames []string `bson:"additional_attachment_filenames,omitempty" json:"additional_attachment_filenames,omitempty"`
	// GeneralInstructions holds LLM-extracted instructions that apply to the whole RFQ
	// (e.g. "provide datasheet, warranty, delivery terms") — not specific to any single product.
	GeneralInstructions string `bson:"general_instructions,omitempty" json:"general_instructions,omitempty"`
	// QuotationIDs / QuotationCodes link to customer quotations created from this RFQ's price comparison.
	QuotationIDs   []primitive.ObjectID `bson:"quotation_ids,omitempty"   json:"quotation_ids,omitempty"`
	QuotationCodes []string             `bson:"quotation_codes,omitempty" json:"quotation_codes,omitempty"`
}

func rfqReceivedCollection(storeID primitive.ObjectID) string {
	return "rfq_received"
}

// MakeRFQCode generates a serial code for the RFQ using the store's RFQReceivedSerialNumber settings.
func (rfq *RFQReceived) MakeRFQCode() error {
	store, err := FindStoreByID(&rfq.StoreID, bson.M{})
	if err != nil {
		return err
	}
	sn := store.RFQReceivedSerialNumber
	redisKey := rfq.StoreID.Hex() + "_rfq_received_counter"

	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, _ := store.GetCountByCollection("rfq_received")
		start := sn.StartFromCount
		if start == 0 {
			start = 1
		}
		db.RedisClient.Set(redisKey, start+count-1, 0)
	}
	n, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	prefix := strings.TrimSpace(sn.Prefix)
	padding := sn.PaddingCount
	if padding == 0 {
		padding = 4
	}
	if prefix == "" {
		rfq.Code = fmt.Sprintf("%0*d", padding, n)
	} else {
		rfq.Code = fmt.Sprintf("%s-%0*d", strings.ToUpper(prefix), padding, n)
	}
	return nil
}

func CreateRFQReceived(rfq *RFQReceived) error {
	if rfq.ID.IsZero() {
		rfq.ID = primitive.NewObjectID()
	}
	now := time.Now()
	rfq.ReceivedAt = now
	if rfq.Code == "" {
		rfq.MakeRFQCode() // best-effort; errors don't block creation
	}

	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(rfq.StoreID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.InsertOne(ctx, rfq)
	return err
}

// AppendRFQLog appends a single activity-log entry to the RFQ document.
// Safe to call from a goroutine; errors are silently discarded so logging
// never blocks or fails the main processing path.
func AppendRFQLog(storeID, rfqID primitive.ObjectID, entry RFQActivityLog) {
	if entry.ID.IsZero() {
		entry.ID = primitive.NewObjectID()
	}
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	col.UpdateOne(ctx,
		bson.M{"_id": rfqID, "store_id": storeID},
		bson.M{"$push": bson.M{"activity_logs": entry}},
	)
}

func UpdateRFQReceived(rfq *RFQReceived) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(rfq.StoreID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.ReplaceOne(ctx, bson.M{"_id": rfq.ID}, rfq)
	return err
}

// SetRFQMatchedSupplierIDs updates only the matched_supplier_ids field, preserving activity_logs and all other fields.
func SetRFQMatchedSupplierIDs(storeID, rfqID primitive.ObjectID, ids []primitive.ObjectID) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(
		ctx,
		bson.M{"_id": rfqID, "store_id": storeID},
		bson.M{"$set": bson.M{"matched_supplier_ids": ids}},
	)
	return err
}

// SetRFQCategories updates only the categories field via $set, preserving all other fields
// (including activity_logs appended after the rfq was first saved).
func SetRFQCategories(storeID, rfqID primitive.ObjectID, categories []string) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": rfqID, "store_id": storeID},
		bson.M{"$set": bson.M{"categories": categories}},
	)
	return err
}

type RFQReceivedListResult struct {
	Items      []RFQReceived `json:"items"`
	TotalCount int64         `json:"total_count"`
}

// EnsureRFQReceivedIndexes creates text and supporting indexes on the rfq_received collection.
// Call once at startup (idempotent).
func EnsureRFQReceivedIndexes() {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "code", Value: "text"},
				{Key: "from_phone", Value: "text"},
				{Key: "from_name", Value: "text"},
				{Key: "text_content", Value: "text"},
				{Key: "categories", Value: "text"},
				{Key: "customer_name", Value: "text"},
			},
			Options: options.Index().SetName("rfq_received_text_idx"),
		},
		{Keys: bson.D{{Key: "store_id", Value: 1}, {Key: "received_at", Value: -1}}},
		{Keys: bson.D{{Key: "store_id", Value: 1}, {Key: "status", Value: 1}}},
	})
}

func ListRFQReceived(storeID primitive.ObjectID, page, limit int64, statusFilter, search string) (*RFQReceivedListResult, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"store_id": storeID}
	if statusFilter != "" {
		filter["status"] = statusFilter
	}
	if search != "" {
		filter["$or"] = bson.A{
			bson.M{"code": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"from_phone": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"from_name": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"text_content": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"categories": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"customer_name": bson.M{"$regex": search, "$options": "i"}},
		}
	}

	total, _ := col.CountDocuments(ctx, filter)

	skip := (page - 1) * limit
	// Exclude heavy inline-blob fields (base64 attachments, raw log arrays) from list queries.
	// These are only needed in the detail/preview views, not the index table.
	projection := bson.M{
		"additional_attachment_filenames": 0,
		"activity_logs":                   0,
		"supplier_replies":                0,
		"buyer_relays":                    0,
		"extracted_text":                  0,
		"meta_media_ids":                  0,
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "received_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit).
		SetProjection(projection)

	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []RFQReceived{}
	}
	return &RFQReceivedListResult{Items: items, TotalCount: total}, nil
}

// BuyerRelayRecord records a message the bot sent to the buyer relaying a supplier reply.
// Storing the WhatsApp message ID lets us detect when the buyer replies to that relay
// (via contextInfo.stanzaId) and route the follow-up to the correct supplier.
type BuyerRelayRecord struct {
	MsgID         string    `bson:"msg_id" json:"msg_id"`
	SupplierPhone string    `bson:"supplier_phone" json:"supplier_phone"`
	SentAt        time.Time `bson:"sent_at" json:"sent_at"`
}

// AddBuyerRelayToRFQ appends a BuyerRelayRecord to the rfq_received document.
func AddBuyerRelayToRFQ(storeID, rfqID primitive.ObjectID, rec BuyerRelayRecord) error {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": rfqID, "store_id": storeID},
		bson.M{"$push": bson.M{"buyer_relays": rec}},
	)
	return err
}

// FindRFQByBuyerRelayMsgID returns the RFQ and supplier phone associated with a relay
// message that the bot sent to the buyer. relayMsgID comes from contextInfo.stanzaId
// when the buyer replies to that relay.
func FindRFQByBuyerRelayMsgID(storeID primitive.ObjectID, relayMsgID string) (*RFQReceived, string, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var rfq RFQReceived
	err := col.FindOne(ctx, bson.M{
		"store_id":              storeID,
		"buyer_relays.msg_id":   relayMsgID,
	}).Decode(&rfq)
	if err != nil {
		return nil, "", err
	}
	for _, r := range rfq.BuyerRelays {
		if r.MsgID == relayMsgID {
			return &rfq, r.SupplierPhone, nil
		}
	}
	return nil, "", mongo.ErrNoDocuments
}

// FindLatestRFQBySupplierPhone returns the most recent RFQ forwarded to supplierPhone.
// Used to route supplier replies back to the original buyer.
func FindLatestRFQBySupplierPhone(storeID primitive.ObjectID, supplierPhone string) (*RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := options.FindOne().SetSort(bson.D{{Key: "received_at", Value: -1}})
	var rfq RFQReceived
	err := col.FindOne(ctx, bson.M{
		"store_id":           storeID,
		"forwarded_to.phone": supplierPhone,
		"forwarded_to.status": "sent",
	}, opts).Decode(&rfq)
	if err != nil {
		return nil, err
	}
	return &rfq, nil
}

func FindRFQReceivedByID(id, storeID primitive.ObjectID) (*RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var rfq RFQReceived
	err := col.FindOne(ctx, bson.M{"_id": id, "store_id": storeID}).Decode(&rfq)
	if err != nil {
		return nil, err
	}
	return &rfq, nil
}

// AddSupplierReplyToRFQ appends a SupplierReply to the rfq_received document.
func AddSupplierReplyToRFQ(storeID, rfqID primitive.ObjectID, reply SupplierReply) error {
	if reply.ID.IsZero() {
		reply.ID = primitive.NewObjectID()
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": rfqID, "store_id": storeID},
		bson.M{"$push": bson.M{"supplier_replies": reply}},
	)
	return err
}

// FindRFQByCode returns the RFQ with the given code for a store.
func FindRFQByCode(storeID primitive.ObjectID, code string) (*RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var rfq RFQReceived
	err := col.FindOne(ctx, bson.M{"store_id": storeID, "code": bson.M{"$regex": "^" + code + "$", "$options": "i"}}).Decode(&rfq)
	if err != nil {
		return nil, err
	}
	return &rfq, nil
}

// FindRFQsByPhones returns RFQs that have any of the given phones in their forwarded_to list
// or as customer_phone. Used to map unread WhatsApp threads back to RFQs.
func FindRFQsByPhones(storeID primitive.ObjectID, phones []string) ([]RFQReceived, error) {
	if len(phones) == 0 {
		return nil, nil
	}
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build $in variants: with and without leading "+" for each phone
	variants := make(bson.A, 0, len(phones)*2)
	for _, p := range phones {
		variants = append(variants, p)
		if !strings.HasPrefix(p, "+") {
			variants = append(variants, "+"+p)
		} else {
			variants = append(variants, strings.TrimPrefix(p, "+"))
		}
	}

	filter := bson.M{
		"store_id": storeID,
		"$or": bson.A{
			bson.M{"forwarded_to.phone": bson.M{"$in": variants}},
			bson.M{"customer_phone": bson.M{"$in": variants}},
		},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "received_at", Value: -1}}).
		SetLimit(200).
		SetProjection(bson.M{
			"_id": 1, "code": 1,
			"customer_name": 1, "customer_phone": 1,
			"forwarded_to": 1,
		})
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindRecentRFQsByPhone returns up to limit RFQs from a given WhatsApp phone number within
// the lookback window, newest first. Used to detect reminder/follow-up messages.
func FindRecentRFQsByPhone(storeID primitive.ObjectID, phone string, lookback time.Duration, limit int64) ([]RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	since := time.Now().Add(-lookback)
	filter := bson.M{
		"store_id":    storeID,
		"source":      "whatsapp",
		"received_at": bson.M{"$gte": since},
		"from_phone":  bson.M{"$regex": phone, "$options": "i"},
	}
	opts := options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(limit)
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindRecentRFQsByEmail returns up to limit RFQs from a given email address within the
// lookback window, newest first. Used to detect reminder/follow-up emails.
func FindRecentRFQsByEmail(storeID primitive.ObjectID, email string, lookback time.Duration, limit int64) ([]RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(storeID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	since := time.Now().Add(-lookback)
	filter := bson.M{
		"store_id":    storeID,
		"source":      "email",
		"received_at": bson.M{"$gte": since},
		"$or": bson.A{
			bson.M{"from_phone": bson.M{"$regex": email, "$options": "i"}},
			bson.M{"customer_email": bson.M{"$regex": email, "$options": "i"}},
		},
	}
	opts := options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(limit)
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// DeleteSupplierReplyFromRFQ removes a SupplierReply from the rfq_received document by its ID.
func DeleteSupplierReplyFromRFQ(storeID, rfqID, replyID primitive.ObjectID) error {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": rfqID, "store_id": storeID},
		bson.M{"$pull": bson.M{"supplier_replies": bson.M{"_id": replyID}}},
	)
	return err
}

// UpdateSupplierReplyPrices sets the extracted prices and status on one SupplierReply.
func UpdateSupplierReplyPrices(storeID, rfqID, replyID primitive.ObjectID, prices []SupplierReplyPrice, isQuotation bool, status, errMsg string) error {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": rfqID, "store_id": storeID, "supplier_replies._id": replyID},
		bson.M{"$set": bson.M{
			"supplier_replies.$.prices":            prices,
			"supplier_replies.$.is_quotation":      isQuotation,
			"supplier_replies.$.extraction_status": status,
			"supplier_replies.$.extraction_error":  errMsg,
		}},
	)
	return err
}

// DeleteAllRFQReceived hard-deletes every RFQ received record for a store.
// Returns the number of documents deleted.
func DeleteAllRFQReceived(storeID primitive.ObjectID) (int64, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := col.DeleteMany(ctx, bson.M{"store_id": storeID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// DeleteRFQReceived hard-deletes a single RFQ received record and returns it so
// callers can unlink any connected procurement message.
func DeleteRFQReceived(storeID, rfqID primitive.ObjectID) (*RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var rfq RFQReceived
	err := col.FindOneAndDelete(ctx, bson.M{"_id": rfqID, "store_id": storeID}).Decode(&rfq)
	if err != nil {
		return nil, err
	}
	return &rfq, nil
}

// AddQuotationLinkToRFQ appends a quotation ID+code to the rfq_received document.
func AddQuotationLinkToRFQ(storeID, rfqID, quotationID primitive.ObjectID, quotationCode string) error {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": rfqID, "store_id": storeID},
		bson.M{
			"$addToSet": bson.M{
				"quotation_ids":   quotationID,
				"quotation_codes": quotationCode,
			},
		},
	)
	return err
}

// phoneCoreSuffix extracts the significant local digits from a phone number so that
// international (966501971075) and local (0501971075) formats match each other.
// It strips non-digits, removes a leading zero if present, then returns the last 9 digits.
func phoneCoreSuffix(phone string) string {
	digits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}
	if digits == "" {
		return phone
	}
	// Strip leading zero (local format: 0501971075 → 501971075)
	if len(digits) > 1 && digits[0] == '0' {
		digits = digits[1:]
	}
	// Use last 9 digits to tolerate different country-code lengths
	if len(digits) > 9 {
		digits = digits[len(digits)-9:]
	}
	return digits
}

// FindRFQsForwardedToPhone returns RFQs that were forwarded to the given supplier phone
// within the lookback window, newest first. Used for matching supplier quotation replies.
// Phone matching is format-agnostic: both international (966501971075) and local (0501971075)
// formats resolve to the same 9-digit core and match each other.
func FindRFQsByCustomerID(storeID, customerID primitive.ObjectID, limit int64) ([]RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{
		"store_id":    storeID,
		"customer_id": customerID,
	}
	opts := options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(limit)
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindRFQsByCustomerEmail returns RFQs received from a customer identified by email.
func FindRFQsByCustomerEmail(storeID primitive.ObjectID, email string, limit int64) ([]RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{
		"store_id":       storeID,
		"customer_email": bson.M{"$regex": strings.TrimSpace(email), "$options": "i"},
	}
	opts := options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(limit)
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// FindRFQsBySupplierEmail returns RFQs where a supplier with the given email has replied.
func FindRFQsBySupplierEmail(storeID primitive.ObjectID, email string, limit int64) ([]RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	emailLower := strings.ToLower(strings.TrimSpace(email))
	filter := bson.M{
		"store_id": storeID,
		"supplier_replies": bson.M{"$elemMatch": bson.M{
			"supplier_email": bson.M{"$regex": emailLower, "$options": "i"},
		}},
	}
	opts := options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(limit)
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func FindRFQsForwardedToPhone(storeID primitive.ObjectID, phone string, lookback time.Duration, limit int64) ([]RFQReceived, error) {
	return FindRFQsForwardedToSupplier(storeID, phone, "", primitive.NilObjectID, nil, limit)
}

// FindRFQsForwardedToSupplier returns RFQs forwarded to a supplier identified by any
// combination of supplier_id, phones (primary + phone2), and/or supplier name.
// Each non-empty argument adds an $or clause so RFQs stored under any alias are returned.
// lookback is no longer applied here — callers pass a limit instead (no date cutoff).
func FindRFQsForwardedToSupplier(storeID primitive.ObjectID, phone, supplierName string, supplierID primitive.ObjectID, extraPhones []string, limit int64) ([]RFQReceived, error) {
	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Build $or conditions for the forwarded_to $elemMatch.
	var orClauses bson.A

	// Most reliable: match by supplier ObjectID stored in forwarded_to.supplier_id.
	if !supplierID.IsZero() {
		orClauses = append(orClauses, bson.M{
			"forwarded_to": bson.M{"$elemMatch": bson.M{
				"supplier_id": supplierID,
			}},
		})
	}

	seen := map[string]bool{}
	addPhone := func(p string) {
		if p == "" {
			return
		}
		pat := phoneCoreSuffix(p)
		if seen[pat] {
			return
		}
		seen[pat] = true
		orClauses = append(orClauses, bson.M{
			"forwarded_to": bson.M{"$elemMatch": bson.M{
				"phone": bson.M{"$regex": pat, "$options": "i"},
			}},
		})
	}
	addPhone(phone)
	for _, p := range extraPhones {
		addPhone(p)
	}
	if supplierName != "" {
		orClauses = append(orClauses, bson.M{
			"forwarded_to": bson.M{"$elemMatch": bson.M{
				"supplier_name": bson.M{"$regex": "^" + regexp.QuoteMeta(supplierName) + "$", "$options": "i"},
			}},
		})
	}
	if len(orClauses) == 0 {
		return []RFQReceived{}, nil
	}

	filter := bson.M{
		"store_id": storeID,
		"$or":      orClauses,
	}
	opts := options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(limit)
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []RFQReceived
	if err := cur.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}
