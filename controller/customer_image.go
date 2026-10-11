package controller

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func UploadCustomerImage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	customerID := r.FormValue("id")
	if customerID == "" {
		http.Error(w, "Customer ID required", http.StatusBadRequest)
		return
	}
	storeID := r.FormValue("storeID")
	if storeID == "" {
		http.Error(w, "Store ID required", http.StatusBadRequest)
		return
	}
	store := imageStore(w, r, storeID)
	if store == nil {
		return
	}
	customerObjectID, err := primitive.ObjectIDFromHex(customerID)
	if err != nil {
		http.Error(w, "invalid customer id", http.StatusBadRequest)
		return
	}
	if !imageRecordExists(store, "customer", &customerObjectID) {
		http.Error(w, "Customer not found", http.StatusNotFound)
		return
	}

	file, handler, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Image file is required:"+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file:"+err.Error(), http.StatusInternalServerError)
		return
	}

	ext := getFileExtension(handler)
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	mimeType := handler.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	relKey := fmt.Sprintf("images/%s/customers/%s/%s", storeID, customerID, filename)
	cdnURL := models.SaveFileToStorage(relKey, fileBytes, mimeType)
	if cdnURL == "" {
		http.Error(w, "Failed to save image", http.StatusInternalServerError)
		return
	}

	err = store.SaveCustomerImage(&customerObjectID, cdnURL)
	if err != nil {
		http.Error(w, "error saving image to db", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"url":"%s"}`, cdnURL)
}

func DeleteCustomerImage(w http.ResponseWriter, r *http.Request) {
	imageUrl := r.URL.Query().Get("url")
	customerID := r.URL.Query().Get("id")
	storeID := r.URL.Query().Get("storeID")

	if imageUrl == "" || customerID == "" || storeID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	store := imageStore(w, r, storeID)
	if store == nil {
		return
	}
	customerObjectID, err := primitive.ObjectIDFromHex(customerID)
	if err != nil {
		http.Error(w, "invalid customer id", http.StatusBadRequest)
		return
	}

	customer, err := store.FindCustomerByID(&customerObjectID, bson.M{})
	if err != nil {
		http.Error(w, "Customer not found", http.StatusNotFound)
		return
	}
	before := len(customer.Images)
	customer.Images = removeItem(customer.Images, imageUrl)
	if len(customer.Images) == before {
		customer.Images = removeItem(customer.Images, filepath.Base(imageUrl))
	}
	if len(customer.Images) < before {
		removeLocalImage(imageUrl) // only a file the record really had
	}
	customer.Update()

	w.WriteHeader(http.StatusOK)
}
