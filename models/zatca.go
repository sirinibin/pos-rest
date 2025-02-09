package models

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// Invoice struct to hold XML data
type Invoice struct {
	XMLName                 xml.Name                `xml:"Invoice"`
	Xmlns                   string                  `xml:"xmlns,attr"`
	Cac                     string                  `xml:"xmlns:cac,attr"`
	Cbc                     string                  `xml:"xmlns:cbc,attr"`
	Ext                     string                  `xml:"xmlns:ext,attr"`
	ProfileID               string                  `xml:"cbc:ProfileID"`
	ID                      string                  `xml:"cbc:ID"`
	UUID                    string                  `xml:"cbc:UUID"`
	IssueDate               string                  `xml:"cbc:IssueDate"`
	IssueTime               string                  `xml:"cbc:IssueTime"`
	InvoiceTypeCode         InvoiceTypeCode         `xml:"cbc:InvoiceTypeCode"`
	DocumentCurrencyCode    string                  `xml:"cbc:DocumentCurrencyCode"`
	TaxCurrencyCode         string                  `xml:"cbc:TaxCurrencyCode"`
	Note                    *Note                   `xml:"cbc:Note"`
	AdditionalDocumentRefs  []AdditionalDocumentRef `xml:"cac:AdditionalDocumentReference"`
	AccountingSupplierParty AccountingSupplierParty `xml:"cac:AccountingSupplierParty"`
	AccountingCustomerParty AccountingCustomerParty `xml:"cac:AccountingCustomerParty"`
	Delivery                Delivery                `xml:"cac:Delivery"`
	PaymentMeans            []PaymentMeans          `xml:"cac:PaymentMeans"`
	AllowanceCharge         []AllowanceCharge       `xml:"cac:AllowanceCharge"`
	TaxTotals               []TaxTotal              `xml:"cac:TaxTotal"`
	LegalMonetaryTotal      LegalMonetaryTotal      `xml:"cac:LegalMonetaryTotal"`
	InvoiceLines            []InvoiceLine           `xml:"cac:InvoiceLine"`
}

type AccountingSupplierParty struct {
	XMLName xml.Name `xml:"cac:AccountingSupplierParty"`
	Party   Party    `xml:"cac:Party"`
}

type AccountingCustomerParty struct {
	XMLName xml.Name `xml:"cac:AccountingCustomerParty"`
	Party   Party    `xml:"cac:Party"`
}

// LegalMonetaryTotal represents the <cac:LegalMonetaryTotal> element
type LegalMonetaryTotal struct {
	XMLName              xml.Name       `xml:"cac:LegalMonetaryTotal"`
	LineExtensionAmount  MonetaryAmount `xml:"cbc:LineExtensionAmount"`
	TaxExclusiveAmount   MonetaryAmount `xml:"cbc:TaxExclusiveAmount"`
	TaxInclusiveAmount   MonetaryAmount `xml:"cbc:TaxInclusiveAmount"`
	AllowanceTotalAmount MonetaryAmount `xml:"cbc:AllowanceTotalAmount"`
	ChargeTotalAmount    MonetaryAmount `xml:"cbc:ChargeTotalAmount"`
	PrepaidAmount        MonetaryAmount `xml:"cbc:PrepaidAmount"`
	PayableAmount        MonetaryAmount `xml:"cbc:PayableAmount"`
}

// MonetaryAmount represents a monetary value with a currencyID attribute
type MonetaryAmount struct {
	//XMLName    xml.Name `xml:",any"`
	Value      float64 `xml:",chardata"`       // The amount value
	CurrencyID string  `xml:"currencyID,attr"` // currencyID attribute
}

type Note struct {
	LanguageID string `xml:"languageID,attr"` // Attribute
	Value      string `xml:",chardata"`
}

type InvoiceTypeCode struct {
	Name  string `xml:"name,attr"` // Attribute
	Value string `xml:",chardata"`
}

// Additional Document Reference
type AdditionalDocumentRef struct {
	ID         string      `xml:"cbc:ID"`
	UUID       string      `xml:"cbc:UUID,omitempty"`
	Attachment *Attachment `xml:"cac:Attachment,omitempty"`
}

type Attachment struct {
	EmbeddedDocumentBinaryObject BinaryObject `xml:"cbc:EmbeddedDocumentBinaryObject"`
}

type BinaryObject struct {
	MimeCode string `xml:"mimeCode,attr"` // Attribute
	Value    string `xml:",chardata"`
}

// Party Details (Supplier/Customer)
type Party struct {
	PartyIdentification PartyIdentification `xml:"cac:PartyIdentification"`
	PostalAddress       Address             `xml:"cac:PostalAddress"`
	PartyTaxScheme      PartyTaxScheme      `xml:"cac:PartyTaxScheme"`
	PartyLegalEntity    LegalEntity         `xml:"cac:PartyLegalEntity"`
}

type PartyTaxScheme struct {
	CompanyID string    `xml:"cbc:CompanyID"`
	TaxScheme TaxScheme `xml:"cac:TaxScheme"`
}

type PartyIdentification struct {
	ID IdentificationID `xml:"cbc:ID"`
}

type IdentificationID struct {
	SchemeID string `xml:"schemeID,attr"` // Attribute
	Value    string `xml:",chardata"`
}

type Address struct {
	StreetName      string `xml:"cbc:StreetName"`
	BuildingNumber  string `xml:"cbc:BuildingNumber"`
	CitySubdivision string `xml:"cbc:CitySubdivisionName"`
	CityName        string `xml:"cbc:CityName"`
	PostalZone      string `xml:"cbc:PostalZone"`
	CountryCode     string `xml:"cac:Country>cbc:IdentificationCode"`
}

type LegalEntity struct {
	RegistrationName string `xml:"cbc:RegistrationName"`
}

// Delivery Details
type Delivery struct {
	ActualDeliveryDate string `xml:"cbc:ActualDeliveryDate"`
}

// Payment Means
type PaymentMeans struct {
	PaymentMeansCode string `xml:"cbc:PaymentMeansCode"`
}

/*
type Amount struct {
	XMLName    xml.Name `xml:"cbc:PriceAmount"`
	CurrencyID string   `xml:"currencyID,attr"`
	Value      float64  `xml:",chardata"`
}
*/

// TaxCategory represents the <cac:TaxCategory> element
type TaxCategory struct {
	XMLName   xml.Name  `xml:"cac:TaxCategory"`
	ID        IDField   `xml:"cbc:ID"`
	Percent   float64   `xml:"cbc:Percent"`
	TaxScheme TaxScheme `xml:"cac:TaxScheme"`
}

/*
type TaxCategory struct {
	ID      string  `xml:"cbc:ID"`
	Percent float64 `xml:"cbc:Percent"`
}

*/
// IDField represents the <cbc:ID> element with attributes
type IDField struct {
	XMLName  xml.Name `xml:"cbc:ID"`
	Value    string   `xml:",chardata"`           // The text content inside the tag
	SchemeID string   `xml:"schemeID,attr"`       // schemeID attribute
	AgencyID string   `xml:"schemeAgencyID,attr"` // schemeAgencyID attribute
}

// TaxScheme represents the <cac:TaxScheme> element
type TaxScheme struct {
	XMLName xml.Name `xml:"cac:TaxScheme"`
	ID      IDField  `xml:"cbc:ID"`
}

/*
type TaxScheme struct {
	ID string `xml:"cbc:ID"`
}
*/

// Tax Total
type TaxTotal struct {
	TaxAmount      TaxAmount       `xml:"cbc:TaxAmount"`
	RoundingAmount *RoundingAmount `xml:"cbc:RoundingAmount"`
	TaxSubtotal    *TaxSubtotal    `xml:"cac:TaxSubtotal,omitempty"`
}

type RoundingAmount struct {
	XMLName    xml.Name `xml:"cbc:RoundingAmount"`
	Value      float64  `xml:",chardata"`       // The text content inside the tag (tax amount)
	CurrencyID string   `xml:"currencyID,attr"` // currencyID attribute
}

type TaxSubtotal struct {
	TaxableAmount TaxableAmount `xml:"cbc:TaxableAmount"`
	TaxAmount     TaxAmount     `xml:"cbc:TaxAmount"`
	TaxCategory   TaxCategory   `xml:"cac:TaxCategory"`
}

type TaxableAmount struct {
	Value      float64 `xml:",chardata"`       // The text content inside the tag (tax amount)
	CurrencyID string  `xml:"currencyID,attr"` // currencyID attribute
}

// TaxAmount represents the <cbc:TaxAmount> element with currencyID attribute
type TaxAmount struct {
	XMLName    xml.Name `xml:"cbc:TaxAmount"`
	Value      float64  `xml:",chardata"`       // The text content inside the tag (tax amount)
	CurrencyID string   `xml:"currencyID,attr"` // currencyID attribute
}

// Invoice Line
type InvoiceLine struct {
	ID                  string           `xml:"cbc:ID"`
	InvoicedQuantity    InvoicedQuantity `xml:"cbc:InvoicedQuantity"`
	LineExtensionAmount LineAmount       `xml:"cbc:LineExtensionAmount"`
	TaxTotal            TaxTotal         `xml:"cac:TaxTotal"`
	Item                Item             `xml:"cac:Item"`
	Price               Price            `xml:"cac:Price"`
}

type InvoicedQuantity struct {
	UnitCode string  `xml:"unitCode,attr"`
	Value    float64 `xml:",chardata"`
}

type LineAmount struct {
	CurrencyID string  `xml:"currencyID,attr"`
	Value      float64 `xml:",chardata"`
}

// Price represents the <cac:Price> element
type Price struct {
	XMLName         xml.Name         `xml:"cac:Price"`
	PriceAmount     PriceAmount      `xml:"cbc:PriceAmount"`
	BaseQuantity    BaseQuantity     `xml:"cbc:BaseQuantity"`
	AllowanceCharge *AllowanceCharge `xml:"cac:AllowanceCharge"`
}

// Allowance Charge
type AllowanceCharge struct {
	ChargeIndicator           bool         `xml:"cbc:ChargeIndicator"`
	AllowanceChargeReasonCode string       `xml:"cbc:AllowanceChargeReasonCode"`
	AllowanceChargeReason     string       `xml:"cbc:AllowanceChargeReason"`
	Amount                    Amount       `xml:"cbc:Amount"`
	BaseAmount                *BaseAmount  `xml:"cbc:BaseAmount"`
	TaxCategory               *TaxCategory `xml:"cac:TaxCategory"`
}

type BaseAmount struct {
	XMLName    xml.Name `xml:"cbc:BaseAmount"`
	CurrencyID string   `xml:"currencyID,attr"`
	Value      float64  `xml:",chardata"`
}

// BaseQuantity represents the <cbc:BaseQuantity> element (BT-149)
type BaseQuantity struct {
	UnitCode string  `xml:"unitCode,attr"`
	Value    float64 `xml:",chardata"`
}

// PriceAmount represents the <cbc:PriceAmount> element with currency attribute
type PriceAmount struct {
	XMLName    xml.Name `xml:"cbc:PriceAmount"`
	CurrencyID string   `xml:"currencyID,attr"`
	Value      float64  `xml:",chardata"`
}

// Amount represents the <cbc:Amount> element with currency attribute
type Amount struct {
	XMLName    xml.Name `xml:"cbc:Amount"`
	CurrencyID string   `xml:"currencyID,attr"`
	Value      float64  `xml:",chardata"`
}

type Item struct {
	XMLName               xml.Name              `xml:"cac:Item"`
	Name                  string                `xml:"cbc:Name"`
	ClassifiedTaxCategory ClassifiedTaxCategory `xml:"cac:ClassifiedTaxCategory"`
}

// ClassifiedTaxCategory represents the <cac:ClassifiedTaxCategory> element
type ClassifiedTaxCategory struct {
	XMLName   xml.Name  `xml:"cac:ClassifiedTaxCategory"`
	ID        string    `xml:"cbc:ID"`
	Percent   float64   `xml:"cbc:Percent"`
	TaxScheme TaxScheme `xml:"cac:TaxScheme"`
}

// Canonicalize XML using C14N11
func CanonicalizeXML(xmlInput string) (string, error) {
	// Parse XML
	doc := etree.NewDocument()
	err := doc.ReadFromString(xmlInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse XML: %v", err)
	}

	// Get root element
	root := doc.Root()
	if root == nil {
		return "", fmt.Errorf("XML does not have a root element")
	}

	// Perform C14N11 canonicalization
	canonicalizer := dsig.MakeC14N11Canonicalizer() // ✅ No arguments needed
	canonicalXML, err := canonicalizer.Canonicalize(root)
	if err != nil {
		return "", fmt.Errorf("failed to canonicalize XML: %v", err)
	}

	return string(canonicalXML), nil
}

// Function to generate SHA256 hash in Base64
func GenerateInvoiceHash(xmlInput string) (string, error) {
	var err error
	canonicalXML := "0"

	if xmlInput != "" {
		// Canonicalize XML
		canonicalXML, err = CanonicalizeXML(xmlInput)
		if err != nil {
			return "", err
		}
	}

	// Generate SHA-256 Hash
	hash := sha256.Sum256([]byte(canonicalXML))
	hashStr := hex.EncodeToString(hash[:])
	base64Str := base64.StdEncoding.EncodeToString([]byte(hashStr))

	// Convert Hash to Base64
	return base64Str, nil
}
func (order *Order) GetPreviousRecord() (previousOrder *Order, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(
		ctx,
		bson.M{
			"created_at": bson.M{"$lt": order.CreatedAt},
			"store_id":   order.StoreID,
		}, // Find where `_id` is less than the given ID
		options.FindOne().SetSort(bson.D{{"created_at", -1}}), // Sort in descending order to get the latest previous record
	).Decode(&previousOrder)

	return previousOrder, nil
}

func (order *Order) MakeXMLContent() (string, error) {
	var err error
	xmlContent := ""

	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return xmlContent, err
	}

	customer, err := FindCustomerByID(order.CustomerID, bson.M{})
	if err != nil {
		return xmlContent, err
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
	invoice.IssueDate = order.Date.Format("2006-01-02")
	invoice.IssueTime = order.Date.Format("15:04:05")

	if strings.TrimSpace(customer.VATNo) != "" {
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

	previousOrder, err := order.GetPreviousRecord()
	if err != nil {
		return xmlContent, err
	}

	if previousOrder != nil && previousOrder.Hash != "" {
		order.PrevHash = previousOrder.Hash
	} else {
		invoiceXMLContent := ""
		order.PrevHash, err = GenerateInvoiceHash(invoiceXMLContent)
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

	invoice.AccountingSupplierParty = AccountingSupplierParty{
		Party: Party{
			PartyIdentification: PartyIdentification{
				ID: IdentificationID{
					SchemeID: "CRN",
					Value:    store.RegistrationNumber,
				},
			},
			PostalAddress: Address{
				StreetName:      store.NationalAddresss.StreetName,
				BuildingNumber:  store.NationalAddresss.BuildingNo,
				CitySubdivision: store.NationalAddresss.DistrictName,
				CityName:        store.NationalAddresss.CityName,
				PostalZone:      store.NationalAddresss.ZipCode,
				CountryCode:     "SA",
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
				RegistrationName: store.Name,
			},
		}}

	invoice.AccountingCustomerParty = AccountingCustomerParty{
		Party: Party{
			PartyIdentification: PartyIdentification{
				ID: IdentificationID{
					SchemeID: "CRN",
					Value:    customer.RegistrationNumber,
				},
			},
			PostalAddress: Address{
				StreetName:      customer.NationalAddresss.StreetName,
				BuildingNumber:  customer.NationalAddresss.BuildingNo,
				CitySubdivision: customer.NationalAddresss.DistrictName,
				CityName:        customer.NationalAddresss.CityName,
				PostalZone:      customer.NationalAddresss.ZipCode,
				CountryCode:     "SA",
			},
			PartyTaxScheme: PartyTaxScheme{
				CompanyID: customer.VATNo,
				TaxScheme: TaxScheme{
					ID: IDField{
						Value: "VAT",
					},
				},
			},
			PartyLegalEntity: LegalEntity{
				RegistrationName: customer.Name,
			},
		}}

	invoice.Delivery = Delivery{
		ActualDeliveryDate: order.Date.Format("2006-01-02"),
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
				PaymentMeansCode: "54",
			})
		} else if paymentMethod == "credit_card" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "55",
			})
		} else if paymentMethod == "bank_card" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "48",
			})
		} else if paymentMethod == "bank_transfer" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "30",
			})
		} else if paymentMethod == "cheque" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "20",
			})
		} else if paymentMethod == "customer_account" {
			invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
				PaymentMeansCode: "1",
			})
		}
	}

	invoice.AllowanceCharge = []AllowanceCharge{}

	if order.Discount > 0 {
		invoice.AllowanceCharge = append(invoice.AllowanceCharge,
			AllowanceCharge{
				ChargeIndicator:       false,
				AllowanceChargeReason: "discount",
				Amount: Amount{
					Value:      ToFixed(order.Discount, 2),
					CurrencyID: "SAR",
				},
				TaxCategory: &TaxCategory{
					ID: IDField{
						Value:    "S",
						SchemeID: "UN/ECE 5305",
						AgencyID: "6",
					},
					Percent: ToFixed(store.VatPercent, 2),
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
					Value:      ToFixed(order.ShippingOrHandlingFees, 2),
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
					Percent: ToFixed(store.VatPercent, 2),
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
				Value:      ToFixed(order.VatPrice, 2),
				CurrencyID: "SAR",
			},
		},
		TaxTotal{
			TaxAmount: TaxAmount{
				Value:      ToFixed(order.VatPrice, 2),
				CurrencyID: "SAR",
			},
			TaxSubtotal: &TaxSubtotal{
				TaxableAmount: TaxableAmount{
					Value:      ToFixed((order.NetTotal - order.VatPrice), 2),
					CurrencyID: "SAR",
				},
				TaxAmount: TaxAmount{
					Value:      ToFixed(order.VatPrice, 2),
					CurrencyID: "SAR",
				},
				TaxCategory: TaxCategory{
					ID: IDField{
						Value:    "S",
						SchemeID: "UN/ECE 5305",
						AgencyID: "6",
					},
					Percent: ToFixed(store.VatPercent, 2),
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

	invoice.LegalMonetaryTotal = LegalMonetaryTotal{
		LineExtensionAmount:  MonetaryAmount{Value: ToFixed(order.Total, 2), CurrencyID: "SAR"},
		TaxExclusiveAmount:   MonetaryAmount{Value: ToFixed(((order.Total + order.ShippingOrHandlingFees) - order.Discount), 2), CurrencyID: "SAR"},
		TaxInclusiveAmount:   MonetaryAmount{Value: ToFixed(order.NetTotal, 2), CurrencyID: "SAR"},
		AllowanceTotalAmount: MonetaryAmount{Value: ToFixed(totalAllowance, 2), CurrencyID: "SAR"},
		ChargeTotalAmount:    MonetaryAmount{Value: ToFixed(chargeTotalAmount, 2), CurrencyID: "SAR"},
		PrepaidAmount:        MonetaryAmount{Value: ToFixed(prePaidAmount, 2), CurrencyID: "SAR"},
		PayableAmount:        MonetaryAmount{Value: ToFixed((order.NetTotal), 2), CurrencyID: "SAR"},
	}

	invoice.InvoiceLines = []InvoiceLine{}

	for i, product := range order.Products {
		//lineExtensionAmount := (product.UnitPrice - (product.Discount / product.Quantity)) * product.Quantity
		lineExtensionAmount := ((product.UnitPrice - product.UnitDiscount) * product.Quantity)
		taxTotal := lineExtensionAmount * (*order.VatPercent / 100)

		price := Price{
			PriceAmount: PriceAmount{
				Value:      ToFixed((product.UnitPrice - product.UnitDiscount), 2),
				CurrencyID: "SAR",
			},
			BaseQuantity: BaseQuantity{
				UnitCode: "PCE",
				Value:    1,
			},
		}

		if product.Discount > 0 {
			price.AllowanceCharge = &AllowanceCharge{
				ChargeIndicator:       false,
				AllowanceChargeReason: "discount",
				Amount: Amount{
					Value:      ToFixed(product.UnitDiscount, 2),
					CurrencyID: "SAR",
				},
				BaseAmount: &BaseAmount{
					CurrencyID: "SAR",
					Value:      ToFixed(product.UnitPrice, 2),
				},
			}
		}

		invoice.InvoiceLines = append(invoice.InvoiceLines, InvoiceLine{
			ID: strconv.Itoa((i + 1)),
			InvoicedQuantity: InvoicedQuantity{
				UnitCode: "PCE",
				Value:    ToFixed(product.Quantity, 2),
			},
			LineExtensionAmount: LineAmount{
				Value:      ToFixed(lineExtensionAmount, 2),
				CurrencyID: "SAR",
			},
			TaxTotal: TaxTotal{
				TaxAmount: TaxAmount{
					Value:      ToFixed(taxTotal, 2),
					CurrencyID: "SAR",
				},
				RoundingAmount: &RoundingAmount{
					Value:      ToFixed((lineExtensionAmount + taxTotal), 2),
					CurrencyID: "SAR",
				},
			},
			Item: Item{
				Name: product.Name,
				ClassifiedTaxCategory: ClassifiedTaxCategory{
					ID:      "S",
					Percent: ToFixed(*order.VatPercent, 2),
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

	filePath := "/Users/sirin/go/src/github.com/sirinibin/ZatcaPython/templates/invoice.xml"
	// **Save Updated XML**
	err = os.WriteFile(filePath, []byte(updatedXML2), 0644)
	if err != nil {
		log.Print("Error writing file:")
		return xmlContent, err
	}
	//log.Print("Going to write file8:")

	// Verify file exists
	/*
		if _, err := os.Stat("zatca/updated_standard_invoice.xml"); os.IsNotExist(err) {
			fmt.Println("File not created:", err)
		} else {
			fmt.Println("File successfully written:", "zatca/updated_standard_invoice.xml")
		}
	*/

	return xmlContent, nil
}

func (order *Order) MakeHash() error {
	var err error

	_, err = order.MakeXMLContent()
	if err != nil {
		return err
	}

	return nil
}
