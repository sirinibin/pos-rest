package models

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// An edit may have more lines than the saved return (or lines in another
// order): the quantity the saved return took of a product must be found
// without indexing past its lines (that used to panic).
func TestOldPurchaseReturnQuantity(t *testing.T) {
	a, b, c := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	old := &PurchaseReturn{Products: []PurchaseReturnProduct{
		{ProductID: a, Quantity: 2, Selected: true},
		{ProductID: b, Quantity: 3, Selected: false},
	}}
	for _, tc := range []struct {
		name  string
		old   *PurchaseReturn
		index int
		id    primitive.ObjectID
		want  float64
	}{
		{"no saved return", nil, 0, a, 0},
		{"same line", old, 0, a, 2},
		{"line not selected before", old, 1, b, 0},
		{"new line past the saved ones", old, 5, c, 0},
		{"product moved to a new line", old, 3, a, 2},
		{"other product at the index", old, 0, c, 0},
	} {
		if got := oldPurchaseReturnQuantity(tc.old, tc.index, tc.id); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
