package main

import (
	"log"
	"net/http"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/route"
)

func main() {
	config.InitDatabase()

	port := config.GetPort()
	mux := http.NewServeMux()
	mux.HandleFunc("/", route.URL)

	log.Printf("[CallTag Backend gocroot] Server berjalan pada http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("[CallTag Backend gocroot] Server berhenti: %v\n", err)
	}
}
