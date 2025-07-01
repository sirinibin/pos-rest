package models

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
)

type ComplianceCheck struct {
	SimplifiedInvoice    bool `json:"simplified_invoice" bson:"simplified_invoice"`
	SimplifiedCreditNote bool `json:"simplified_credit_note" bson:"simplified_credit_note"`
	SimplifiedDebitNote  bool `json:"simplified_debit_note" bson:"simplified_debit_note`
	StandardInvoice      bool `json:"standard_invoice" bson:"standard_invoice"`
	StandardCreditNote   bool `json:"standard_credit_note" bson:"standard_credit_note"`
	StandardDebitNote    bool `json:"standard_debit_note" bson:"standard_debit_note"`
}

// Invoice struct to hold XML data
type Invoice struct {
	XMLName xml.Name `xml:"Invoice"`
	Xmlns   string   `xml:"xmlns,attr"`
	Cac     string   `xml:"xmlns:cac,attr"`
	Cbc     string   `xml:"xmlns:cbc,attr"`
	Ext     string   `xml:"xmlns:ext,attr"`
	//UBLExtensions          *UBLExtensions          `xml:"UBLExtensions"`
	UBLExtensions           *UBLExtensions          `xml:"ext:UBLExtensions"`
	ProfileID               string                  `xml:"cbc:ProfileID"`
	ID                      string                  `xml:"cbc:ID"`
	UUID                    string                  `xml:"cbc:UUID"`
	IssueDate               string                  `xml:"cbc:IssueDate"`
	IssueTime               string                  `xml:"cbc:IssueTime"`
	InvoiceTypeCode         InvoiceTypeCode         `xml:"cbc:InvoiceTypeCode"`
	Note                    *Note                   `xml:"cbc:Note"`
	DocumentCurrencyCode    string                  `xml:"cbc:DocumentCurrencyCode"`
	TaxCurrencyCode         string                  `xml:"cbc:TaxCurrencyCode"`
	BillingReference        *BillingReference       `xml:"cac:BillingReference"`
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

type BillingReference struct {
	XMLName                  xml.Name                 `xml:"cac:BillingReference"`
	InvoiceDocumentReference InvoiceDocumentReference `xml:"cac:InvoiceDocumentReference"`
}

type InvoiceDocumentReference struct {
	XMLName xml.Name `xml:"cac:InvoiceDocumentReference"`
	ID      string   `xml:"cbc:ID"`
}

type InvoiceToRead struct {
	XMLName                 xml.Name                      `xml:"Invoice"`
	Xmlns                   string                        `xml:"xmlns,attr"`
	Cac                     string                        `xml:"xmlns:cac,attr"`
	Cbc                     string                        `xml:"xmlns:cbc,attr"`
	Ext                     string                        `xml:"xmlns:ext,attr"`
	UBLExtensions           *UBLExtensions                `xml:"UBLExtensions"`
	ProfileID               string                        `xml:"cbc:ProfileID"`
	ID                      string                        `xml:"cbc:ID"`
	UUID                    string                        `xml:"cbc:UUID"`
	IssueDate               string                        `xml:"cbc:IssueDate"`
	IssueTime               string                        `xml:"cbc:IssueTime"`
	InvoiceTypeCode         InvoiceTypeCode               `xml:"cbc:InvoiceTypeCode"`
	DocumentCurrencyCode    string                        `xml:"cbc:DocumentCurrencyCode"`
	TaxCurrencyCode         string                        `xml:"cbc:TaxCurrencyCode"`
	Note                    *Note                         `xml:"cbc:Note"`
	AdditionalDocumentRefs  []AdditionalDocumentRefToRead `xml:"AdditionalDocumentReference"`
	AccountingSupplierParty AccountingSupplierParty       `xml:"cac:AccountingSupplierParty"`
	AccountingCustomerParty AccountingCustomerParty       `xml:"cac:AccountingCustomerParty"`
	Delivery                Delivery                      `xml:"cac:Delivery"`
	PaymentMeans            []PaymentMeans                `xml:"cac:PaymentMeans"`
	AllowanceCharge         []AllowanceCharge             `xml:"cac:AllowanceCharge"`
	TaxTotals               []TaxTotal                    `xml:"cac:TaxTotal"`
	LegalMonetaryTotal      LegalMonetaryTotal            `xml:"cac:LegalMonetaryTotal"`
	InvoiceLines            []InvoiceLine                 `xml:"cac:InvoiceLine"`
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
	XMLName               xml.Name       `xml:"cac:LegalMonetaryTotal"`
	LineExtensionAmount   MonetaryAmount `xml:"cbc:LineExtensionAmount"`
	TaxExclusiveAmount    MonetaryAmount `xml:"cbc:TaxExclusiveAmount"`
	TaxInclusiveAmount    MonetaryAmount `xml:"cbc:TaxInclusiveAmount"`
	AllowanceTotalAmount  MonetaryAmount `xml:"cbc:AllowanceTotalAmount"`
	ChargeTotalAmount     MonetaryAmount `xml:"cbc:ChargeTotalAmount"`
	PrepaidAmount         MonetaryAmount `xml:"cbc:PrepaidAmount"`
	PayableRoundingAmount MonetaryAmount `xml:"cbc:PayableRoundingAmount"`
	PayableAmount         MonetaryAmount `xml:"cbc:PayableAmount"`
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

type AdditionalDocumentRefToRead struct {
	ID         string            `xml:"ID"`
	UUID       string            `xml:"UUID,omitempty"`
	Attachment *AttachmentToRead `xml:"Attachment,omitempty"`
}

type AttachmentToRead struct {
	EmbeddedDocumentBinaryObject BinaryObject `xml:"EmbeddedDocumentBinaryObject"`
	//EmbeddedDocumentBinaryObjectResponse *BinaryObject `xml:"EmbeddedDocumentBinaryObject"`
}

type Attachment struct {
	EmbeddedDocumentBinaryObject BinaryObject `xml:"cbc:EmbeddedDocumentBinaryObject"`
	//EmbeddedDocumentBinaryObjectResponse *BinaryObject `xml:"EmbeddedDocumentBinaryObject"`
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
	PaymentMeansCode string           `xml:"cbc:PaymentMeansCode"`
	InstructionNote  *InstructionNote `xml:"cbc:InstructionNote"`
}

type InstructionNote struct {
	XMLName xml.Name `xml:"cbc:InstructionNote"`
	Value   string   `xml:",chardata"`
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
	Note                *Note            `xml:"cbc:Note"`
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

// Function to generate SHA256 hash in Base64
func GenerateInvoiceHash(inputString string) (string, error) {
	// Generate SHA-256 Hash
	hash := sha256.Sum256([]byte(inputString))
	hashStr := hex.EncodeToString(hash[:])
	base64Str := base64.StdEncoding.EncodeToString([]byte(hashStr))

	// Convert Hash to Base64
	return base64Str, nil
}
