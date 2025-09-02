package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-co-op/gocron"
	socketio "github.com/googollee/go-socket.io"
	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"github.com/sirinibin/pos-rest/controller"
	"github.com/sirinibin/pos-rest/db"
	"github.com/sirinibin/pos-rest/env"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var socketServer *socketio.Server

// testing
func main() {
	fmt.Println("Start POS Restful API")
	db.Client("")
	db.InitRedis()
	go db.StartCleanupRoutine(1*time.Minute, 20*time.Minute)
	//go models.SetIndexes()

	httpPort := env.Getenv("API_PORT", "2000")
	httpsPort, err := strconv.Atoi(httpPort)
	if err != nil {
		log.Print(err)
		return
	}
	httpsPort = httpsPort + 1

	router := mux.NewRouter()

	//API Info
	router.HandleFunc("/v1/info", controller.APIInfo).Methods("GET")

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
	router.HandleFunc("/v1/customer/restore/{id}", controller.RestoreCustomer).Methods("POST")
	router.HandleFunc("/v1/customer/upload-image", controller.UploadCustomerImage).Methods("POST")
	router.HandleFunc("/v1/customer/delete-image", controller.DeleteCustomerImage).Methods("POST")

	//Product
	router.HandleFunc("/v1/product", controller.CreateProduct).Methods("POST")
	router.HandleFunc("/v1/product", controller.ListProduct).Methods("GET")
	router.HandleFunc("/v1/product/json", controller.ListProductJson).Methods("GET")
	router.HandleFunc("/v1/product/{id}", controller.ViewProduct).Methods("GET")
	router.HandleFunc("/v1/product/code/{code}", controller.ViewProductByItemCode).Methods("GET")
	router.HandleFunc("/v1/product/barcode/{barcode}", controller.ViewProductByBarCode).Methods("GET")
	router.HandleFunc("/v1/product/{id}", controller.UpdateProduct).Methods("PUT")
	router.HandleFunc("/v1/product/{id}", controller.DeleteProduct).Methods("DELETE")
	router.HandleFunc("/v1/product/restore/{id}", controller.RestoreProduct).Methods("POST")
	router.HandleFunc("/v1/product/upload-image", controller.UploadProductImage).Methods("POST")
	router.HandleFunc("/v1/product/delete-image", controller.DeleteProductImage).Methods("POST")

	//ProductHistory
	router.HandleFunc("/v1/product/history/{id}", controller.ListProductHistory).Methods("GET")

	//Expense
	router.HandleFunc("/v1/expense", controller.CreateExpense).Methods("POST")
	router.HandleFunc("/v1/expense", controller.ListExpense).Methods("GET")
	router.HandleFunc("/v1/expense/{id}", controller.ViewExpense).Methods("GET")
	router.HandleFunc("/v1/expense/code/{code}", controller.ViewExpenseByCode).Methods("GET")
	router.HandleFunc("/v1/expense/{id}", controller.UpdateExpense).Methods("PUT")
	router.HandleFunc("/v1/expense/{id}", controller.DeleteExpense).Methods("DELETE")

	//CustomeDeposit
	router.HandleFunc("/v1/customer-deposit", controller.CreateCustomerDeposit).Methods("POST")
	router.HandleFunc("/v1/customer-deposit", controller.ListCustomerDeposit).Methods("GET")
	router.HandleFunc("/v1/customer-deposit/{id}", controller.ViewCustomerDeposit).Methods("GET")
	router.HandleFunc("/v1/customer-deposit/code/{code}", controller.ViewCustomerDepositByCode).Methods("GET")
	router.HandleFunc("/v1/customer-deposit/{id}", controller.UpdateCustomerDeposit).Methods("PUT")
	router.HandleFunc("/v1/customer-deposit/{id}", controller.DeleteCustomerDeposit).Methods("DELETE")

	//CustomeWithdrawal
	router.HandleFunc("/v1/customer-withdrawal", controller.CreateCustomerWithdrawal).Methods("POST")
	router.HandleFunc("/v1/customer-withdrawal", controller.ListCustomerWithdrawal).Methods("GET")
	router.HandleFunc("/v1/customer-withdrawal/{id}", controller.ViewCustomerWithdrawal).Methods("GET")
	router.HandleFunc("/v1/customer-withdrawal/code/{code}", controller.ViewCustomerWithdrawalByCode).Methods("GET")
	router.HandleFunc("/v1/customer-withdrawal/{id}", controller.UpdateCustomerWithdrawal).Methods("PUT")
	router.HandleFunc("/v1/customer-withdrawal/{id}", controller.DeleteCustomerWithdrawal).Methods("DELETE")

	//CapitalWithdrawal
	router.HandleFunc("/v1/capital-withdrawal", controller.CreateCapitalWithdrawal).Methods("POST")
	router.HandleFunc("/v1/capital-withdrawal", controller.ListCapitalWithdrawal).Methods("GET")
	router.HandleFunc("/v1/capital-withdrawal/{id}", controller.ViewCapitalWithdrawal).Methods("GET")
	router.HandleFunc("/v1/capital-withdrawal/code/{code}", controller.ViewCapitalWithdrawalByCode).Methods("GET")
	router.HandleFunc("/v1/capital-withdrawal/{id}", controller.UpdateCapitalWithdrawal).Methods("PUT")
	router.HandleFunc("/v1/capital-withdrawal/{id}", controller.DeleteCapitalWithdrawal).Methods("DELETE")

	//Capital
	router.HandleFunc("/v1/capital", controller.CreateCapital).Methods("POST")
	router.HandleFunc("/v1/capital", controller.ListCapital).Methods("GET")
	router.HandleFunc("/v1/capital/{id}", controller.ViewCapital).Methods("GET")
	router.HandleFunc("/v1/capital/code/{code}", controller.ViewCapitalByCode).Methods("GET")
	router.HandleFunc("/v1/capital/{id}", controller.UpdateCapital).Methods("PUT")
	router.HandleFunc("/v1/capital/{id}", controller.DeleteCapital).Methods("DELETE")

	//Divident
	router.HandleFunc("/v1/divident", controller.CreateDivident).Methods("POST")
	router.HandleFunc("/v1/divident", controller.ListDivident).Methods("GET")
	router.HandleFunc("/v1/divident/{id}", controller.ViewDivident).Methods("GET")
	router.HandleFunc("/v1/divident/code/{code}", controller.ViewDividentByCode).Methods("GET")
	router.HandleFunc("/v1/divident/{id}", controller.UpdateDivident).Methods("PUT")
	router.HandleFunc("/v1/divident/{id}", controller.DeleteDivident).Methods("DELETE")

	//ProductCategory
	router.HandleFunc("/v1/product-category", controller.CreateProductCategory).Methods("POST")
	router.HandleFunc("/v1/product-category", controller.ListProductCategory).Methods("GET")
	router.HandleFunc("/v1/product-category/{id}", controller.ViewProductCategory).Methods("GET")
	router.HandleFunc("/v1/product-category/{id}", controller.UpdateProductCategory).Methods("PUT")
	router.HandleFunc("/v1/product-category/{id}", controller.DeleteProductCategory).Methods("DELETE")
	router.HandleFunc("/v1/product-category/restore/{id}", controller.RestoreProductCategory).Methods("POST")

	//ProductBrand
	router.HandleFunc("/v1/product-brand", controller.CreateProductBrand).Methods("POST")
	router.HandleFunc("/v1/product-brand", controller.ListProductBrand).Methods("GET")
	router.HandleFunc("/v1/product-brand/{id}", controller.ViewProductBrand).Methods("GET")
	router.HandleFunc("/v1/product-brand/{id}", controller.UpdateProductBrand).Methods("PUT")
	router.HandleFunc("/v1/product-brand/{id}", controller.DeleteProductBrand).Methods("DELETE")
	router.HandleFunc("/v1/product-brand/restore/{id}", controller.RestoreProductBrand).Methods("POST")

	//ExpenseCategory
	router.HandleFunc("/v1/expense-category", controller.CreateExpenseCategory).Methods("POST")
	router.HandleFunc("/v1/expense-category", controller.ListExpenseCategory).Methods("GET")
	router.HandleFunc("/v1/expense-category/{id}", controller.ViewExpenseCategory).Methods("GET")
	router.HandleFunc("/v1/expense-category/{id}", controller.UpdateExpenseCategory).Methods("PUT")
	router.HandleFunc("/v1/expense-category/{id}", controller.DeleteExpenseCategory).Methods("DELETE")

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
	router.HandleFunc("/v1/quotation/calculate-net-total", controller.CalculateQuotationNetTotal).Methods("POST")
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
	router.HandleFunc("/v1/order/calculate-net-total", controller.CalculateSalesNetTotal).Methods("POST")
	router.HandleFunc("/v1/order/{id}", controller.UpdateOrder).Methods("PUT")
	router.HandleFunc("/v1/order", controller.ListOrder).Methods("GET")
	router.HandleFunc("/v1/order/{id}", controller.ViewOrder).Methods("GET")
	router.HandleFunc("/v1/previous-order/{id}", controller.ViewPreviousOrder).Methods("GET")
	router.HandleFunc("/v1/next-order/{id}", controller.ViewNextOrder).Methods("GET")
	router.HandleFunc("/v1/last-order", controller.ViewLastOrder).Methods("GET")

	//SalesHistory
	router.HandleFunc("/v1/sales/history", controller.ListSalesHistory).Methods("GET")
	//SalesReturnHistory
	router.HandleFunc("/v1/sales-return/history", controller.ListSalesReturnHistory).Methods("GET")
	//QuotationSalesReturnHistory
	router.HandleFunc("/v1/quotation-sales-return/history", controller.ListQuotationSalesReturnHistory).Methods("GET")

	//PurchaseHistory
	router.HandleFunc("/v1/purchase/history", controller.ListPurchaseHistory).Methods("GET")
	//PurchaseReturnHistory
	router.HandleFunc("/v1/purchase-return/history", controller.ListPurchaseReturnHistory).Methods("GET")
	//upload image
	router.HandleFunc("/v1/purchase/upload/image", controller.ParsePurchaseBill).Methods("POST")

	//SalesReturn
	router.HandleFunc("/v1/sales-return", controller.CreateSalesReturn).Methods("POST")
	router.HandleFunc("/v1/sales-return/calculate-net-total", controller.CalculateSalesReturnNetTotal).Methods("POST")
	router.HandleFunc("/v1/sales-return/{id}", controller.UpdateSalesReturn).Methods("PUT")
	router.HandleFunc("/v1/sales-return", controller.ListSalesReturn).Methods("GET")
	router.HandleFunc("/v1/sales-return/{id}", controller.ViewSalesReturn).Methods("GET")

	//QuotationSalesReturn
	router.HandleFunc("/v1/quotation-sales-return", controller.CreateQuotationSalesReturn).Methods("POST")
	router.HandleFunc("/v1/quotation-sales-return/calculate-net-total", controller.CalculateQuotationSalesReturnNetTotal).Methods("POST")
	router.HandleFunc("/v1/quotation-sales-return/{id}", controller.UpdateQuotationSalesReturn).Methods("PUT")
	router.HandleFunc("/v1/quotation-sales-return", controller.ListQuotationSalesReturn).Methods("GET")
	router.HandleFunc("/v1/quotation-sales-return/{id}", controller.ViewQuotationSalesReturn).Methods("GET")
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
	router.HandleFunc("/v1/vendor/restore/{id}", controller.RestoreVendor).Methods("POST")
	router.HandleFunc("/v1/vendor/upload-image", controller.UploadVendorImage).Methods("POST")
	router.HandleFunc("/v1/vendor/delete-image", controller.DeleteVendorImage).Methods("POST")

	//Purchase
	router.HandleFunc("/v1/purchase", controller.CreatePurchase).Methods("POST")
	router.HandleFunc("/v1/purchase/calculate-net-total", controller.CalculatePurchaseNetTotal).Methods("POST")
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
	router.HandleFunc("/v1/purchase-return/calculate-net-total", controller.CalculatePurchaseReturnNetTotal).Methods("POST")
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
	router.HandleFunc("/v1/sales-payment/{id}", controller.DeleteSalesPayment).Methods("DELETE")

	//SalesReturnPayment
	router.HandleFunc("/v1/sales-return-payment", controller.CreateSalesReturnPayment).Methods("POST")
	router.HandleFunc("/v1/sales-return-payment", controller.ListSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.ViewSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.UpdateSalesReturnPayment).Methods("PUT")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.DeleteSalesReturnPayment).Methods("DELETE")

	//QuotationSalesReturnPayment
	router.HandleFunc("/v1/quotation-sales-return-payment", controller.CreateQuotationSalesReturnPayment).Methods("POST")
	router.HandleFunc("/v1/quotation-sales-return-payment", controller.ListQuotationSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/quotation-sales-return-payment/{id}", controller.ViewQuotationSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/quotation-sales-return-payment/{id}", controller.UpdateQuotationSalesReturnPayment).Methods("PUT")
	router.HandleFunc("/v1/quotation-sales-return-payment/{id}", controller.DeleteQuotationSalesReturnPayment).Methods("DELETE")

	//PurchasePayment
	router.HandleFunc("/v1/purchase-payment", controller.CreatePurchasePayment).Methods("POST")
	router.HandleFunc("/v1/purchase-payment", controller.ListPurchasePayment).Methods("GET")
	router.HandleFunc("/v1/purchase-payment/{id}", controller.ViewPurchasePayment).Methods("GET")
	router.HandleFunc("/v1/purchase-payment/{id}", controller.UpdatePurchasePayment).Methods("PUT")
	router.HandleFunc("/v1/purchase-payment/{id}", controller.DeletePurchasePayment).Methods("DELETE")

	//PurchaseReturnPayment
	router.HandleFunc("/v1/purchase-return-payment", controller.CreatePurchaseReturnPayment).Methods("POST")
	router.HandleFunc("/v1/purchase-return-payment", controller.ListPurchaseReturnPayment).Methods("GET")
	router.HandleFunc("/v1/purchase-return-payment/{id}", controller.ViewPurchaseReturnPayment).Methods("GET")
	router.HandleFunc("/v1/purchase-return-payment/{id}", controller.UpdatePurchaseReturnPayment).Methods("PUT")
	router.HandleFunc("/v1/purchase-return-payment/{id}", controller.DeletePurchaseReturnPayment).Methods("DELETE")

	//Zatca
	router.HandleFunc("/v1/store/zatca/connect", controller.ConnectStoreToZatca).Methods("POST")
	router.HandleFunc("/v1/order/zatca/report/{id}", controller.ReportOrderToZatca).Methods("POST")
	router.HandleFunc("/v1/sales-return/zatca/report/{id}", controller.ReportSalesReturnToZatca).Methods("POST")
	router.HandleFunc("/v1/store/zatca/disconnect", controller.DisconnectStoreFromZatca).Methods("POST")

	//Ledger
	router.HandleFunc("/v1/ledger", controller.ListLedger).Methods("GET")
	//Accounts
	router.HandleFunc("/v1/account", controller.ListAccounts).Methods("GET")
	router.HandleFunc("/v1/account/{id}", controller.ViewAccount).Methods("GET")
	router.HandleFunc("/v1/account/{id}", controller.DeleteAccount).Methods("DELETE")
	router.HandleFunc("/v1/account/restore/{id}", controller.RestoreAccount).Methods("POST")
	//Postings
	router.HandleFunc("/v1/posting", controller.ListPostings).Methods("GET")

	router.HandleFunc("/v1/translate", controller.TranslateHandler).Methods("POST")

	router.PathPrefix("/images/").Handler(http.StripPrefix("/images/", http.FileServer(http.Dir("./images/"))))
	router.PathPrefix("/pdfs/").Handler(http.StripPrefix("/pdfs/", http.FileServer(http.Dir("./pdfs/"))))
	router.PathPrefix("/zatca/").Handler(http.StripPrefix("/zatca/", http.FileServer(http.Dir("./zatca/"))))
	router.PathPrefix("/html-templates/").Handler(http.StripPrefix("/html-templates/", http.FileServer(http.Dir("./html-templates/"))))

	router.HandleFunc("/v1/socket", controller.WebSocketHandler).Methods("GET")
	//router.HandleFunc("/sockjs-node", controller.WebSocketHandler).Methods("GET")

	router.HandleFunc("/v1/upload-pdf", controller.SavePdf).Methods("POST")

	server := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}

	server.OnConnect("/", func(s socketio.Conn) error {
		fmt.Println("New connection:", s.ID())
		s.SetContext("")
		return nil
	})

	server.OnError("/", func(s socketio.Conn, e error) {
		fmt.Println("Socket error:", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, reason string) {
		fmt.Println("Client disconnected:", reason)
	})

	//log.Fatal(server.Serve())

	//http.HandleFunc("/ws", controller.WebSocketHandler)
	// Enable CORS
	corsHandler := cors.Default().Handler(router) // Apply CORS middleware

	//router.HandleFunc("/v1/account", controller.ListAccounts)

	//http.HandleFunc("/socket.io", server) // WebSocket Endpoint
	//cronJobsEveryHour()
	s := gocron.NewScheduler(time.UTC)
	s.Every(8).Hour().Do(cronJobsEveryHour)
	s.StartAsync()

	go func() {
		log.Fatal(http.ListenAndServeTLS(":"+strconv.Itoa(httpsPort), "localhost.cert.pem", "localhost.key.pem", corsHandler))
	}()

	// Initialize the Socket.io server

	/*
		socketServer = socketio.NewServer(nil) // Default server options

		socketServer.OnConnect("/", func(s socketio.Conn) error {
			fmt.Println("New client connected")
			s.SetContext("")
			return nil
		})

		socketServer.OnDisconnect("/", func(s socketio.Conn, reason string) {
			fmt.Println("Client disconnected:", reason)
		})

		socketServer.OnEvent("/", "chat", func(s socketio.Conn, msg string) {
			fmt.Println("Received message:", msg)
			s.Emit("reply", "Message received")
		})

		socketServer.OnError("/", func(s socketio.Conn, err error) {
			fmt.Println("Error", err.Error())
		}) */

	//log.Fatal(socketServer.Serve())
	//http.HandleFunc("/socket.io", socketServer)

	//http.HandleFunc("/socket.io", controller.WebSocketHandler)
	//router.HandleFunc("/socket.io", controller.WebSocketHandler).Methods("GET")

	//socketServer.ServeHTTP(controller.WebSocketHandler())
	//log.Fatal(socketServer.Serve())

	//router.Handle("/socket.io/", controller.WebSocketHandler())

	log.Printf("API serving @ http://localhost:%s\n", httpPort)
	log.Fatal(http.ListenAndServe(":"+httpPort, corsHandler))

}

func ListAllIndexes(collectionName string) {
	collection := db.Client("").Database(db.GetPosDB()).Collection(collectionName)
	indexView := collection.Indexes()
	opts := options.ListIndexes().SetMaxTime(2 * time.Second)
	cursor, err := indexView.List(context.TODO(), opts)

	if err != nil {
		log.Fatal(err)
	}

	var result []bson.M
	if err = cursor.All(context.TODO(), &result); err != nil {
		log.Fatal(err)
	}

	for _, v := range result {
		for k1, v1 := range v {
			fmt.Printf("%v: %v\n", k1, v1)
		}
		fmt.Println()
	}
}

func cronJobsEveryHour() error {
	log.Print("Cron job is set to run every 8 hours")

	/*
		err := models.ProcessPurchases()
		if err != nil {
			log.Print(err)
		}*/

	/*
		err := models.ProcessCustomerDeposits()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessCustomerWithdrawals()
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

		err = models.ProcessQuotationSalesReturns()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
	 */
	/*
		err := models.ProcessCustomers()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessVendors()
		if err != nil {
			log.Print(err)
		}*/
	/*
		err := models.ProcessProducts()
		if err != nil {
			log.Print(err)
		}*/

	/*
		err := models.ProcessPostings()
		if err != nil {
			log.Print(err)
		}*/

	/*
		err := models.ProcessPurchaseReturns()
		if err != nil {
			log.Print(err)
		}*/

	/*


		err = models.ProcessPurchases()
		if err != nil {
			log.Print(err)
		}



		err = models.ProcessDeliveryNotes()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessAccounts()
		if err != nil {
			log.Print(err)
		}*/

	/*
		err := models.ProcessProducts()
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

		err = models.ProcessQuotationSalesReturns()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessDeliveryNotes()
		if err != nil {
			log.Print(err)
		}*/

	/*
		err = models.ProcessCustomerWithdrawals()
		if err != nil {
			log.Print(err)
		}*/

	/*
		err := models.ProcessQuotationSalesReturns()
		if err != nil {
			log.Print(err)
		}*/
	/*





		err = models.ProcessQuotations()
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

		err = models.ProcessExpenses()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessCapitals()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessCustomerDeposits()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessCustomerWithdrawals()
		if err != nil {
			log.Print(err)
		}
	*/

	/*


		err := models.ProcessOrders()
		if err != nil {
			log.Print(err)
		}*/
	/*

			err = models.ProcessVendors()
			if err != nil {
				log.Print(err)
			}



		/*

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

			err = models.ProcessCustomerDeposits()
			if err != nil {
				log.Print(err)
			}

			err = models.ProcessCustomerWithdrawals()
			if err != nil {
				log.Print(err)
			}
	*/

	/*


		err = models.ProcessCustomers()
		if err != nil {
			log.Print(err)
		}*/

	/*




		err = models.ProcessProducts()
		if err != nil {
			log.Print(err)
		}*/

	/*

		/*
			err := models.ProcessVendors()
			if err != nil {
				log.Print(err)
			}

			err = models.ProcessCustomers()
			if err != nil {
				log.Print(err)
			}*/

	/*
		err := models.ProcessOrders()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessSalesReturns()
		if err != nil {
			log.Print(err)
		}*/

	/*
		err := models.ProcessCustomers()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessVendors()
		if err != nil {
			log.Print(err)
		}
	*/

	/*


		/*


			err := models.ProcessSalesReturns()
			if err != nil {
				log.Print(err)
			}*/

	/*
		err := models.ProcessOrders()
		if err != nil {
			log.Print(err)
		}

			err := models.ProcessPurchases()
			if err != nil {
				log.Print(err)
			}
				err := models.ProcessSalesReturns()
				if err != nil {
					log.Print(err)
				}*/

	/*


		err = models.ProcessExpenseCategories()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessExpenses()
		if err != nil {
			log.Print(err)
		}


			err := models.ProcessOrders()
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
	*/

	/*
		err = models.ProcessSalesPayments()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessSalesReturnPayments()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessPurchasePayments()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessPurchaseReturnPayments()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
			err := models.ProcessSalesHistory()
			if err != nil {
				log.Print(err)
			}


		err := models.ProcessSalesReturnHistory()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessPurchaseHistory()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessPurchaseReturnHistory()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessQuotationHistory()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessDeliveryNoteHistory()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessOrders()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessSalesReturns()
		if err != nil {
			log.Print(err)
		}





		err = models.ProcessDeliveryNotes()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessSalesCashDiscounts()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessSalesCashDiscounts()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessSalesReturns()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessSalesReturns()
		if err != nil {
			log.Print(err)
		}

		err = models.ProcessPurchaseReturns()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
			err := models.ProcessCapitals()
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

			err = models.ProcessOrders()
			if err != nil {
				log.Print(err)
			}




		err := models.ProcessSalesReturns()
		if err != nil {
			log.Print(err)
		}

	*/

	/*
		err := models.ProcessAccounts()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessExpenses()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		go func() {
			err := models.ProcessOrders()
			if err != nil {
				log.Print(err)
			}
		}()

		go func() {
			err := models.ProcessSalesReturns()
			if err != nil {
				log.Print(err)
			}
		}()

		go func() {
			err := models.ProcessPurchases()
			if err != nil {
				log.Print(err)
			}
		}()

		go func() {
			err := models.ProcessPurchaseReturns()
			if err != nil {
				log.Print(err)
			}
		}()
	*/

	/*
		err := models.ProcessSalesReturns()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessPurchases()
		if err != nil {
			log.Print(err)
		}
	*/

	/*
		err := models.ProcessPurchaseReturns()
		if err != nil {
			log.Print(err)
		}
	*/

	return nil
}
