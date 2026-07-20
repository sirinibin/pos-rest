package models

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"strings"
)

func (deposit *CustomerDeposit) ValidateZatcaReporting() (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(deposit.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store"
		return errs
	}

	if govalidator.IsNull(store.VATNo) {
		errs["store_vat_no"] = "Store VAT No. is required for ZATCA reporting"
	}

	if deposit.Zatca.ReportingPassed {
		errs["already_reported"] = "Already reported to ZATCA"
	}

	return errs
}

func (deposit *CustomerDeposit) FindLastReportedDeposit(selectFields map[string]interface{}) (lastReported *CustomerDeposit, err error) {
	collection := db.GetDB("store_" + deposit.StoreID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"zatca.reporting_passed_at": -1})

	err = collection.FindOne(ctx, bson.M{
		"zatca.reporting_passed": true,
		"store_id":               deposit.StoreID,
	}, findOneOptions).Decode(&lastReported)
	if err != nil {
		return nil, err
	}
	return lastReported, nil
}

func (deposit *CustomerDeposit) RecordZatcaReportingFailure(errorMessage string) error {
	now := time.Now()
	deposit.Zatca.ReportingPassed = false
	deposit.Zatca.ReportingFailedCount++
	deposit.Zatca.ReportingErrors = append(deposit.Zatca.ReportingErrors, errorMessage)
	deposit.Zatca.ReportingLastFailedAt = &now
	return nil
}

func (deposit *CustomerDeposit) RecordZatcaReportingSuccess(reportingResponse ZatcaReportingResponse) error {
	now := time.Now()
	deposit.Zatca.ReportingPassed = true
	deposit.Zatca.ReportedAt = &now
	deposit.Zatca.ReportingInvoiceHash = reportingResponse.InvoiceHash
	deposit.Hash = reportingResponse.InvoiceHash
	return nil
}

func (deposit *CustomerDeposit) MakeXMLContent() (string, error) {
	store, err := FindStoreByID(deposit.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	var customer *Customer
	if deposit.CustomerID != nil && !deposit.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(deposit.CustomerID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return "", errors.New("error finding customer: " + err.Error())
		}
	}

	var vendor *Vendor
	if deposit.VendorID != nil && !deposit.VendorID.IsZero() {
		vendor, err = store.FindVendorByID(deposit.VendorID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return "", errors.New("error finding vendor: " + err.Error())
		}
	}

	xmlFile, err := os.Open("zatca/standard_invoice.xml")
	if err != nil {
		return "", err
	}
	defer xmlFile.Close()

	xmlData, err := io.ReadAll(xmlFile)
	if err != nil {
		return "", err
	}

	var invoice Invoice
	if err = xml.Unmarshal(xmlData, &invoice); err != nil {
		return "", err
	}

	invoice.ProfileID = "reporting:1.0"
	invoice.Xmlns = "urn:oasis:names:specification:ubl:schema:xsd:Invoice-2"
	invoice.Cac = "urn:oasis:names:specification:ubl:schema:xsd:CommonAggregateComponents-2"
	invoice.Cbc = "urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2"
	invoice.Ext = "urn:oasis:names:specification:ubl:schema:xsd:CommonExtensionComponents-2"

	invoice.ID = deposit.Code
	invoice.UUID = deposit.UUID

	loc, err := time.LoadLocation("Asia/Riyadh")
	if err != nil {
		return "", err
	}
	invoice.IssueDate = deposit.Date.In(loc).Format("2006-01-02")
	invoice.IssueTime = deposit.Date.In(loc).Format("15:04:05")

	isSimplified := true
	if customer != nil && customer.IsB2B() {
		isSimplified = false
	} else if vendor != nil && !govalidator.IsNull(vendor.VATNo) && IsValidDigitNumber(vendor.VATNo, "15") {
		isSimplified = false
	}

	if isSimplified {
		invoice.InvoiceTypeCode.Name = "0200000"
	} else {
		invoice.InvoiceTypeCode.Name = "0100000"
	}
	invoice.InvoiceTypeCode.Value = "381" // Debit Note
	invoice.Note = &Note{
		LanguageID: "en",
		Value:      "Receivable debit note",
	}
	invoice.BillingReference = &BillingReference{
		InvoiceDocumentReference: InvoiceDocumentReference{ID: deposit.Code},
	}

	invoice.DocumentCurrencyCode = "SAR"
	invoice.TaxCurrencyCode = "SAR"

	invoice.AdditionalDocumentRefs = []AdditionalDocumentRef{
		{
			ID:   "ICV",
			UUID: strconv.FormatInt(deposit.InvoiceCountValue, 10),
		},
	}

	lastReported, err := deposit.FindLastReportedDeposit(bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return "", errors.New("error finding previous deposit: " + err.Error())
	}

	if lastReported != nil && lastReported.Hash != "" {
		deposit.PrevHash = lastReported.Hash
	} else {
		deposit.PrevHash, err = GenerateInvoiceHash("0")
		if err != nil {
			return "", err
		}
	}

	invoice.AdditionalDocumentRefs = append(invoice.AdditionalDocumentRefs, AdditionalDocumentRef{
		ID: "PIH",
		Attachment: &Attachment{EmbeddedDocumentBinaryObject: BinaryObject{
			MimeCode: "text/plain",
			Value:    deposit.PrevHash,
		}},
	})

	// Supplier party (store)
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
			PartyIdentification: &PartyIdentification{
				ID: IdentificationID{
					SchemeID: "CRN",
					Value:    store.RegistrationNumber,
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
				TaxScheme: TaxScheme{ID: IDField{Value: "VAT"}},
			},
			PartyLegalEntity: LegalEntity{RegistrationName: storeName},
		},
	}

	// Customer party
	customerStreetName := ""
	customerDistrictName := ""
	customerCityName := ""
	customerName := ""
	customerCountryCode := "SA"
	customerNationalAddressBuildingNo := ""
	customerNationalAddressZipCode := ""
	customerVATNo := ""

	if customer != nil {
		customerNationalAddressBuildingNo = customer.NationalAddress.BuildingNo
		customerNationalAddressZipCode = customer.NationalAddress.ZipCode
		customerVATNo = customer.VATNo
		if customer.CountryCode != "" {
			customerCountryCode = customer.CountryCode
		}
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
	} else if vendor != nil {
		customerNationalAddressBuildingNo = vendor.NationalAddress.BuildingNo
		customerNationalAddressZipCode = vendor.NationalAddress.ZipCode
		customerVATNo = vendor.VATNo
		if vendor.CountryCode != "" {
			customerCountryCode = vendor.CountryCode
		}
		customerStreetName = vendor.NationalAddress.StreetName
		if !govalidator.IsNull(strings.TrimSpace(vendor.NationalAddress.StreetNameArabic)) {
			customerStreetName = vendor.NationalAddress.StreetName + " | " + vendor.NationalAddress.StreetNameArabic
		}
		customerDistrictName = vendor.NationalAddress.DistrictName
		if !govalidator.IsNull(strings.TrimSpace(vendor.NationalAddress.DistrictNameArabic)) {
			customerDistrictName = vendor.NationalAddress.DistrictName + " | " + vendor.NationalAddress.DistrictNameArabic
		}
		customerCityName = vendor.NationalAddress.CityName
		if !govalidator.IsNull(strings.TrimSpace(vendor.NationalAddress.CityNameArabic)) {
			customerCityName = vendor.NationalAddress.CityName + " | " + vendor.NationalAddress.CityNameArabic
		}
		customerName = vendor.Name
		if !govalidator.IsNull(strings.TrimSpace(vendor.NameInArabic)) {
			customerName = vendor.Name + " | " + vendor.NameInArabic
		}
	}

	if customerName == "" {
		customerName = deposit.CustomerName
	}
	if customerName == "" {
		customerName = deposit.VendorName
	}
	if isSimplified && customerName == "" {
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
		PartyTaxScheme: PartyTaxScheme{
			CompanyID: customerVATNo,
			TaxScheme: TaxScheme{ID: IDField{Value: "VAT"}},
		},
		PartyLegalEntity: LegalEntity{RegistrationName: customerName},
	}
	if isSimplified {
		party.PartyIdentification = &PartyIdentification{
			ID: IdentificationID{SchemeID: "OTH", Value: "CASH"},
		}
	}
	invoice.AccountingCustomerParty = AccountingCustomerParty{Party: party}

	invoice.Delivery = Delivery{
		ActualDeliveryDate: deposit.Date.In(loc).Format("2006-01-02"),
	}

	// Payment means
	invoice.PaymentMeans = []PaymentMeans{}
	for _, method := range deposit.PaymentMethods {
		code := paymentMethodToZatcaCode(method)
		invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
			PaymentMeansCode: code,
			InstructionNote:  &InstructionNote{Value: "Receivable debit note"},
		})
	}
	if len(deposit.PaymentMethods) == 0 {
		invoice.PaymentMeans = append(invoice.PaymentMeans, PaymentMeans{
			PaymentMeansCode: "1",
			InstructionNote:  &InstructionNote{Value: "Receivable debit note"},
		})
	}

	// VAT calculation — treat NetTotal as tax-inclusive
	vatPercent := store.VatPercent
	var vatAmount float64
	var taxExclusiveAmount float64
	if vatPercent > 0 {
		taxExclusiveAmount = RoundTo2Decimals(deposit.NetTotal / (1 + vatPercent/100))
		vatAmount = RoundTo2Decimals(deposit.NetTotal - taxExclusiveAmount)
	} else {
		taxExclusiveAmount = RoundTo2Decimals(deposit.NetTotal)
		vatAmount = 0
	}

	invoice.AllowanceCharge = []AllowanceCharge{}

	invoice.TaxTotals = []TaxTotal{
		{
			TaxAmount: TaxAmount{Value: ToFixed2(vatAmount, 2), CurrencyID: "SAR"},
		},
		{
			TaxAmount: TaxAmount{Value: ToFixed2(vatAmount, 2), CurrencyID: "SAR"},
			TaxSubtotal: &TaxSubtotal{
				TaxableAmount: TaxableAmount{Value: ToFixed2(taxExclusiveAmount, 2), CurrencyID: "SAR"},
				TaxAmount:     TaxAmount{Value: ToFixed2(vatAmount, 2), CurrencyID: "SAR"},
				TaxCategory: TaxCategory{
					ID: IDField{Value: "S", SchemeID: "UN/ECE 5305", AgencyID: "6"},
					Percent: TaxPercent(ToFixed2(vatPercent, 2)),
					TaxScheme: TaxScheme{
						ID: IDField{Value: "VAT", SchemeID: "UN/ECE 5153", AgencyID: "6"},
					},
				},
			},
		},
	}

	invoice.LegalMonetaryTotal = LegalMonetaryTotal{
		LineExtensionAmount:   MonetaryAmount{Value: ToFixed2(taxExclusiveAmount, 2), CurrencyID: "SAR"},
		TaxExclusiveAmount:    MonetaryAmount{Value: ToFixed2(taxExclusiveAmount, 2), CurrencyID: "SAR"},
		TaxInclusiveAmount:    MonetaryAmount{Value: ToFixed2(deposit.NetTotal, 2), CurrencyID: "SAR"},
		AllowanceTotalAmount:  MonetaryAmount{Value: 0.00, CurrencyID: "SAR"},
		ChargeTotalAmount:     MonetaryAmount{Value: 0.00, CurrencyID: "SAR"},
		PrepaidAmount:         MonetaryAmount{Value: 0.00, CurrencyID: "SAR"},
		PayableRoundingAmount: MonetaryAmount{Value: 0.00, CurrencyID: "SAR"},
		PayableAmount:         MonetaryAmount{Value: ToFixed2(deposit.NetTotal, 2), CurrencyID: "SAR"},
	}

	// Single line item representing the deposit
	description := deposit.Description
	if description == "" {
		description = "Receivable"
	}
	roundingAmount := RoundTo2Decimals(deposit.NetTotal)
	invoice.InvoiceLines = []InvoiceLine{
		{
			ID: "1",
			InvoicedQuantity: InvoicedQuantity{UnitCode: "C62", Value: 1.00},
			LineExtensionAmount: LineAmount{
				Value:      taxExclusiveAmount,
				CurrencyID: "SAR",
			},
			TaxTotal: TaxTotal{
				TaxAmount:      TaxAmount{Value: vatAmount, CurrencyID: "SAR"},
				RoundingAmount: &RoundingAmount{Value: roundingAmount, CurrencyID: "SAR"},
			},
			Item: Item{
				Name: description,
				ClassifiedTaxCategory: ClassifiedTaxCategory{
					ID:      "S",
					Percent: TaxPercent(ToFixed2(vatPercent, 2)),
					TaxScheme: TaxScheme{
						ID: IDField{Value: "VAT", SchemeID: "UN/ECE 5153", AgencyID: "6"},
					},
				},
			},
			Price: Price{
				PriceAmount: PriceAmount{Value: RoundTo8Decimals(taxExclusiveAmount), CurrencyID: "SAR"},
				BaseQuantity: BaseQuantity{UnitCode: "C62", Value: 1},
			},
		},
	}

	updatedXML, err := xml.MarshalIndent(invoice, "", "  ")
	if err != nil {
		return "", err
	}
	updatedXML2 := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + string(updatedXML)

	filePath := "ZatcaPython/templates/debit_note_" + deposit.Code + ".xml"
	if err = os.WriteFile(filePath, []byte(updatedXML2), 0644); err != nil {
		return "", err
	}
	return "", nil
}

func (deposit *CustomerDeposit) ReportToZatca() error {
	store, err := FindStoreByID(deposit.StoreID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	isSimplified := true
	if deposit.CustomerID != nil && !deposit.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(deposit.CustomerID, bson.M{})
		if err == nil && customer != nil && customer.IsB2B() {
			isSimplified = false
		}
	} else if deposit.VendorID != nil && !deposit.VendorID.IsZero() {
		vendor, err := store.FindVendorByID(deposit.VendorID, bson.M{})
		if err == nil && vendor != nil && !govalidator.IsNull(vendor.VATNo) && IsValidDigitNumber(vendor.VATNo, "15") {
			isSimplified = false
		}
	}

	if _, err = deposit.MakeXMLContent(); err != nil {
		return errors.New("error making xml: " + err.Error())
	}

	payload := map[string]interface{}{
		"env":                              store.Zatca.Env,
		"private_key":                      store.Zatca.PrivateKey,
		"production_binary_security_token": store.Zatca.ProductionBinarySecurityToken,
		"production_secret":                store.Zatca.ProductionSecret,
		"xml_file_path":                    "ZatcaPython/templates/debit_note_" + deposit.Code + ".xml",
		"is_simplified":                    isSimplified,
		"store_id":                         store.ID.Hex(),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	cmd := exec.Command("ZatcaPython/venv/bin/python", "ZatcaPython/reporting_and_clearance.py")
	cmd.Stdin = bytes.NewReader(jsonData)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	reportingResponse := ZatcaReportingResponse{}
	err = cmd.Run()
	if err != nil {
		if jsonErr := json.Unmarshal(output.Bytes(), &reportingResponse); jsonErr == nil && reportingResponse.Error != "" {
			errMsg := "error running reporting script: " + reportingResponse.Error
			deposit.RecordZatcaReportingFailure(errMsg)
			return errors.New(errMsg)
		}
		errMsg := "error running reporting script: " + err.Error()
		deposit.RecordZatcaReportingFailure(errMsg)
		return errors.New(errMsg)
	}

	if err = json.Unmarshal(output.Bytes(), &reportingResponse); err != nil {
		errMsg := "error unmarshal reporting response: " + err.Error()
		deposit.RecordZatcaReportingFailure(errMsg)
		return errors.New(errMsg)
	}

	if reportingResponse.Error != "" || !reportingResponse.ReportingPassed {
		errMsg := "error reporting: " + reportingResponse.Error
		deposit.RecordZatcaReportingFailure(errMsg)
		return errors.New(errMsg)
	}

	if err = deposit.RecordZatcaReportingSuccess(reportingResponse); err != nil {
		return err
	}
	return deposit.SaveClearedInvoiceData(reportingResponse)
}

func (deposit *CustomerDeposit) SaveClearedInvoiceData(reportingResponse ZatcaReportingResponse) error {
	xmlData, err := base64.StdEncoding.DecodeString(reportingResponse.ClearedInvoice)
	if err != nil {
		fmt.Println("Error decoding Base64:", err)
		return err
	}

	xmlResponseFilePath := "zatca/" + deposit.StoreID.Hex() + "/receivables/xml/" + deposit.Code + ".xml"
	if err = os.MkdirAll("zatca/"+deposit.StoreID.Hex()+"/receivables/xml", 0755); err != nil {
		return err
	}
	if err = os.WriteFile(xmlResponseFilePath, xmlData, 0644); err != nil {
		return err
	}

	data, err := os.ReadFile(xmlResponseFilePath)
	if err != nil {
		return err
	}

	var invoice InvoiceToRead
	if err = xml.Unmarshal(data, &invoice); err != nil {
		fmt.Println("Error unmarshaling XML:", err)
		return err
	}

	deposit.Zatca.SigningCertificateHash = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningCertificate.Cert.CertDigest.DigestValue

	deposit.Zatca.ReportingInvoiceHash = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.SignedInfo.References[0].DigestValue
	if deposit.Zatca.ReportingInvoiceHash != deposit.Hash {
		return errors.New("invalid hash")
	}

	deposit.Zatca.XadesSignedPropertiesHash = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.SignedInfo.References[1].DigestValue
	deposit.Zatca.ECDSASignature = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.SignatureValue
	deposit.Zatca.X509DigitalCertificate = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.KeyInfo.X509Data.X509Certificate

	loc, err := time.LoadLocation("Asia/Riyadh")
	if err != nil {
		return err
	}
	signingTime, err := time.ParseInLocation("2006-01-02T15:04:05",
		invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningTime, loc)
	if err != nil {
		return err
	}
	signingTime = signingTime.UTC()
	deposit.Zatca.SigningTime = &signingTime

	deposit.Zatca.X509DigitalCertificateIssuerName = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningCertificate.Cert.IssuerSerial.X509IssuerName
	deposit.Zatca.X509DigitalCertificateSerialNumber = invoice.UBLExtensions.UBLExtension.ExtensionContent.UBLDocumentSignatures.SignatureInformation.Signature.Object.QualifyingProperties.SignedProperties.SignedSignatureProperties.SigningCertificate.Cert.IssuerSerial.X509SerialNumber

	for _, doc := range invoice.AdditionalDocumentRefs {
		if doc.ID == "QR" {
			deposit.Zatca.QrCode = doc.Attachment.EmbeddedDocumentBinaryObject.Value
			break
		}
	}
	deposit.Zatca.IsSimplified = reportingResponse.IsSimplified

	// Delete temp XML
	xmlFilePath := "ZatcaPython/templates/debit_note_" + deposit.Code + ".xml"
	if _, err := os.Stat(xmlFilePath); err == nil {
		os.Remove(xmlFilePath)
	}
	return nil
}

// paymentMethodToZatcaCode maps payment method strings to ZATCA payment means codes.
func paymentMethodToZatcaCode(method string) string {
	switch method {
	case "cash":
		return "10"
	case "debit_card":
		return "55"
	case "credit_card":
		return "54"
	case "bank_card":
		return "48"
	case "bank_transfer":
		return "30"
	case "bank_cheque":
		return "20"
	default:
		return "1"
	}
}

// EnsureInvoiceCountValue assigns InvoiceCountValue from Redis if it's 0 (old records created before ZATCA).
func (deposit *CustomerDeposit) EnsureInvoiceCountValue() error {
	if deposit.InvoiceCountValue != 0 {
		return nil
	}
	redisKey := deposit.StoreID.Hex() + "_customer_deposit_counter"
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		store, err := FindStoreByID(deposit.StoreID, bson.M{})
		if err != nil {
			return err
		}
		count, err := store.GetCountByCollection("customerdeposit")
		if err != nil {
			return err
		}
		startFrom := store.CustomerDepositSerialNumber.StartFromCount
		if err = db.RedisClient.Set(redisKey, startFrom+count-1, 0).Err(); err != nil {
			return err
		}
	}
	globalIncr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}
	deposit.InvoiceCountValue = globalIncr
	return nil
}
