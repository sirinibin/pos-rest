package models

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ClientsMu guards Clients, in this package and in controller.
var ClientsMu sync.Mutex
var Clients = make(map[string]map[string][]*websocket.Conn) // Store Active User Connections

type Event struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

func NotifyUserStatusChange() error {
	admins, err := GetOnlineAdminUsers()
	if err != nil {
		return err
	}

	for _, user := range admins {
		for _, device := range user.Devices {
			if device.Connected {
				Emit(user.ID.Hex(), device.DeviceID, "user_status_change", nil)
			}
		}
	}

	return nil
}

func NotifyUserDeviceCountChange() error {
	admins, err := GetOnlineAdminUsers()
	if err != nil {
		return err
	}

	for _, user := range admins {
		for _, device := range user.Devices {
			if device.Connected {
				Emit(user.ID.Hex(), device.DeviceID, "user_device_count_change", nil)
			}
		}
	}

	return nil
}

func SendPong(conn *websocket.Conn) error {
	event := "pong"
	data := map[string]interface{}{
		"message": "pong",
	}
	payload := Event{
		Event: event,
		Data:  data,
	}

	jsonData, _ := json.Marshal(payload)

	err := conn.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		conn.Close()
		return err
	}

	return nil
}

func Emit(userID string, deviceID string, event string, data interface{}) {
	ClientsMu.Lock()
	defer ClientsMu.Unlock()

	payload := Event{
		Event: event,
		Data:  data,
	}
	_, ok := Clients[userID]
	if !ok {
		return
	}

	_, ok = Clients[userID][deviceID]
	if !ok {
		return
	}

	connections := Clients[userID][deviceID]
	for _, conn := range connections {
		jsonData, _ := json.Marshal(payload)

		err := conn.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			//fmt.Println("Send Error:", err)
			conn.Close()
		} else {
			//log.Printf("Message Sent: userId=%s, deviceId=%s, event=%s\n", userID, deviceID, event)
		}

	}
}

func ConvertToDevice(data interface{}) (Device, error) {
	var device Device

	// Convert interface{} to JSON bytes
	jsonData, err := json.Marshal(data)
	if err != nil {
		return device, fmt.Errorf("failed to marshal data: %v", err)
	}

	// Convert JSON bytes to Device struct
	err = json.Unmarshal(jsonData, &device)
	if err != nil {
		return device, fmt.Errorf("failed to unmarshal data: %v", err)
	}

	return device, nil
}

func ConvertToLocation(data interface{}) (Location, error) {
	var location Location

	// Convert interface{} to JSON bytes
	jsonData, err := json.Marshal(data)
	if err != nil {
		return location, fmt.Errorf("failed to marshal data: %v", err)
	}

	// Convert JSON bytes to Device struct
	err = json.Unmarshal(jsonData, &location)
	if err != nil {
		return location, fmt.Errorf("failed to unmarshal data: %v", err)
	}

	return location, nil
}

func NotifyUserByID(userID *primitive.ObjectID, event string, data interface{}) error {
	userIDStr := userID.Hex()

	ClientsMu.Lock()
	userDevices, hasUser := Clients[userIDStr]
	deviceIDs := make([]string, 0, len(userDevices))
	for deviceID := range userDevices {
		deviceIDs = append(deviceIDs, deviceID)
	}
	ClientsMu.Unlock()

	if hasUser && len(deviceIDs) > 0 {
		for _, deviceID := range deviceIDs {
			Emit(userIDStr, deviceID, event, data)
		}
		return nil
	}

	// Fallback: use DB device.Connected when user has no in-memory connection
	user, err := FindUserByID(userID, bson.M{})
	if err != nil {
		return err
	}
	for _, device := range user.Devices {
		if device.Connected {
			Emit(user.ID.Hex(), device.DeviceID, event, data)
		}
	}
	return nil
}

func (store *Store) NotifyUsers(event string) error {
	users, err := GetOnlineUsersByStoreID(&store.ID)
	if err != nil {
		return err
	}

	for _, user := range users {
		for _, device := range user.Devices {
			if device.Connected {
				Emit(user.ID.Hex(), device.DeviceID, event, nil)
			}
		}
	}

	return nil
}

// emitToStoreUsers sends an event to all currently connected WebSocket clients that belong to
// the given store. It uses the in-memory Clients map as the source of truth for live connections
// (more reliable than iterating user.Devices from the DB, which can be stale).
func emitToStoreUsers(storeID primitive.ObjectID, event string) {
	// Snapshot connected userIDs under the mutex so we don't hold it during DB calls.
	ClientsMu.Lock()
	userIDs := make([]string, 0, len(Clients))
	for uid := range Clients {
		userIDs = append(userIDs, uid)
	}
	ClientsMu.Unlock()

	for _, userIDStr := range userIDs {
		objID, err := primitive.ObjectIDFromHex(userIDStr)
		if err != nil {
			continue
		}
		user, err := FindUserByID(&objID, bson.M{"store_ids": 1, "role": 1, "admin": 1})
		if err != nil {
			continue
		}
		// Check if user belongs to this store or is an admin.
		belongs := user.Admin || user.Role == "Admin"
		if !belongs {
			for _, sid := range user.StoreIDs {
				if sid != nil && *sid == storeID {
					belongs = true
					break
				}
			}
		}
		if !belongs {
			continue
		}
		// Emit to all open device connections for this user.
		ClientsMu.Lock()
		deviceMap, ok := Clients[userIDStr]
		deviceIDs := make([]string, 0, len(deviceMap))
		if ok {
			for did := range deviceMap {
				deviceIDs = append(deviceIDs, did)
			}
		}
		ClientsMu.Unlock()
		for _, did := range deviceIDs {
			Emit(userIDStr, did, event, nil)
		}
	}
}

// NotifyStoreUsersWAUnread broadcasts "wa_unread_changed" to all connected users of a store.
// Called whenever a new inbound WhatsApp message arrives so the header badge updates in real time.
func NotifyStoreUsersWAUnread(storeID primitive.ObjectID) {
	emitToStoreUsers(storeID, "wa_unread_changed")
}

// NotifyStoreUsersEmailUnread broadcasts "email_unread_changed" to all connected users of a store.
// Called when a new inbound email arrives or when an email is marked as read.
func NotifyStoreUsersEmailUnread(storeID primitive.ObjectID) {
	emitToStoreUsers(storeID, "email_unread_changed")
}
