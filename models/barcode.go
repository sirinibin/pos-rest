package models

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"log"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/ean"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func (product *Product) GenerateBarCodeBase64ByStoreID() (err error) {
	if product.Ean12 == "" && product.BarCode == "" {
		return nil
	}
	scale := 6
	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	data := ""
	if product.Ean12 != "" {
		data = product.Ean12
	} else {
		data = product.BarCode
	}

	img1 := image.NewRGBA(image.Rect(0, 0, 144*scale, 106*scale)) // x1,y1,  x2,y2 of background rectangle
	whiteColor := color.RGBA{255, 255, 255, 255}                  //  R, G, B, Alpha
	draw.Draw(img1, img1.Bounds(), &image.Uniform{whiteColor}, image.Point{}, draw.Src)

	productNameSize := 7.8
	width := 0
	for {
		width, err = addLabel(img1, 10*scale, 23*scale, product.Name, color.Black, productNameSize*float64(scale), true)
		if err != nil {
			return errors.New("error add label1: " + err.Error())
		}

		if width <= 762 {
			break
		}
		productNameSize -= 0.10
		img1 = image.NewRGBA(image.Rect(0, 0, 144*scale, 106*scale)) // x1,y1,  x2,y2 of background rectangle
		draw.Draw(img1, img1.Bounds(), &image.Uniform{whiteColor}, image.Point{}, draw.Src)
	}

	storeName := store.Name

	/*
		width, err = addLabel(img1, 10*scale, 15*scale, storeName, color.Black, 6.8*float64(scale), true)
		if err != nil {
			return err
		}
		log.Print(width)
	*/

	storeNameSize := 10.00
	width = 0
	for {
		width, err = addLabel(img1, 10*scale, 15*scale, storeName, color.Black, storeNameSize*float64(scale), true)
		if err != nil {
			return errors.New("error add label2: " + err.Error())
		}

		if width <= 781 {
			break
		}

		storeNameSize -= 0.10

		//Reset image
		img1 = image.NewRGBA(image.Rect(0, 0, 144*scale, 106*scale)) // x1,y1,  x2,y2 of background rectangle
		draw.Draw(img1, img1.Bounds(), &image.Uniform{whiteColor}, image.Point{}, draw.Src)

		_, err = addLabel(img1, 10*scale, 23*scale, product.Name, color.Black, productNameSize*float64(scale), true)
		if err != nil {
			return errors.New("error add label3: " + err.Error())
		}
	}

	purchaseUnitPriceSecret := ""
	vatPercent := 15.00
	price := "N/A"

	retailUnitPriceWithVAT, err := product.getRetailUnitPriceWithVATByStoreID(store.ID)
	if err != nil {
		return errors.New("error get retail unit price: " + err.Error())
	}

	purchaseUnitPriceSecret, err = product.getPurchaseUnitPriceSecretByStoreID(store.ID)
	if err != nil {
		return errors.New("error get purchase unit price secret: " + err.Error())
	}

	vatPercent = store.VatPercent
	if retailUnitPriceWithVAT > 0 {
		price = fmt.Sprintf("%.02f", retailUnitPriceWithVAT)
	}

	addLabel(img1, 10*scale, 72*scale, "SAR: "+price, color.Black, 14*float64(scale), true)
	addLabel(img1, 10*scale, 80*scale, "(INCLUDES "+fmt.Sprintf("%.02f", vatPercent)+"% VAT)", color.Black, 8*float64(scale), true)
	addLabel(img1, 10*scale, 60*scale, data, color.Black, 8*float64(scale), true)

	addLabel(img1, 102*scale, 80*scale, purchaseUnitPriceSecret, color.Black, 9*float64(scale), true)
	rack := ""
	if product.Rack != "" {
		rack = ", Loc:" + product.Rack
	}
	addLabel(img1, 10*scale, 90*scale, "Part #"+product.PartNumber+rack, color.Black, 9*float64(scale), true)

	barCodeImage, err := makeBarcodeImage(data, scale)
	if err != nil {
		if err.Error() == "checksum missmatch" || err.Error() == "invalid ean code data" {
			return nil
		}
		return errors.New("error make barcode image: " + err.Error())
	}

	//	barCodeImage.
	//barCodeImage.Bounds().Add(image.Point{X: 10 * scale, Y: 30 * scale})
	barCodeRect := image.Rect(10*scale, 30*scale, 135*scale, 50*scale)
	draw.Draw(img1, barCodeRect, barCodeImage, image.Point{}, draw.Src)

	// create buffer
	buff := new(bytes.Buffer)

	// encode image to buffer
	err = png.Encode(buff, img1)
	if err != nil {
		return errors.New("error encode: " + err.Error())
	}

	product.BarcodeBase64 = "data:image/png;base64,"
	product.BarcodeBase64 += ToBase64(buff.Bytes())

	return err
}

func ToBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func makeBarcodeImage(data string, scale int) (barCode barcode.Barcode, err error) {

	var BarcodeIntCS barcode.BarcodeIntCS
	if len(data) >= 7 {
		BarcodeIntCS, err = ean.Encode(data)
	} else {
		BarcodeIntCS, err = code128.Encode(data)
	}

	if err != nil {
		log.Print("1:")
		log.Print(err)
		return barCode, err
	}

	barCode, err = barcode.Scale(BarcodeIntCS, 125*scale, 40*scale)
	if err != nil {
		log.Print(err)
		return barCode, err
	}
	return barCode, nil
}

func addLabel(img *image.RGBA, x, y int, label string, color color.Color, size float64, bold bool) (width int, err error) {
	//You can download amiri font from this link: https://fonts.google.com/specimen/Amiri?preview.text=%D8%A8%D9%90%D8%A7%D9%84%D8%B9%D9%8E%D8%B1%D9%8E%D8%A8%D9%90%D9%91%D9%8A&preview.text_type=custom#standard-styles
	var b []byte
	if bold {
		b, err = ioutil.ReadFile("fonts/Amiri-Bold.ttf")
	} else {
		b, err = ioutil.ReadFile("fonts/Amiri-Regular.ttf")
	}

	if err != nil {
		return 0.0, err
	}

	ttf, err := opentype.Parse(b)
	if err != nil {
		return 0., err
	}
	//Create Font.Face from font
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return 0, err
	}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color),
		Face: face,
		Dot:  fixed.P(x, y),
	}

	d.DrawString(label)
	w := d.MeasureString(label)
	return w.Round(), nil
}
