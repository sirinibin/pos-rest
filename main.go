package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/controller"
	"github.com/sirinibin/pos-rest/db"
	"github.com/sirinibin/pos-rest/env"
	"github.com/sirinibin/pos-rest/models"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// testing
func main() {
	fmt.Println("Start POS Restful API")
	db.Client()
	db.InitRedis()
	/*
		RemoveAllIndexes()

		fields := bson.M{"ean_12": 1}
		CreateIndex("product", fields, true, false, "")

		fields = bson.M{"part_number": 1}
		CreateIndex("product", fields, true, false, "")

		fields = bson.M{"name": "text"}
		CreateIndex("product", fields, false, true, "")

		/*
			fields = bson.M{"name": "text", "name_in_arabic": "text"}
			CreateIndex("product", fields, false, true, "arabic")
	*/

	/*
		fields = bson.M{"created_at": -1}
		CreateIndex("product", fields, false, false, "")
	*/

	/*
		fields = bson.M{"created_at": -1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"store_id": 1}
		CreateIndex("order", fields, false, false, "")
		fields = bson.M{"store_id": 1}
		CreateIndex("salesreturn", fields, false, false, "")

		fields = bson.M{"store_id": 1}
		CreateIndex("product", fields, false, false, "")

		/*
			fields = bson.M{"stores.store_id": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.purchase_unit_price": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.wholesale_unit_price": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.retail_unit_price": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.wholesale_unit_profit": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.retail_unit_profit": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.wholesale_unit_profit_perc": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.retail_unit_profit_perc": 1}
			CreateIndex("product", fields, false, false, "")

			fields = bson.M{"stores.stock": 1}
			CreateIndex("product", fields, false, false, "")
	*/

	/*
		fields = bson.M{"category_id": 1}
		CreateIndex("product", fields, false, false, "")

		fields = bson.M{"created_by": 1}
		CreateIndex("product", fields, false, false, "")

		fields = bson.M{"store_id": 1}
		CreateIndex("purchase", fields, false, false, "")
		fields = bson.M{"store_id": 1}
		CreateIndex("purchasereturn", fields, false, false, "")

		fields = bson.M{"created_by": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"customer_id": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"discount": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"discount_percent": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"date": -1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"code": 1}
		CreateIndex("order", fields, true, false, "")

		fields = bson.M{"date": -1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"net_total": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"payment_status": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"net_profit": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"loss": 1}
		CreateIndex("order", fields, false, false, "")

		fields = bson.M{"date": -1}
		CreateIndex("expense", fields, false, false, "")

		fields = bson.M{"amount": 1}
		CreateIndex("expense", fields, false, false, "")

		fields = bson.M{"vendor_invoice_no": "text"}
		CreateIndex("purchase", fields, false, true, "")

		fields = bson.M{"vendor_id": 1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"discount": 1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"vat_price": 1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"net_retail_profit": 1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"net_wholesale_profit": 1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"created_by": 1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"created_at": -1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"date": -1}
		CreateIndex("purchase", fields, false, false, "")

		fields = bson.M{"net_total": 1}
		CreateIndex("purchase", fields, false, false, "")

		//Sales Return indexes
		fields = bson.M{"date": -1}
		CreateIndex("salesreturn", fields, false, false, "")

		fields = bson.M{"net_total": 1}
		CreateIndex("salesreturn", fields, false, false, "")

		fields = bson.M{"net_profit": 1}
		CreateIndex("salesreturn", fields, false, false, "")

		fields = bson.M{"loss": 1}
		CreateIndex("salesreturn", fields, false, false, "")

		fields = bson.M{"code": 1}
		CreateIndex("salesreturn", fields, true, false, "")

		fields = bson.M{"order_code": 1}
		CreateIndex("salesreturn", fields, false, false, "")

		fields = bson.M{"code": 1}
		CreateIndex("purchase", fields, true, false, "")

		fields = bson.M{"code": 1}
		CreateIndex("purchasereturn", fields, true, false, "")

		fields = bson.M{"date": -1}
		CreateIndex("purchasereturn", fields, false, false, "")

		fields = bson.M{"net_total": 1}
		CreateIndex("purchasereturn", fields, false, false, "")

		fields = bson.M{"purchase_code": 1}
		CreateIndex("purchasereturn", fields, false, false, "")

		fields = bson.M{"code": 1}
		CreateIndex("quotation", fields, true, false, "")
	*/

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

	//Product
	router.HandleFunc("/v1/product", controller.CreateProduct).Methods("POST")
	router.HandleFunc("/v1/product", controller.ListProduct).Methods("GET")
	router.HandleFunc("/v1/product/json", controller.ListProductJson).Methods("GET")
	router.HandleFunc("/v1/product/{id}", controller.ViewProduct).Methods("GET")
	router.HandleFunc("/v1/product/code/{code}", controller.ViewProductByItemCode).Methods("GET")
	router.HandleFunc("/v1/product/barcode/{barcode}", controller.ViewProductByBarCode).Methods("GET")
	router.HandleFunc("/v1/product/{id}", controller.UpdateProduct).Methods("PUT")
	router.HandleFunc("/v1/product/{id}", controller.DeleteProduct).Methods("DELETE")

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
	router.HandleFunc("/v1/sales-return/{id}", controller.UpdateSalesReturn).Methods("PUT")
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
	router.HandleFunc("/v1/sales-payment/{id}", controller.DeleteSalesPayment).Methods("DELETE")

	//SalesReturnPayment
	router.HandleFunc("/v1/sales-return-payment", controller.CreateSalesReturnPayment).Methods("POST")
	router.HandleFunc("/v1/sales-return-payment", controller.ListSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.ViewSalesReturnPayment).Methods("GET")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.UpdateSalesReturnPayment).Methods("PUT")
	router.HandleFunc("/v1/sales-return-payment/{id}", controller.DeleteSalesReturnPayment).Methods("DELETE")

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

	//Ledger
	router.HandleFunc("/v1/ledger", controller.ListLedger).Methods("GET")
	//Accounts
	router.HandleFunc("/v1/account", controller.ListAccounts).Methods("GET")
	router.HandleFunc("/v1/account/{id}", controller.ViewAccount).Methods("GET")
	//Postings
	router.HandleFunc("/v1/posting", controller.ListPostings).Methods("GET")

	router.PathPrefix("/images/").Handler(http.StripPrefix("/images/", http.FileServer(http.Dir("./images/"))))
	router.PathPrefix("/html-templates/").Handler(http.StripPrefix("/html-templates/", http.FileServer(http.Dir("./html-templates/"))))

	//cronJobsEveryHour()
	s := gocron.NewScheduler(time.UTC)
	s.Every(8).Hour().Do(cronJobsEveryHour)
	s.StartAsync()

	go func() {
		log.Fatal(http.ListenAndServeTLS(":"+strconv.Itoa(httpsPort), "localhost.cert.pem", "localhost.key.pem", router))
	}()

	/*
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
		}*/
	log.Printf("API serving @ http://localhost:%s\n", httpPort)
	log.Fatal(http.ListenAndServe(":"+httpPort, router))

}

func ListAllIndexes(collectionName string) {
	collection := db.Client().Database(db.GetPosDB()).Collection(collectionName)
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

func RemoveAllIndexes() {
	log.Print("Removing all indexes")
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	collection.Indexes().DropAll(context.Background())

	collection = db.Client().Database(db.GetPosDB()).Collection("order")
	collection.Indexes().DropAll(context.Background())

	collection = db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	collection.Indexes().DropAll(context.Background())

	collection = db.Client().Database(db.GetPosDB()).Collection("purchase")
	collection.Indexes().DropAll(context.Background())

	collection = db.Client().Database(db.GetPosDB()).Collection("purchasereturn")
	collection.Indexes().DropAll(context.Background())

}

// CreateIndex - creates an index for a specific field in a collection
func CreateIndex(collectionName string, fields bson.M, unique bool, text bool, overrideLang string) error {
	collection := db.Client().Database(db.GetPosDB()).Collection(collectionName)
	//collection.Indexes().DropAll(context.Background())

	indexOptions := options.Index()
	if text {
		indexOptions.SetDefaultLanguage("english")
	}

	if unique {
		indexOptions.SetUnique(true)
	}

	if overrideLang != "" {
		indexOptions.SetLanguageOverride(overrideLang)
	}

	// 1. Lets define the keys for the index we want to create
	//var mod mongo.IndexModel
	mod := mongo.IndexModel{
		Keys:    fields, // index in ascending order or -1 for descending order
		Options: indexOptions,
	}

	// 2. Create the context for this operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 4. Create a single index
	indexName, err := collection.Indexes().CreateOne(ctx, mod)
	if err != nil {
		// 5. Something went wrong, we log it and return false
		log.Printf("Failed to create Index for field:%v", fields)
		fmt.Println(err.Error())
		return err
	}

	log.Printf("Created Index:%s for collection:%s for fields %v", indexName, collectionName, fields)

	// 6. All went well, we return true
	return nil
}

func cronJobsEveryHour() error {
	log.Print("Cron job is set to run every 8 hours")

	err := models.ProcessOrders()
	if err != nil {
		log.Print(err)
	}

	err = models.ProcessSalesReturns()
	if err != nil {
		log.Print(err)
	}

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
		err := models.ProcessProducts()
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
