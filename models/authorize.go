package models

import (
	"errors"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jameskeane/bcrypt"
)

//Authorize : Authorize structure
type AuthorizeRequest struct {
	Email    string `bson:"email" json:"email"`
	Password string `bson:"password" json:"password"`
}

type AuthCodeResponse struct {
	Code      string `bson:"code" json:"code"`
	ExpiresAt int64  `bson:"expires_at" json:"expires_at"`
}

func AuthenticateByAuthCode(tokenStr string) (tokenClaims TokenClaims, err error) {

	if govalidator.IsNull(tokenStr) {
		return tokenClaims, errors.New("auth_code is required")
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

//GenerateAuthCode : generate and return authcode
func (auth *AuthorizeRequest) GenerateAuthCode() (authCode AuthCodeResponse, err error) {

	// Generate Auth code
	expiresAt := time.Now().Add(time.Hour * 5) // expiry for auth code is 5min
	token, err := generateAndSaveToken(auth.Email, expiresAt, "auth_code")
	authCode.ExpiresAt = token.ExpiresAt
	authCode.Code = token.TokenStr

	return authCode, err
}

//Validate : Validate authorization data
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
	}

	return errs
}
