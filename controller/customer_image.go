package controller

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirinibin/pos-rest/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func UploadCustomerImage(w http.ResponseWriter, r *http.Request) {
	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20) // 10MB limit
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
		http.Error(w, "Product ID required", http.StatusBadRequest)
		return
	}

	// Get the file
	file, handler, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Image file is required:"+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Ensure upload directory exists
	uploadDir := fmt.Sprintf("./images/customers/%s", customerID)
	err = os.MkdirAll(uploadDir, os.ModePerm)
	if err != nil {
		http.Error(w, "Unable to create upload directory:"+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a unique filename
	ext := getFileExtension(handler)
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	savePath := filepath.Join(uploadDir, filename)
	// Save the file
	out, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Unable to save the file:"+err.Error(), http.StatusInternalServerError)
		return
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		http.Error(w, "Failed to write the file:"+err.Error(), http.StatusInternalServerError)
		return
	}

	storeObjectID, err := primitive.ObjectIDFromHex(storeID)
	if err != nil {
		http.Error(w, "invalid store id", http.StatusInternalServerError)
		return

	}
	store, err := models.FindStoreByID(&storeObjectID, bson.M{})
	if err != nil {
		http.Error(w, "invalid store ", http.StatusInternalServerError)
		return
	}

	customerObjectID, err := primitive.ObjectIDFromHex(customerID)
	if err != nil {
		http.Error(w, "invalid store id", http.StatusInternalServerError)
		return
	}

	imageUrl := fmt.Sprintf("/images/customers/%s/%s", customerID, filename)
	if fileExists(savePath) {
		err = store.SaveCustomerImage(&customerObjectID, imageUrl)
		if err != nil {
			http.Error(w, "error saving image to db", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"url":"%s"}`, imageUrl)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"url":"not saved"}`)
	}
}

func DeleteCustomerImage(w http.ResponseWriter, r *http.Request) {
	imageUrl := r.URL.Query().Get("url")
	customerID := r.URL.Query().Get("id")
	storeID := r.URL.Query().Get("storeID")

	if imageUrl == "" || customerID == "" || storeID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	// Delete file from disk
	filePath := "." + imageUrl
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		http.Error(w, "Error deleting image", http.StatusInternalServerError)
		return
	}

	storeObjectID, err := primitive.ObjectIDFromHex(storeID)
	if err != nil {
		if err != nil {
			http.Error(w, "invalid store id", http.StatusInternalServerError)
			return
		}
	}
	store, err := models.FindStoreByID(&storeObjectID, bson.M{})
	if err != nil {
		if err != nil {
			http.Error(w, "invalid store ", http.StatusInternalServerError)
			return
		}
	}

	customerObjectID, err := primitive.ObjectIDFromHex(customerID)
	if err != nil {
		if err != nil {
			http.Error(w, "invalid store id", http.StatusInternalServerError)
			return
		}
	}

	customer, _ := store.FindCustomerByID(&customerObjectID, bson.M{})
	customer.Images = removeItem(customer.Images, imageUrl)
	customer.Update()

	w.WriteHeader(http.StatusOK)
}
