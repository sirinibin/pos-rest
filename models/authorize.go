package models

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jameskeane/bcrypt"
)

// Authorize : Authorize structure
type AuthorizeRequest struct {
	Email    string `bson:"email" json:"email"`
	Password string `bson:"password" json:"password"`
}

type AuthCodeResponse struct {
	Code      string `bson:"code" json:"code"`
	ExpiresAt int64  `bson:"expires_at" json:"expires_at"`
}

func AuthenticateByAuthCode(r *http.Request) (tokenClaims TokenClaims, err error) {

	tokenStr, err := ParseAuthCodeFromRequest(r)
	if err != nil {
		return tokenClaims, err
	}

	tokenClaims, err = AuthenticateByJWTToken(tokenStr)
	if err != nil {
		return tokenClaims, err
	}
	if tokenClaims.Type != "auth_code" {
		return tokenClaims, errors.New("Invalid auth code")
	}

	return tokenClaims, nil

}

func ParseAuthCodeFromRequest(r *http.Request) (string, error) {
	keys, ok := r.URL.Query()["auth_code"]
	tokenStr := ""
	if !ok || len(keys[0]) < 1 {
		tokenStr = r.Header.Get("auth_code")
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
		return "", errors.New("auth_code is required")
	}
	return tokenStr, nil
}

// GenerateAuthCode : generate and return authcode
func (auth *AuthorizeRequest) GenerateAuthCode() (authCode AuthCodeResponse, err error) {

	// Generate Auth code
	expiresAt := time.Now().Add(time.Hour * 5) // expiry for auth code is 5min
	token, err := generateAndSaveToken(auth.Email, expiresAt, "auth_code")
	authCode.ExpiresAt = token.ExpiresAt
	authCode.Code = token.TokenStr

	return authCode, err
}

// Validate : Validate authorization data
func (auth *AuthorizeRequest) Authenticate() (errs map[string]string) {

	errs = make(map[string]string)

	if govalidator.IsNull(auth.Email) {
		errs["email"] = "E-mail is required"
	}
	if govalidator.IsNull(auth.Password) {
		errs["password"] = "Password is required"
	}

	if !govalidator.IsNull(auth.Password) && !govalidator.IsNull(auth.Email) {
		user, err := FindUserByEmail(auth.Email)
		if err != nil {
			errs["password"] = "Error finding user record:" + err.Error()
		}

		if user == nil || !bcrypt.Match(auth.Password, user.Password) {
			errs["password"] = "E-mail or Password is wrong"
		}

		if user.Deleted {
			errs["password"] = "Account deleted"
		}
	}

	return errs
}
