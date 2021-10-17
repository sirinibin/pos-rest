package main

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/controller"
	"github.com/sirinibin/pos-rest/db"
)

func main() {
	fmt.Println("A GoLang / Myql Microservice [OAuth2,Redis & JWT used for token management]!")
	db.InitRedis()

	router := mux.NewRouter()

	//API Info
	router.HandleFunc("/", controller.APIInfo).Methods("GET")

	// Register a new user account
	router.HandleFunc("/v1/register", controller.Register).Methods("POST")

	// OAuth2 Authentication
	router.HandleFunc("/v1/authorize", controller.Authorize).Methods("POST")

	go func() {
		log.Fatal(http.ListenAndServeTLS(":2001", "localhost.cert.pem", "localhost.key.pem", router))
	}()

	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			log.Printf("Serving @ https://" + ip.String() + ":2001 /\n")
			log.Printf("Serving @ http://" + ip.String() + ":2000 /\n")
		}
	}
	log.Fatal(http.ListenAndServe(":2000", router))

}
