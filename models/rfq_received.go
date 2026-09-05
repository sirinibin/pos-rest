package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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

// RFQReceived stores every incoming message delivered to the Bot WhatsApp number.
type RFQReceived struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	StoreID     primitive.ObjectID  `bson:"store_id" json:"store_id"`
	ReceivedAt  time.Time           `bson:"received_at" json:"received_at"`
	FromPhone   string              `bson:"from_phone" json:"from_phone"`
	FromName    string              `bson:"from_name,omitempty" json:"from_name,omitempty"`
	// WhatsApp message ID of the original buyer message — used to quote it when relaying supplier replies
	BuyerMsgID  string              `bson:"buyer_msg_id,omitempty" json:"buyer_msg_id,omitempty"`
	// text | image | document | mixed
	MessageType string        `bson:"message_type" json:"message_type"`
	TextContent string        `bson:"text_content,omitempty" json:"text_content,omitempty"`
	MediaURLs   []string      `bson:"media_urls,omitempty" json:"media_urls,omitempty"`
	Documents   []RFQDocument `bson:"documents,omitempty" json:"documents,omitempty"`
	// LLM-identified product categories
	Categories []string `bson:"categories,omitempty" json:"categories,omitempty"`
	// ExtractedText holds text extracted from attached documents (XLSX, CSV) for LLM context only.
	// It is NOT included in messages sent to suppliers.
	ExtractedText string `bson:"extracted_text,omitempty" json:"extracted_text,omitempty"`
	// received | processing | forwarded | failed
	Status      string             `bson:"status" json:"status"`
	ProcessedAt *time.Time         `bson:"processed_at,omitempty" json:"processed_at,omitempty"`
	ForwardedTo []RFQForwardRecord `bson:"forwarded_to,omitempty" json:"forwarded_to,omitempty"`
	ErrorMsg    string             `bson:"error_msg,omitempty" json:"error_msg,omitempty"`
	// BuyerRelays tracks each message the bot sends to the buyer relaying a supplier reply.
	// The MsgID is used to detect when the buyer replies to a relay (vs. sending a new RFQ).
	BuyerRelays []BuyerRelayRecord `bson:"buyer_relays,omitempty" json:"buyer_relays,omitempty"`
}

func rfqReceivedCollection(storeID primitive.ObjectID) string {
	return "rfq_received"
}

func CreateRFQReceived(rfq *RFQReceived) error {
	if rfq.ID.IsZero() {
		rfq.ID = primitive.NewObjectID()
	}
	now := time.Now()
	rfq.ReceivedAt = now

	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(rfq.StoreID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.InsertOne(ctx, rfq)
	return err
}

func UpdateRFQReceived(rfq *RFQReceived) error {
	col := db.Client("").Database(db.GetPosDB()).Collection(rfqReceivedCollection(rfq.StoreID))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.ReplaceOne(ctx, bson.M{"_id": rfq.ID}, rfq)
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
				{Key: "from_phone", Value: "text"},
				{Key: "from_name", Value: "text"},
				{Key: "text_content", Value: "text"},
				{Key: "categories", Value: "text"},
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
			bson.M{"from_phone": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"from_name": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"text_content": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"categories": bson.M{"$regex": search, "$options": "i"}},
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
