package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type BICustomQuestion struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Category  string             `bson:"category" json:"category"`
	Question  string             `bson:"question" json:"question"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

func ListBICustomQuestions() ([]BICustomQuestion, error) {
	col := db.GetDB("").Collection("bi_custom_question")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := col.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"created_at": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var questions []BICustomQuestion
	if err := cursor.All(ctx, &questions); err != nil {
		return nil, err
	}
	if questions == nil {
		questions = []BICustomQuestion{}
	}
	return questions, nil
}

func (q *BICustomQuestion) Insert() error {
	col := db.GetDB("").Collection("bi_custom_question")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q.ID = primitive.NewObjectID()
	q.CreatedAt = time.Now().UTC()
	_, err := col.InsertOne(ctx, q)
	return err
}

func DeleteBICustomQuestion(id primitive.ObjectID) error {
	col := db.GetDB("").Collection("bi_custom_question")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := col.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
