package controller

// mcp_auth.go — MCP auth & session endpoints under /v1/mcp/
//
//   POST /v1/mcp/login   — email/password → access_token + stores
//   GET  /v1/mcp/stores  — list stores for the authenticated user
//   GET  /v1/mcp/me      — current user profile (slim)

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPLogin handles POST /v1/mcp/login
// Body: {"email":"...","password":"..."}
// Returns: access_token, active_store, stores[]
func MCPLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		mcpWriteError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Email == "" || body.Password == "" {
		mcpWriteError(w, "email and password are required", http.StatusBadRequest)
		return
	}

	// Step 1: authenticate credentials
	auth := &models.AuthorizeRequest{Email: body.Email, Password: body.Password}
	if errs := auth.Authenticate(); len(errs) > 0 {
		msg := "invalid credentials"
		for _, v := range errs {
			msg = v
			break
		}
		mcpWriteError(w, msg, http.StatusUnauthorized)
		return
	}

	// Step 2: generate access token directly (skip the auth-code round-trip)
	accessToken, err := models.GenerateAccesstoken(body.Email)
	if err != nil {
		mcpWriteError(w, "failed to generate access token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 3: fetch all stores
	stores, err := models.GetAllStores()
	if err != nil {
		mcpWriteError(w, "failed to fetch stores: "+err.Error(), http.StatusInternalServerError)
		return
	}

	type storeItem struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		BranchName  string `json:"branch_name"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
	}
	storeList := make([]storeItem, 0, len(stores))
	for _, s := range stores {
		storeList = append(storeList, storeItem{
			ID:          s.ID.Hex(),
			Name:        s.Name,
			BranchName:  s.BranchName,
			Country:     s.CountryName,
			CountryCode: s.CountryCode,
		})
	}

	note := ""
	if len(storeList) > 1 {
		note = "First store auto-selected. Use store_id param to switch."
	}
	activeStore := map[string]string{}
	if len(storeList) > 0 {
		activeStore = map[string]string{
			"id":           storeList[0].ID,
			"name":         storeList[0].Name,
			"branch_name":  storeList[0].BranchName,
			"country_code": storeList[0].CountryCode,
		}
	}

	mcpWriteJSON(w, map[string]interface{}{
		"status":       "ok",
		"access_token": accessToken.Token,
		"active_store": activeStore,
		"total_stores": len(storeList),
		"stores":       storeList,
		"note":         note,
	})
}

// MCPListStores handles GET /v1/mcp/stores
func MCPListStores(w http.ResponseWriter, r *http.Request) {
	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		mcpWriteError(w, "Invalid access token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	stores, err := models.GetAllStores()
	if err != nil {
		mcpWriteError(w, "failed to fetch stores: "+err.Error(), http.StatusInternalServerError)
		return
	}

	activeStoreID := r.URL.Query().Get("active_store_id")

	type storeItem struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		BranchName  string `json:"branch_name"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Active      bool   `json:"active"`
	}
	list := make([]storeItem, 0, len(stores))
	for _, s := range stores {
		list = append(list, storeItem{
			ID:          s.ID.Hex(),
			Name:        s.Name,
			BranchName:  s.BranchName,
			Country:     s.CountryName,
			CountryCode: s.CountryCode,
			Active:      s.ID.Hex() == activeStoreID,
		})
	}

	mcpWriteJSON(w, map[string]interface{}{
		"total":           len(list),
		"active_store_id": activeStoreID,
		"stores":          list,
	})
}

// MCPMe handles GET /v1/mcp/me
func MCPMe(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		mcpWriteError(w, "Invalid access token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		mcpWriteError(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	user, err := models.FindUserByID(&userID, bson.M{})
	if err != nil {
		mcpWriteError(w, "user not found: "+err.Error(), http.StatusInternalServerError)
		return
	}

	storeIDs := make([]string, 0)
	for _, sid := range user.StoreIDs {
		if sid != nil {
			storeIDs = append(storeIDs, sid.Hex())
		}
	}

	mcpWriteJSON(w, map[string]interface{}{
		"id":          user.ID.Hex(),
		"name":        user.Name,
		"email":       user.Email,
		"mobile":      user.Mob,
		"role":        user.Role,
		"admin":       user.Admin,
		"store_ids":   storeIDs,
		"store_names": user.StoreNames,
	})
}
