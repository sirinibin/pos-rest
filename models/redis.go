package models

import (
	"errors"
	"time"

	"github.com/sirinibin/startpos/backend/db"
)

func (token *Token) SaveToRedis() error {
	expires := time.Unix(token.ExpiresAt, 0) //converting Unix to UTC(to Time object)
	now := time.Now()
	errAccess := db.RedisClient.Set(token.AccessUUID, token.UserID, expires.Sub(now)).Err()

	return errAccess
}

func (token *Token) ExistsInRedis() error {

	userID, err := db.RedisClient.Get(token.AccessUUID).Result()
	if err != nil {
		return err
	}

	if token.UserID != userID {
		return errors.New("User id doesn't exist in redis!")
	}

	return nil

}
