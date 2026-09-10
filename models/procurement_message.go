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
	Attachments []ProcurementAttachment `bson:"attachments,omitempty" json:"attachments,omitempty"`
	// Tracking
	ExternalID     string              `bson:"external_id,omitempty" json:"external_id,omitempty"`
	Read           bool                `bson:"read" json:"read"`
	ProcessedAsRFQ bool                `bson:"processed_as_rfq" json:"processed_as_rfq"`
	RFQReceivedID  *primitive.ObjectID `bson:"rfq_received_id,omitempty" json:"rfq_received_id,omitempty"`
	MessageDate    *time.Time          `bson:"message_date,omitempty" json:"message_date,omitempty"`
	CreatedAt      time.Time           `bson:"created_at" json:"created_at"`
}

func procurementMessageCol() *mongo.Collection {
	return db.Client("").Database(db.GetPosDB()).Collection("procurement_messages")
}

// SaveProcurementMessage inserts a new message record.
func SaveProcurementMessage(msg *ProcurementMessage) error {
	if msg.ID.IsZero() {
		msg.ID = primitive.NewObjectID()
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
func ListProcurementMessages(storeID primitive.ObjectID, msgType, direction, search string, page, limit int) ([]ProcurementMessage, int64, error) {
	filter := bson.M{"store_id": storeID}
	if msgType != "" {
		filter["type"] = msgType
	}
	if direction != "" {
		filter["direction"] = direction
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
		SetSort(bson.M{"created_at": -1}).
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

// DeleteProcurementMessage deletes a single message.
func DeleteProcurementMessage(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().DeleteOne(ctx, bson.M{"_id": id})
	return err
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
