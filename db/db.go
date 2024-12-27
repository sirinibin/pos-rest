package db

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var once sync.Once

var (
	clientInstance *mongo.Client
)

// Client : function to get Mongo Client
func Client() *mongo.Client {

	once.Do(func() { // <-- atomic, does not allow repeating
		clientInstance = Connect()
	})

	return clientInstance
}

// Connect : To connect to the mongoDb
func Connect() *mongo.Client {
	//mongo-go-driver settings
	mongoClient, err := GetMongoClient()
	if err != nil {
		panic(err)
	}
	return mongoClient
}

// GetPantahubBaseDB : Get Pantahub Base DB
func GetPosDB() string {
	return Getenv("MONGO_DB", "pos")
}

func Getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

// GetMongoClient : To Get Mongo Client Object
func GetMongoClient() (*mongo.Client, error) {
	MongoDb := GetPosDB()
	user := os.Getenv("MONGO_USER")
	pass := os.Getenv("MONGO_PASS")
	host := Getenv("MONGO_HOST", "localhost")
	port := Getenv("MONGO_PORT", "27017")
	mongoRs := Getenv("MONGO_RS", "")

	//Setting Client Options
	clientOptions := options.Client()
	mongoConnect := "mongodb://"
	if user != "" {
		mongoConnect += user
		if pass != "" {
			mongoConnect += ":"
			mongoConnect += pass
		}
		mongoConnect += "@"
	}
	mongoConnect += host

	if port != "" {
		mongoConnect += ":"
		mongoConnect += port
	}

	mongoConnect += "/?"

	if user != "" {
		mongoConnect += "authSource=" + MongoDb
		mongoConnect += "&authMechanism=SCRAM-SHA-1"
	}

	if mongoRs != "" {
		mongoConnect += "&replicaSet=" + mongoRs
	}

	clientOptions = clientOptions.ApplyURI(mongoConnect)
	if mongoRs != "" {
		clientOptions = clientOptions.SetReplicaSet(mongoRs)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Println("Will connect to mongo PROD db with: " + mongoConnect)
	client, err := mongo.Connect(ctx, clientOptions)

	return client, err
}
