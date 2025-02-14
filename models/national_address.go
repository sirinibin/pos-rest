package models

type NationalAddress struct {
	BuildingNo         string `bson:"building_no,omitempty" json:"building_no"`
	BuildingNoArabic   string `bson:"building_no_arabic,omitempty" json:"building_no_arabic"`
	StreetName         string `bson:"street_name,omitempty" json:"street_name"`
	StreetNameArabic   string `bson:"street_name_arabic,omitempty" json:"street_name_arabic"`
	DistrictName       string `bson:"district_name,omitempty" json:"district_name"`
	DistrictNameArabic string `bson:"district_name_arabic,omitempty" json:"district_name_arabic"`
	CityName           string `bson:"city_name,omitempty" json:"city_name"`
	CityNameArabic     string `bson:"city_name_arabic,omitempty" json:"city_name_arabic"`
	ZipCode            string `bson:"zipcode,omitempty" json:"zipcode"`
	ZipCodeArabic      string `bson:"zipcode_arabic,omitempty" json:"zipcode_arabic"`
	AdditionalNo       string `bson:"additional_no,omitempty" json:"additional_no"`
	AdditionalNoArabic string `bson:"additional_no_arabic,omitempty" json:"additional_no_arabic"`
	UnitNo             string `bson:"unit_no,omitempty" json:"unit_no"`
	UnitNoArabic       string `bson:"unit_no_arabic,omitempty" json:"unit_no_arabic"`
}
