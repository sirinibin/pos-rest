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
	// RFC 2822 Message-ID header of the original email, used for reply threading (In-Reply-To / References).
	EmailMessageID string              `bson:"email_message_id,omitempty" json:"email_message_id,omitempty"`
	Read           bool                `bson:"read" json:"read"`
	ProcessedAsRFQ  bool                `bson:"processed_as_rfq" json:"processed_as_rfq"`
	RFQReceivedID   *primitive.ObjectID `bson:"rfq_received_id,omitempty" json:"rfq_received_id,omitempty"`
	RFQReceivedCode string              `bson:"rfq_received_code,omitempty" json:"rfq_received_code,omitempty"`
	// IsSupplierQuotation is true when the user (or system) labels this message as a supplier quotation.
	IsSupplierQuotation    bool                `bson:"is_supplier_quotation" json:"is_supplier_quotation"`
	// LinkedRFQReceivedID points to the RFQReceived document this quotation was matched to.
	LinkedRFQReceivedID   *primitive.ObjectID `bson:"linked_rfq_received_id,omitempty"   json:"linked_rfq_received_id,omitempty"`
	LinkedRFQReceivedCode string              `bson:"linked_rfq_received_code,omitempty" json:"linked_rfq_received_code,omitempty"`
	MessageDate    *time.Time          `bson:"message_date,omitempty" json:"message_date,omitempty"`
	// Auto-generated human-readable code, e.g. EM-000001 or WA-000001
	Code           string              `bson:"code,omitempty" json:"code,omitempty"`
	// SenderName / SenderType are resolved by looking up the From phone number
	// against RFQ suppliers and customers for this store.
	SenderName string `bson:"sender_name,omitempty" json:"sender_name,omitempty"`
	SenderType string `bson:"sender_type,omitempty" json:"sender_type,omitempty"` // "supplier" | "customer" | ""
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
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
	// Resolve sender identity for inbound messages
	if msg.Direction == "in" && msg.From != "" && msg.SenderName == "" {
		msg.SenderName, msg.SenderType = ResolveProcurementSender(msg.StoreID, msg.From)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().InsertOne(ctx, msg)
	return err
}

// ListProcurementMessages returns paginated messages for a store.
// rfqFilter: "" = all, "yes" = processed_as_rfq=true, "no" = processed_as_rfq=false/missing,
// "quotation" = is_supplier_quotation=true, "other" = not RFQ and not quotation.
func ListProcurementMessages(storeID primitive.ObjectID, msgType, direction, search, rfqFilter string, page, limit int, hasAttachments bool) ([]ProcurementMessage, int64, error) {
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
	} else if rfqFilter == "quotation" {
		filter["is_supplier_quotation"] = true
	} else if rfqFilter == "other" {
		filter["$and"] = []bson.M{
			{"$or": []bson.M{{"processed_as_rfq": false}, {"processed_as_rfq": bson.M{"$exists": false}}}},
			{"$or": []bson.M{{"is_supplier_quotation": bson.M{"$ne": true}}, {"is_supplier_quotation": bson.M{"$exists": false}}}},
		}
	}
	if search != "" {
		filter["$or"] = []bson.M{
			{"from": bson.M{"$regex": search, "$options": "i"}},
			{"subject": bson.M{"$regex": search, "$options": "i"}},
			{"body_text": bson.M{"$regex": search, "$options": "i"}},
		}
	}
	if hasAttachments {
		filter["attachments"] = bson.M{"$exists": true, "$not": bson.M{"$size": 0}}
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
func LinkProcurementMessageToRFQ(msgID primitive.ObjectID, rfqID primitive.ObjectID, rfqCode ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	update := bson.M{
		"processed_as_rfq": true,
		"rfq_received_id":  rfqID,
	}
	if len(rfqCode) > 0 && rfqCode[0] != "" {
		update["rfq_received_code"] = rfqCode[0]
	}
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": msgID},
		bson.M{"$set": update},
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

// IsKnownEmailContact returns true if this email address is already in our procurement
// thread (i.e., we have previously sent an email to them). Used as a fallback when the
// In-Reply-To header is unavailable — if the sender is already a contact, their reply
// bypasses keyword/LLM filters.
func IsKnownEmailContact(storeID primitive.ObjectID, email string) bool {
	if email == "" {
		return false
	}
	email = strings.ToLower(strings.TrimSpace(email))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Check if this email appears as a recipient in any of our outbound messages
	// OR as a sender in any of our inbound messages.
	n, _ := procurementMessageCol().CountDocuments(ctx, bson.M{
		"store_id": storeID,
		"type":     "email",
		"$or": bson.A{
			bson.M{"direction": "out", "to": bson.M{"$elemMatch": bson.M{"$regex": email, "$options": "i"}}},
			bson.M{"direction": "in", "from": bson.M{"$regex": email, "$options": "i"}},
		},
	})
	return n > 0
}

// IsReplyToOurMessage returns true if inReplyTo matches the email_message_id of any
// message we have sent (direction=out) for this store. Used to bypass keyword/LLM
// filters for legitimate customer replies to our own outbound emails.
func IsReplyToOurMessage(storeID primitive.ObjectID, inReplyTo string) bool {
	if inReplyTo == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	n, _ := procurementMessageCol().CountDocuments(ctx, bson.M{
		"store_id":         storeID,
		"direction":        "out",
		"email_message_id": inReplyTo,
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

// LinkMessageAsQuotation marks a procurement message as a supplier quotation and links it to an RFQ.
func LinkMessageAsQuotation(msgID primitive.ObjectID, rfqID *primitive.ObjectID, rfqCode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	update := bson.M{
		"is_supplier_quotation": true,
	}
	if rfqID != nil {
		update["linked_rfq_received_id"] = rfqID
	}
	if rfqCode != "" {
		update["linked_rfq_received_code"] = rfqCode
	}
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": msgID},
		bson.M{"$set": update},
	)
	return err
}

// UnlinkMessageAsQuotation removes the quotation label and RFQ link from a procurement message.
// UnlinkRFQFromProcurementMessage clears the rfq_received_id / rfq_received_code fields
// from a procurement message when the linked RFQ is deleted.
func UnlinkRFQFromProcurementMessage(msgID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": msgID},
		bson.M{
			"$set":   bson.M{"processed_as_rfq": false},
			"$unset": bson.M{"rfq_received_id": "", "rfq_received_code": ""},
		},
	)
	return err
}

func UnlinkMessageAsQuotation(msgID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": msgID},
		bson.M{
			"$set":   bson.M{"is_supplier_quotation": false},
			"$unset": bson.M{"linked_rfq_received_id": "", "linked_rfq_received_code": ""},
		},
	)
	return err
}

// DeleteProcurementMessagesByFilter deletes all messages matching an arbitrary filter.
func DeleteProcurementMessagesByFilter(filter bson.M) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := procurementMessageCol().DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
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

// ContactThread is a summary of all messages between this store and one contact.
type ContactThread struct {
	ContactPhone    string     `bson:"contact_phone" json:"contact_phone"`
	LastMessageText string     `bson:"last_message_text" json:"last_message_text"`
	LastMessageDate *time.Time `bson:"last_message_date" json:"last_message_date"`
	UnreadCount     int        `bson:"unread_count" json:"unread_count"`
	MessageCount    int        `bson:"message_count" json:"message_count"`
	SenderName      string     `bson:"sender_name" json:"sender_name"`
	SenderType      string     `bson:"sender_type" json:"sender_type"`
	Pinned          bool       `bson:"pinned" json:"pinned"`
	PinnedAt        *time.Time `bson:"pinned_at,omitempty" json:"pinned_at,omitempty"`
}

// PinnedContact stores a pinned conversation for a store.
type PinnedContact struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	StoreID  primitive.ObjectID `bson:"store_id"`
	Contact  string             `bson:"contact"`
	MsgType  string             `bson:"msg_type"` // "whatsapp" | "email"
	PinnedAt time.Time          `bson:"pinned_at"`
}

func pinnedContactsCol() *mongo.Collection {
	return db.Client("").Database(db.GetPosDB()).Collection("procurement_pinned_contacts")
}

// PinContact marks a contact thread as pinned.
func PinContact(storeID primitive.ObjectID, contact, msgType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	col := pinnedContactsCol()
	filter := bson.M{"store_id": storeID, "contact": contact, "msg_type": msgType}
	update := bson.M{"$set": bson.M{"store_id": storeID, "contact": contact, "msg_type": msgType, "pinned_at": time.Now()}}
	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, filter, update, opts)
	return err
}

// UnpinContact removes a pinned contact.
func UnpinContact(storeID primitive.ObjectID, contact, msgType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pinnedContactsCol().DeleteOne(ctx, bson.M{"store_id": storeID, "contact": contact, "msg_type": msgType})
	return err
}

// getPinnedContacts returns a set of pinned contact keys ("contact|type") for fast lookup.
func getPinnedContacts(storeID primitive.ObjectID, msgType string) map[string]time.Time {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cur, err := pinnedContactsCol().Find(ctx, bson.M{"store_id": storeID, "msg_type": msgType})
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	var rows []PinnedContact
	_ = cur.All(ctx, &rows)
	m := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		m[r.Contact] = r.PinnedAt
	}
	return m
}

// ListContactThreads returns one row per distinct supplier contact, sorted by last message date desc.
func ListContactThreads(storeID primitive.ObjectID, msgType, search string, page, limit int, phones []string) ([]ContactThread, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	col := procurementMessageCol()

	matchFilter := bson.M{"store_id": storeID}
	if msgType != "" {
		matchFilter["type"] = msgType
	}

	// normalizeEmailExpr uses $regexFind to extract a bare email address from any
	// "from" / "to" format: "Name <email>", "Name &lt;email&gt;", or plain "email".
	// Falls back to the lowercased field value if no email pattern is found.
	normalizeEmailExpr := func(fieldExpr interface{}) bson.M {
		emailRegex := "[a-zA-Z0-9._%+\\-]+@[a-zA-Z0-9.\\-]+\\.[a-zA-Z]{2,}"
		regexMatch := bson.M{"$regexFind": bson.M{"input": fieldExpr, "regex": emailRegex}}
		hasMatch := bson.M{"$ne": bson.A{regexMatch, nil}}
		extractedEmail := bson.M{"$getField": bson.M{"field": "match", "input": regexMatch}}
		return bson.M{
			"$toLower": bson.M{
				"$trim": bson.M{
					"input": bson.M{
						"$cond": bson.A{hasMatch, extractedEmail, fieldExpr},
					},
				},
			},
		}
	}

	contactExpr := bson.M{"$cond": bson.A{
		bson.M{"$eq": bson.A{"$direction", "in"}},
		normalizeEmailExpr("$from"),
		normalizeEmailExpr(bson.M{"$arrayElemAt": bson.A{"$to", 0}}),
	}}

	pipeline := mongo.Pipeline{
		bson.D{bson.E{Key: "$match", Value: matchFilter}},
		bson.D{bson.E{Key: "$group", Value: bson.M{
			"_id":               contactExpr,
			"last_message_text": bson.M{"$last": "$body_text"},
			"last_message_date": bson.M{"$last": "$message_date"},
			"unread_count": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$direction", "in"}},
					bson.M{"$ne": bson.A{"$read", true}},
				}},
				1, 0,
			}}},
			"message_count": bson.M{"$sum": 1},
			"sender_name":   bson.M{"$max": "$sender_name"},
			"sender_type":   bson.M{"$max": "$sender_type"},
		}}},
		bson.D{bson.E{Key: "$project", Value: bson.M{
			"_id":               0,
			"contact_phone":     "$_id",
			"last_message_text": 1,
			"last_message_date": 1,
			"unread_count":      1,
			"message_count":     1,
			"sender_name":       1,
			"sender_type":       1,
		}}},
		bson.D{bson.E{Key: "$sort", Value: bson.M{"last_message_date": -1}}},
	}

	if len(phones) > 0 {
		phonesIface := make(bson.A, len(phones))
		for i, p := range phones {
			phonesIface[i] = p
		}
		pipeline = append(pipeline, bson.D{bson.E{Key: "$match", Value: bson.M{
			"contact_phone": bson.M{"$in": phonesIface},
		}}})
	}

	if search != "" {
		pipeline = append(pipeline, bson.D{bson.E{Key: "$match", Value: bson.M{
			"$or": bson.A{
				bson.M{"contact_phone": bson.M{"$regex": search, "$options": "i"}},
				bson.M{"sender_name": bson.M{"$regex": search, "$options": "i"}},
			},
		}}})
	}

	// Count
	countPipeline := append(pipeline, bson.D{bson.E{Key: "$count", Value: "total"}})
	countCur, _ := col.Aggregate(ctx, countPipeline)
	var countResult []struct {
		Total int64 `bson:"total"`
	}
	_ = countCur.All(ctx, &countResult)
	var total int64
	if len(countResult) > 0 {
		total = countResult[0].Total
	}

	skip := int64((page - 1) * limit)
	pipeline = append(pipeline,
		bson.D{bson.E{Key: "$skip", Value: skip}},
		bson.D{bson.E{Key: "$limit", Value: int64(limit)}},
	)

	cur, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, err
	}
	var threads []ContactThread
	if err := cur.All(ctx, &threads); err != nil {
		return nil, 0, err
	}
	// Annotate pinned state and sort pinned threads to the top.
	pins := getPinnedContacts(storeID, msgType)
	for i := range threads {
		if t, ok := pins[threads[i].ContactPhone]; ok {
			threads[i].Pinned = true
			threads[i].PinnedAt = &t
		}
	}
	// Stable sort: pinned first (by pin time desc), then rest (already sorted by last message)
	pinned := threads[:0:0]
	rest := threads[:0:0]
	for _, th := range threads {
		if th.Pinned {
			pinned = append(pinned, th)
		} else {
			rest = append(rest, th)
		}
	}
	// Sort pinned by pinned_at desc
	for i := 0; i < len(pinned)-1; i++ {
		for j := i + 1; j < len(pinned); j++ {
			if pinned[j].PinnedAt != nil && (pinned[i].PinnedAt == nil || pinned[j].PinnedAt.After(*pinned[i].PinnedAt)) {
				pinned[i], pinned[j] = pinned[j], pinned[i]
			}
		}
	}
	return append(pinned, rest...), total, nil
}

// ListThreadMessages returns all messages between this store and contactPhone in chronological order.
func ListThreadMessages(storeID primitive.ObjectID, contactPhone, msgType string, page, limit int) ([]ProcurementMessage, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	col := procurementMessageCol()

	// Match bare email or "Name <email>" format in the from field,
	// and any element in the to array containing the email.
	emailPat := "(?i)(^|<)" + regexp.QuoteMeta(contactPhone) + "(>|$)"
	filter := bson.M{
		"store_id": storeID,
		"$or": bson.A{
			bson.M{"from": bson.M{"$regex": emailPat}},
			bson.M{"to": bson.M{"$regex": contactPhone, "$options": "i"}},
		},
	}
	if msgType != "" {
		filter["type"] = msgType
	}

	total, _ := col.CountDocuments(ctx, filter)
	skip := int64((page - 1) * limit)
	opts := options.Find().
		SetSort(bson.M{"message_date": 1}).
		SetSkip(skip).
		SetLimit(int64(limit))

	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	var msgs []ProcurementMessage
	if err := cur.All(ctx, &msgs); err != nil {
		return nil, 0, err
	}
	return msgs, total, nil
}

// MarkThreadMessagesRead marks all unread inbound messages from contactPhone as read.
func MarkThreadMessagesRead(storeID primitive.ObjectID, contactPhone, msgType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	emailPat2 := "(?i)(^|<)" + regexp.QuoteMeta(contactPhone) + "(>|$)"
	filter := bson.M{
		"store_id":  storeID,
		"direction": "in",
		"from":      bson.M{"$regex": emailPat2},
		"read":      bson.M{"$ne": true},
	}
	if msgType != "" {
		filter["type"] = msgType
	}
	_, err := procurementMessageCol().UpdateMany(ctx, filter, bson.M{"$set": bson.M{"read": true}})
	return err
}

// ResolveProcurementSender looks up the given phone number or email address in
// RFQ suppliers then customers for the given store.
// Returns (name, type) where type is "supplier" or "customer", or ("", "") if not found.
func ResolveProcurementSender(storeID primitive.ObjectID, phone string) (string, string) {
	if phone == "" {
		return "", ""
	}

	// Extract bare email from "Name <email>" format.
	bareEmail := ""
	if strings.Contains(phone, "@") {
		if m := regexp.MustCompile(`<([^>@\s]+@[^>]+)>`).FindStringSubmatch(phone); len(m) > 1 {
			bareEmail = strings.TrimSpace(m[1])
		} else if strings.Contains(phone, "@") {
			bareEmail = strings.TrimSpace(phone)
		}
		bareEmail = strings.ToLower(bareEmail)
	}

	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_suppliers")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Email lookup: check customer collection first when input is an email address.
	if bareEmail != "" {
		custCol := db.Client("").Database("store_"+storeID.Hex()).Collection("customer")
		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()
		var cust struct {
			Name string `bson:"name"`
		}
		if err := custCol.FindOne(ctx2, bson.M{
			"deleted": bson.M{"$ne": true},
			"email":   bson.M{"$regex": "^" + regexp.QuoteMeta(bareEmail) + "$", "$options": "i"},
		}).Decode(&cust); err == nil && cust.Name != "" {
			return cust.Name, "customer"
		}
		// No match by email — nothing more to try for email inputs.
		return "", ""
	}

	// Try supplier first (phone is international without +, e.g. "966501971075")
	phones := phoneLookupVariants(phone)
	var sup struct {
		Name string `bson:"name"`
	}
	// Check phone2 first — a supplier who set phone2 explicitly takes priority over
	// another supplier whose phone1 happens to match the same number.
	if err := col.FindOne(ctx, bson.M{
		"store_id": storeID,
		"phone2":   bson.M{"$in": phones},
	}).Decode(&sup); err == nil && sup.Name != "" {
		return sup.Name, "supplier"
	}
	if err := col.FindOne(ctx, bson.M{
		"store_id": storeID,
		"phone":    bson.M{"$in": phones},
	}).Decode(&sup); err == nil && sup.Name != "" {
		return sup.Name, "supplier"
	}

	// Try customer (per-store collection) — exact match first
	custCol := db.Client("").Database("store_"+storeID.Hex()).Collection("customer")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	var cust struct {
		Name string `bson:"name"`
	}
	if err := custCol.FindOne(ctx2, bson.M{
		"deleted": bson.M{"$ne": true},
		"$or":     buildPhoneOrQuery(phones),
	}).Decode(&cust); err == nil && cust.Name != "" {
		return cust.Name, "customer"
	}

	// Regex fallback: match phones stored with spaces/+ (e.g. "+966 55 601 4267")
	digits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}
	if digits != "" {
		pattern := `^[^\d]*`
		for i, d := range digits {
			if i > 0 {
				pattern += `[^\d]*`
			}
			pattern += string(d)
		}
		pattern += `[^\d]*$`
		reFilter := bson.M{"$regex": pattern}
		ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel3()
		var cust2 struct {
			Name string `bson:"name"`
		}
		if err := custCol.FindOne(ctx3, bson.M{
			"deleted": bson.M{"$ne": true},
			"$or":     []bson.M{{"phone": reFilter}, {"phone2": reFilter}},
		}).Decode(&cust2); err == nil && cust2.Name != "" {
			return cust2.Name, "customer"
		}
	}

	return "", ""
}

// phoneLookupVariants generates multiple normalised forms of a phone number
// to handle local vs international formats ("0501971075" ↔ "966501971075").
func phoneLookupVariants(phone string) []string {
	// Normalize: strip leading + and any whitespace (handles "+966 594546011" → "966594546011")
	phone = strings.TrimPrefix(phone, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	set := map[string]bool{
		phone:        true,
		"+" + phone: true, // customers may be stored with leading +
	}
	// Saudi: strip leading 966 → add leading 0
	if len(phone) == 12 && phone[:3] == "966" {
		set["0"+phone[3:]] = true
	}
	// Saudi: strip leading 0 → add 966
	if len(phone) == 10 && phone[0] == '0' {
		set["966"+phone[1:]] = true
		set["+966"+phone[1:]] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

func buildPhoneOrQuery(phones []string) []bson.M {
	clauses := make([]bson.M, 0, len(phones)*2)
	for _, p := range phones {
		clauses = append(clauses, bson.M{"phone": p}, bson.M{"phone2": p})
	}
	return clauses
}

// UpdateProcurementMessageSenderType saves only the sender_type (used for outbound messages
// where the sender_name is the store itself, but we want to label the contact type).
func UpdateProcurementMessageSenderType(id primitive.ObjectID, senderType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"sender_type": senderType}},
	)
	return err
}

// UpdateProcurementMessageSender saves resolved sender_name and sender_type back to the DB.
func UpdateProcurementMessageSender(id primitive.ObjectID, name, senderType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := procurementMessageCol().UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"sender_name": name, "sender_type": senderType}},
	)
	return err
}

// BackfillProcurementSenders resolves sender identities for messages that have no
// sender_name set, up to limit per call (0 = all). Returns number updated.
func BackfillProcurementSenders(storeID primitive.ObjectID, limit int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Process all inbound messages (re-resolve even if sender_name already set,
	// so that updated supplier/customer data always wins over a stale cached name).
	filter := bson.M{
		"store_id":  storeID,
		"direction": "in",
	}
	opts := options.Find().SetProjection(bson.M{"_id": 1, "from": 1})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cur, err := procurementMessageCol().Find(ctx, filter, opts)
	if err != nil {
		return 0, err
	}
	var msgs []struct {
		ID   primitive.ObjectID `bson:"_id"`
		From string             `bson:"from"`
	}
	if err := cur.All(ctx, &msgs); err != nil {
		return 0, err
	}

	updated := 0
	for _, m := range msgs {
		name, sType := ResolveProcurementSender(storeID, m.From)
		if name == "" {
			continue
		}
		if err := UpdateProcurementMessageSender(m.ID, name, sType); err == nil {
			updated++
		}
	}

	// Also process outbound messages: set sender_type based on the recipient (to[0])
	// so outbound-only threads (e.g. a message sent to a customer) are labelled correctly.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()
	filterOut := bson.M{
		"store_id":  storeID,
		"direction": "out",
		"to":        bson.M{"$ne": nil, "$not": bson.M{"$size": 0}},
	}
	optsOut := options.Find().SetProjection(bson.M{"_id": 1, "to": 1})
	if limit > 0 {
		optsOut.SetLimit(int64(limit))
	}
	curOut, err := procurementMessageCol().Find(ctx2, filterOut, optsOut)
	if err == nil {
		var outMsgs []struct {
			ID primitive.ObjectID `bson:"_id"`
			To []string           `bson:"to"`
		}
		if curOut.All(ctx2, &outMsgs) == nil {
			for _, m := range outMsgs {
				if len(m.To) == 0 {
					continue
				}
				_, sType := ResolveProcurementSender(storeID, m.To[0])
				if sType == "" {
					continue
				}
				if updateErr := UpdateProcurementMessageSenderType(m.ID, sType); updateErr == nil {
					updated++
				}
			}
		}
	}

	return updated, nil
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
