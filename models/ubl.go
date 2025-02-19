package models

// UBLExtensions represents the extension structure
// UBLExtensions represents the extension structure
type UBLExtensions struct {
	UBLExtension UBLExtension `xml:"UBLExtension"`
}

// UBLExtension holds the digital signature details
type UBLExtension struct {
	ExtensionURI     string           `xml:"ExtensionURI"`
	ExtensionContent ExtensionContent `xml:"ExtensionContent"`
}

// ExtensionContent contains the UBL Document Signatures
type ExtensionContent struct {
	UBLDocumentSignatures UBLDocumentSignatures `xml:"UBLDocumentSignatures"`
}

// UBLDocumentSignatures represents the signature details
type UBLDocumentSignatures struct {
	SignatureInformation SignatureInformation `xml:"SignatureInformation"`
}

// SignatureInformation holds metadata for the signature
type SignatureInformation struct {
	ID                    string    `xml:"ID"`
	ReferencedSignatureID string    `xml:"ReferencedSignatureID"`
	Signature             Signature `xml:"Signature"`
}

// Signature represents the digital signature details
type Signature struct {
	ID             string      `xml:"Id,attr"`
	SignedInfo     SignedInfo  `xml:"SignedInfo"`
	SignatureValue string      `xml:"SignatureValue"`
	KeyInfo        KeyInfo     `xml:"KeyInfo"`
	Object         XadesObject `xml:"Object"`
}

// SignedInfo contains the signed information
type SignedInfo struct {
	CanonicalizationMethod Algorithm   `xml:"CanonicalizationMethod"`
	SignatureMethod        Algorithm   `xml:"SignatureMethod"`
	References             []Reference `xml:"Reference"`
}

// Algorithm represents an XML signature algorithm
type Algorithm struct {
	Algorithm string `xml:"Algorithm,attr"`
}

// Reference represents an XML signature reference
type Reference struct {
	ID           string      `xml:"Id,attr,omitempty"`
	URI          string      `xml:"URI,attr"`
	Transforms   []Transform `xml:"Transforms>ds:Transform"`
	DigestMethod Algorithm   `xml:"DigestMethod"`
	DigestValue  string      `xml:"DigestValue"`
}

// Transform represents a transformation step
type Transform struct {
	Algorithm string `xml:"Algorithm,attr"`
	XPath     string `xml:"XPath,omitempty"`
}

// KeyInfo holds certificate data
type KeyInfo struct {
	X509Data X509Data `xml:"X509Data"`
}

// X509Data contains the certificate
type X509Data struct {
	X509Certificate string `xml:"X509Certificate"`
}

// XadesObject represents the XAdES properties
type XadesObject struct {
	QualifyingProperties QualifyingProperties `xml:"QualifyingProperties"`
}

// QualifyingProperties contains the signed properties
type QualifyingProperties struct {
	SignedProperties SignedProperties `xml:"SignedProperties"`
}

// SignedProperties holds signed signature properties
type SignedProperties struct {
	ID                        string                    `xml:"Id,attr"`
	SignedSignatureProperties SignedSignatureProperties `xml:"SignedSignatureProperties"`
}

// SignedSignatureProperties contains signing time & certificate
type SignedSignatureProperties struct {
	SigningTime        string             `xml:"SigningTime"`
	SigningCertificate SigningCertificate `xml:"SigningCertificate"`
}

// SigningCertificate contains certificate digest
type SigningCertificate struct {
	Cert Cert `xml:"Cert"`
}

// Cert contains certificate digest and issuer details
type Cert struct {
	CertDigest   CertDigest   `xml:"CertDigest"`
	IssuerSerial IssuerSerial `xml:"IssuerSerial"`
}

// CertDigest holds certificate hash
type CertDigest struct {
	DigestMethod Algorithm `xml:"DigestMethod"`
	DigestValue  string    `xml:"DigestValue"`
}

// IssuerSerial contains certificate issuer information
type IssuerSerial struct {
	X509IssuerName   string `xml:"X509IssuerName"`
	X509SerialNumber string `xml:"X509SerialNumber"`
}
