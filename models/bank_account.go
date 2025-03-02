package models

type BankAccount struct {
	BankName    string `bson:"bank_name,omitempty" json:"bank_name"`
	CustomerNo  string `bson:"customer_no,omitempty" json:"customer_no"`
	IBAN        string `bson:"iban,omitempty" json:"iban"`
	AccountName string `bson:"account_name,omitempty" json:"account_name"`
	AccountNo   string `bson:"account_no,omitempty" json:"account_no"`
}
