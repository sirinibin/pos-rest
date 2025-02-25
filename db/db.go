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
	connections    = make(map[string]*StoreConnection)
	dbs            = make(map[string]*mongo.Database)
)

type StoreConnection struct {
	Client   *mongo.Client
	LastUsed time.Time
}

func GetDB(dbName string) *mongo.Database {
	if dbName != "" {
		_, dbExists := dbs[dbName]
		connection, connectionExists := connections[dbName]

		if dbExists && connectionExists {
			connection.LastUsed = time.Now()
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
			connection, exists := connections[dbName]
			if !exists {
				connection.Client = Connect(dbName)
				connection.LastUsed = time.Now()
			}
		}
	})

	if dbName != "" {
		connection, connectionExists := connections[dbName]
		if !connectionExists {
			newClient := Connect(dbName)
			connections[dbName] = &StoreConnection{
				Client:   newClient,
				LastUsed: time.Now(),
			}
			return connections[dbName].Client
		}

		connection.LastUsed = time.Now()
		return connection.Client
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

	mongoConnect += "/" + MongoDb + "?"

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

// Periodic cleanup (can be run in a separate goroutine)
func StartCleanupRoutine(interval time.Duration, maxIdleConnectionDuration time.Duration) {
	log.Print("Starting periodic connection cleanup every " + interval.String() + " to delete unused db connections")
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			CloseConnections(maxIdleConnectionDuration)
		}
	}()
}

// Cleanup function to close unused connections
func CloseConnections(maxIdleConnectionDuration time.Duration) {
	mu.Lock()
	defer mu.Unlock()

	for storeID, connection := range connections {
		if time.Since(connection.LastUsed) > maxIdleConnectionDuration {
			_ = connection.Client.Disconnect(context.Background())
			delete(connections, storeID)
			log.Print("Disconnected & Deleted connection from db: " + storeID + " as its idle for last " + time.Since(connection.LastUsed).String())
		} else {
			//log.Print("Max. idle duration not reached for db: " + storeID)
			//log.Print("Last used since: " + time.Since(connection.LastUsed).String())
		}
	}
}
