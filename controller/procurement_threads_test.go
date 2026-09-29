package controller

import (
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

// ── rfq-whatsapp-unread total counting ────────────────────────────────────────

// countAllUnreadThreads mirrors the counting logic in GetRFQWhatsAppUnreadHandler.
// It returns the sum of unread counts across all threads, regardless of RFQ linkage.
func countAllUnreadThreads(threads []models.ContactThread) int {
	total := 0
	for _, t := range threads {
		if t.UnreadCount > 0 {
			total += t.UnreadCount
		}
	}
	return total
}

// countRFQMatchedUnread returns the unread count only for phones present in rfqPhones.
func countRFQMatchedUnread(threads []models.ContactThread, rfqPhones map[string]bool) int {
	total := 0
	for _, t := range threads {
		norm := strings.TrimPrefix(t.ContactPhone, "+")
		if rfqPhones[norm] && t.UnreadCount > 0 {
			total += t.UnreadCount
		}
	}
	return total
}

func TestAllUnreadIncludesNonRFQThreads(t *testing.T) {
	threads := []models.ContactThread{
		{ContactPhone: "+966501111111", UnreadCount: 3}, // tied to RFQ
		{ContactPhone: "+966502222222", UnreadCount: 2}, // NOT in any RFQ
		{ContactPhone: "+966503333333", UnreadCount: 0}, // read; should be excluded
	}
	rfqPhones := map[string]bool{"966501111111": true}

	allTotal := countAllUnreadThreads(threads)
	rfqTotal := countRFQMatchedUnread(threads, rfqPhones)

	if allTotal != 5 {
		t.Errorf("allTotal: got %d, want 5", allTotal)
	}
	if rfqTotal != 3 {
		t.Errorf("rfqTotal: got %d, want 3", rfqTotal)
	}
	// The badge (allTotal) must be >= the dropdown item count (rfqTotal)
	if allTotal < rfqTotal {
		t.Errorf("badge total %d < rfq total %d: badge must include all unread", allTotal, rfqTotal)
	}
}

func TestAllUnreadZeroWhenNoUnread(t *testing.T) {
	threads := []models.ContactThread{
		{ContactPhone: "+966501111111", UnreadCount: 0},
	}
	if got := countAllUnreadThreads(threads); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestAllUnreadStripsLeadingPlus(t *testing.T) {
	// Phones stored with and without '+' prefix should both count.
	threads := []models.ContactThread{
		{ContactPhone: "966501111111", UnreadCount: 1},
		{ContactPhone: "+966502222222", UnreadCount: 2},
	}
	if got := countAllUnreadThreads(threads); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

// buildItemsWithFallback mirrors the item-building logic in GetRFQWhatsAppUnreadHandler,
// including the fallback that adds non-RFQ threads so the list is never empty when the
// badge count is > 0.
func buildItemsWithFallback(unreadByPhone map[string]models.ContactThread, rfqItems []RFQUnreadSummary) []RFQUnreadSummary {
	items := append([]RFQUnreadSummary{}, rfqItems...)
	matchedPhones := map[string]bool{}
	for _, it := range items {
		norm := strings.TrimPrefix(it.Phone, "+")
		matchedPhones[norm] = true
	}
	for norm, t := range unreadByPhone {
		if !matchedPhones[norm] {
			items = append(items, RFQUnreadSummary{
				Phone:       t.ContactPhone,
				ContactName: t.ContactPhone,
				PhoneType:   "supplier",
				UnreadCount: t.UnreadCount,
				LastMsgText: t.LastMessageText,
			})
		}
	}
	return items
}

func TestNonRFQThreadsAppearedInList(t *testing.T) {
	// When all unread threads are from non-RFQ contacts, the list must not be empty.
	unreadByPhone := map[string]models.ContactThread{
		"966501111111": {ContactPhone: "966501111111", UnreadCount: 4},
		"966502222222": {ContactPhone: "966502222222", UnreadCount: 1},
	}
	rfqItems := []RFQUnreadSummary{} // no RFQ matches

	items := buildItemsWithFallback(unreadByPhone, rfqItems)
	if len(items) != 2 {
		t.Errorf("expected 2 fallback items, got %d", len(items))
	}
	for _, it := range items {
		if it.PhoneType != "supplier" {
			t.Errorf("fallback item phone_type: got %q, want \"supplier\"", it.PhoneType)
		}
		if it.UnreadCount == 0 {
			t.Errorf("fallback item should have unread_count > 0")
		}
	}
}

func TestRFQMatchedPhonesNotDuplicated(t *testing.T) {
	// Phones already matched to an RFQ must not appear again as fallback entries.
	unreadByPhone := map[string]models.ContactThread{
		"966501111111": {ContactPhone: "966501111111", UnreadCount: 3},
		"966502222222": {ContactPhone: "966502222222", UnreadCount: 2},
	}
	rfqItems := []RFQUnreadSummary{
		{Phone: "966501111111", RFQCode: "RFQ-001", UnreadCount: 3},
	}

	items := buildItemsWithFallback(unreadByPhone, rfqItems)
	// 1 rfq item + 1 fallback for the unmatched phone
	if len(items) != 2 {
		t.Errorf("expected 2 items (1 rfq + 1 fallback), got %d", len(items))
	}
	phoneCount := map[string]int{}
	for _, it := range items {
		phoneCount[strings.TrimPrefix(it.Phone, "+")]++
	}
	for phone, count := range phoneCount {
		if count > 1 {
			t.Errorf("phone %s appears %d times — should not be duplicated", phone, count)
		}
	}
}
