package main

import (
	"log"
	"net/http"
	"os"

	"awaken/src/server"
)

func main() {
	addr := os.Getenv("AWAKEN_CORE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8787"
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: server.NewRouter(),
	}

	log.Printf("awaken-core listening on http://%s", addr)
	log.Fatal(srv.ListenAndServe())
}
