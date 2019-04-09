package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/koofr/go-dropboxclient/mockdropbox"
)

func main() {
	var addr string
	flag.StringVar(&addr, "addr", "localhost:7162", "Listen address")
	flag.Parse()

	handler := mockdropbox.New()

	log.Printf("MockDropbox server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}
