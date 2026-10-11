//go:build integration

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The procurement routes with a real token, user, stores and messages in MongoDB.
func TestProcurementAccess_Integration(t *testing.T) {
	ctx := context.Background()
	newStore := func(name string) *models.Store {
		s := &models.Store{Name: name, Code: "PA-" + primitive.NewObjectID().Hex()[:6], Email: "pa@example.com"}
		if err := s.Insert(); err != nil {
			t.Fatalf("store.Insert: %v", err)
		}
		t.Cleanup(func() { _ = s.PermanentlyDelete() })
		return s
	}
	mine, other := newStore("Procurement Access Mine"), newStore("Procurement Access Other")

	email := "pa-" + primitive.NewObjectID().Hex() + "@example.com"
	userID := primitive.NewObjectID()
	users := db.Client("").Database(db.GetPosDB()).Collection("user")
	if _, err := users.InsertOne(ctx, bson.M{"_id": userID, "name": "PA", "email": email, "role": "Manager", "store_ids": []primitive.ObjectID{mine.ID}}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = users.DeleteOne(ctx, bson.M{"_id": userID}) })
	tok, err := models.GenerateAccesstoken(email)
	if err != nil {
		t.Fatalf("GenerateAccesstoken: %v", err)
	}

	now := time.Now()
	msg := func(store primitive.ObjectID) primitive.ObjectID {
		m := &models.ProcurementMessage{StoreID: store, Type: "email", Direction: "in", Provider: "test", From: "s@example.com", Subject: "secret quote", MessageDate: &now}
		if err := models.SaveProcurementMessage(m); err != nil {
			t.Fatalf("SaveProcurementMessage: %v", err)
		}
		t.Cleanup(func() {
			_, _ = db.Client("").Database(db.GetPosDB()).Collection("procurement_messages").DeleteOne(ctx, bson.M{"_id": m.ID})
		})
		return m.ID
	}
	myMsg, otherMsg := msg(mine.ID), msg(other.ID)

	r := mux.NewRouter()
	r.HandleFunc("/v1/email-unread", RequireProcurementAccess(GetEmailUnreadHandler)).Methods("GET")
	r.HandleFunc("/v1/procurement-messages/{id}", RequireProcurementAccess(GetProcurementMessageHandler)).Methods("GET")
	call := func(path, token string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr.Code
	}

	cases := []struct {
		name, path, token string
		want              int
	}{
		{"no token", "/v1/email-unread?store_id=" + mine.ID.Hex(), "", http.StatusUnauthorized},
		{"own store", "/v1/email-unread?store_id=" + mine.ID.Hex(), tok.Token, http.StatusOK},
		{"own store, Bearer prefix", "/v1/email-unread?store_id=" + mine.ID.Hex(), "Bearer " + tok.Token, http.StatusOK},
		{"other store", "/v1/email-unread?store_id=" + other.ID.Hex(), tok.Token, http.StatusForbidden},
		{"message, no token", "/v1/procurement-messages/" + myMsg.Hex(), "", http.StatusUnauthorized},
		{"own message", "/v1/procurement-messages/" + myMsg.Hex(), tok.Token, http.StatusOK},
		{"other store's message", "/v1/procurement-messages/" + otherMsg.Hex(), tok.Token, http.StatusForbidden},
		{"unknown message", "/v1/procurement-messages/" + primitive.NewObjectID().Hex(), tok.Token, http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := call(c.path, c.token); got != c.want {
				t.Fatalf("status %d, want %d", got, c.want)
			}
		})
	}
}
