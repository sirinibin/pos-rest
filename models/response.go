package models

type Response struct {
	Status     bool              `bson:"status" json:"status"`
	Criterias  interface{}       `bson:"criterias,omitempty" json:"criterias,omitempty"`
	TotalCount int64             `bson:"total_count,omitempty" json:"total_count,omitempty"`
	Result     interface{}       `bson:"result,omitempty" json:"result,omitempty"`
	Errors     map[string]string `bson:"errors,omitempty" json:"errors,omitempty"`
}
