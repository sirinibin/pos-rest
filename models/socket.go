package models

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

var mutex = sync.Mutex{}
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
	mutex.Lock()
	defer mutex.Unlock()

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
