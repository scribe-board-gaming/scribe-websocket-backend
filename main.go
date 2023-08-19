package main

import (
	"net/http"
	"os"
	"scribe-backend/handlers"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	logger.Info().Msg("Starting server")

	router := mux.NewRouter()
	handlers.Routes(router, logger)

	if err := http.ListenAndServe(":8080", router); err != nil {
		logger.Fatal().Err(err).Msg("Error starting server")
	}
}
