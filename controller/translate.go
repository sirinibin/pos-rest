package controller

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"cloud.google.com/go/translate"
	"github.com/sirinibin/pos-rest/env"
	"github.com/sirinibin/pos-rest/models"
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

	// Parse the incoming JSON request
	var req TranslationRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	//Adding
	// Set up Google Translate client
	//aws: /home/ubuntu/google-account.json
	//local: /Users/sirin/Downloads/startpos-464823-f6608ae0765c.json
	ctx := context.Background()
	client, err := translate.NewClient(ctx, option.WithCredentialsFile(env.Getenv("GOOGLE_APPLICATION_CREDENTIALS", "/home/ubuntu/google-account.json")))
	if err != nil {
		http.Error(w, "Failed to create Google Translate client", http.StatusInternalServerError)
		log.Print("Error creating client: ", err)
		return
	}
	defer client.Close()
	//API KEY: AIzaSyDgiFpYptFRq5zk873EW9-SgaUv8kcWvKA

	// Perform the translation
	targetLang := language.Make("ar") // Convert target language to language.Tag
	resp, err := client.Translate(ctx, []string{req.Text}, targetLang, nil)
	if err != nil {
		http.Error(w, "Failed to translate text", http.StatusInternalServerError)
		log.Print("Error translating text: ", err)
		return
	}

	// Extract the translated text
	if len(resp) == 0 {
		http.Error(w, "No translation found", http.StatusInternalServerError)
		return
	}
	translatedText := resp[0].Text

	// Return the translated text as JSON
	response := TranslationResponse{TranslatedText: translatedText}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
