package handlers

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"time"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	//maxMessageSize = 512
)

func unRegisterAndCloseConnection(c *Client) {
	c.hub.unregister <- c
	c.webSocketConnection.Close()
}

func setSocketPayloadReadConfig(c *Client) {
	//c.webSocketConnection.SetReadLimit(maxMessageSize)
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

	if game.owner == nil {
		game.owner = client
	}

	hub.logger.Info().Msgf("msg logger %s", client.userID)
	hub.register <- client
	hub.logger.Info().Msgf("msg logger %s", client.userID)

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	var socketEventPayload SocketEventStruct

	defer unRegisterAndCloseConnection(c)

	setSocketPayloadReadConfig(c)

	c.hub.logger.Info().Msgf("Reading payload")
	for {
		_, payload, err := c.webSocketConnection.ReadMessage()

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.hub.logger.Error().Err(err).Interface("client", c.userID).Msgf("Error reading payload, client disconnected")
				break
			}

			c.hub.logger.Error().Err(err).Interface("client", c.userID).Msgf("Error reading payload")
			break
		}
		//if payload == nil {
		//	c.hub.logger.Error().Msgf("Payload is nil")
		//	break
		//}
		c.hub.logger.Info().Msgf("Received payload %s", payload)
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoderErr := decoder.Decode(&socketEventPayload)

		if decoderErr != nil {
			c.hub.logger.Error().Err(decoderErr).Msgf("Error decoding socket payload")
			break
		}

		handleSocketPayloadEvents(c, socketEventPayload)
	}
}

func handleSocketPayloadEvents(client *Client, socketEventPayload SocketEventStruct) {
	switch socketEventPayload.EventName {
	case "sync":
		ownerID := client.game.owner.userID
		if ownerID != client.userID {
			return
		}

		hydrate := socketEventPayload.EventPayload.(map[string]interface{})["hydrate"].(interface{})
		event := SocketEventStruct{
			EventName: "hydrate response",
			EventPayload: map[string]interface{}{
				"message": hydrate,
			},
		}

		var hydrateUsers []string
		for c := range client.game.clients {
			if c.userID == ownerID {
				continue
			}
			hydrateUsers = append(hydrateUsers, c.userID)
		}

		hydrateClients(client, event, hydrateUsers, client.hub.logger)
	case "hydrate":
		hydrateUser := socketEventPayload.EventPayload.(map[string]interface{})["hydrateUser"].(string)
		hydrate := socketEventPayload.EventPayload.(map[string]interface{})["hydrate"].(interface{})
		event := SocketEventStruct{
			EventName: "hydrate response",
			EventPayload: map[string]interface{}{
				"message": hydrate,
			},
		}
		hydrateClients(client, event, []string{hydrateUser}, client.hub.logger)

	case "join":
		event := SocketEventStruct{
			EventName: "join response",
			EventPayload: map[string]interface{}{
				"userID": client.userID,
			},
		}
		connectNewClients(client, event)
	case "disconnect":
		event := SocketEventStruct{
			EventName: "disconnect response",
			EventPayload: map[string]interface{}{
				"userID": client.userID,
			},
		}
		EmitToConnectedClients(client.game, event, client.userID, client.hub.logger)

	case "message":
		event := SocketEventStruct{
			EventName: "message response",
			EventPayload: map[string]interface{}{
				"message": socketEventPayload.EventPayload.(map[string]any),
				"userID":  client.userID,
			},
		}
		EmitToConnectedClients(client.game, event, client.userID, client.hub.logger)
	}
}

func connectNewClients(client *Client, event SocketEventStruct) {
	ownerID := client.game.owner.userID
	for c := range client.game.clients {
		if c.userID == ownerID {
			c.send <- event
		}
	}
}

func hydrateClients(client *Client, socketEventResponse SocketEventStruct, userID []string, logger zerolog.Logger) {
	if client.game.owner.userID != client.userID {
		return
	}
	for client := range client.game.clients {
		for _, id := range userID {
			if client.userID == id {
				logger.Info().Interface("socketEvent", socketEventResponse).Msgf("Emitting to client %s", client.userID)
				client.send <- socketEventResponse
			}
		}
	}
}
func EmitToConnectedClients(game *Game, socketEventResponse SocketEventStruct, userID string, logger zerolog.Logger) {
	for client := range game.clients {
		if client.userID != userID {
			logger.Info().Interface("socketEvent", socketEventResponse).Msgf("Emitting to client %s", client.userID)
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

	hub.mu.Lock()
	defer hub.mu.Unlock()
	game := Game{
		id:         gameName,
		clients:    make(map[*Client]bool),
		password:   password,
		maxClients: 4, // @todo: make this dynamic
	}
	RegisterGame(hub, &game)

	return &game
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
			hub.logger.Info().Msgf("Registering client %s", client.userID)
			game.clients[client] = true
		}
	}

	if client.userID == client.game.owner.userID {
		return
	}
	hub.logger.Info().Msgf("Emitting join event for client %s", client.userID)
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
				hub.logger.Info().Msgf("Unregistering client %s", client.userID)
				delete(game.clients, client)
				close(client.send)

				if len(game.clients) == 0 {
					delete(hub.games, game)
					return
				}

				handleSocketPayloadEvents(client, SocketEventStruct{
					EventName:    "disconnect",
					EventPayload: client.userID,
				})
			}
		}
	}
}
