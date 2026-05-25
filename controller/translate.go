package controller

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"cloud.google.com/go/translate"
	"github.com/sirinibin/startpos/backend/env"
	"github.com/sirinibin/startpos/backend/models"
	"golang.org/x/text/language"
	"google.golang.org/api/option"
)

type TranslationRequest struct {
	Text string `json:"text"`
}

type TranslationResponse struct {
	TranslatedText string `json:"translatedText"`
}

func TranslateHandler(w http.ResponseWriter, r *http.Request) {
	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		http.Error(w, "UnAuthorized", http.StatusUnauthorized)
		return
	}

	var req TranslationRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// Set up Google Translate client
	//aws: /home/ubuntu/google-account.json
	//local: /Users/sirin/Downloads/startpos-464823-f6608ae0765c.json
	ctx := context.Background()
	client, err := translate.NewClient(ctx, option.WithCredentialsFile(env.Getenv("GOOGLE_APPLICATION_CREDENTIALS", "/home/ubuntu/google-account.json")))
	if err != nil {
		http.Error(w, "Failed to create Google Translate client", http.StatusInternalServerError)
		log.Print("Error creating Google Translate client: ", err)
		return
	}
	defer client.Close()

	targetLang := language.Make("ar")
	resp, err := client.Translate(ctx, []string{req.Text}, targetLang, nil)
	if err != nil {
		http.Error(w, "Failed to translate text", http.StatusInternalServerError)
		log.Print("Error translating text: ", err)
		return
	}

	if len(resp) == 0 {
		http.Error(w, "No translation found", http.StatusInternalServerError)
		return
	}

	response := TranslationResponse{TranslatedText: resp[0].Text}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
