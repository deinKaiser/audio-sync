package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Room struct {
	ID      string
	Clients map[*websocket.Conn]bool
	mutex   sync.RWMutex
}

type Hub struct {
	rooms map[string]*Room
	mutex sync.RWMutex
}

var hub = &Hub{
	rooms: make(map[string]*Room),
}

type Message struct {
	Type   string  `json:"type"`
	RoomID string  `json:"roomId"`
	Time   float64 `json:"time"`
	Count  int     `json:"count"`
}

func main() {
	const PORT int = 8080

	if err := os.MkdirAll("uploads", 0755); err != nil {
		log.Fatal("Failed to create uploads directory:", err)
	}

	router := gin.Default()

	router.Static("/static", "./static")

	setupRoutes(router)

	log.Printf("Server starting on :%d", PORT)

	router.Run(fmt.Sprintf(":%d", PORT))
}

func setupRoutes(router *gin.Engine) {
	router.GET("/", handleIndex)
	router.POST("/upload", handleUpload)
	router.GET("/room/:id", handleRoom)
	router.GET("/audio/:id", handleAudio)
	router.GET("/ws/:id", handleWebSocket)
}

func handleIndex(c *gin.Context) {
	c.File("static/index.html")
}

func handleRoom(c *gin.Context) {
	c.File("static/room.html")
}

func handleAudio(c *gin.Context) {
	roomId := c.Param("id")

	files, err := filepath.Glob(filepath.Join("uploads", roomId+".*"))
	if err != nil || len(files) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Audio file not found"})
		return
	}

	c.File(files[0])
}

func handleWebSocket(c *gin.Context) {
	roomID := c.Param("id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	room := getOrCreateRoom(roomID)
	addClientToRoom(room, conn)

	broadcastUserCount(room)

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}

		handleMessage(room, conn, &msg)
	}

	removeClientFromRoom(room, conn)
	broadcastUserCount(room)
}

func getOrCreateRoom(roomID string) *Room {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()

	room, exists := hub.rooms[roomID]
	if !exists {
		room = &Room{
			ID:      roomID,
			Clients: make(map[*websocket.Conn]bool),
		}
		hub.rooms[roomID] = room
	}

	return room
}

func addClientToRoom(room *Room, conn *websocket.Conn) {
	room.mutex.Lock()
	defer room.mutex.Unlock()
	room.Clients[conn] = true
}

func removeClientFromRoom(room *Room, conn *websocket.Conn) {
	room.mutex.Lock()
	defer room.mutex.Unlock()
	delete(room.Clients, conn)

	if len(room.Clients) == 0 {
		hub.mutex.Lock()
		defer hub.mutex.Unlock()
		delete(hub.rooms, room.ID)
	}
}

func broadcastUserCount(room *Room) {
	room.mutex.RLock()
	count := len(room.Clients)
	clients := make([]*websocket.Conn, 0, count)
	for client := range room.Clients {
		clients = append(clients, client)
	}
	room.mutex.RUnlock()

	msg := Message{
		Type:  "user_count",
		Count: count,
	}

	for _, client := range clients {
		client.WriteJSON(msg)
	}
}

func handleMessage(room *Room, sender *websocket.Conn, msg *Message) {
	room.mutex.RLock()
	clients := make([]*websocket.Conn, 0, len(room.Clients))
	for client := range room.Clients {
		if client != sender {
			clients = append(clients, client)
		}
	}
	room.mutex.RUnlock()

	for _, client := range clients {
		client.WriteJSON(msg)
	}
}

func generateRoomID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func handleUpload(c *gin.Context) {
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}
	defer file.Close()

	roomID := generateRoomID()

	ext := filepath.Ext(header.Filename)
	filename := roomID + ext
	filePath := filepath.Join("uploads", filename)

	if err := c.SaveUploadedFile(header, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"roomId":  roomID,
		"message": "File uploaded successfully",
	})
}
