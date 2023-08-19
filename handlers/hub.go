package handlers

import (
	"github.com/rs/zerolog"
	"sync"
)

type Hub struct {
	register   chan *Client
	unregister chan *Client
	logger     zerolog.Logger

	mu    sync.Mutex
	games map[*Game]bool
}

func NewHub(logger zerolog.Logger) *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		games:      make(map[*Game]bool),
		logger:     logger,
	}
}

func RegisterGame(hub *Hub, game *Game) {
	hub.logger.Info().Msgf("Registering game %s", game.id)
	hub.games[game] = true
}

func (hub *Hub) Run() {
	for {
		select {
		case client := <-hub.register:
			HandleUserRegisterEvent(hub, client)

		case client := <-hub.unregister:
			HandleUserDisconnectEvent(hub, client)
		}
	}
}

func (hub *Hub) GetGameNames() []string {
	var gameNames []string
	for game := range hub.games {
		gameNames = append(gameNames, game.id)
	}

	return gameNames
}
