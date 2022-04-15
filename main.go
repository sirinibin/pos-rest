package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/controller"
	"github.com/sirinibin/pos-rest/db"
	"github.com/sirinibin/pos-rest/env"
	"github.com/sirinibin/pos-rest/models"
)

func main() {
	fmt.Println("A GoLang / Myql Microservice [OAuth2,Redis & JWT used for token management]!")
	db.Client()
	db.InitRedis()

	httpPort := env.Getenv("API_PORT", "2000")
	httpsPort, err := strconv.Atoi(httpPort)
	if err != nil {
		log.Print(err)
		return
	}
	httpsPort = httpsPort + 1

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

	//Store
	router.HandleFunc("/v1/store", controller.CreateStore).Methods("POST")
	router.HandleFunc("/v1/store", controller.ListStore).Methods("GET")
	router.HandleFunc("/v1/store/{id}", controller.ViewStore).Methods("GET")
	router.HandleFunc("/v1/store/{id}", controller.UpdateStore).Methods("PUT")
	router.HandleFunc("/v1/store/{id}", controller.DeleteStore).Methods("DELETE")

	//Customer
	router.HandleFunc("/v1/customer", controller.CreateCustomer).Methods("POST")
	router.HandleFunc("/v1/customer", controller.ListCustomer).Methods("GET")
	router.HandleFunc("/v1/customer/{id}", controller.ViewCustomer).Methods("GET")
	router.HandleFunc("/v1/customer/{id}", controller.UpdateCustomer).Methods("PUT")
	router.HandleFunc("/v1/customer/{id}", controller.DeleteCustomer).Methods("DELETE")

	//Product
	router.HandleFunc("/v1/product", controller.CreateProduct).Methods("POST")
	router.HandleFunc("/v1/product", controller.ListProduct).Methods("GET")
	router.HandleFunc("/v1/product/json", controller.ListProductJson).Methods("GET")
	router.HandleFunc("/v1/product/{id}", controller.ViewProduct).Methods("GET")
	router.HandleFunc("/v1/product/code/{code}", controller.ViewProductByItemCode).Methods("GET")
	router.HandleFunc("/v1/product/barcode/{barcode}", controller.ViewProductByBarCode).Methods("GET")
	router.HandleFunc("/v1/product/{id}", controller.UpdateProduct).Methods("PUT")
	router.HandleFunc("/v1/product/{id}", controller.DeleteProduct).Methods("DELETE")

	//ProductCategory
	router.HandleFunc("/v1/product-category", controller.CreateProductCategory).Methods("POST")
	router.HandleFunc("/v1/product-category", controller.ListProductCategory).Methods("GET")
	router.HandleFunc("/v1/product-category/{id}", controller.ViewProductCategory).Methods("GET")
	router.HandleFunc("/v1/product-category/{id}", controller.UpdateProductCategory).Methods("PUT")
	router.HandleFunc("/v1/product-category/{id}", controller.DeleteProductCategory).Methods("DELETE")

	//User
	router.HandleFunc("/v1/user", controller.CreateUser).Methods("POST")
	router.HandleFunc("/v1/user", controller.ListUser).Methods("GET")
	router.HandleFunc("/v1/user/{id}", controller.ViewUser).Methods("GET")
	router.HandleFunc("/v1/user/{id}", controller.UpdateUser).Methods("PUT")
	router.HandleFunc("/v1/user/{id}", controller.DeleteUser).Methods("DELETE")

	//Signature
	router.HandleFunc("/v1/signature", controller.CreateSignature).Methods("POST")
	router.HandleFunc("/v1/signature", controller.ListSignature).Methods("GET")
	router.HandleFunc("/v1/signature/{id}", controller.ViewSignature).Methods("GET")
	router.HandleFunc("/v1/signature/{id}", controller.UpdateSignature).Methods("PUT")
	router.HandleFunc("/v1/signature/{id}", controller.DeleteSignature).Methods("DELETE")

	//QuotationHistory
	router.HandleFunc("/v1/quotation/history", controller.ListQuotationHistory).Methods("GET")

	//Quotation
	router.HandleFunc("/v1/quotation", controller.CreateQuotation).Methods("POST")
	router.HandleFunc("/v1/quotation", controller.ListQuotation).Methods("GET")
	router.HandleFunc("/v1/quotation/{id}", controller.ViewQuotation).Methods("GET")
	router.HandleFunc("/v1/quotation/{id}", controller.UpdateQuotation).Methods("PUT")
	router.HandleFunc("/v1/quotation/{id}", controller.DeleteQuotation).Methods("DELETE")

	//DeliveryHistory
	router.HandleFunc("/v1/delivery-note/history", controller.ListDeliveryNoteHistory).Methods("GET")

	//DeliveryNote
	router.HandleFunc("/v1/delivery-note", controller.CreateDeliveryNote).Methods("POST")
	router.HandleFunc("/v1/delivery-note", controller.ListDeliveryNote).Methods("GET")
	router.HandleFunc("/v1/delivery-note/{id}", controller.ViewDeliveryNote).Methods("GET")
	router.HandleFunc("/v1/delivery-note/{id}", controller.UpdateDeliveryNote).Methods("PUT")

	//Order
	router.HandleFunc("/v1/order", controller.CreateOrder).Methods("POST")
	router.HandleFunc("/v1/order/{id}", controller.UpdateOrder).Methods("PUT")
	router.HandleFunc("/v1/order", controller.ListOrder).Methods("GET")
	router.HandleFunc("/v1/order/{id}", controller.ViewOrder).Methods("GET")

	//SalesHistory
	router.HandleFunc("/v1/sales/history", controller.ListSalesHistory).Methods("GET")
	//SalesReturnHistory
	router.HandleFunc("/v1/sales-return/history", controller.ListSalesReturnHistory).Methods("GET")
	//PurchaseHistory
	router.HandleFunc("/v1/purchase/history", controller.ListPurchaseHistory).Methods("GET")
	//PurchaseReturnHistory
	router.HandleFunc("/v1/purchase-return/history", controller.ListPurchaseReturnHistory).Methods("GET")

	//SalesReturn
	router.HandleFunc("/v1/sales-return", controller.CreateSalesReturn).Methods("POST")
	router.HandleFunc("/v1/sales-return", controller.ListSalesReturn).Methods("GET")
	router.HandleFunc("/v1/sales-return/{id}", controller.ViewSalesReturn).Methods("GET")
	/*
		router.HandleFunc("/v1/order/{id}", controller.UpdateOrder).Methods("PUT")
		router.HandleFunc("/v1/order/{id}", controller.DeleteOrder).Methods("DELETE")
	*/

	//Vendor
	router.HandleFunc("/v1/vendor", controller.CreateVendor).Methods("POST")
	router.HandleFunc("/v1/vendor", controller.ListVendor).Methods("GET")
	router.HandleFunc("/v1/vendor/{id}", controller.ViewVendor).Methods("GET")
	router.HandleFunc("/v1/vendor/{id}", controller.UpdateVendor).Methods("PUT")
	router.HandleFunc("/v1/vendor/{id}", controller.DeleteVendor).Methods("DELETE")

	//Purchase
	router.HandleFunc("/v1/purchase", controller.CreatePurchase).Methods("POST")
	router.HandleFunc("/v1/purchase", controller.ListPurchase).Methods("GET")
	router.HandleFunc("/v1/purchase/{id}", controller.ViewPurchase).Methods("GET")
	router.HandleFunc("/v1/purchase/{id}", controller.UpdatePurchase).Methods("PUT")
	router.HandleFunc("/v1/purchase/{id}", controller.DeletePurchase).Methods("DELETE")

	//PurchaseCashDiscount
	router.HandleFunc("/v1/purchase-cash-discount", controller.CreatePurchaseCashDiscount).Methods("POST")
	router.HandleFunc("/v1/purchase-cash-discount", controller.ListPurchaseCashDiscount).Methods("GET")
	router.HandleFunc("/v1/purchase-cash-discount/{id}", controller.ViewPurchaseCashDiscount).Methods("GET")
	router.HandleFunc("/v1/purchase-cash-discount/{id}", controller.UpdatePurchaseCashDiscount).Methods("PUT")

	//Purchase Return
	router.HandleFunc("/v1/purchase-return", controller.CreatePurchaseReturn).Methods("POST")
	router.HandleFunc("/v1/purchase-return", controller.ListPurchaseReturn).Methods("GET")
	router.HandleFunc("/v1/purchase-return/{id}", controller.ViewPurchaseReturn).Methods("GET")
	router.HandleFunc("/v1/purchase-return/{id}", controller.UpdatePurchaseReturn).Methods("PUT")
	router.HandleFunc("/v1/purchase-return/{id}", controller.DeletePurchaseReturn).Methods("DELETE")

	//SalesCashDiscount
	router.HandleFunc("/v1/sales-cash-discount", controller.CreateSalesCashDiscount).Methods("POST")
	router.HandleFunc("/v1/sales-cash-discount", controller.ListSalesCashDiscount).Methods("GET")
	router.HandleFunc("/v1/sales-cash-discount/{id}", controller.ViewSalesCashDiscount).Methods("GET")
	router.HandleFunc("/v1/sales-cash-discount/{id}", controller.UpdateSalesCashDiscount).Methods("PUT")

	//SalesPayment
	router.HandleFunc("/v1/sales-payment", controller.CreateSalesPayment).Methods("POST")
	router.HandleFunc("/v1/sales-payment", controller.ListSalesPayment).Methods("GET")
	router.HandleFunc("/v1/sales-payment/{id}", controller.ViewSalesPayment).Methods("GET")
	router.HandleFunc("/v1/sales-payment/{id}", controller.UpdateSalesPayment).Methods("PUT")

	//SalesReturnPayment
	router.HandleFunc("/v1/sales-return-payment", controller.CreateSalesReturnPayment).Methods("POST")
	router.HandleFunc("/v1/sales-return-payment", controller.ListSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.ViewSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.UpdateSalesReturnPayment).Methods("PUT")

	//PurchasePayment
	router.HandleFunc("/v1/purchase-payment", controller.CreatePurchasePayment).Methods("POST")
	router.HandleFunc("/v1/purchase-payment", controller.ListPurchasePayment).Methods("GET")
	router.HandleFunc("/v1/purchase-payment/{id}", controller.ViewPurchasePayment).Methods("GET")
	router.HandleFunc("/v1/purchase-payment/{id}", controller.UpdatePurchasePayment).Methods("PUT")

	//PurchasePayment
	router.HandleFunc("/v1/purchase-return-payment", controller.CreatePurchaseReturnPayment).Methods("POST")
	router.HandleFunc("/v1/purchase-return-payment", controller.ListPurchaseReturnPayment).Methods("GET")
	router.HandleFunc("/v1/purchase-return-payment/{id}", controller.ViewPurchaseReturnPayment).Methods("GET")
	router.HandleFunc("/v1/purchase-return-payment/{id}", controller.UpdatePurchaseReturnPayment).Methods("PUT")

	router.PathPrefix("/images/").Handler(http.StripPrefix("/images/", http.FileServer(http.Dir("./images/"))))
	router.PathPrefix("/html-templates/").Handler(http.StripPrefix("/html-templates/", http.FileServer(http.Dir("./html-templates/"))))

	//cronJobsEveryHour()
	s := gocron.NewScheduler(time.UTC)
	s.Every(8).Hour().Do(cronJobsEveryHour)
	s.StartAsync()

	go func() {
		log.Fatal(http.ListenAndServeTLS(":"+strconv.Itoa(httpsPort), "localhost.cert.pem", "localhost.key.pem", router))

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
			log.Printf("Serving @ https://" + ip.String() + ":" + strconv.Itoa(httpsPort) + " /\n")
			log.Printf("Serving @ http://" + ip.String() + ":" + httpPort + " /\n")
		}
	}
	log.Fatal(http.ListenAndServe(":"+httpPort, router))

}

func cronJobsEveryHour() {
	log.Print("Inside Cron job")
	var err error
	err = models.ClearSalesHistory()
	if err != nil {
		log.Print(err)
	}

	err = models.ClearSalesReturnHistory()
	if err != nil {
		log.Print(err)
	}

	err = models.ClearPurchaseHistory()
	if err != nil {
		log.Print(err)
	}

	err = models.ClearPurchaseReturnHistory()
	if err != nil {
		log.Print(err)
	}

	err = models.ClearQuotationHistory()
	if err != nil {
		log.Print(err)
	}

	err = models.ClearDeliveryNoteHistory()
	if err != nil {
		log.Print(err)
	}

	err = models.ClearSalesReturnHistory()
	if err != nil {
		log.Print(err)
	}

	/*
		err = models.ClearSalesReturnPayments()
		if err != nil {
			log.Print(err)
		}

		err = models.ClearPurchaseReturnPayments()
		if err != nil {
			log.Print(err)
		}
	*/

	err = models.ProcessProducts()
	if err != nil {
		log.Print(err)
	}

	err = models.ProcessOrders()
	if err != nil {
		log.Print(err)
	}

	err = models.ProcessSalesReturns()
	if err != nil {
		log.Print(err)
	}

	err = models.ProcessPurchases()
	if err != nil {
		log.Print(err)
	}

	err = models.ProcessPurchaseReturns()
	if err != nil {
		log.Print(err)
	}

	err = models.ProcessQuotations()
	if err != nil {
		log.Print(err)
	}

	err = models.ProcessDeliveryNotes()
	if err != nil {
		log.Print(err)
	}

}
