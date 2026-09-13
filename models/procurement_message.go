package models

import (
	"context"
	"fmt"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ProcurementAttachment represents a file attached to an email or WhatsApp message.
type ProcurementAttachment struct {
	Filename    string `bson:"filename" json:"filename"`
	ContentType string `bson:"content_type" json:"content_type"`
	Size        int64  `bson:"size" json:"size"`
	URL         string `bson:"url,omitempty" json:"url,omitempty"`
}

// ProcurementMessage is a single inbound or outbound email or WhatsApp message
// recorded by the procurement module for audit and review.
type ProcurementMessage struct {
	ID        primitive.ObjectID  `bson:"_id" json:"id"`
	StoreID   primitive.ObjectID  `bson:"store_id" json:"store_id"`
	Type      string              `bson:"type" json:"type"`      // "email" | "whatsapp"
	Direction string              `bson:"direction" json:"direction"` // "in" | "out"
	Provider  string              `bson:"provider" json:"provider"`
	From      string              `bson:"from" json:"from"`
	To        []string            `bson:"to" json:"to"`
	// Email-specific
	Subject  string `bson:"subject,omitempty" json:"subject,omitempty"`
	BodyText string `bson:"body_text,omitempty" json:"body_text,omitempty"`
	BodyHTML string `bson:"body_html,omitempty" json:"body_html,omitempty"`
	// WhatsApp-specific
	WAMessageType     string `bson:"wa_message_type,omitempty" json:"wa_message_type,omitempty"` // text|image|document|audio|video
	WABAPhoneNumberID string `bson:"waba_phone_number_id,omitempty" json:"waba_phone_number_id,omitempty"`
	// Attachments
	Attachments      []ProcurementAttachment `bson:"attachments,omitempty" json:"attachments,omitempty"`
	// AttachmentMissing is true when the email body mentions attachments but none
	// were received/downloaded. RFQ creation is blocked for such messages.
	AttachmentMissing bool `bson:"attachment_missing,omitempty" json:"attachment_missing,omitempty"`
	// Tracking
	ExternalID     string              `bson:"external_id,omitempty" json:"external_id,omitempty"`
	Read           bool                `bson:"read" json:"read"`
	ProcessedAsRFQ bool                `bson:"processed_as_rfq" json:"processed_as_rfq"`
	RFQReceivedID  *primitive.ObjectID `bson:"rfq_received_id,omitempty" json:"rfq_received_id,omitempty"`
	MessageDate    *time.Time          `bson:"message_date,omitempty" json:"message_date,omitempty"`
	// Auto-generated human-readable code, e.g. EM-000001 or WA-000001
	Code           string              `bson:"code,omitempty" json:"code,omitempty"`
	CreatedAt      time.Time           `bson:"created_at" json:"created_at"`
}

// MakeProcurementMessageCode generates a serial code for the message.
// msgType should be "email" or "whatsapp"; storeID is the owning store.
func MakeProcurementMessageCode(storeID primitive.ObjectID, msgType string) string {
	prefix := "EM"
	redisKey := storeID.Hex() + "_pm_em_counter"
	if msgType == "whatsapp" {
		prefix = "WA"
		redisKey = storeID.Hex() + "_pm_wa_counter"
	}
	n, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s-%06d", prefix, n)
}

func procurementMessageCol() *mongo.Collection {
	return db.Client("").Database(db.GetPosDB()).Collection("procurement_messages")
}

// SaveProcurementMessage inserts a new message record.
func SaveProcurementMessage(msg *ProcurementMessage) error {
	if msg.ID.IsZero() {
		msg.ID = primitive.NewObjectID()
	}
	if msg.Code == "" {
		msg.Code = MakeProcurementMessageCode(msg.StoreID, msg.Type)
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.To == nil {
		msg.To = []string{}
	}
	if msg.Attachments == nil {
		msg.Attachments = []ProcurementAttachment{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().InsertOne(ctx, msg)
	return err
}

// ListProcurementMessages returns paginated messages for a store.
// rfqFilter: "" = all, "yes" = processed_as_rfq=true, "no" = processed_as_rfq=false/missing.
func ListProcurementMessages(storeID primitive.ObjectID, msgType, direction, search, rfqFilter string, page, limit int) ([]ProcurementMessage, int64, error) {
	filter := bson.M{"store_id": storeID}
	if msgType != "" {
		filter["type"] = msgType
	}
	if direction != "" {
		filter["direction"] = direction
	}
	if rfqFilter == "yes" {
		filter["processed_as_rfq"] = true
	} else if rfqFilter == "no" {
		filter["$or"] = []bson.M{
			{"processed_as_rfq": false},
			{"processed_as_rfq": bson.M{"$exists": false}},
		}
	}
	if search != "" {
		filter["$or"] = []bson.M{
			{"from": bson.M{"$regex": search, "$options": "i"}},
			{"subject": bson.M{"$regex": search, "$options": "i"}},
			{"body_text": bson.M{"$regex": search, "$options": "i"}},
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, _ := procurementMessageCol().CountDocuments(ctx, filter)

	skip := int64((page - 1) * limit)
	opts := options.Find().
		SetSort(bson.D{bson.E{Key: "message_date", Value: -1}, bson.E{Key: "created_at", Value: -1}}).
		SetSkip(skip).
		SetLimit(int64(limit))

	cur, err := procurementMessageCol().Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	var msgs []ProcurementMessage
	if err := cur.All(ctx, &msgs); err != nil {
		return nil, 0, err
	}
	return msgs, total, nil
}

// LinkProcurementMessageToRFQ marks a procurement message as processed and links it to the RFQ.
func LinkProcurementMessageToRFQ(msgID primitive.ObjectID, rfqID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": msgID},
		bson.M{"$set": bson.M{
			"processed_as_rfq": true,
			"rfq_received_id":  rfqID,
		}},
	)
	return err
}

// ProcurementMessageExternalIDExists returns true if a message with the given
// external_id already exists for this store, used to skip duplicate ingestion.
func ProcurementMessageExternalIDExists(storeID primitive.ObjectID, externalID string) bool {
	if externalID == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	n, _ := procurementMessageCol().CountDocuments(ctx, bson.M{
		"store_id":    storeID,
		"external_id": externalID,
	})
	return n > 0
}

// GetProcurementMessage returns a single message by ID.
func GetProcurementMessage(id primitive.ObjectID) (*ProcurementMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var msg ProcurementMessage
	err := procurementMessageCol().FindOne(ctx, bson.M{"_id": id}).Decode(&msg)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// UpdateProcurementMessageAttachments updates a message's attachments and clears the attachment_missing flag.
func UpdateProcurementMessageAttachments(id primitive.ObjectID, attachments []ProcurementAttachment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"attachments":        attachments,
			"attachment_missing": false,
		}},
	)
	return err
}

// DeleteProcurementMessage deletes a single message.
func DeleteProcurementMessage(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// FetchMessagesForCleanup returns only the _id and attachments fields of messages
// matching the filter — used before bulk deletion to find disk files to remove.
func FetchMessagesForCleanup(filter bson.M) ([]ProcurementMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cursor, err := procurementMessageCol().Find(ctx, filter,
		options.Find().SetProjection(bson.M{"_id": 1, "attachments": 1}))
	if err != nil {
		return nil, err
	}
	var msgs []ProcurementMessage
	_ = cursor.All(ctx, &msgs)
	return msgs, nil
}

// MarkProcurementMessageRead marks a message as read.
func MarkProcurementMessageRead(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"read": true}},
	)
	return err
}

// DeleteAllProcurementMessages deletes ALL messages for a store (type-filtered if msgType != "").
func DeleteAllProcurementMessages(storeID primitive.ObjectID, msgType string) (int64, error) {
	filter := bson.M{"store_id": storeID}
	if msgType != "" {
		filter["type"] = msgType
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := procurementMessageCol().DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// DeleteOldProcurementMessages deletes messages older than days for a store.
// days=0 means disabled (no deletion).
func DeleteOldProcurementMessages(storeID primitive.ObjectID, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := procurementMessageCol().DeleteMany(ctx, bson.M{
		"store_id":   storeID,
		"created_at": bson.M{"$lt": cutoff},
	})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}

// EnsureProcurementMessageIndexes creates the necessary MongoDB indexes.
func EnsureProcurementMessageIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	col := procurementMessageCol()
	col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{
			bson.E{Key: "store_id", Value: 1},
			bson.E{Key: "type", Value: 1},
			bson.E{Key: "created_at", Value: -1},
		}},
		{Keys: bson.D{
			bson.E{Key: "store_id", Value: 1},
			bson.E{Key: "created_at", Value: 1},
		}},
	})
}
