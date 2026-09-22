package controller

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func UploadVendorImage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	vendorID := r.FormValue("id")
	if vendorID == "" {
		http.Error(w, "Vendor ID required", http.StatusBadRequest)
		return
	}
	storeID := r.FormValue("storeID")
	if storeID == "" {
		http.Error(w, "Store ID required", http.StatusBadRequest)
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

	storeObjectID, err := primitive.ObjectIDFromHex(storeID)
	if err != nil {
		http.Error(w, "invalid store id", http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjectID, bson.M{})
	if err != nil {
		http.Error(w, "invalid store", http.StatusInternalServerError)
		return
	}
	vendorObjectID, err := primitive.ObjectIDFromHex(vendorID)
	if err != nil {
		http.Error(w, "invalid vendor id", http.StatusBadRequest)
		return
	}

	ext := getFileExtension(handler)
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	mimeType := handler.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	relKey := fmt.Sprintf("images/%s/vendors/%s/%s", storeID, vendorID, filename)
	cdnURL := models.SaveFileToStorage(relKey, fileBytes, mimeType)
	if cdnURL == "" {
		http.Error(w, "Failed to save image", http.StatusInternalServerError)
		return
	}

	err = store.SaveVendorImage(&vendorObjectID, cdnURL)
	if err != nil {
		http.Error(w, "error saving image to db", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"url":"%s"}`, cdnURL)
}

func DeleteVendorImage(w http.ResponseWriter, r *http.Request) {
	imageUrl := r.URL.Query().Get("url")
	vendorID := r.URL.Query().Get("id")
	storeID := r.URL.Query().Get("storeID")

	if imageUrl == "" || vendorID == "" || storeID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	_ = os.Remove("." + imageUrl)

	storeObjectID, err := primitive.ObjectIDFromHex(storeID)
	if err != nil {
		http.Error(w, "invalid store id", http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjectID, bson.M{})
	if err != nil {
		http.Error(w, "invalid store", http.StatusInternalServerError)
		return
	}
	vendorObjectID, err := primitive.ObjectIDFromHex(vendorID)
	if err != nil {
		http.Error(w, "invalid vendor id", http.StatusBadRequest)
		return
	}

	vendor, _ := store.FindVendorByID(&vendorObjectID, bson.M{})
	before := len(vendor.Images)
	vendor.Images = removeItem(vendor.Images, imageUrl)
	if len(vendor.Images) == before {
		vendor.Images = removeItem(vendor.Images, filepath.Base(imageUrl))
	}
	vendor.Update()

	w.WriteHeader(http.StatusOK)
}
