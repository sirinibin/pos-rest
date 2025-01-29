package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis"
)

var RedisClient *redis.Client

func InitRedis() {
	var err error
	//Initializing redis
	dsn := os.Getenv("REDIS_DSN")
	if len(dsn) == 0 {
		dsn = "localhost:6379"
	}

	for i := 0; i < 10; i++ { // Retry 5 times
		RedisClient = redis.NewClient(&redis.Options{
			Addr: dsn, //redis port
			//Password: "123",
		})
		_, err = RedisClient.Ping().Result()
		if err == nil {
			//fmt.Println("Connected to Redis")
			log.Printf("Connected to Redis  @localhost:6379")
			//log.Printf("Redis serving @ localhost:6379 /\n")
			return
		}

		fmt.Printf("Redis connection failed: %v. Retrying...\n", err)
		time.Sleep(3 * time.Second) // Wait before retrying
	}

	panic("Could not connect to Redis after 10 retries:" + err.Error())
}
