package models

import (
	"context"
	"fmt"
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
	PartNo   string  `bson:"part_no,omitempty"   json:"part_no,omitempty"`
	Name     string  `bson:"name"                json:"name"`
	Quantity float64 `bson:"quantity,omitempty"  json:"quantity,omitempty"`
	Unit     string  `bson:"unit,omitempty"      json:"unit,omitempty"`
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
	Prices           []SupplierReplyPrice `bson:"prices,omitempty"           json:"prices,omitempty"`
	// pending | done | failed
	ExtractionStatus string               `bson:"extraction_status,omitempty" json:"extraction_status,omitempty"`
	ExtractionError  string               `bson:"extraction_error,omitempty"  json:"extraction_error,omitempty"`
	// Source: whatsapp | email | manual
	Source           string               `bson:"source,omitempty"           json:"source,omitempty"`
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
	CustomerID      *primitive.ObjectID `bson:"customer_id,omitempty"      json:"customer_id,omitempty"`
	CustomerName    string              `bson:"customer_name,omitempty"    json:"customer_name,omitempty"`
	CustomerPhone   string              `bson:"customer_phone,omitempty"   json:"customer_phone,omitempty"`
	CustomerEmail   string              `bson:"customer_email,omitempty"   json:"customer_email,omitempty"`
	CustomerCompany string              `bson:"customer_company,omitempty" json:"customer_company,omitempty"`
	CustomerAddress string              `bson:"customer_address,omitempty" json:"customer_address,omitempty"`
	// received | processing | ready_to_send | forwarded | failed | ignored
	Status      string             `bson:"status"                  json:"status"`
	ProcessedAt *time.Time         `bson:"processed_at,omitempty"  json:"processed_at,omitempty"`
	ForwardedTo []RFQForwardRecord `bson:"forwarded_to,omitempty"  json:"forwarded_to,omitempty"`
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
	opts := options.Find().
		SetSort(bson.D{{Key: "received_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(limit)

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
