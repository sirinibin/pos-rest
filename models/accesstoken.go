package models

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AccesstokenRequest : Access token request structure
type AccesstokenRequest struct {
	Code string `bson:"auth_code" json:"auth_code"`
}

// RefreshAccesstokenRequest : Refresh Access token request structure
type RefreshAccesstokenRequest struct {
	RefreshToken string `bson:"refresh_token" json:"refresh_token"`
}

type AccessTokenResponse struct {
	Token            string `bson:"access_token" json:"access_token"`
	ExpiresAt        int64  `bson:"expires_at" json:"expires_at"`
	RefreshToken     string `bson:"refresh_token" json:"refresh_token"`
	RefreshExpiresAt int64  `bson:"refresh_expires_at" json:"refresh_expires_at"`
}

func ParseAccessTokenFromRequest(r *http.Request) (string, error) {
	keys, ok := r.URL.Query()["access_token"]
	tokenStr := ""
	if !ok || len(keys[0]) < 1 {
		tokenStr = r.Header.Get("access_token")
	} else {
		tokenStr = keys[0]
	}

	if govalidator.IsNull(tokenStr) {
		bearToken := r.Header.Get("Authorization")
		strArr := strings.Split(bearToken, " ")
		if len(strArr) == 1 {
			tokenStr = strArr[0]
		} else if len(strArr) == 2 {
			tokenStr = strArr[1]
		}
	}

	if govalidator.IsNull(tokenStr) {
		return "", errors.New("access_token is required")
	}
	return tokenStr, nil
}

func ParseRefreshTokenFromRequest(r *http.Request) (string, error) {
	keys, ok := r.URL.Query()["refresh_token"]
	tokenStr := ""
	if !ok || len(keys[0]) < 1 {
		tokenStr = r.Header.Get("refresh_token")
	} else {
		tokenStr = keys[0]
	}

	if govalidator.IsNull(tokenStr) {
		bearToken := r.Header.Get("Authorization")
		strArr := strings.Split(bearToken, " ")
		if len(strArr) == 1 {
			tokenStr = strArr[0]
		} else if len(strArr) == 2 {
			tokenStr = strArr[1]
		}
	}

	if govalidator.IsNull(tokenStr) {
		return "", errors.New("refresh_token is required")
	}
	return tokenStr, nil
}

func AuthenticateByAccessToken(r *http.Request) (tokenClaims TokenClaims, err error) {

	tokenStr, err := ParseAccessTokenFromRequest(r)
	if err != nil {
		return tokenClaims, err

	}

	tokenClaims, err = AuthenticateByJWTToken(tokenStr)
	if err != nil {
		return tokenClaims, err
	}
	if tokenClaims.Type != "access_token" {
		return tokenClaims, errors.New("invalid access token.")
	}
	TokenClaimsObject = tokenClaims
	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return tokenClaims, err
	}

	UserObject, err = FindUserByID(&userID, bson.M{"id": 1, "name": 1, "deleted": 1})
	if err != nil {
		return tokenClaims, err
	}

	if UserObject.Deleted {
		return tokenClaims, errors.New("Account deleted")
	}

	return tokenClaims, nil
}

var TokenClaimsObject TokenClaims
var UserObject *User

func AuthenticateByRefreshToken(r *http.Request) (tokenClaims TokenClaims, err error) {
	tokenStr, err := ParseRefreshTokenFromRequest(r)
	if err != nil {
		return tokenClaims, err

	}

	tokenClaims, err = AuthenticateByJWTToken(tokenStr)
	if err != nil {
		return tokenClaims, err
	}
	if tokenClaims.Type != "refresh_token" {
		return tokenClaims, errors.New("Invalid refresh token.")
	}
	return tokenClaims, nil

}

// GenerateAuthCode : generate and return authcode
func GenerateAccesstoken(email string) (accessToken AccessTokenResponse, err error) {

	// Generate Access token
	expiresAt := time.Now().Add(time.Hour * 24 * 365) // 365 days Expiry
	token, err := generateAndSaveToken(email, expiresAt, "access_token")
	accessToken.ExpiresAt = token.ExpiresAt
	accessToken.Token = token.TokenStr

	// Generate Refresh token
	expiresAt = time.Now().Add(time.Hour * 24 * 7) // 7 days expiry
	token, err = generateAndSaveToken(email, expiresAt, "refresh_token")
	accessToken.RefreshExpiresAt = token.ExpiresAt
	accessToken.RefreshToken = token.TokenStr

	return accessToken, err
}
