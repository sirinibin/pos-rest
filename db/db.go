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
	mu             sync.Mutex
	clientInstance *mongo.Client
	connections    = make(map[string]*mongo.Client)
	dbs            = make(map[string]*mongo.Database)
)

func GetDB(dbName string) *mongo.Database {
	if dbName != "" {
		_, dbExists := dbs[dbName]
		_, connectionExists := connections[dbName]

		if dbExists && connectionExists {
			return dbs[dbName]
		}

		dbs[dbName] = Client(dbName).Database(dbName)
		return dbs[dbName]
	} else {
		return Client(GetPosDB()).Database(GetPosDB())
	}
}

// Client : function to get Mongo Client
func Client(dbName string) *mongo.Client {

	once.Do(func() { // <-- atomic, does not allow repeating
		if dbName == "" {
			clientInstance = Connect(GetPosDB())
		} else {
			_, exists := connections[dbName]
			if !exists {
				connections[dbName] = Connect(dbName)
			}
		}
	})

	if dbName != "" {
		_, connectionExists := connections[dbName]
		if !connectionExists {
			connections[dbName] = Connect(dbName)
		}
		return connections[dbName]
	}

	return clientInstance
}

// Connect : To connect to the mongoDb
func Connect(dbName string) *mongo.Client {
	//mongo-go-driver settings
	mongoClient, err := GetMongoClient(dbName)
	if err != nil {
		panic(err)
	}
	return mongoClient
}

// GetMongoClient : To Get Mongo Client Object
func GetMongoClient(dbName string) (*mongo.Client, error) {
	var MongoDb string
	if dbName != "" {
		MongoDb = dbName
	} else {
		MongoDb = GetPosDB()
	}

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

	//log.Println("Trying to connect to mongo db with: " + mongoConnect)
	client, err := mongo.Connect(ctx, clientOptions)
	if err == nil {
		log.Printf("Connected to mongodb with " + mongoConnect)
	}

	return client, err
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

// Cleanup function to close unused connections
func CloseConnections() {
	mu.Lock()
	defer mu.Unlock()

	for storeID, client := range connections {
		_ = client.Disconnect(context.Background())
		delete(connections, storeID)
	}
}

// Periodic cleanup (can be run in a separate goroutine)
func StartCleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			CloseConnections()
		}
	}()
}
