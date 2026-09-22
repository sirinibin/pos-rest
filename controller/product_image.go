package controller

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func UploadProductImage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	productID := r.FormValue("id")
	if productID == "" {
		http.Error(w, "Product ID required", http.StatusBadRequest)
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
	productObjectID, err := primitive.ObjectIDFromHex(productID)
	if err != nil {
		http.Error(w, "invalid product id", http.StatusBadRequest)
		return
	}

	ext := getFileExtension(handler)
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	mimeType := handler.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	relKey := fmt.Sprintf("images/%s/products/%s/%s", storeID, productID, filename)
	cdnURL := models.SaveFileToStorage(relKey, fileBytes, mimeType)
	if cdnURL == "" {
		http.Error(w, "Failed to save image", http.StatusInternalServerError)
		return
	}

	err = store.SaveProductImage(&productObjectID, cdnURL)
	if err != nil {
		http.Error(w, "error saving image to db:"+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"url":"%s"}`, cdnURL)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func DeleteProductImage(w http.ResponseWriter, r *http.Request) {
	imageUrl := r.URL.Query().Get("url")
	productID := r.URL.Query().Get("id")
	storeID := r.URL.Query().Get("storeID")

	if imageUrl == "" || productID == "" || storeID == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	// Best-effort local file removal (no-op for S3/cdn URLs)
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
	productObjectID, err := primitive.ObjectIDFromHex(productID)
	if err != nil {
		http.Error(w, "invalid product id", http.StatusBadRequest)
		return
	}

	product, _ := store.FindProductByID(&productObjectID, bson.M{})
	before := len(product.Images)
	product.Images = removeItem(product.Images, imageUrl) // exact match for /cdn/ URLs
	if len(product.Images) == before {
		product.Images = removeItem(product.Images, filepath.Base(imageUrl)) // basename for old records
	}
	_ = product.Update(&store.ID)

	w.WriteHeader(http.StatusOK)
}

func removeItem(slice []string, item string) []string {
	for i, v := range slice {
		if v == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// Helper to get extension
func getFileExtension(handler *multipart.FileHeader) string {
	ext := filepath.Ext(handler.Filename)
	if ext != "" {
		return ext
	}
	switch handler.Header.Get("Content-Type") {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}
