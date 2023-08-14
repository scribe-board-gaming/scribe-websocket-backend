package handlers

import "github.com/gorilla/websocket"

type Game struct {
	clients    map[*Client]bool
	id         string
	password   string
	maxClients int
}

type Client struct {
	hub                 *Hub
	webSocketConnection *websocket.Conn
	send                chan SocketEventStruct
	username            string
	userID              string
	game                *Game
}

type SocketEventStruct struct {
	EventName    string `json:"eventName"`
	EventPayload any    `json:"eventPayload"`
}
