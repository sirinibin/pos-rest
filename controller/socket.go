package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirinibin/pos-rest/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var upgrader = websocket.Upgrader{}
var mutex = sync.Mutex{}

func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	//log.Print("Inside socket handler")
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("WebSocket Upgrade Error:", err)
		return
	}

	userID := r.URL.Query().Get("userId")
	deviceID := r.URL.Query().Get("deviceId")

	if userID == "" || deviceID == "" {
		log.Println("Missing userId or deviceId in query parameters")
		conn.Close()
		return
	}

	_, err = primitive.ObjectIDFromHex(userID)
	if err != nil {
		fmt.Println("invalid user id: " + err.Error())
		conn.Close()
		return
	}

	mutex.Lock()
	if _, exists := models.Clients[userID]; !exists {
		models.Clients[userID] = make(map[string][]*websocket.Conn)
	}
	models.Clients[userID][deviceID] = append(models.Clients[userID][deviceID], conn)
	mutex.Unlock()
	/*
		fmt.Println("User Connected:", userID)
		fmt.Println("Clients count:")
		fmt.Println(len(models.Clients))
		fmt.Println("Devices count:")
		fmt.Println(len(models.Clients[userID]))
		fmt.Println("Connections count on device: " + deviceID)
		fmt.Println(len(models.Clients[userID][deviceID]))
	*/

	/*
		Emit(userID, deviceID, "role_updated", map[string]string{
			"role":    "Cool",
			"message": "Hello " + userID,
		})*/

	defer conn.Close()
	for {
		messageType, msg, err := conn.ReadMessage()
		if err != nil {
			//fmt.Println("User Disconnected:", userID)
			//fmt.Println("error: " + err.Error())
			mutex.Lock()

			connections := models.Clients[userID][deviceID]
			for i, c := range connections {
				if c == conn {
					models.Clients[userID][deviceID] = append(connections[:i], connections[i+1:]...)
					break
				}
			}

			now := time.Now()
			userObjectID, err := primitive.ObjectIDFromHex(userID)
			if err != nil {
				fmt.Println("invalid user id: " + err.Error())
				return
			}
			user, err := models.FindUserByID(&userObjectID, bson.M{})
			if err != nil {
				fmt.Println("error finding user: " + err.Error())
				return
			}
			_, ok := user.Devices[deviceID]
			if ok {
				deviceData := user.Devices[deviceID]
				deviceData.TabsOpen = len(models.Clients[userID][deviceID])
				if deviceData.TabsOpen == 0 {
					deviceData.Connected = false
					deviceData.LastDisConnectedAt = &now
				}
				user.Devices[deviceID] = deviceData

				oldOnlineStatus := user.Online
				oldConnectedComputers := user.ConnectedComputers
				oldConnectedMobiles := user.ConnectedMobiles
				oldConnectedTabs := user.ConnectedTabs

				user.SetOnlineStatus()
				user.SetDeviceCounts()
				err = user.Update()
				if err != nil {
					fmt.Println("error updating user:", err.Error())
				}

				if user.Online != oldOnlineStatus {
					//log.Print("Online status changed for user after disconnection:" + user.Name)
					err = models.NotifyUserStatusChange()
					if err != nil {
						fmt.Println("error sending status change notification:", err.Error())
					}
				}

				if user.ConnectedComputers != oldConnectedComputers {
					//log.Print("computers count changed for user after disconnection:" + user.Name)
					err = models.NotifyUserDeviceCountChange()
					if err != nil {
						fmt.Println("error sending computer count change notification:", err.Error())
					}
				}

				if user.ConnectedMobiles != oldConnectedMobiles {
					//log.Print("mobiles count changed for user after disconnection:" + user.Name)
					err = models.NotifyUserDeviceCountChange()
					if err != nil {
						fmt.Println("error sending mobile count change notification:", err.Error())
					}
				}

				if user.ConnectedTabs != oldConnectedTabs {
					//log.Print("tabs count changed for user after disconnection:" + user.Name)
					err = models.NotifyUserDeviceCountChange()
					if err != nil {
						fmt.Println("error sending tabs count change notification:", err.Error())
					}
				}
			}

			// Remove device if no connections remain
			if len(models.Clients[userID][deviceID]) == 0 {
				delete(models.Clients[userID], deviceID)
			}

			// Remove user if no devices remain
			if len(models.Clients[userID]) == 0 {
				delete(models.Clients, userID)
			}

			mutex.Unlock()
			//fmt.Println("Remaining devices for", userID, ":", len(models.Clients[userID]))
			//fmt.Println("Remaining Connections for", userID, " and device:", deviceID, ":", len(models.Clients[userID][deviceID]))
			break
		}

		if messageType == websocket.TextMessage {
			userObjectID, err := primitive.ObjectIDFromHex(userID)
			if err != nil {
				fmt.Println("invalid user id: " + err.Error())
				conn.Close()
				return
			}
			user, err := models.FindUserByID(&userObjectID, bson.M{})
			if err != nil {
				fmt.Println("error finding user: " + err.Error())
				return
			}
			now := time.Now()

			var event models.Event
			err = json.Unmarshal(msg, &event)
			if err != nil {
				fmt.Println("Invalid Message Received:", string(msg))
				continue
			}

			//fmt.Println("Event Received:", event.Event)
			//fmt.Println("Data:", event.Data)

			if event.Event == "connection_open" {
				deviceData, err := models.ConvertToDevice(event.Data)
				if err != nil {
					fmt.Println("error marshaling to device: " + err.Error())
				}

				deviceData.LastConnectedAt = &now
				deviceData.Connected = true
				deviceData.TabsOpen = len(models.Clients[userID][deviceID])
				_, ok := user.Devices[deviceID]
				if !ok {
					if len(user.Devices) == 0 {
						user.Devices = make(map[string]*models.Device)
						user.Devices[deviceID] = &models.Device{}
					}
					//log.Print("Adding new device")
				} else {
					//log.Print("Updating existing device")
				}

				user.Devices[deviceID] = &deviceData

				oldOnlineStatus := user.Online
				oldConnectedComputers := user.ConnectedComputers
				oldConnectedMobiles := user.ConnectedMobiles
				oldConnectedTabs := user.ConnectedTabs

				user.SetOnlineStatus()
				user.SetDeviceCounts()
				err = user.Update()
				if err != nil {
					fmt.Println("error updating user:", err.Error())
				}

				if user.Online != oldOnlineStatus {
					//log.Print("Online status changed for user after connection open" + user.Name)
					err = models.NotifyUserStatusChange()
					if err != nil {
						fmt.Println("error sending status change notification:", err.Error())
					}
				}

				if user.ConnectedComputers != oldConnectedComputers {
					//log.Print("computers count changed for user after connection open:" + user.Name)
					err = models.NotifyUserDeviceCountChange()
					if err != nil {
						fmt.Println("error sending computer count change notification:", err.Error())
					}
				}

				if user.ConnectedMobiles != oldConnectedMobiles {
					//log.Print("mobiles count changed for user after connection open:" + user.Name)
					err = models.NotifyUserDeviceCountChange()
					if err != nil {
						fmt.Println("error sending mobile count change notification:", err.Error())
					}
				}

				if user.ConnectedTabs != oldConnectedTabs {
					//log.Print("tabs count changed for user after connection open:" + user.Name)
					err = models.NotifyUserDeviceCountChange()
					if err != nil {
						fmt.Println("error sending tabs count change notification:", err.Error())
					}
				}

			} else if event.Event == "location_update" {
				locationData, err := models.ConvertToLocation(event.Data)
				if err != nil {
					fmt.Println("error marshaling to device: " + err.Error())
				}

				locationData.LastUpdatedAt = &now
				_, ok := user.Devices[deviceID]
				if ok {
					user.Devices[deviceID].Location = locationData
					err = user.Update()
					if err != nil {
						fmt.Println("error updating user:", err.Error())
					}
					//log.Print("Updating existing device location, device id:" + deviceID)
				}
			} else if event.Event == "ping" {
				//log.Print("Ping received")
				err = models.SendPong(conn)
				if err != nil {
					fmt.Println("error sending pong: ", err.Error())
				}

				//models.Emit(userID, deviceID, "pong", map[string]interface{}{"message": "pong"})
			}

			// You can add more custom event handling here...
		}
	}
}
