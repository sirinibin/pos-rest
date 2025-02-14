package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/env"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

// Define a struct to hold the JSON response
type PythonResponse struct {
	PrivateKey               string `json:"private_key"`
	Csr                      string `json:"csr"`
	CcsidRequestID           int64  `json:"ccsid_requestID"`
	CcsidBinarySecurityToken string `json:"ccsid_binarySecurityToken"`
	CcsidSecret              string `json:"ccsid_secret"`
	PcsidRequestID           int64  `json:"pcsid_requestID"`
	PcsidBinarySecurityToken string `json:"pcsid_binarySecurityToken"`
	PcsidSecret              string `json:"pcsid_secret"`
	Error                    string `json:"error"`
	Traceback                string `json:"traceback,omitempty"`
}

// ConnectStoreToZatc : handler for POST /store/zatca/connect
func ConnectStoreToZatca(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var zatcaConnectInput *models.ZatcaConnectInput
	// Decode data
	if !utils.Decode(w, r, &zatcaConnectInput) {
		return
	}

	storeID, err := primitive.ObjectIDFromHex(zatcaConnectInput.StoreID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid Store ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["store_id"] = "Error finding store: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Validate data
	if errs := zatcaConnectInput.Validate(w, r); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	/*
		pythonPath := "/Users/sirin/go/src/github.com/sirinibin/ZatcaPython/venv/bin/python" // Change this to match your system
		scriptPath := "csr_and_onboarding.py"                                                // Ensure this path is correct

		cmd := exec.Command(pythonPath, scriptPath)

		// Redirect output and error messages
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			fmt.Println("Error executing Python script:", err)
		}
	*/

	// Run the script with venv activated
	//cmd := exec.Command("bash", "-c", "source /Users/sirin/go/src/github.com/sirinibin/ZatcaPython/venv/bin/activate && python /Users/sirin/go/src/github.com/sirinibin/ZatcaPython/csr_and_onboarding.py")
	//cmd := exec.Command("/opt/homebrew/Cellar/python@3.13/3.13.1/Frameworks/Python.framework/Versions/3.13/bin/python3.13", "/Users/sirin/go/src/github.com/sirinibin/ZatcaPython/csr_and_onboarding.py")

	//1-GUOJ|2-111708|3-4bd41220-f619-47bc-830b-7fedd3b33032
	//uid := "4bd41220-f619-47bc-830b-7fedd3b33032"

	//strings.Split(store.SalesSerialNumber.Prefix, "-")
	serialNumberTemplate := fmt.Sprintf("%s-%0*d", store.SalesSerialNumber.Prefix, store.SalesSerialNumber.PaddingCount, 1)

	parts := strings.Split(serialNumberTemplate, "-")
	serialNumber := ""
	for k, part := range parts {
		serialNumber += strconv.Itoa((k + 1)) + "-" + part + "|"
	}

	serialNumber += strconv.Itoa((len(parts) + 1)) + "-4bd41220-f619-47bc-830b-7fedd3b33032"

	log.Print("serialNumber:" + serialNumber)

	// Create JSON payload
	payload := map[string]interface{}{
		"env":               env.Getenv("ZATCA_ENV", "NonProduction"),
		"otp":               zatcaConnectInput.Otp,
		"crn":               store.RegistrationNumber,
		"serial_number":     serialNumber,
		"vat":               store.VATNo,
		"name":              store.Name,
		"branch_name":       store.BranchName,
		"country_code":      "SA",
		"invoice_type":      "1100",
		"address":           store.Address,
		"business_category": store.BusinessCategory,
	}

	// Convert payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		response.Status = false
		response.Errors["marhsallng_json"] = "Error marshalling JSON: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	pythonBinary := "ZatcaPython/venv/bin/python"
	scriptPath := "ZatcaPython/csr_and_onboarding.py"

	// Create command
	cmd := exec.Command(pythonBinary, scriptPath)

	// Set up pipes
	cmd.Stdin = bytes.NewReader(jsonData) // Send JSON data to stdin
	var output bytes.Buffer
	cmd.Stdout = &output // Capture stdout
	cmd.Stderr = &output // Capture stderr

	// Run the command
	err = cmd.Run()
	if err != nil {
		fmt.Println("Error running Python script:", err)
		response.Status = false
		// Parse JSON response
		var pythonResponse PythonResponse
		err = json.Unmarshal(output.Bytes(), &pythonResponse)
		if err != nil {
			response.Errors["otp"] = "Error parsing error messages from zatca:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		//log.Print(pythonResponse.Error)
		if pythonResponse.Error != "" {
			response.Status = false
			response.Errors["otp"] = "Error connecting to zatac: " + pythonResponse.Error
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		return
	}

	/*
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			fmt.Println("Error executing Python script:", err)
		}
	*/

	// Capture output
	/*
		output, err := cmd.CombinedOutput()
		if err != nil {
			response.Status = false
			// Parse JSON response
			var pythonResponse PythonResponse
			err = json.Unmarshal(output, &pythonResponse)
			if err != nil {
				response.Errors["otp"] = "Error parsing error messages from zatca:" + err.Error()
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(response)
				return
			}

			//log.Print(pythonResponse.Error)
			if pythonResponse.Error != "" {
				response.Status = false
				response.Errors["otp"] = "Error connecting to zatac: " + pythonResponse.Error
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(response)
				return
			}

		}
	*/

	// Parse JSON response
	var pythonResponse PythonResponse
	err = json.Unmarshal(output.Bytes(), &pythonResponse)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	log.Print("pythonResponse:")
	log.Print(pythonResponse)

	if pythonResponse.Error != "" {
		log.Print(pythonResponse.Error)
		response.Status = false
		response.Errors["otp"] = "Error connecting to zatac: " + pythonResponse.Error
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	store.Zatca.Otp = zatcaConnectInput.Otp
	store.Zatca.PrivateKey = pythonResponse.PrivateKey
	store.Zatca.Csr = pythonResponse.Csr

	//compliance
	store.Zatca.ComplianceRequestID = pythonResponse.CcsidRequestID
	store.Zatca.BinarySecurityToken = pythonResponse.CcsidBinarySecurityToken
	store.Zatca.Secret = pythonResponse.CcsidSecret

	//production
	store.Zatca.ProductionRequestID = pythonResponse.PcsidRequestID
	store.Zatca.ProductionBinarySecurityToken = pythonResponse.PcsidBinarySecurityToken
	store.Zatca.ProductionSecret = pythonResponse.PcsidSecret

	store.Zatca.Connected = true
	store.Zatca.ConnectedBy = &userID

	now := time.Now()
	store.Zatca.LastConnectedAt = &now

	err = store.Update()
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["updating_store"] = "Error updating store: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Print parsed JSON values
	//fmt.Println("Message from Python:")
	//log.Print(pythonResponse)

	// Print output
	//fmt.Println(string(output))

	/*
		cmd := exec.Command("/Users/sirin/go/src/github.com/sirinibin/ZatcaPython/venv/bin/python", "csr_and_onboarding.py")

		// Capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		// Print output
		fmt.Println(string(output))
	*/

	response.Status = true
	//response.Result = store

	json.NewEncoder(w).Encode(response)

}

// ConnectStoreToZatc : handler for POST /store/zatca/connect
func DisconnectStoreFromZatca(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var zatcaConnectInput *models.ZatcaConnectInput
	// Decode data
	if !utils.Decode(w, r, &zatcaConnectInput) {
		return
	}

	storeID, err := primitive.ObjectIDFromHex(zatcaConnectInput.StoreID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid Store ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["store_id"] = "Error finding store: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.Zatca.Connected = false
	store.Zatca.DisconnectedBy = &userID

	now := time.Now()
	store.Zatca.LastDisconnectedAt = &now

	err = store.Update()
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["updating_store"] = "Error updating store: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Print parsed JSON values
	//fmt.Println("Message from Python:")
	//log.Print(pythonResponse)

	// Print output
	//fmt.Println(string(output))

	/*
		cmd := exec.Command("/Users/sirin/go/src/github.com/sirinibin/ZatcaPython/venv/bin/python", "csr_and_onboarding.py")

		// Capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		// Print output
		fmt.Println(string(output))
	*/

	response.Status = true
	//response.Result = store

	json.NewEncoder(w).Encode(response)

}
