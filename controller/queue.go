package controller

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// OrderRequest represents an incoming order request
type Request struct {
	Token string // random ID to identify request
}

// SafeQueue is a thread-safe FIFO queue
type SafeQueue struct {
	mu    sync.Mutex
	queue []Request
}

// NewSafeQueue initializes an empty queue
func NewSafeQueue() *SafeQueue {
	return &SafeQueue{
		queue: []Request{},
	}
}

// Enqueue adds a new request to the queue
func (q *SafeQueue) Enqueue(req Request) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, req)
}

// Peek returns the first item without removing it
func (q *SafeQueue) Peek() (*Request, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return nil, errors.New("queue is empty")
	}
	return &q.queue[0], nil
}

// Pop removes and returns the first item
func (q *SafeQueue) Pop() (*Request, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return nil, errors.New("queue is empty")
	}
	item := q.queue[0]
	q.queue = q.queue[1:]
	return &item, nil
}

// FrontToken returns the token of the item at the front
func (q *SafeQueue) FrontToken() (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return "", errors.New("queue is empty")
	}
	return q.queue[0].Token, nil
}

// CheckIfMyTurn checks if this token is at front of the queue
func (q *SafeQueue) CheckIfMyTurn(token string) bool {
	frontToken, err := q.FrontToken()
	if err != nil {
		return false
	}
	return frontToken == token
}

// WaitUntilMyTurn blocks until the given token is at the front of the queue
func (q *SafeQueue) WaitUntilMyTurn(token string) {
	for {
		if q.CheckIfMyTurn(token) {
			return
		}
		time.Sleep(100 * time.Millisecond) // Adjust interval as needed
	}
}

// Helper: Generate random token
func generateQueueToken() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Intn(1000))
}

var (
	storeSalesQueues                = make(map[string]*SafeQueue)
	storeSalesReturnQueues          = make(map[string]*SafeQueue)
	storePurchaseQueues             = make(map[string]*SafeQueue)
	storePurchaseReturnQueues       = make(map[string]*SafeQueue)
	storeQuotationQueues            = make(map[string]*SafeQueue)
	storeQuotationSalesReturnQueues = make(map[string]*SafeQueue)
	queueMu                         sync.Mutex
)

// GetOrCreateQueue returns the queue for the store or creates it.
func GetOrCreateQueue(storeID string, modelName string) *SafeQueue {
	queueMu.Lock()
	defer queueMu.Unlock()

	if modelName == "sales" {
		q, exists := storeSalesQueues[storeID]
		if !exists {
			q = NewSafeQueue()
			storeSalesQueues[storeID] = q
		}
		return q

	} else if modelName == "sales_return" {
		q, exists := storeSalesReturnQueues[storeID]
		if !exists {
			q = NewSafeQueue()
			storeSalesReturnQueues[storeID] = q
		}
		return q
	} else if modelName == "purchase" {
		q, exists := storePurchaseQueues[storeID]
		if !exists {
			q = NewSafeQueue()
			storePurchaseQueues[storeID] = q
		}
		return q
	} else if modelName == "purchase_return" {
		q, exists := storePurchaseReturnQueues[storeID]
		if !exists {
			q = NewSafeQueue()
			storePurchaseReturnQueues[storeID] = q
		}
		return q
	} else if modelName == "quotation" {
		q, exists := storeQuotationQueues[storeID]
		if !exists {
			q = NewSafeQueue()
			storeQuotationQueues[storeID] = q
		}
		return q
	} else if modelName == "quotation_sales_return" {
		q, exists := storeQuotationSalesReturnQueues[storeID]
		if !exists {
			q = NewSafeQueue()
			storeQuotationSalesReturnQueues[storeID] = q
		}
		return q
	}

	return nil
}

// CleanupQueueIfEmpty removes the queue if it's empty.
func CleanupQueueIfEmpty(storeID string, modelName string) {
	queueMu.Lock()
	defer queueMu.Unlock()

	if modelName == "sales" {
		q, exists := storeSalesQueues[storeID]
		if exists && len(q.queue) == 0 {
			delete(storeSalesQueues, storeID)
		}
	} else if modelName == "sales_return" {
		q, exists := storeSalesReturnQueues[storeID]
		if exists && len(q.queue) == 0 {
			delete(storeSalesReturnQueues, storeID)
		}
	} else if modelName == "purchase" {
		q, exists := storePurchaseQueues[storeID]
		if exists && len(q.queue) == 0 {
			delete(storePurchaseQueues, storeID)
		}
	} else if modelName == "purchase_return" {
		q, exists := storePurchaseReturnQueues[storeID]
		if exists && len(q.queue) == 0 {
			delete(storePurchaseReturnQueues, storeID)
		}
	} else if modelName == "quotation" {
		q, exists := storeQuotationQueues[storeID]
		if exists && len(q.queue) == 0 {
			delete(storeQuotationQueues, storeID)
		}
	} else if modelName == "quotation_sales_return" {
		q, exists := storeQuotationSalesReturnQueues[storeID]
		if exists && len(q.queue) == 0 {
			delete(storeQuotationSalesReturnQueues, storeID)
		}
	}
}
