package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/didip/tollbooth/v7"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

func Routes(r *mux.Router, logger zerolog.Logger) http.Handler {
	hub := NewHub(logger)
	go hub.Run()
	ep := Endpoint{hub: hub, logger: logger}

	wsRouter := r.PathPrefix("/ws").Subrouter()
	wsRouter.HandleFunc("/{game}/{password}", ep.WSConnection)

	r.HandleFunc("/games", ep.currentGames)
	r.HandleFunc("/health", ep.healthCheck)

	lmt := tollbooth.NewLimiter(float64(1), nil)
	lmt.SetIPLookups([]string{"RemoteAddr", "X-Forwarded-For", "X-Real-IP"}).SetMethods([]string{"POST", "PUT", "GET"})

	return tollbooth.LimitHandler(lmt, r)
}

type Endpoint struct {
	hub    *Hub
	logger zerolog.Logger
}

func (ep Endpoint) healthCheck(w http.ResponseWriter, r *http.Request) {
	hub := ep.hub
	gameNames := hub.GetGameNames()
	ep.logger.Info().Msgf("Health check: total games %d, %s", len(hub.games), gameNames)
}

func (ep Endpoint) WSConnection(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		ep.logger.Error().Msgf("Error upgrading connection: %s", err)
	}

	gameName := mux.Vars(r)["game"]
	password := mux.Vars(r)["password"]

	game := CreateGame(ep.hub, gameName, password)
	if game == nil {
		game = GetGame(ep.hub, gameName, password)
	}
	if game == nil {
		ep.logger.Info().Msgf("password did not match for game %s", gameName)
		return
	}

	ep.logger.Info().Msgf("Registering client %s, password %s", gameName, password)
	CreateNewSocketUser(ep.hub, ws, game)
}

func (ep Endpoint) currentGames(w http.ResponseWriter, r *http.Request) {
	gameNames := ep.hub.GetGameNames()
	ep.logger.Info().Msgf("Current games: %s", gameNames)

	w.Header().Set("Content-Type", "application/json")

	type output struct {
		Games []string `json:"games"`
	}

	err := json.NewEncoder(w).Encode(output{Games: gameNames})
	if err != nil {
		ep.logger.Error().Msgf("Error encoding json: %s", err)
		w.Write([]byte(`{"error": "error encoding json"}`))
	}
}
