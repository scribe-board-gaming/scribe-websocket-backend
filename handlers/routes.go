package handlers

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

func Routes(r *mux.Router) {
	hub := NewHub()
	go hub.Run()
	ep := Endpoint{hub: hub}

	r.HandleFunc("/ws/{game}/{password}", ep.WSEndpoint)
	r.HandleFunc("/health", ep.HealthCheck)
}

type Endpoint struct {
	hub *Hub
}

func (ep Endpoint) HealthCheck(w http.ResponseWriter, r *http.Request) {
	hub := ep.hub
	fmt.Fprintf(w, "OK: total games %d", len(hub.games))
}

func (ep Endpoint) WSEndpoint(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
	}

	gameName := mux.Vars(r)["game"]
	password := mux.Vars(r)["password"]

	game := CreateGame(ep.hub, gameName, password)
	if game != nil {
		RegisterGame(ep.hub, game)
	} else {
		game = GetGame(ep.hub, gameName, password)
	}

	fmt.Println(game, gameName, password)
	CreateNewSocketUser(ep.hub, ws, game)
	fmt.Println(game, gameName, password)
}
