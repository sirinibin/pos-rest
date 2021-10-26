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
	router.HandleFunc("/v1/accesstoken", controller.Accesstoken).Methods("POST")

	// Refresh access token
	router.HandleFunc("/v1/refresh", controller.RefreshAccesstoken).Methods("POST")

	//Me
	router.HandleFunc("/v1/me", controller.Me).Methods("GET")
	// Logout
	router.HandleFunc("/v1/logout", controller.LogOut).Methods("DELETE")

	//Business
	router.HandleFunc("/v1/business", controller.CreateBusiness).Methods("POST")
	router.HandleFunc("/v1/business", controller.ListBusiness).Methods("GET")
	router.HandleFunc("/v1/business/{id}", controller.ViewBusiness).Methods("GET")
	router.HandleFunc("/v1/business", controller.UpdateBusiness).Methods("PUT")
	router.HandleFunc("/v1/business/{id}", controller.DeleteBusiness).Methods("DELETE")

	//Customer
	router.HandleFunc("/v1/customer", controller.CreateCustomer).Methods("POST")
	router.HandleFunc("/v1/customer", controller.ListCustomer).Methods("GET")
	router.HandleFunc("/v1/customer/{id}", controller.ViewCustomer).Methods("GET")
	router.HandleFunc("/v1/customer", controller.UpdateCustomer).Methods("PUT")
	router.HandleFunc("/v1/customer/{id}", controller.DeleteCustomer).Methods("DELETE")

	//Product
	router.HandleFunc("/v1/product", controller.CreateProduct).Methods("POST")
	router.HandleFunc("/v1/product", controller.ListProduct).Methods("GET")
	router.HandleFunc("/v1/product/{id}", controller.ViewProduct).Methods("GET")
	router.HandleFunc("/v1/product", controller.UpdateProduct).Methods("PUT")
	router.HandleFunc("/v1/product/{id}", controller.DeleteProduct).Methods("DELETE")

	//ProductCategory
	router.HandleFunc("/v1/product-category", controller.CreateProductCategory).Methods("POST")
	router.HandleFunc("/v1/product-category", controller.ListProductCategory).Methods("GET")
	router.HandleFunc("/v1/product-category/{id}", controller.ViewProductCategory).Methods("GET")
	router.HandleFunc("/v1/product-category", controller.UpdateProductCategory).Methods("PUT")
	router.HandleFunc("/v1/product-category/{id}", controller.DeleteProductCategory).Methods("DELETE")

	//User
	router.HandleFunc("/v1/user", controller.CreateUser).Methods("POST")
	router.HandleFunc("/v1/user", controller.ListUser).Methods("GET")
	router.HandleFunc("/v1/user/{id}", controller.ViewUser).Methods("GET")
	router.HandleFunc("/v1/user", controller.UpdateUser).Methods("PUT")
	router.HandleFunc("/v1/user/{id}", controller.DeleteUser).Methods("DELETE")

	//Signature
	router.HandleFunc("/v1/signature", controller.CreateSignature).Methods("POST")
	router.HandleFunc("/v1/signature", controller.ListSignature).Methods("GET")
	router.HandleFunc("/v1/signature/{id}", controller.ViewSignature).Methods("GET")
	router.HandleFunc("/v1/signature", controller.UpdateSignature).Methods("PUT")
	router.HandleFunc("/v1/signature/{id}", controller.DeleteSignature).Methods("DELETE")

	//Quotation
	router.HandleFunc("/v1/quotation", controller.CreateQuotation).Methods("POST")
	router.HandleFunc("/v1/quotation", controller.ListQuotation).Methods("GET")
	router.HandleFunc("/v1/quotation/{id}", controller.ViewQuotation).Methods("GET")
	router.HandleFunc("/v1/quotation", controller.UpdateQuotation).Methods("PUT")
	router.HandleFunc("/v1/quotation/{id}", controller.DeleteQuotation).Methods("DELETE")

	//Order
	router.HandleFunc("/v1/order", controller.CreateOrder).Methods("POST")
	router.HandleFunc("/v1/order", controller.ListOrder).Methods("GET")
	router.HandleFunc("/v1/order/{id}", controller.ViewOrder).Methods("GET")
	router.HandleFunc("/v1/order", controller.UpdateOrder).Methods("PUT")
	router.HandleFunc("/v1/order/{id}", controller.DeleteOrder).Methods("DELETE")

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
