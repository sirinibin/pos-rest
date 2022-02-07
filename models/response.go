package models

type Response struct {
	Status     bool                   `bson:"status" json:"status"`
	Criterias  interface{}            `bson:"criterias,omitempty" json:"criterias,omitempty"`
	TotalCount int64                  `bson:"total_count,omitempty" json:"total_count"`
	Result     interface{}            `bson:"result,omitempty" json:"result,omitempty"`
	Errors     map[string]string      `bson:"errors,omitempty" json:"errors,omitempty"`
	Meta       map[string]interface{} `bson:"meta,omitempty" json:"meta,omitempty"`
}
