package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// headS3Object returns true if key exists in S3 (SigV4-signed HEAD request).
func headS3Object(s models.AdminSettings, key string) bool {
	now := time.Now().UTC()
	dateStr := now.Format("20060102")
	timeStr := now.Format("20060102T150405Z")
	region := s.S3Region
	if region == "" {
		region = "us-east-1"
	}
	emptyHash := fmt.Sprintf("%x", sha256sum([]byte{}))
	host := s3Host(s)
	headURL := s3PutURL(s, key)

	req, err := http.NewRequest("HEAD", headURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("X-Amz-Content-Sha256", emptyHash)

	urlPath := "/" + key
	if s.S3Endpoint != "" {
		urlPath = "/" + s.S3BucketName + "/" + key
	}
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, emptyHash, timeStr)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{"HEAD", urlPath, "", canonicalHeaders, signedHeaders, emptyHash}, "\n")

	credentialScope := strings.Join([]string{dateStr, region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", timeStr, credentialScope,
		fmt.Sprintf("%x", sha256sum([]byte(canonicalRequest))),
	}, "\n")
	signingKey := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256(
		[]byte("AWS4"+s.S3SecretKey), []byte(dateStr)),
		[]byte(region)), []byte("s3")), []byte("aws4_request"))
	signature := fmt.Sprintf("%x", hmacSHA256(signingKey, []byte(stringToSign)))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.S3AccessKeyID, credentialScope, signedHeaders, signature))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// entityCleanupStats holds per-entity counters streamed to the frontend.
type entityCleanupStats struct {
	Name         string `json:"name"`
	Checked      int    `json:"checked"`       // /cdn/ URLs examined
	Verified     int    `json:"verified"`      // confirmed in S3
	Deleted      int    `json:"deleted"`       // local disk files removed
	NotInS3      int    `json:"not_in_s3"`     // S3 HEAD returned 404
	MongoCleaned int    `json:"mongo_cleaned"` // inline MongoDB data unset
}

// VerifyAndCleanupDiskHandler verifies each /cdn/ attachment exists in S3,
// then deletes the local disk copy. For RFQ records it also unsets the
// imagescontent field from MongoDB once the corresponding S3 file is confirmed.
// POST /v1/verify-cleanup-disk
func VerifyAndCleanupDiskHandler(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}

	s := loadAdminS3Settings()
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured or not enabled"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Minute)
	defer cancel()

	// verifyCDNAndDelete HEAD-checks S3 and removes the local disk copy if confirmed.
	verifyCDNAndDelete := func(cdnURL string) (verified, deleted bool) {
		key := strings.TrimPrefix(cdnURL, "/cdn/")
		if key == cdnURL || key == "" {
			return false, false
		}
		if !headS3Object(s, key) {
			return false, false
		}
		verified = true
		if os.Remove("./"+key) == nil {
			deleted = true
		}
		return
	}

	posDB := db.Client("").Database(db.GetPosDB())

	// Collect all store IDs and names
	storeRows, _ := posDB.Collection("store").Find(ctx, bson.M{})
	var storeIDs []string
	storeNames := map[string]string{}
	if storeRows != nil {
		for storeRows.Next(ctx) {
			var row struct {
				ID   primitive.ObjectID `bson:"_id"`
				Name string             `bson:"name"`
			}
			if storeRows.Decode(&row) == nil {
				storeIDs = append(storeIDs, row.ID.Hex())
				storeNames[row.ID.Hex()] = row.Name
			}
		}
		storeRows.Close(ctx)
	}

	const totalEntities = 16
	sendSSE(w, flusher, map[string]interface{}{"type": "start", "total_entities": totalEntities})

	var summary []entityCleanupStats

	// emit sends entity_done and appends to summary
	emitEntity := func(idx int, st entityCleanupStats) {
		sendSSE(w, flusher, map[string]interface{}{
			"type": "entity_done", "index": idx, "total": totalEntities,
			"name": st.Name, "checked": st.Checked, "verified": st.Verified,
			"deleted": st.Deleted, "not_in_s3": st.NotInS3, "mongo_cleaned": st.MongoCleaned,
		})
		summary = append(summary, st)
	}

	// ── per-store images array helper ────────────────────────────────────────
	cleanImages := func(name, colName string) entityCleanupStats {
		st := entityCleanupStats{Name: name}
		proj := options.Find().SetProjection(bson.M{"_id": 1, "images": 1})
		for _, sid := range storeIDs {
			sendSSE(w, flusher, map[string]interface{}{"type": "store_progress", "store_id": sid, "store_name": storeNames[sid]})
			col := db.Client("").Database("store_" + sid).Collection(colName)
			cur, err := col.Find(ctx, bson.M{"images": bson.M{"$exists": true, "$ne": nil}}, proj)
			if err != nil {
				continue
			}
			for cur.Next(ctx) {
				var doc struct {
					ID     primitive.ObjectID `bson:"_id"`
					Images []string           `bson:"images"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				for _, url := range doc.Images {
					if !strings.HasPrefix(url, "/cdn/") {
						continue
					}
					st.Checked++
					v, d := verifyCDNAndDelete(url)
					if v {
						st.Verified++
					} else {
						st.NotInS3++
					}
					if d {
						st.Deleted++
					}
				}
			}
			cur.Close(ctx)
		}
		return st
	}

	// ── per-store ZATCA cleared_xml_url helper ────────────────────────────────
	cleanZatca := func(name, colName string) entityCleanupStats {
		st := entityCleanupStats{Name: name}
		proj := options.Find().SetProjection(bson.M{"_id": 1, "zatca.cleared_xml_url": 1})
		filter := bson.M{"zatca.cleared_xml_url": bson.M{"$exists": true, "$ne": ""}}
		for _, sid := range storeIDs {
			sendSSE(w, flusher, map[string]interface{}{"type": "store_progress", "store_id": sid, "store_name": storeNames[sid]})
			col := db.Client("").Database("store_" + sid).Collection(colName)
			cur, err := col.Find(ctx, filter, proj)
			if err != nil {
				continue
			}
			for cur.Next(ctx) {
				var doc struct {
					ID    primitive.ObjectID `bson:"_id"`
					Zatca struct {
						ClearedXMLURL string `bson:"cleared_xml_url"`
					} `bson:"zatca"`
				}
				if cur.Decode(&doc) != nil || doc.Zatca.ClearedXMLURL == "" {
					continue
				}
				st.Checked++
				v, d := verifyCDNAndDelete(doc.Zatca.ClearedXMLURL)
				if v {
					st.Verified++
				} else {
					st.NotInS3++
				}
				if d {
					st.Deleted++
				}
			}
			cur.Close(ctx)
		}
		return st
	}

	// ── Entity 1: Product photos ──────────────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 1, "total": totalEntities, "name": "Product Photos"})
	emitEntity(1, cleanImages("Product Photos", "product"))

	// ── Entity 2: Customer photos ─────────────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 2, "total": totalEntities, "name": "Customer Photos"})
	emitEntity(2, cleanImages("Customer Photos", "customer"))

	// ── Entity 3: Vendor photos + logos ───────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 3, "total": totalEntities, "name": "Vendor Photos"})
	{
		st := cleanImages("Vendor Photos", "vendor")
		// also vendor.logo (single string)
		proj := options.Find().SetProjection(bson.M{"_id": 1, "logo": 1})
		for _, sid := range storeIDs {
			sendSSE(w, flusher, map[string]interface{}{"type": "store_progress", "store_id": sid, "store_name": storeNames[sid]})
			col := db.Client("").Database("store_" + sid).Collection("vendor")
			cur, err := col.Find(ctx, bson.M{"logo": bson.M{"$regex": "^/cdn/"}}, proj)
			if err == nil && cur != nil {
				for cur.Next(ctx) {
					var doc struct {
						ID   primitive.ObjectID `bson:"_id"`
						Logo string             `bson:"logo"`
					}
					if cur.Decode(&doc) != nil || !strings.HasPrefix(doc.Logo, "/cdn/") {
						continue
					}
					st.Checked++
					v, d := verifyCDNAndDelete(doc.Logo)
					if v {
						st.Verified++
					} else {
						st.NotInS3++
					}
					if d {
						st.Deleted++
					}
				}
				cur.Close(ctx)
			}
		}
		emitEntity(3, st)
	}

	// ── Entity 4: Receivable (customer_deposit) attachments ───────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 4, "total": totalEntities, "name": "Receivable Attachments"})
	emitEntity(4, cleanImages("Receivable Attachments", "customerdeposit"))

	// ── Entity 5: Payable (customer_withdrawal) attachments ───────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 5, "total": totalEntities, "name": "Payable Attachments"})
	emitEntity(5, cleanImages("Payable Attachments", "customerwithdrawal"))

	// ── Entity 6: Expense attachments ─────────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 6, "total": totalEntities, "name": "Expense Attachments"})
	emitEntity(6, cleanImages("Expense Attachments", "expense"))

	// ── Entity 7: Capital Investment attachments ──────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 7, "total": totalEntities, "name": "Capital Investment Attachments"})
	emitEntity(7, cleanImages("Capital Investment Attachments", "capital"))

	// ── Entity 8: Drawings attachments ────────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 8, "total": totalEntities, "name": "Drawing Attachments"})
	emitEntity(8, cleanImages("Drawing Attachments", "capitalwithdrawal"))

	// ── Entity 9: ZATCA Sales XMLs ────────────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 9, "total": totalEntities, "name": "ZATCA Sales XMLs"})
	emitEntity(9, cleanZatca("ZATCA Sales XMLs", "order"))

	// ── Entity 10: ZATCA Sales Return XMLs ───────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 10, "total": totalEntities, "name": "ZATCA Sales Return XMLs"})
	emitEntity(10, cleanZatca("ZATCA Sales Return XMLs", "salesreturn"))

	// ── Entity 11: ZATCA Receivable XMLs ─────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 11, "total": totalEntities, "name": "ZATCA Receivable XMLs"})
	emitEntity(11, cleanZatca("ZATCA Receivable XMLs", "customerdeposit"))

	// ── Entity 12: ZATCA Payable XMLs ─────────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 12, "total": totalEntities, "name": "ZATCA Payable XMLs"})
	emitEntity(12, cleanZatca("ZATCA Payable XMLs", "customerwithdrawal"))

	// ── Entity 13: WhatsApp attachments ──────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 13, "total": totalEntities, "name": "WhatsApp Attachments"})
	{
		st := entityCleanupStats{Name: "WhatsApp Attachments"}
		proj := options.Find().SetProjection(bson.M{"_id": 1, "attachments": 1})
		cur, err := posDB.Collection("procurement_messages").Find(ctx,
			bson.M{"type": "whatsapp", "attachments.url": bson.M{"$regex": "^/cdn/"}}, proj)
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID          primitive.ObjectID `bson:"_id"`
					Attachments []struct {
						URL string `bson:"url"`
					} `bson:"attachments"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				for _, att := range doc.Attachments {
					if !strings.HasPrefix(att.URL, "/cdn/") {
						continue
					}
					st.Checked++
					v, d := verifyCDNAndDelete(att.URL)
					if v {
						st.Verified++
					} else {
						st.NotInS3++
					}
					if d {
						st.Deleted++
					}
				}
			}
			cur.Close(ctx)
		}
		emitEntity(13, st)
	}

	// ── Entity 14: Email attachments ──────────────────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 14, "total": totalEntities, "name": "Email Attachments"})
	{
		st := entityCleanupStats{Name: "Email Attachments"}
		proj := options.Find().SetProjection(bson.M{"_id": 1, "attachments": 1})
		cur, err := posDB.Collection("procurement_messages").Find(ctx,
			bson.M{"type": "email", "attachments.url": bson.M{"$regex": "^/cdn/"}}, proj)
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID          primitive.ObjectID `bson:"_id"`
					Attachments []struct {
						URL string `bson:"url"`
					} `bson:"attachments"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				for _, att := range doc.Attachments {
					if !strings.HasPrefix(att.URL, "/cdn/") {
						continue
					}
					st.Checked++
					v, d := verifyCDNAndDelete(att.URL)
					if v {
						st.Verified++
					} else {
						st.NotInS3++
					}
					if d {
						st.Deleted++
					}
				}
			}
			cur.Close(ctx)
		}
		emitEntity(14, st)
	}

	// ── Entity 15: RFQ attachments + MongoDB imagescontent cleanup ─────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 15, "total": totalEntities, "name": "RFQ Attachments"})
	{
		st := entityCleanupStats{Name: "RFQ Attachments"}
		rfqCol := posDB.Collection("rfq_received")
		proj := options.Find().SetProjection(bson.M{
			"_id": 1, "attachment_urls": 1, "additional_attachment_urls": 1, "imagescontent": 1,
		})
		cur, err := rfqCol.Find(ctx, bson.M{}, proj)
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID                       primitive.ObjectID `bson:"_id"`
					AttachmentURLs           []string           `bson:"attachment_urls"`
					AdditionalAttachmentURLs []string           `bson:"additional_attachment_urls"`
					ImagesContent            []string           `bson:"imagescontent"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				hasMigratedURL := false
				for _, url := range append(doc.AttachmentURLs, doc.AdditionalAttachmentURLs...) {
					if !strings.HasPrefix(url, "/cdn/") {
						continue
					}
					st.Checked++
					hasMigratedURL = true
					v, d := verifyCDNAndDelete(url)
					if v {
						st.Verified++
					} else {
						st.NotInS3++
					}
					if d {
						st.Deleted++
					}
				}
				// Unset MongoDB inline base64 data if attachments are already in S3
				if len(doc.ImagesContent) > 0 && hasMigratedURL {
					res, err := rfqCol.UpdateOne(ctx, bson.M{"_id": doc.ID},
						bson.M{"$unset": bson.M{"imagescontent": ""}})
					if err == nil && res.ModifiedCount > 0 {
						st.MongoCleaned++
					}
				}
			}
			cur.Close(ctx)
		}
		emitEntity(15, st)
	}

	// ── Entity 16: Store logo + invoice background ────────────────────────────
	sendSSE(w, flusher, map[string]interface{}{"type": "entity_start", "index": 16, "total": totalEntities, "name": "Store Attachments"})
	{
		st := entityCleanupStats{Name: "Store Attachments"}
		proj := options.Find().SetProjection(bson.M{"_id": 1, "logo": 1, "invoice_background": 1})
		cur, err := posDB.Collection("store").Find(ctx, bson.M{}, proj)
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID                primitive.ObjectID `bson:"_id"`
					Logo              string             `bson:"logo"`
					InvoiceBackground string             `bson:"invoice_background"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				for _, url := range []string{doc.Logo, doc.InvoiceBackground} {
					if !strings.HasPrefix(url, "/cdn/") {
						continue
					}
					st.Checked++
					v, d := verifyCDNAndDelete(url)
					if v {
						st.Verified++
					} else {
						st.NotInS3++
					}
					if d {
						st.Deleted++
					}
				}
			}
			cur.Close(ctx)
		}
		emitEntity(16, st)
	}

	// ── Final totals ──────────────────────────────────────────────────────────
	totals := struct {
		Checked      int
		Verified     int
		Deleted      int
		NotInS3      int
		MongoCleaned int
	}{}
	for _, st := range summary {
		totals.Checked += st.Checked
		totals.Verified += st.Verified
		totals.Deleted += st.Deleted
		totals.NotInS3 += st.NotInS3
		totals.MongoCleaned += st.MongoCleaned
	}
	sendSSE(w, flusher, map[string]interface{}{
		"type":               "done",
		"total_checked":      totals.Checked,
		"total_verified":     totals.Verified,
		"total_deleted":      totals.Deleted,
		"total_not_in_s3":     totals.NotInS3,
		"total_mongo_cleaned": totals.MongoCleaned,
		"summary":             summary,
	})
}

// FixDirectS3URLsHandler scans all entity image/attachment fields in MongoDB and
// rewrites any direct S3 URLs (e.g. https://bucket.s3.region.amazonaws.com/key)
// to /cdn/key so the app always routes files through the CdnFileHandler.
// POST /v1/fix-direct-s3-urls
func FixDirectS3URLsHandler(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}

	s := loadAdminS3Settings()
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured or not enabled"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	// baseURL is the S3 bucket root that old code stored as a URL prefix.
	baseURL := strings.TrimRight(s3BaseURL(s), "/")

	// rewriteURL converts a direct S3 URL to a /cdn/ path; returns ("", false) if not an S3 URL.
	rewriteURL := func(url string) (string, bool) {
		if strings.HasPrefix(url, "/cdn/") || url == "" {
			return url, false
		}
		if !strings.HasPrefix(url, baseURL+"/") {
			return url, false
		}
		relKey := strings.TrimPrefix(url, baseURL+"/")
		return "/cdn/" + relKey, true
	}

	posDB := db.Client("").Database(db.GetPosDB())

	storeRows, _ := posDB.Collection("store").Find(ctx, bson.M{})
	var storeIDs []string
	fixStoreNames := map[string]string{}
	if storeRows != nil {
		for storeRows.Next(ctx) {
			var row struct {
				ID   primitive.ObjectID `bson:"_id"`
				Name string             `bson:"name"`
			}
			if storeRows.Decode(&row) == nil {
				storeIDs = append(storeIDs, row.ID.Hex())
				fixStoreNames[row.ID.Hex()] = row.Name
			}
		}
		storeRows.Close(ctx)
	}

	fixed := 0
	scanned := 0

	// fixImagesArray rewrites direct S3 URLs in an images[] field.
	fixImagesArray := func(colName string) {
		proj := options.Find().SetProjection(bson.M{"_id": 1, "images": 1})
		for _, sid := range storeIDs {
			sendSSE(w, flusher, map[string]interface{}{"type": "store_progress", "store_id": sid, "store_name": fixStoreNames[sid]})
			col := db.Client("").Database("store_" + sid).Collection(colName)
			cur, err := col.Find(ctx, bson.M{"images": bson.M{"$exists": true, "$ne": nil}}, proj)
			if err != nil {
				continue
			}
			for cur.Next(ctx) {
				var doc struct {
					ID     primitive.ObjectID `bson:"_id"`
					Images []string           `bson:"images"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				scanned++
				newImages := make([]string, len(doc.Images))
				changed := false
				for i, img := range doc.Images {
					if nu, ok := rewriteURL(img); ok {
						newImages[i] = nu
						changed = true
					} else {
						newImages[i] = img
					}
				}
				if changed {
					col.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": bson.M{"images": newImages}})
					fixed++
				}
			}
			cur.Close(ctx)
		}
	}

	sendSSE(w, flusher, map[string]interface{}{"type": "start", "base_url": baseURL})

	// Per-store images arrays
	for _, colName := range []string{"product", "customer", "vendor", "expense", "capital", "capitalwithdrawal", "customerdeposit", "customerwithdrawal"} {
		fixImagesArray(colName)
		sendSSE(w, flusher, map[string]interface{}{"type": "progress", "collection": colName, "scanned": scanned, "fixed": fixed})
	}

	// Vendor logos (single string)
	for _, sid := range storeIDs {
		sendSSE(w, flusher, map[string]interface{}{"type": "store_progress", "store_id": sid, "store_name": fixStoreNames[sid]})
		col := db.Client("").Database("store_" + sid).Collection("vendor")
		cur, err := col.Find(ctx, bson.M{"logo": bson.M{"$exists": true, "$ne": ""}},
			options.Find().SetProjection(bson.M{"_id": 1, "logo": 1}))
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID   primitive.ObjectID `bson:"_id"`
					Logo string             `bson:"logo"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				scanned++
				if nu, ok := rewriteURL(doc.Logo); ok {
					col.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": bson.M{"logo": nu}})
					fixed++
				}
			}
			cur.Close(ctx)
		}
	}
	sendSSE(w, flusher, map[string]interface{}{"type": "progress", "collection": "vendor_logo", "scanned": scanned, "fixed": fixed})

	// Store logo + invoice_background
	{
		cur, err := posDB.Collection("store").Find(ctx, bson.M{},
			options.Find().SetProjection(bson.M{"_id": 1, "logo": 1, "invoice_background": 1}))
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID                primitive.ObjectID `bson:"_id"`
					Logo              string             `bson:"logo"`
					InvoiceBackground string             `bson:"invoice_background"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				scanned++
				update := bson.M{}
				if nu, ok := rewriteURL(doc.Logo); ok {
					update["logo"] = nu
				}
				if nu, ok := rewriteURL(doc.InvoiceBackground); ok {
					update["invoice_background"] = nu
				}
				if len(update) > 0 {
					posDB.Collection("store").UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": update})
					fixed++
				}
			}
			cur.Close(ctx)
		}
	}
	sendSSE(w, flusher, map[string]interface{}{"type": "progress", "collection": "store", "scanned": scanned, "fixed": fixed})

	// ZATCA cleared_xml_url
	for _, colName := range []string{"order", "salesreturn", "customerdeposit", "customerwithdrawal"} {
		for _, sid := range storeIDs {
			sendSSE(w, flusher, map[string]interface{}{"type": "store_progress", "store_id": sid, "store_name": fixStoreNames[sid]})
			col := db.Client("").Database("store_" + sid).Collection(colName)
			cur, err := col.Find(ctx, bson.M{"zatca.cleared_xml_url": bson.M{"$exists": true, "$ne": ""}},
				options.Find().SetProjection(bson.M{"_id": 1, "zatca.cleared_xml_url": 1}))
			if err != nil || cur == nil {
				continue
			}
			for cur.Next(ctx) {
				var doc struct {
					ID    primitive.ObjectID `bson:"_id"`
					Zatca struct {
						ClearedXMLURL string `bson:"cleared_xml_url"`
					} `bson:"zatca"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				scanned++
				if nu, ok := rewriteURL(doc.Zatca.ClearedXMLURL); ok {
					col.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": bson.M{"zatca.cleared_xml_url": nu}})
					fixed++
				}
			}
			cur.Close(ctx)
		}
	}
	sendSSE(w, flusher, map[string]interface{}{"type": "progress", "collection": "zatca", "scanned": scanned, "fixed": fixed})

	// Procurement message attachments (WhatsApp + Email)
	{
		col := posDB.Collection("procurement_messages")
		cur, err := col.Find(ctx, bson.M{"attachments": bson.M{"$exists": true, "$ne": nil}},
			options.Find().SetProjection(bson.M{"_id": 1, "attachments": 1}))
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID          primitive.ObjectID `bson:"_id"`
					Attachments []struct {
						Filename    string `bson:"filename"`
						ContentType string `bson:"content_type"`
						Size        int64  `bson:"size"`
						URL         string `bson:"url"`
					} `bson:"attachments"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				scanned++
				changed := false
				for i, att := range doc.Attachments {
					if nu, ok := rewriteURL(att.URL); ok {
						doc.Attachments[i].URL = nu
						changed = true
					}
				}
				if changed {
					col.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": bson.M{"attachments": doc.Attachments}})
					fixed++
				}
			}
			cur.Close(ctx)
		}
	}
	sendSSE(w, flusher, map[string]interface{}{"type": "progress", "collection": "procurement_messages", "scanned": scanned, "fixed": fixed})

	// RFQ attachment_urls + additional_attachment_urls
	{
		col := posDB.Collection("rfq_received")
		cur, err := col.Find(ctx, bson.M{},
			options.Find().SetProjection(bson.M{"_id": 1, "attachment_urls": 1, "additional_attachment_urls": 1}))
		if err == nil && cur != nil {
			for cur.Next(ctx) {
				var doc struct {
					ID                       primitive.ObjectID `bson:"_id"`
					AttachmentURLs           []string           `bson:"attachment_urls"`
					AdditionalAttachmentURLs []string           `bson:"additional_attachment_urls"`
				}
				if cur.Decode(&doc) != nil {
					continue
				}
				scanned++
				update := bson.M{}
				rewriteSlice := func(urls []string) ([]string, bool) {
					ch := false
					out := make([]string, len(urls))
					for i, u := range urls {
						if nu, ok := rewriteURL(u); ok {
							out[i] = nu
							ch = true
						} else {
							out[i] = u
						}
					}
					return out, ch
				}
				if nu, ch := rewriteSlice(doc.AttachmentURLs); ch {
					update["attachment_urls"] = nu
				}
				if nu, ch := rewriteSlice(doc.AdditionalAttachmentURLs); ch {
					update["additional_attachment_urls"] = nu
				}
				if len(update) > 0 {
					col.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": update})
					fixed++
				}
			}
			cur.Close(ctx)
		}
	}

	sendSSE(w, flusher, map[string]interface{}{
		"type": "done", "scanned": scanned, "fixed": fixed,
	})
}
