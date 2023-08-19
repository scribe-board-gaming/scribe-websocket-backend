# Scribe-backend
Scribe-backend is a backend service for scribe.gg, it provides a websocket connection for the frontend to connect to and a REST API for the frontend to query.

The websocket is used to proxy data from clients in real time to other clients. The REST API is used to query data to see what is currently happening in the game.

## Installation
`make run`

## Usage
`make run`

In your scribe app of choice, connect to the websocket at `ws://localhost:8080/ws`
or in insomnia, postman, or your browser, query the REST API at `http://localhost:8080/games`

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

## TODO
- [ ] Add tests
- [ ] Add CI/CD
- [ ] Add documentation
- [ ] Add redis caching so game can be hosted on multiple servers