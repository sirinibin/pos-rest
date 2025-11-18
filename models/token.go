package models

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/sirinibin/startpos/backend/env"
	"github.com/twinj/uuid"
)

type Token struct {
	TokenStr   string
	ExpiresAt  int64
	AccessUUID string
	UserID     string
}

type TokenClaims struct {
	AccessUUID string
	UserID     string
	Authorized bool
	Email      string
	Mob        string
	Exp        int64
	Type       string // values: access_token | refresh_token | auth_code
}

func AuthenticateByJWTToken(tokenStr string) (tokenClaims TokenClaims, err error) {

	jwtToken, err := IsJWTTokenValid(tokenStr)
	if err != nil {
		return tokenClaims, err
	}

	if !jwtToken.Valid {
		return tokenClaims, errors.New("Invalid token")
	}

	tokenClaims, err = getJWTTokenClaims(jwtToken)
	if err != nil {
		return tokenClaims, err
	}
	var token Token
	token.AccessUUID = tokenClaims.AccessUUID
	token.UserID = tokenClaims.UserID

	err = token.ExistsInRedis()
	if err != nil {
		return tokenClaims, err
	}

	return tokenClaims, nil
}

func IsJWTTokenValid(tokenStr string) (*jwt.Token, error) {
	jwtToken, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {

		//Make sure that the token method conform to "SigningMethodHMAC"
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		//tkn, err := jwt.Parse(accesstokenRequest.Code, func(token *jwt.Token) (interface{}, error) {
		return []byte(env.GetJWTAccessSecret()), nil
	})

	return jwtToken, err
}

func getJWTTokenClaims(jwtToken *jwt.Token) (tokenClaims TokenClaims, err error) {

	claims, ok := jwtToken.Claims.(jwt.MapClaims)

	if ok && jwtToken.Valid {

		tokenClaims.Email, ok = claims["email"].(string)
		if !ok {
			return tokenClaims, errors.New("Not able extract email from token")
		}

		tokenClaims.AccessUUID, ok = claims["access_uuid"].(string)
		if !ok {
			return tokenClaims, errors.New("Not able extract access_uuid from token")
		}

		tokenClaims.UserID, ok = claims["user_id"].(string)
		if !ok {
			return tokenClaims, errors.New("Not able extract user_id from token")
		}

		tokenClaims.Authorized, ok = claims["authorized"].(bool)
		if !ok {
			return tokenClaims, errors.New("Not able extract authorized from token")
		}

		var exp uint64

		exp, err = strconv.ParseUint(fmt.Sprintf("%.f", claims["exp"]), 10, 64)
		if err != nil {
			return tokenClaims, errors.New("Not able extract exp from token")
		}
		tokenClaims.Exp = int64(exp)

		tokenClaims.Type, ok = claims["type"].(string)
		if !ok {
			return tokenClaims, errors.New("Not able extract type from token")
		}

	}

	return tokenClaims, err
}

func generateAndSaveToken(email string, expiresAt time.Time, tokenType string) (token Token, err error) {
	token, err = generateJWTToken(email, expiresAt, tokenType)
	if err != nil {
		return token, err
	}

	err = token.SaveToRedis()
	return token, err
}

func generateJWTToken(email string, expiresAt time.Time, tokenType string) (token Token, err error) {

	user, err := FindUserByEmail(email)
	if err != nil && err != sql.ErrNoRows {
		return token, err
	}

	uuidString := uuid.NewV4().String()

	token.ExpiresAt = expiresAt.Unix()
	token.AccessUUID = uuidString
	token.UserID = user.ID.Hex()

	// Setting claim informations
	claims := jwt.MapClaims{}
	claims["authorized"] = true
	claims["access_uuid"] = uuidString
	claims["user_id"] = user.ID
	claims["email"] = user.Email
	claims["exp"] = expiresAt.Unix()
	claims["type"] = tokenType

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	code, err := jwtToken.SignedString([]byte(env.GetJWTAccessSecret()))
	if err != nil {
		return token, err
	}

	token.TokenStr = code

	return token, err
}
