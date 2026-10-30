// An explicit fake-data issuer for local browser verification, never production.
package main

import (
	"flag"
	"github.com/rushikeshg25/runnelix/internal/testoidc"
	"log"
	"net"
	"net/http"
	"time"
)

func main() {
	address := flag.String("address", "127.0.0.1:8900", "literal loopback address")
	redirect := flag.String("redirect", "http://127.0.0.1:5173/v1/auth/callback", "exact browser callback")
	flag.Parse()
	host, _, err := net.SplitHostPort(*address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("fixture requires literal loopback")
	}
	issuer, err := testoidc.New()
	if err != nil {
		log.Fatal("fixture key generation failed")
	}
	issuer.URL = "http://" + *address
	issuer.Redirect = *redirect
	server := &http.Server{Addr: *address, Handler: issuer, ReadHeaderTimeout: 3 * time.Second}
	log.Print("fake-data identity fixture starting")
	log.Fatal(server.ListenAndServe())
}
