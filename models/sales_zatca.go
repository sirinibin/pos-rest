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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (orderProduct OrderProduct) GetZatcaUnit() string {
	switch orderProduct.Unit {
	// Physical product units
	case "drum":
		return "DRM"
	case "Kg":
		return "KGM"
	case "Meter(s)":
		return "MTR"
	case "Gm":
		return "GRM"
	case "L":
		return "LTR"
	case "Mg":
		return "MG"
	case "set":
		return "SET"
	case "MMT":
		return "MMT"
	case "CMT":
		return "CMT"
	// Service units (UN/CEFACT Rec 20) — legacy string values
	case "hour":
		return "HUR"
	case "day":
		return "DAY"
	case "month":
		return "MON"
	case "session", "package", "visit":
		return "C62"
	// Direct UN/CEFACT Rec 20 codes — pass through when unit is already stored as a code
	case "C62", "HUR", "DAY", "WEE", "MON", "ANN", "EA":
		return orderProduct.Unit
	}

	if orderProduct.IsService {
		// Per Visit or any unrecognised service unit → "one" (C62)
		return "C62"
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
	isSimplified := !customer.IsB2B()

	if isSimplified {
		invoice.InvoiceTypeCode.Name = "0200000" //simplified invoice
	} else {
		invoice.InvoiceTypeCode.Name = "0100000" //standard invoice
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

	//log.Print("CRN:" + store.RegistrationNumber)
	invoice.AccountingSupplierParty = AccountingSupplierParty{
		Party: Party{
			PartyIdentification: &PartyIdentification{
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
	//customerRegistrationNumber := ""
	customerNationalAddressBuildingNo := ""
	customerNationalAddressZipCode := ""
	customerVATNo := ""

	if customer != nil {
		//customerRegistrationNumber = customer.RegistrationNumber
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

	var customerPartyIdentification PartyIdentification

	/*
		if customerRegistrationNumber != "" && customerVATNo == "" {
			customerPartyIdentification = PartyIdentification{
				ID: IdentificationID{
					SchemeID: "CRN",
					Value:    customerRegistrationNumber,
				},
			}
		} else {
			if isSimplified && customerVATNo == "" {
				customerPartyIdentification = PartyIdentification{
					ID: IdentificationID{
						SchemeID: "OTH",
						Value:    "CASH",
					},
				}
			}
		}*/

	if isSimplified {
		customerPartyIdentification = PartyIdentification{
			ID: IdentificationID{
				SchemeID: "OTH",
				Value:    "CASH",
			},
		}
	}

	var customerTaxScheme PartyTaxScheme

	if customerVATNo != "" && !isSimplified {
		customerTaxScheme = PartyTaxScheme{
			CompanyID: customerVATNo,
			TaxScheme: TaxScheme{
				ID: IDField{
					Value: "VAT",
				},
			},
		}
	}

	if customerName == "" && isSimplified {
		customerName = "Cash Customer"
	}

	party := Party{
		PostalAddress: Address{
			StreetName:      customerStreetName,
			BuildingNumber:  customerNationalAddressBuildingNo,
			CitySubdivision: customerDistrictName,
			CityName:        customerCityName,
			PostalZone:      customerNationalAddressZipCode,
			CountryCode:     customerCountryCode,
		},
		PartyTaxScheme: customerTaxScheme,
		PartyLegalEntity: LegalEntity{
			RegistrationName: customerName,
		},
	}

	if isSimplified {
		party.PartyIdentification = &customerPartyIdentification
	}

	invoice.AccountingCustomerParty = AccountingCustomerParty{
		Party: party,
	}

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
		} else if paymentMethod == "customer_account" || paymentMethod == "sales_return" || paymentMethod == "purchase" {
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
				ChargeIndicator:           false,
				AllowanceChargeReasonCode: "95",
				AllowanceChargeReason:     "discount",
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
					Percent: TaxPercent(ToFixed2(store.VatPercent, 2)),
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
					Percent: TaxPercent(ToFixed2(store.VatPercent, 2)),
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
					Value:      RoundTo2Decimals((order.NetTotal - order.RoundingAmount) - order.VatPrice),
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
					Percent: TaxPercent(ToFixed2(store.VatPercent, 2)),
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

	totalAllowance := float64(0.00)
	chargeTotalAmount := float64(0.00)

	/*
		for _, product := range order.Products {
			totalAllowance += product.Discount
		}*/

	totalAllowance += order.Discount
	chargeTotalAmount += order.ShippingOrHandlingFees

	taxExclusiveAmount := RoundTo2Decimals((order.NetTotal - order.RoundingAmount) - order.VatPrice)

	//taxExclusiveAmount := (order.Total - totalAllowance + chargeTotalAmount)
	// Fix floating-point error by rounding
	//taxExclusiveAmount = math.Round(taxExclusiveAmount*100) / 100

	invoice.LegalMonetaryTotal = LegalMonetaryTotal{
		LineExtensionAmount:   MonetaryAmount{Value: ToFixed2(order.Total, 2), CurrencyID: "SAR"},
		TaxExclusiveAmount:    MonetaryAmount{Value: ToFixed2(taxExclusiveAmount, 2), CurrencyID: "SAR"},
		TaxInclusiveAmount:    MonetaryAmount{Value: RoundTo2Decimals(order.NetTotal - order.RoundingAmount), CurrencyID: "SAR"},
		AllowanceTotalAmount:  MonetaryAmount{Value: ToFixed2(totalAllowance, 2), CurrencyID: "SAR"},
		ChargeTotalAmount:     MonetaryAmount{Value: ToFixed2(chargeTotalAmount, 2), CurrencyID: "SAR"},
		PrepaidAmount:         MonetaryAmount{Value: ToFixed2(prePaidAmount, 2), CurrencyID: "SAR"},
		PayableRoundingAmount: MonetaryAmount{Value: RoundTo2Decimals(order.RoundingAmount), CurrencyID: "SAR"},
		PayableAmount:         MonetaryAmount{Value: RoundTo2Decimals((order.NetTotal)), CurrencyID: "SAR"},
	}

	invoice.InvoiceLines = []InvoiceLine{}

	for i, product := range order.Products {
		lineExtensionAmount := ((product.UnitPrice - product.UnitDiscount) * product.Quantity)
		lineExtensionAmount = RoundTo2Decimals(lineExtensionAmount)
		taxTotal := lineExtensionAmount * (*order.VatPercent / 100)
		taxTotal = RoundTo2Decimals(taxTotal)
		roundingAmount := RoundTo2Decimals(lineExtensionAmount + taxTotal)

		price := Price{
			PriceAmount: PriceAmount{
				Value: RoundTo8Decimals(product.UnitPrice - product.UnitDiscount),
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
				ChargeIndicator:           false,
				AllowanceChargeReasonCode: "95",
				AllowanceChargeReason:     "discount",
				Amount: Amount{
					Value:      RoundTo8Decimals(product.UnitDiscount),
					CurrencyID: "SAR",
				},
				BaseAmount: &BaseAmount{
					CurrencyID: "SAR",
					//Value:      ToFixed(product.UnitPrice, 2),
					Value: RoundTo8Decimals(product.UnitPrice),
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
				//Value:      ToFixed2(lineExtensionAmount, 2),
				Value:      lineExtensionAmount,
				CurrencyID: "SAR",
			},
			TaxTotal: TaxTotal{
				TaxAmount: TaxAmount{
					//Value:      ToFixed2(taxTotal, 2),
					Value:      taxTotal,
					CurrencyID: "SAR",
				},
				RoundingAmount: &RoundingAmount{
					//Value:      ToFixed2((lineExtensionAmount + taxTotal), 2),
					Value:      roundingAmount,
					CurrencyID: "SAR",
				},
			},
			Item: Item{
				Name: product.Name,
				ClassifiedTaxCategory: ClassifiedTaxCategory{
					ID:      "S",
					Percent: TaxPercent(ToFixed2(*order.VatPercent, 2)),
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

	// Prepayment reference lines for ZATCA-reported customer deposits (Debit Notes).
	// Only deposits whose Debit Note was already reported to ZATCA are referenced here;
	// un-reported deposits are silently skipped (no XML line, no PrepaidAmount change).
	vatPercent := float64(0)
	if order.VatPercent != nil {
		vatPercent = *order.VatPercent
	}
	// Use the in-memory PaymentsInput when Payments hasn't been saved yet
	// (auto-ZATCA during creation runs before AddPayments/SetPaymentStatus).
	paymentsToCheck := order.Payments
	if len(paymentsToCheck) == 0 {
		paymentsToCheck = order.PaymentsInput
	}
	for _, payment := range paymentsToCheck {
		depositID := payment.ReceivableID
		if depositID == nil {
			depositID = payment.ReferenceID
		}
		if payment.ReferenceType != "customer_deposit" || depositID == nil {
			continue
		}
		deposit, err := store.FindCustomerDepositByID(depositID, bson.M{})
		if err != nil || deposit == nil || !deposit.Zatca.ReportingPassed {
			continue
		}
		if deposit.NetTotal > order.NetTotal {
			continue
		}
		var depositNetExVat, depositVat float64
		if vatPercent > 0 {
			depositNetExVat = RoundTo2Decimals(deposit.NetTotal / (1 + vatPercent/100))
			depositVat = RoundTo2Decimals(deposit.NetTotal - depositNetExVat)
		} else {
			depositNetExVat = deposit.NetTotal
			depositVat = 0
		}
		prePaidAmount = RoundTo2Decimals(prePaidAmount + deposit.NetTotal)
		lineNum := len(invoice.InvoiceLines) + 1
		// ZATCA BR-KSA-74: KSA-30 (DocumentTypeCode in PrepaymentDocumentRef) must be "386".
		// ZATCA BR-KSA-82: When KSA-30 is present, BT-131 (LineExtensionAmount), KSA-11
		//   (line TaxTotal.TaxAmount), and KSA-12 (RoundingAmount) MUST all be 0.
		// ZATCA BR-KSA-80: PrepaidAmount = KSA-31 (TaxSubtotal.TaxableAmount) + KSA-32
		//   (TaxSubtotal.TaxAmount).  These are informational and distinct from KSA-11/12.
		invoice.InvoiceLines = append(invoice.InvoiceLines, InvoiceLine{
			ID:                  strconv.Itoa(lineNum),
			InvoicedQuantity:    InvoicedQuantity{UnitCode: "PCE", Value: 0},
			LineExtensionAmount: LineAmount{Value: 0, CurrencyID: "SAR"},
			PrepaymentDocRef: &PrepaymentDocumentRef{
				ID:               deposit.Code,
				UUID:             deposit.UUID,
				IssueDate:        deposit.Date.In(loc).Format("2006-01-02"),
				IssueTime:        deposit.Date.In(loc).Format("15:04:05"),
				DocumentTypeCode: "386",
			},
			TaxTotal: TaxTotal{
				TaxAmount:      TaxAmount{Value: 0, CurrencyID: "SAR"},
				RoundingAmount: &RoundingAmount{Value: 0, CurrencyID: "SAR"},
				TaxSubtotal: &TaxSubtotal{
					TaxableAmount: TaxableAmount{Value: depositNetExVat, CurrencyID: "SAR"},
					TaxAmount:     TaxAmount{Value: depositVat, CurrencyID: "SAR"},
					TaxCategory: TaxCategory{
						ID:      IDField{Value: "S", SchemeID: "UN/ECE 5305", AgencyID: "6"},
						Percent: TaxPercent(ToFixed2(vatPercent, 2)),
						TaxScheme: TaxScheme{
							ID: IDField{Value: "VAT", SchemeID: "UN/ECE 5153", AgencyID: "6"},
						},
					},
				},
			},
			Item: Item{
				Name: "Advance Payment / " + deposit.Code,
				ClassifiedTaxCategory: ClassifiedTaxCategory{
					ID:      "S",
					Percent: TaxPercent(ToFixed2(vatPercent, 2)),
					TaxScheme: TaxScheme{
						ID: IDField{Value: "VAT", SchemeID: "UN/ECE 5153", AgencyID: "6"},
					},
				},
			},
			Price: Price{
				PriceAmount:  PriceAmount{Value: 0, CurrencyID: "SAR"},
				BaseQuantity: BaseQuantity{UnitCode: "PCE", Value: 1},
			},
		})
	}
	if prePaidAmount > 0 {
		// Invoice-level LineExtensionAmount / TaxExclusiveAmount / TaxInclusiveAmount /
		// TaxTotals are left at full product values.  The advance deduction is expressed
		// only through PrepaidAmount and PayableAmount (ZATCA handles the cross-reference).
		invoice.LegalMonetaryTotal.PrepaidAmount = MonetaryAmount{Value: ToFixed2(prePaidAmount, 2), CurrencyID: "SAR"}
		invoice.LegalMonetaryTotal.PayableAmount = MonetaryAmount{Value: RoundTo2Decimals(order.NetTotal - order.RoundingAmount - prePaidAmount), CurrencyID: "SAR"}
	}

	// **Marshal Back to XML**
	// Some whole-number payable amounts (e.g. 50, 70, 71 SAR) fail ZATCA QR
	// scanning when formatted as "50.00". For those amounts, suppress the ".00"
	// suffix so whole numbers are encoded without trailing decimal zeros.
	payableVal := RoundTo2Decimals(order.NetTotal)
	if payableVal < 100 {
		zatcaRawWholeAmounts = true
	}
	updatedXML, err := xml.MarshalIndent(invoice, "", "  ")
	zatcaRawWholeAmounts = false
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

	if !order.ID.IsZero() {
		err := order.Update()
		if err != nil {
			return err
		}
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
	isSimplified := !customer.IsB2B()

	// Create JSON payload for reporting/clearance
	{
		payload := map[string]interface{}{
			"env":                              store.Zatca.Env,
			"private_key":                      store.Zatca.PrivateKey,
			"production_binary_security_token": store.Zatca.ProductionBinarySecurityToken,
			"production_secret":                store.Zatca.ProductionSecret,
			"xml_file_path":                    "ZatcaPython/templates/invoice_" + order.Code + ".xml",
			"is_simplified":                    isSimplified,
			"store_id":                         store.ID.Hex(),
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
		var stderrOutput bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &stderrOutput

		reportingResponse := ZatcaReportingResponse{}

		// Run the command
		err = cmd.Run()
		if err != nil {
			if stderrOutput.Len() > 0 {
				log.Printf("[ZATCA reporting] Python stderr: %s", stderrOutput.String())
			}
			log.Printf("[ZATCA reporting] Python stdout: %s", output.String())
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

		//log.Printf("[ZATCA reporting] response: %s", output.String())

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
	xmlResponseFilePath := "zatca/" + order.StoreID.Hex() + "/sales/xml/" + order.Code + ".xml"
	if err = os.MkdirAll("zatca/"+order.StoreID.Hex()+"/sales/xml", 0755); err != nil {
		fmt.Println("Error creating directory:", err)
		return err
	}
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
