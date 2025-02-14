package models

import (
	"net/http"

	"github.com/asaskevich/govalidator"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ZatcaConnectInput struct {
	Otp     string `json:"otp"` //Need to obtain from zatca when going to production level
	StoreID string `json:"id"`
}

func (model *ZatcaConnectInput) Validate(w http.ResponseWriter, r *http.Request) (errs map[string]string) {

	errs = make(map[string]string)

	storeID, err := primitive.ObjectIDFromHex(model.StoreID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["id"] = err.Error()
		return errs
	}

	exists, err := IsStoreExists(&storeID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["id"] = err.Error()
		return errs
	}

	if !exists {
		errs["id"] = "Invalid Store:" + storeID.Hex()
	}

	if govalidator.IsNull(model.Otp) {
		w.WriteHeader(http.StatusBadRequest)
		errs["otp"] = "OTP is required"
	}

	return errs
}
