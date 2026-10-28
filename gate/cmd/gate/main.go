package main

import (
	"log"
	"net/http"
	"os"

	"portage/gate/internal/server"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("gate listening on :%s", port)
	if err := http.ListenAndServe(":"+port, server.New()); err != nil {
		log.Fatal(err)
	}
}
