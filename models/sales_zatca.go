package models

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/mgo.v2/bson"
)

func (orderProduct OrderProduct) GetZatcaUnit() string {
	if orderProduct.Unit == "drum" {
		return "DRM"
	} else if orderProduct.Unit == "Kg" {
		return "KGM"
	} else if orderProduct.Unit == "Meter(s)" {
		return "MTR"
	} else if orderProduct.Unit == "Gm" {
		return "GRM"
	} else if orderProduct.Unit == "L" {
		return "LTR"
	} else if orderProduct.Unit == "Mg" {
		return "MG"
	} else if orderProduct.Unit == "set" {
		return "SET"
	} else if orderProduct.Unit == "MMT" {
		return "MMT"
	} else if orderProduct.Unit == "CMT" {
		return "CMT"
	}

	return "PCE"
}

func (order *Order) MakeXMLContent() (string, error) {
	var err error
	xmlContent := ""

	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return xmlContent, err
	}

	customer, err := store.FindCustomerByID(order.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return xmlContent, errors.New("error finding customer: " + err.Error())
	}

	// Load XML file
	xmlFile, err := os.Open("zatca/standard_invoice.xml")
	if err != nil {
		return xmlContent, err
	}
	defer xmlFile.Close()

	// Read XML content using io.ReadAll (instead of ioutil.ReadAll)
	xmlData, err := io.ReadAll(xmlFile)
	if err != nil {
		return xmlContent, err
	}

	// Unmarshal XML into struct
	var invoice Invoice
	err = xml.Unmarshal(xmlData, &invoice)
	if err != nil {
		return xmlContent, err
	}

	invoice.ProfileID = "reporting:1.0"
	invoice.Xmlns = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	invoice.Cac = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	invoice.Cbc = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	invoice.Ext = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"

	invoice.ID = order.Code
	invoice.UUID = order.UUID
	// Load Saudi Arabia timezone (AST is UTC+3)
	loc, err := time.LoadLocation("Asia/Riyadh")
	if err != nil {
		fmt.Println("Error loading location:", err)
		return "", err
	}

	invoice.IssueDate = order.Date.In(loc).Format("2006-01-02")
	invoice.IssueTime = order.Date.In(loc).Format("15:04:05")

	if customer != nil && strings.TrimSpace(customer.VATNo) != "" {
		invoice.InvoiceTypeCode.Name = "0100000" //standard invoice
	} else {
		invoice.InvoiceTypeCode.Name = "0200000" //simplified invoice
	}

	invoice.InvoiceTypeCode.Value = "388"
	invoice.DocumentCurrencyCode = "SAR"
	invoice.TaxCurrencyCode = "SAR"

	invoice.AdditionalDocumentRefs = []AdditionalDocumentRef{
		AdditionalDocumentRef{
			ID:   "ICV",
			UUID: strconv.FormatInt(order.InvoiceCountValue, 10),
		},
	}

	lastReportedOrder, err := order.FindLastReportedOrder(bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return xmlContent, errors.New("error finding previous order: " + err.Error())
	}

	//log.Print("lastReportedOrder.Code:")
	//log.Print(lastReportedOrder.Code)

	if lastReportedOrder != nil && lastReportedOrder.Hash != "" {
		order.PrevHash = lastReportedOrder.Hash
	} else {
		order.PrevHash, err = GenerateInvoiceHash("0") //Make hash of 0
		if err != nil {
			return xmlContent, err
		}
	}

	invoice.AdditionalDocumentRefs = append(invoice.AdditionalDocumentRefs, AdditionalDocumentRef{
		ID: "PIH",
		Attachment: &Attachment{EmbeddedDocumentBinaryObject: BinaryObject{
			MimeCode: "text/plain",
			Value:    order.PrevHash,
		}},
	})

	//invoice.Note.LanguageID = "ar"

	storeStreetName := store.NationalAddress.StreetName
	if !govalidator.IsNull(strings.TrimSpace(store.NationalAddress.StreetNameArabic)) {
		storeStreetName = store.NationalAddress.StreetName + " | " + store.NationalAddress.StreetNameArabic
	}

	storeDistrictName := store.NationalAddress.DistrictName
	if !govalidator.IsNull(strings.TrimSpace(store.NationalAddress.DistrictNameArabic)) {
		storeDistrictName = store.NationalAddress.DistrictName + " | " + store.NationalAddress.DistrictNameArabic
	}

	storeCityName := store.NationalAddress.CityName
	if !govalidator.IsNull(strings.TrimSpace(store.NationalAddress.CityNameArabic)) {
		storeCityName = store.NationalAddress.CityName + " | " + store.NationalAddress.CityNameArabic
	}

	storeName := store.Name
	if !govalidator.IsNull(strings.TrimSpace(store.NameInArabic)) {
		storeName = store.Name + " | " + store.NameInArabic
	}

	storeCountryCode := store.CountryCode
	if govalidator.IsNull(storeCountryCode) {
		storeCountryCode = "SA"
	}

	invoice.AccountingSupplierParty = AccountingSupplierParty{
		Party: Party{
			PartyIdentification: PartyIdentification{
				ID: IdentificationID{
					SchemeID: "CRN",
					//Value:    "5903506195",
					Value: store.RegistrationNumber,
				},
			},
			PostalAddress: Address{
				StreetName:      storeStreetName,
				BuildingNumber:  store.NationalAddress.BuildingNo,
				CitySubdivision: storeDistrictName,
				CityName:        storeCityName,
				PostalZone:      store.NationalAddress.ZipCode,
				CountryCode:     storeCountryCode,
			},
			PartyTaxScheme: PartyTaxScheme{
				CompanyID: store.VATNo,
				TaxScheme: TaxScheme{
					ID: IDField{
						Value: "VAT",
					},
				},
			},
			PartyLegalEntity: LegalEntity{
				RegistrationName: storeName,
			},
		}}

	customerStreetName := ""
	customerDistrictName := ""
	customerCityName := ""
	customerName := ""
	customerCountryCode := "SA"
	customerRegistrationNumber := ""
	customerNationalAddressBuildingNo := ""
	customerNationalAddressZipCode := ""
	customerVATNo := ""

	if customer != nil {
		customerRegistrationNumber = customer.RegistrationNumber
		customerNationalAddressBuildingNo = customer.NationalAddress.BuildingNo
		customerNationalAddressZipCode = customer.NationalAddress.ZipCode
		customerVATNo = customer.VATNo

		customerStreetName = customer.NationalAddress.StreetName
		if !govalidator.IsNull(strings.TrimSpace(customer.NationalAddress.StreetNameArabic)) {
			customerStreetName = customer.NationalAddress.StreetName + " | " + customer.NationalAddress.StreetNameArabic
		}

		customerDistrictName = customer.NationalAddress.DistrictName
		if !govalidator.IsNull(strings.TrimSpace(customer.NationalAddress.DistrictNameArabic)) {
			customerDistrictName = customer.NationalAddress.DistrictName + " | " + customer.NationalAddress.DistrictNameArabic
		}

		customerCityName = customer.NationalAddress.CityName
		if !govalidator.IsNull(strings.TrimSpace(customer.NationalAddress.CityNameArabic)) {
			customerCityName = customer.NationalAddress.CityName + " | " + customer.NationalAddress.CityNameArabic
		}

		customerName = customer.Name
		if !govalidator.IsNull(strings.TrimSpace(customer.NameInArabic)) {
			customerName = customer.Name + " | " + customer.NameInArabic
		}

		if customer.CountryCode != "" {
			customerCountryCode = customer.CountryCode
		}
	}

	invoice.AccountingCustomerParty = AccountingCustomerParty{
		Party: Party{
			PartyIdentification: PartyIdentification{
				ID: IdentificationID{
					SchemeID: "CRN",
					Value:    customerRegistrationNumber,
				},
			},
			PostalAddress: Address{
				StreetName:      customerStreetName,
				BuildingNumber:  customerNationalAddressBuildingNo,
				CitySubdivision: customerDistrictName,
				CityName:        customerCityName,
				PostalZone:      customerNationalAddressZipCode,
				CountryCode:     customerCountryCode,
			},
			PartyTaxScheme: PartyTaxScheme{
				CompanyID: customerVATNo,
				TaxScheme: TaxScheme{
					ID: IDField{
						Value: "VAT",
					},
				},
			},
			PartyLegalEntity: LegalEntity{
				RegistrationName: customerName,
			},
		}}

	invoice.Delivery = Delivery{
		ActualDeliveryDate: order.Date.In(loc).Format("2006-01-02"),
	}

	// 10: Cash
	// 55: Debit Card
	// 54: Credit Card
	// 48: Bank Card
	// 30: Bank Transfer / Wire Transfer (Credit Transfer)
	// 20: Cheque
	// 68: Online payment service
	//  1: Customer Account (or Instrument not defined), use   <cbc:PrepaidAmount currencyID="SAR">1150.00</cbc:PrepaidAmount>

	invoice.PaymentMeans = []PaymentMeans{}

	for _, paymentMethod := range order.PaymentMethods {
		if paymentMethod == "cash" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "10",
			})
		} else if paymentMethod == "debit_card" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "55",
			})
		} else if paymentMethod == "credit_card" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "54",
			})
		} else if paymentMethod == "bank_card" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "48",
			})
		} else if paymentMethod == "bank_transfer" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "30",
			})
		} else if paymentMethod == "bank_cheque" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "20",
			})
		} else if paymentMethod == "customer_account" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "1",
			})
		}
	}

	if len(order.PaymentMethods) == 0 {
		invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
			PaymentMeansCode: "1",
		})
	}

	invoice.AllowanceCharge = []AllowanceCharge{}

	if order.Discount > 0 {
		invoice.AllowanceCharge = append(invoice.AllowanceCharge,
			AllowanceCharge{
				ChargeIndicator:       false,
				AllowanceChargeReason: "discount",
				Amount: Amount{
					Value:      ToFixed2(order.Discount, 2),
					CurrencyID: "SAR",
				},
				TaxCategory: &TaxCategory{
					ID: IDField{
						Value:    "S",
						SchemeID: "UN/ECE 5305",
						AgencyID: "6",
					},
					Percent: ToFixed2(store.VatPercent, 2),
					TaxScheme: TaxScheme{
						ID: IDField{
							Value:    "VAT",
							SchemeID: "UN/ECE 5153",
							AgencyID: "6",
						},
					},
				},
			})
	}

	if order.ShippingOrHandlingFees > 0 {
		invoice.AllowanceCharge = append(invoice.AllowanceCharge,
			AllowanceCharge{
				ChargeIndicator:           true,
				AllowanceChargeReasonCode: "SAA",
				AllowanceChargeReason:     "Shipping and handling",
				Amount: Amount{
					//Value:      ToFixed(order.ShippingOrHandlingFees+(order.ShippingOrHandlingFees*(store.VatPercent/100)), 2),
					Value:      ToFixed2(order.ShippingOrHandlingFees, 2),
					CurrencyID: "SAR",
				},
				/*BaseAmount: &BaseAmount{
					Value:      ToFixed(order.ShippingOrHandlingFees, 2),
					CurrencyID: "SAR",
				},*/
				TaxCategory: &TaxCategory{
					ID: IDField{
						Value:    "S",
						SchemeID: "UN/ECE 5305",
						AgencyID: "6",
					},
					Percent: ToFixed2(store.VatPercent, 2),
					TaxScheme: TaxScheme{
						ID: IDField{
							Value:    "VAT",
							SchemeID: "UN/ECE 5153",
							AgencyID: "6",
						},
					},
				},
			})
	}

	invoice.TaxTotals = []TaxTotal{
		TaxTotal{
			TaxAmount: TaxAmount{
				Value:      ToFixed2(order.VatPrice, 2),
				CurrencyID: "SAR",
			},
		},
		TaxTotal{
			TaxAmount: TaxAmount{
				Value:      ToFixed2(order.VatPrice, 2),
				CurrencyID: "SAR",
			},
			TaxSubtotal: &TaxSubtotal{
				TaxableAmount: TaxableAmount{
					Value:      ToFixed2((order.NetTotal - order.VatPrice), 2),
					CurrencyID: "SAR",
				},
				TaxAmount: TaxAmount{
					Value:      ToFixed2(order.VatPrice, 2),
					CurrencyID: "SAR",
				},
				TaxCategory: TaxCategory{
					ID: IDField{
						Value:    "S",
						SchemeID: "UN/ECE 5305",
						AgencyID: "6",
					},
					Percent: ToFixed2(store.VatPercent, 2),
					TaxScheme: TaxScheme{
						ID: IDField{
							Value:    "VAT",
							SchemeID: "UN/ECE 5153",
							AgencyID: "6",
						},
					},
				},
			},
		},
	}

	prePaidAmount := float64(0.00)
	/*
		for _, payment := range order.Payments {
			if payment.Method == "customer_account" {
				prePaidAmount += *payment.Amount
			}
		}*/

	totalAllowance := float64(0.00)
	chargeTotalAmount := float64(0.00)

	/*
		for _, product := range order.Products {
			totalAllowance += product.Discount
		}*/

	totalAllowance += order.Discount
	chargeTotalAmount += order.ShippingOrHandlingFees

	taxExclusiveAmount := RoundTo2Decimals(order.NetTotal - order.VatPrice)
	//taxExclusiveAmount := (order.Total - totalAllowance + chargeTotalAmount)
	// Fix floating-point error by rounding
	//taxExclusiveAmount = math.Round(taxExclusiveAmount*100) / 100

	invoice.LegalMonetaryTotal = LegalMonetaryTotal{
		LineExtensionAmount:  MonetaryAmount{Value: ToFixed2(order.Total, 2), CurrencyID: "SAR"},
		TaxExclusiveAmount:   MonetaryAmount{Value: ToFixed2(taxExclusiveAmount, 2), CurrencyID: "SAR"},
		TaxInclusiveAmount:   MonetaryAmount{Value: ToFixed2(order.NetTotal, 2), CurrencyID: "SAR"},
		AllowanceTotalAmount: MonetaryAmount{Value: ToFixed2(totalAllowance, 2), CurrencyID: "SAR"},
		ChargeTotalAmount:    MonetaryAmount{Value: ToFixed2(chargeTotalAmount, 2), CurrencyID: "SAR"},
		PrepaidAmount:        MonetaryAmount{Value: ToFixed2(prePaidAmount, 2), CurrencyID: "SAR"},
		PayableAmount:        MonetaryAmount{Value: ToFixed2(order.NetTotal, 2), CurrencyID: "SAR"},
	}

	invoice.InvoiceLines = []InvoiceLine{}

	for i, product := range order.Products {
		//lineExtensionAmount := (product.UnitPrice - (product.Discount / product.Quantity)) * product.Quantity
		lineExtensionAmount := ((product.UnitPrice - product.UnitDiscount) * product.Quantity)
		lineExtensionAmount = RoundTo2Decimals(lineExtensionAmount)
		taxTotal := lineExtensionAmount * (*order.VatPercent / 100)

		price := Price{
			PriceAmount: PriceAmount{
				Value: RoundTo4Decimals(product.UnitPrice - product.UnitDiscount),
				//Value:      (product.UnitPrice - product.UnitDiscount),
				CurrencyID: "SAR",
			},
			BaseQuantity: BaseQuantity{
				UnitCode: product.GetZatcaUnit(),
				Value:    1,
			},
		}

		if product.UnitDiscount > 0 {
			price.AllowanceCharge = &AllowanceCharge{
				ChargeIndicator:       false,
				AllowanceChargeReason: "discount",
				Amount: Amount{
					Value:      ToFixed(product.UnitDiscount, 2),
					CurrencyID: "SAR",
				},
				BaseAmount: &BaseAmount{
					CurrencyID: "SAR",
					//Value:      ToFixed(product.UnitPrice, 2),
					Value: RoundTo4Decimals(product.UnitPrice),
				},
			}
		}

		invoice.InvoiceLines = append(invoice.InvoiceLines, InvoiceLine{
			ID: strconv.Itoa((i + 1)),
			InvoicedQuantity: InvoicedQuantity{
				UnitCode: product.GetZatcaUnit(),
				Value:    ToFixed(product.Quantity, 2),
			},
			LineExtensionAmount: LineAmount{
				Value:      ToFixed2(lineExtensionAmount, 2),
				CurrencyID: "SAR",
			},
			TaxTotal: TaxTotal{
				TaxAmount: TaxAmount{
					Value:      ToFixed2(taxTotal, 2),
					CurrencyID: "SAR",
				},
				RoundingAmount: &RoundingAmount{
					Value:      ToFixed2((lineExtensionAmount + taxTotal), 2),
					CurrencyID: "SAR",
				},
			},
			Item: Item{
				Name: product.Name,
				ClassifiedTaxCategory: ClassifiedTaxCategory{
					ID:      "S",
					Percent: ToFixed2(*order.VatPercent, 2),
					TaxScheme: TaxScheme{
						ID: IDField{
							Value:    "VAT",
							SchemeID: "UN/ECE 5153",
							AgencyID: "6",
						},
					},
				},
			},
			Price: price,
		})
	}

	// **Marshal Back to XML**
	updatedXML, err := xml.MarshalIndent(invoice, "", "  ")
	if err != nil {
		return xmlContent, err
	}

	updatedXML2 := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + string(updatedXML)

	filePath := "ZatcaPython/templates/invoice_" + order.Code + ".xml"
	// **Save Updated XML**
	err = os.WriteFile(filePath, []byte(updatedXML2), 0644)
	if err != nil {
		log.Print("Error writing file:")
		return xmlContent, err
	}
	//log.Print("Going to write file8:")

	// Verify file exists

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Println("File not created:", err)
	} else {
		//fmt.Println("File successfully written:", filePath)
	}

	return xmlContent, nil
}

type ZatcaComplianceCheckResponse struct {
	InvoiceHash      string `json:"invoice_hash"`
	CompliancePassed bool   `json:"compliance_passed"`
	Error            string `json:"error"`
	Traceback        string `json:"traceback,omitempty"`
}
type ZatcaReportingResponse struct {
	InvoiceHash     string `json:"invoice_hash"`
	ReportingPassed bool   `json:"reporting_passed"`
	Error           string `json:"error"`
	ClearedInvoice  string `json:"cleared_invoice"` //only for b2b (customers with VAT no.)
	IsSimplified    bool   `json:"is_simplified"`   //only for b2b (customers with VAT no.)
	Traceback       string `json:"traceback,omitempty"`
}

func (order *Order) RecordZatcaComplianceCheckFailure(errorMessage string) error {
	now := time.Now()
	order.Zatca.CompliancePassed = false
	order.Zatca.ComplianceCheckFailedCount++
	order.Zatca.ComplianceCheckErrors = append(order.Zatca.ComplianceCheckErrors, errorMessage)
	order.Zatca.ComplianceCheckLastFailedAt = &now

	err := order.Update()
	if err != nil {
		return err
	}
	return nil
}

func (order *Order) RecordZatcaComplianceCheckSuccess(complianceCheckResponse ZatcaComplianceCheckResponse) error {
	now := time.Now()
	order.Zatca.CompliancePassed = true
	order.Zatca.CompliancePassedAt = &now
	order.Zatca.ComplianceInvoiceHash = complianceCheckResponse.InvoiceHash
	/*
		err := order.Update()
		if err != nil {
			return err
		}*/
	return nil
}

func (order *Order) RecordZatcaReportingFailure(errorMessage string) error {
	now := time.Now()
	order.Zatca.ReportingPassed = false
	order.Zatca.ReportingFailedCount++
	order.Zatca.ReportingErrors = append(order.Zatca.ReportingErrors, errorMessage)
	order.Zatca.ReportingLastFailedAt = &now
	/*
		err := order.Update()
		if err != nil {
			return err
		}*/
	return nil
}

func (order *Order) RecordZatcaReportingSuccess(reportingResponse ZatcaReportingResponse) error {
	now := time.Now()
	order.Zatca.ReportingPassed = true
	order.Zatca.ReportedAt = &now
	order.Zatca.ReportingInvoiceHash = reportingResponse.InvoiceHash
	order.Hash = reportingResponse.InvoiceHash
	/*
		err := order.Update()
		if err != nil {
			return err
		}*/

	return nil
}

func (order *Order) ReportToZatca() error {
	var err error

	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	customer, err := store.FindCustomerByID(order.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return errors.New("error finding customer: " + err.Error())
	}

	_, err = order.MakeXMLContent()
	if err != nil {
		return errors.New("error making xml: " + err.Error())
	}
	var isSimplified bool
	if customer != nil && customer.VATNo != "" {
		isSimplified = false
	} else {
		isSimplified = true
	}

	// Create JSON payload
	payload := map[string]interface{}{
		"env":                   store.Zatca.Env,
		"private_key":           store.Zatca.PrivateKey,
		"binary_security_token": store.Zatca.BinarySecurityToken,
		"secret":                store.Zatca.Secret,
		"xml_file_path":         "ZatcaPython/templates/invoice_" + order.Code + ".xml",
		"is_simplified":         isSimplified,
	}

	// Convert payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return errors.New("error marshal payload to compliance check: " + err.Error())
	}

	pythonBinary := "ZatcaPython/venv/bin/python"
	scriptPath := "ZatcaPython/compliance_check.py"

	// Create command
	cmd := exec.Command(pythonBinary, scriptPath)

	// Set up pipes
	cmd.Stdin = bytes.NewReader(jsonData) // Send JSON data to stdin
	var output bytes.Buffer
	cmd.Stdout = &output // Capture stdout
	cmd.Stderr = &output // Capture stderr

	var complianceCheckResponse ZatcaComplianceCheckResponse

	// Run the command
	err = cmd.Run()
	if err != nil {
		fmt.Println("Error running Python script1:", err)
		// Parse JSON response

		err = json.Unmarshal(output.Bytes(), &complianceCheckResponse)
		if err != nil {
			errorMessage := "error unmarshaling compliance check response: " + complianceCheckResponse.Error + ", " + err.Error()
			err = order.RecordZatcaComplianceCheckFailure(errorMessage)
			if err != nil {
				return err
			}
			return errors.New(errorMessage)
		}

		if complianceCheckResponse.Error != "" {
			errorMessage := "Compliance check error 1: " + complianceCheckResponse.Error
			err = order.RecordZatcaComplianceCheckFailure(errorMessage)
			if err != nil {
				return err
			}
			return errors.New(errorMessage)
		}
	}

	// Parse JSON response

	err = json.Unmarshal(output.Bytes(), &complianceCheckResponse)
	if err != nil {
		errorMessage := "error unmarshal compliance check response: " + err.Error()
		err = order.RecordZatcaComplianceCheckFailure(errorMessage)
		if err != nil {
			return err
		}
		return errors.New(errorMessage)
	}

	//log.Print("pythonResponse:")
	//log.Print(pythonResponse)

	if complianceCheckResponse.Error != "" || !complianceCheckResponse.CompliancePassed {
		errorMessage := "compliance check error 2: " + complianceCheckResponse.Error
		err = order.RecordZatcaComplianceCheckFailure(errorMessage)
		if err != nil {
			return err
		}
		return errors.New(errorMessage)
	}

	if complianceCheckResponse.CompliancePassed {
		err = order.RecordZatcaComplianceCheckSuccess(complianceCheckResponse)
		if err != nil {
			return err
		}
	}

	if complianceCheckResponse.CompliancePassed {
		// Create JSON payload
		payload = map[string]interface{}{
			"env":                              store.Zatca.Env,
			"private_key":                      store.Zatca.PrivateKey,
			"production_binary_security_token": store.Zatca.ProductionBinarySecurityToken,
			"production_secret":                store.Zatca.ProductionSecret,
			"xml_file_path":                    "ZatcaPython/templates/invoice_" + order.Code + ".xml",
			"is_simplified":                    isSimplified,
		}

		// Convert payload to JSON
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		pythonBinary := "ZatcaPython/venv/bin/python"
		scriptPath := "ZatcaPython/reporting_and_clearance.py"

		// Create command
		cmd := exec.Command(pythonBinary, scriptPath)

		// Set up pipes
		cmd.Stdin = bytes.NewReader(jsonData) // Send JSON data to stdin
		var output bytes.Buffer
		cmd.Stdout = &output // Capture stdout
		cmd.Stderr = &output // Capture stderr

		var reportingResponse ZatcaReportingResponse

		// Run the command
		err = cmd.Run()
		if err != nil {
			err = json.Unmarshal(output.Bytes(), &reportingResponse)
			if err != nil {
				errorMessage := "error running reporting script &  unmarshal reporting response : " + err.Error()
				err = order.RecordZatcaReportingFailure(errorMessage)
				if err != nil {
					return err
				}
				return errors.New(errorMessage)
			}

			if reportingResponse.Error != "" {
				errorMessage := "error running reporting script: " + reportingResponse.Error
				err = order.RecordZatcaReportingFailure(errorMessage)
				if err != nil {
					return err
				}
				return errors.New(errorMessage)
			}
		}

		// Parse JSON response
		err = json.Unmarshal(output.Bytes(), &reportingResponse)
		if err != nil {
			errorMessage := "error unmarshal reporting response: " + err.Error()
			err = order.RecordZatcaReportingFailure(errorMessage)
			if err != nil {
				return err
			}
			return errors.New(errorMessage)
		}

		if reportingResponse.Error != "" || !reportingResponse.ReportingPassed {
			errorMessage := "error reporting: " + reportingResponse.Error
			err = order.RecordZatcaReportingFailure(errorMessage)
			if err != nil {
				return err
			}
			return errors.New(errorMessage)
		}

		err = order.RecordZatcaReportingSuccess(reportingResponse)
		if err != nil {
			return err
		}

		err = order.SaveClearedInvoiceData(reportingResponse)
		if err != nil {
			return err
		}

	}

	return nil
}

func (order *Order) SaveClearedInvoiceData(reportingResponse ZatcaReportingResponse) error {

	//Trying to get the UBL contents from the xml invoice received from zatca
	// Step 1: Decode Base64
	xmlData, err := base64.StdEncoding.DecodeString(reportingResponse.ClearedInvoice)
	if err != nil {
		fmt.Println("Error decoding Base64:", err)
		return err
	}

	// Step 2: Save to an XML file
	//fileName := "output.xml"
	//xmlResponseFilePath := "ZatcaPython/templates/invoice_" + order.Code + "_response.xml"
	xmlResponseFilePath := "zatca/xml/" + order.Code + ".xml"
	err = os.WriteFile(xmlResponseFilePath, xmlData, 0644)
	if err != nil {
		fmt.Println("Error writing file:", err)
		return err
	}

	data, err := os.ReadFile(xmlResponseFilePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return err
	}

	var invoice InvoiceToRead

	err = xml.Unmarshal(data, &invoice)
	if err != nil {
		fmt.Println("Error unmarshaling XML:", err)
		return err
	}

	order.Zatca.SigningCertificateHash = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningCertificate.Cert.CertDigest.DigestValue

	order.Zatca.ReportingInvoiceHash = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.SignedInfo.References[0].DigestValue
	if order.Zatca.ReportingInvoiceHash != order.Hash {
		return errors.New("invalid hash")
	}

	order.Zatca.XadesSignedPropertiesHash = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.SignedInfo.References[1].DigestValue

	order.Zatca.ECDSASignature = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.SignatureValue
	order.Zatca.X509DigitalCertificate = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.KeyInfo.X509Data.X509Certificate
	// Load Saudi Arabia timezone (AST is UTC+3)
	loc, err := time.LoadLocation("Asia/Riyadh")
	if err != nil {
		fmt.Println("Error loading location:", err)
		return err
	}

	/*
		if !govalidator.IsNull(Customer.Name) {

		}*/
	var signingTime time.Time

	// Note: signing time coming from zatca is already in saudi timezone
	signingTime, err = time.ParseInLocation("2006-01-02T15:04:05", invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningTime, loc)
	if err != nil {
		fmt.Println("Error parsing Saudi time:", err)
		return err
	}

	signingTime = signingTime.UTC() //converting saudi time to utc

	order.Zatca.SigningTime = &signingTime
	order.Zatca.SigningCertificateHash = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningCertificate.Cert.CertDigest.DigestValue
	order.Zatca.X509DigitalCertificateIssuerName = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningCertificate.Cert.IssuerSerial.X509IssuerName
	order.Zatca.X509DigitalCertificateSerialNumber = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningCertificate.Cert.IssuerSerial.X509SerialNumber

	order.Zatca.QrCode = invoice.AdditionalDocumentRefs[2].Attachment.EmbeddedDocumentBinaryObject.Value

	for _, doc := range invoice.AdditionalDocumentRefs {
		if doc.ID == "QR" { // Match <cbc:ID>QR</cbc:ID>
			order.Zatca.QrCode = doc.Attachment.EmbeddedDocumentBinaryObject.Value
			break
		}
	}

	order.Zatca.IsSimplified = reportingResponse.IsSimplified
	/*
		err = order.Update()
		if err != nil {
			return err
		}*/

	// Delete xml files
	xmlFilePath := "ZatcaPython/templates/invoice_" + order.Code + ".xml"
	if _, err := os.Stat(xmlFilePath); err == nil {
		err = os.Remove(xmlFilePath)
		if err != nil {
			return err
		}
	}

	/*
		if _, err := os.Stat(xmlResponseFilePath); err == nil {
			err = os.Remove(xmlResponseFilePath)
			if err != nil {
				return err
			}
		}
	*/

	return nil
}
