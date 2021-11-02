package models

import (
	"strings"
)

type SearchCriterias struct {
	Page     int                    `bson:"page,omitempty" json:"page,omitempty"`
	Size     int                    `bson:"size,omitempty" json:"size,omitempty"`
	Select   map[string]interface{} `bson:"select,omitempty" json:"select,omitempty"`
	SearchBy map[string]interface{} `bson:"search_by,omitempty" json:"search_by,omitempty"`
	SortBy   map[string]interface{} `bson:"sort_by,omitempty" json:"sort_by,omitempty"`
}

func ParseSelectString(selectStr string) (fields map[string]interface{}) {

	if selectStr == "" {
		return nil
	}

	fields = make(map[string]interface{})

	fieldArray := strings.Split(selectStr, ",")

	for _, field := range fieldArray {
		if strings.HasPrefix(field, "-") {
			fields[strings.TrimPrefix(field, "-")] = 0
		} else {
			fields[field] = 1
		}
	}

	return fields
}

func ParseRelationalSelectString(selectFields interface{}, prefix string) (fields map[string]interface{}) {

	fields = make(map[string]interface{})

	fieldArray := []string{}

	_, ok := selectFields.(string)
	if ok {
		fieldArray = strings.Split(selectFields.(string), ",")
		for _, field := range fieldArray {
			if strings.HasPrefix(field, prefix+".") {
				splits := strings.Split(field, ".")
				if len(splits) > 1 {
					if strings.HasPrefix(splits[1], "-") {
						fields[strings.TrimPrefix(splits[1], "-")] = 0
					} else {
						fields[splits[1]] = 1
					}
				}

			}
		}
	}

	value, ok := selectFields.(map[string]interface{})
	if ok {
		for field, _ := range value {
			if strings.HasPrefix(field, prefix+".") {
				splits := strings.Split(field, ".")
				if len(splits) > 1 {
					if strings.HasPrefix(splits[1], "-") {
						fields[strings.TrimPrefix(splits[1], "-")] = 0
					} else {
						fields[splits[1]] = 1
					}
				}

			}
		}
	}

	return fields
}
