package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GET /v1/bi/custom-questions
func ListBICustomQuestions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	questions, err := models.ListBICustomQuestions()
	if err != nil {
		resp.Status = false
		resp.Errors["find"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	resp.Result = questions
	resp.TotalCount = int64(len(questions))
	json.NewEncoder(w).Encode(resp)
}

// POST /v1/bi/custom-questions
func CreateBICustomQuestion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var q models.BICustomQuestion
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		resp.Status = false
		resp.Errors["body"] = "Invalid JSON: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	q.Category = strings.TrimSpace(q.Category)
	q.Question = strings.TrimSpace(q.Question)

	if q.Category == "" || q.Question == "" {
		resp.Status = false
		resp.Errors["validation"] = "category and question are required"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if err := q.Insert(); err != nil {
		resp.Status = false
		resp.Errors["insert"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	resp.Result = q
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// DELETE /v1/bi/custom-questions/{id}
func DeleteBICustomQuestion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		resp.Status = false
		resp.Errors["id"] = "Invalid id"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if err := models.DeleteBICustomQuestion(id); err != nil {
		resp.Status = false
		resp.Errors["delete"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	resp.Result = map[string]string{"deleted": vars["id"]}
	json.NewEncoder(w).Encode(resp)
}
