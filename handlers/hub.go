package handlers

type Hub struct {
	games      map[*Game]bool
	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		games:      make(map[*Game]bool),
	}
}

func RegisterGame(hub *Hub, game *Game) {
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
