package main

import (
	"log"
	"net/http"
	"scribe-backend/handlers"

	"github.com/gorilla/mux"
)

func main() {
	router := mux.NewRouter()
	handlers.Routes(router)
	log.Fatal(http.ListenAndServe(":8080", router))
}
