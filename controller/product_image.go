package controller

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirinibin/pos-rest/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

func UploadProductImage(w http.ResponseWriter, r *http.Request) {
	// Parse the multipart form
	err := r.ParseMultipartForm(10 << 20) // 10MB limit
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	productID := r.FormValue("productID")
	if productID == "" {
		http.Error(w, "Product ID required", http.StatusBadRequest)
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
	uploadDir := fmt.Sprintf("./images/products/%s", productID)
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

	productObjectID, err := primitive.ObjectIDFromHex(productID)
	if err != nil {
		http.Error(w, "invalid store id", http.StatusInternalServerError)
		return
	}

	imageUrl := fmt.Sprintf("/images/products/%s/%s", productID, filename)
	if fileExists(savePath) {
		err = store.SaveProductImage(&productObjectID, imageUrl)
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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func DeleteProductImage(w http.ResponseWriter, r *http.Request) {
	imageUrl := r.URL.Query().Get("url")
	productID := r.URL.Query().Get("productID")
	storeID := r.URL.Query().Get("storeID")

	if imageUrl == "" || productID == "" || storeID == "" {
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

	productObjectID, err := primitive.ObjectIDFromHex(productID)
	if err != nil {
		if err != nil {
			http.Error(w, "invalid store id", http.StatusInternalServerError)
			return
		}
	}

	product, _ := store.FindProductByID(&productObjectID, bson.M{})
	product.Images = removeItem(product.Images, imageUrl)
	product.Update(&store.ID)

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

	// Fallback: detect from MIME
	mimeType := handler.Header.Get("Content-Type")
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin" // fallback to avoid missing extension
	}
}
