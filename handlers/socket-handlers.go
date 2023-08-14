package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"log"
	"time"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

func unRegisterAndCloseConnection(c *Client) {
	c.hub.unregister <- c
	c.webSocketConnection.Close()
}

func setSocketPayloadReadConfig(c *Client) {
	c.webSocketConnection.SetReadLimit(maxMessageSize)
	c.webSocketConnection.SetReadDeadline(time.Now().Add(pongWait))
	c.webSocketConnection.SetPongHandler(func(string) error { c.webSocketConnection.SetReadDeadline(time.Now().Add(pongWait)); return nil })
}

func CreateNewSocketUser(hub *Hub, connection *websocket.Conn, game *Game) {
	uniqueID := uuid.New()
	client := &Client{
		hub:                 hub,
		webSocketConnection: connection,
		send:                make(chan SocketEventStruct),
		userID:              uniqueID.String(),
		game:                game,
	}

	fmt.Print("New User ID: ", client.game.id)
	hub.register <- client

	fmt.Println("hub register")
	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	var socketEventPayload SocketEventStruct

	defer unRegisterAndCloseConnection(c)

	setSocketPayloadReadConfig(c)

	for {
		_, payload, err := c.webSocketConnection.ReadMessage()

		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoderErr := decoder.Decode(&socketEventPayload)

		if decoderErr != nil {
			log.Printf("error: %v", decoderErr)
			break
		}

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error ===: %v", err)
			}
			break
		}

		handleSocketPayloadEvents(c, socketEventPayload)
	}
}

func handleSocketPayloadEvents(client *Client, socketEventPayload SocketEventStruct) {
	var socketEventResponse SocketEventStruct
	switch socketEventPayload.EventName {
	//case "join":
	//	log.Printf("Join Event triggered")
	//	BroadcastSocketEventToAllClient(client.hub, SocketEventStruct{
	//		EventName: socketEventPayload.EventName,
	//		EventPayload: JoinDisconnectPayload{
	//			UserID: client.userID,
	//			Users:  getAllConnectedUsers(client.hub),
	//		},
	//	})
	//
	//case "disconnect":
	//	log.Printf("Disconnect Event triggered")
	//	BroadcastSocketEventToAllClient(client.hub, SocketEventStruct{
	//		EventName: socketEventPayload.EventName,
	//		EventPayload: JoinDisconnectPayload{
	//			UserID: client.userID,
	//			Users:  getAllConnectedUsers(client.hub),
	//		},
	//	})

	case "message":
		log.Printf("Message Event triggered")
		socketEventResponse.EventName = "message response"
		socketEventResponse.EventPayload = map[string]interface{}{
			"message": socketEventPayload.EventPayload.(map[string]any),
			"userID":  client.userID,
		}
		EmitToConnectedClients(client.game, socketEventResponse, client.userID)
	}
}

func EmitToConnectedClients(game *Game, socketEventResponse SocketEventStruct, userID string) {
	for client := range game.clients {
		if client.userID != userID {
			client.send <- socketEventResponse
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.webSocketConnection.Close()
	}()
	for {
		select {
		case payload, ok := <-c.send:
			reqBodyBytes := new(bytes.Buffer)
			json.NewEncoder(reqBodyBytes).Encode(payload)
			finalPayload := reqBodyBytes.Bytes()

			c.webSocketConnection.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.webSocketConnection.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.webSocketConnection.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			w.Write(finalPayload)

			n := len(c.send)
			for i := 0; i < n; i++ {
				json.NewEncoder(reqBodyBytes).Encode(<-c.send)
				w.Write(reqBodyBytes.Bytes())
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.webSocketConnection.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.webSocketConnection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
func CreateGame(hub *Hub, gameName string, password string) *Game {
	// check if game already exists
	for game := range hub.games {
		if game.id == gameName {
			return nil
		}
	}

	// create new game
	return &Game{
		id:         gameName,
		clients:    make(map[*Client]bool),
		password:   password,
		maxClients: 4, // @todo: make this dynamic
	}
}

func GetGame(hub *Hub, gameName string, password string) *Game {
	// check if game already exists
	for game := range hub.games {
		if game.id == gameName && game.password == password {
			return game
		}
	}

	return nil
}

// HandleUserRegisterEvent will handle the Join event for New socket users
func HandleUserRegisterEvent(hub *Hub, client *Client) {
	for game := range hub.games {
		if game.id == client.game.id {
			game.clients[client] = true
		}
	}
	handleSocketPayloadEvents(client, SocketEventStruct{
		EventName:    "join",
		EventPayload: client.userID,
	})
}

func HandleUserDisconnectEvent(hub *Hub, client *Client) {
	for game := range hub.games {
		if game.id == client.game.id {
			_, ok := game.clients[client]
			if ok {
				delete(game.clients, client)
				close(client.send)

				handleSocketPayloadEvents(client, SocketEventStruct{
					EventName:    "disconnect",
					EventPayload: client.userID,
				})
			}
		}
	}
}
