package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	arabic "github.com/abdullahdiaa/garabic"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/hennedo/escpos"
	"github.com/jung-kurt/gofpdf"
	"github.com/sirinibin/pos-rest/models"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func main() {

	PrintBarcode()
	//drawAnImage()

	//GenerateBarcode()
	//GeneratePDF()
}

func getImageFromFilePath(filePath string) (image.Image, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	image, _, err := image.Decode(f)
	return image, err
}

func PrintBarcode() {

	/*
		socket, err := net.Dial("tcp", "localhost")
		if err != nil {
			println(err.Error())
		}
		defer socket.Close()
	*/

	socket, err := os.OpenFile("/dev/usb/lp0", os.O_RDWR, 0)
	//socket, err := os.OpenFile("/devices/pci0000:00/0000:00:14.0/usb3/3-4", os.O_RDWR, 0)
	//f, err := os.Open("/dev/usb/lp0")

	if err != nil {
		panic(err)
	}
	defer socket.Close()

	p := escpos.New(socket)
	/*
		barcode, err := getImageFromFilePath("./two_rectangles.png")
		if err != nil {
			panic(err)
		}
	*/

	//log.Print(barcode)

	//p.PrintImage(barcode)
	/*
		err = p.PrintAndCut()
		if err != nil {
			panic(err)
		}
	*/
	//p.
	//p.SetConfig(escpos.ConfigEpsonTMT20II)

	p.Bold(true).Size(2, 2).Write("Hello World")
	p.LineFeed()
	p.Bold(false).Underline(2).Justify(escpos.JustifyCenter).Write("this is underlined")
	p.LineFeed()
	p.QRCode("https://github.com/hennedo/escpos", true, 10, escpos.QRCodeErrorCorrectionLevelH)

	// You need to use either p.Print() or p.PrintAndCut() at the end to send the data to the printer.
	err = p.PrintAndCut()
	if err != nil {
		panic(err)
	}

	/*
		fmt.Println("Hello Would!")

		//	f, err := os.Open("/dev/usb/lp0")
		f, err := os.OpenFile("/dev/usb/lp0", os.O_RDWR, 0)

		if err != nil {
			panic(err)
		}

		defer f.Close()

		n, err := f.Write([]byte("Hello world!"))
		if err != nil {
			panic(err)
		}
		log.Print(n)
	*/
}

var img = image.NewRGBA(image.Rect(0, 0, 100, 100))
var col color.Color

// HLine draws a horizontal line
func HLine(x1, y, x2 int) {
	for ; x1 <= x2; x1++ {
		img.Set(x1, y, col)
	}
}

// VLine draws a veritcal line
func VLine(x, y1, y2 int) {
	for ; y1 <= y2; y1++ {
		img.Set(x, y1, col)
	}
}

// Rect draws a rectangle utilizing HLine() and VLine()
func Rect(x1, y1, x2, y2 int) {
	HLine(x1, y1, x2)
	HLine(x1, y2, x2)
	VLine(x1, y1, y2)
	VLine(x2, y1, y2)
}

func GetStringFontSize(str string) float64 {
	size := 8.0
	i := 27
	sizeOffset := 1.00
	for {
		if i < len(str) {
			size -= sizeOffset
		} else {
			break
		}
		i += 10
	}
	return models.RoundFloat(size, 2)
}

func drawAnImage() {

	new_png_file := "two_rectangles.png" // output image lives here
	scale := 6

	img1 := image.NewRGBA(image.Rect(0, 0, 144*scale, 106*scale)) // x1,y1,  x2,y2 of background rectangle
	//mygreen := color.RGBA{0, 100, 0, 255}                //  R, G, B, Alpha
	whiteColor := color.RGBA{255, 255, 255, 255} //  R, G, B, Alpha
	// backfill entire background surface with color mygreen
	draw.Draw(img1, img1.Bounds(), &image.Uniform{whiteColor}, image.Point{}, draw.Src)

	productName := "KANA SCRE UGjbh IHB IUJHB IUBIIgy B yYJGHB UHV Y IU"
	productNameSize := 8.0

	for {
		width := addLabel(img1, 10*scale, 23*scale, productName, color.Black, productNameSize*float64(scale), true)
		if width <= 127*scale {
			break
		}
		productNameSize -= 0.20
		img1 = image.NewRGBA(image.Rect(0, 0, 144*scale, 106*scale)) // x1,y1,  x2,y2 of background rectangle
		draw.Draw(img1, img1.Bounds(), &image.Uniform{whiteColor}, image.Point{}, draw.Src)
	}
	addLabel(img1, 10*scale, 15*scale, "GULF UNION OZONE CO.", color.Black, 10*float64(scale), true)

	addLabel(img1, 10*scale, 92*scale, "SAR: 11.50 ", color.Black, 14*float64(scale), true)
	addLabel(img1, 10*scale, 100*scale, "(INCLUDES 15% VAT)", color.Black, 8*float64(scale), true)
	addLabel(img1, 10*scale, 80*scale, "4613563332343", color.Black, 8*float64(scale), true)
	addLabel(img1, 95*scale, 100*scale, "KLMN", color.Black, 10*float64(scale), true)

	b, _ := makeBarcode()

	/*
		red_rect_f, err := os.Open("./barcode_new.png")
		if err != nil {
			fmt.Println(err)
		}

		red_rect2, _, err := image.Decode(red_rect_f)
		if err != nil {
			fmt.Println(err)
		}
	*/

	red_rect := image.Rect(10*scale, 30*scale, 135*scale, 70*scale) //  geometry of 2nd rectangle which we draw atop above rectangle
	//myred := color.RGBA{200, 0, 0, 255}

	// create a red rectangle atop the green surface //&image.Uniform{myred}
	draw.Draw(img1, red_rect, b, image.Point{}, draw.Src)

	// create buffer
	buff := new(bytes.Buffer)

	// encode image to buffer
	err := png.Encode(buff, img1)
	if err != nil {
		fmt.Println("failed to create buffer", err)
	}
	base64Encoding := ""
	mimeType := http.DetectContentType(buff.Bytes())
	switch mimeType {
	case "image/jpeg":
		base64Encoding += "data:image/jpeg;base64,"
	case "image/png":
		base64Encoding += "data:image/png;base64,"
	}

	base64Encoding += ToBase64(buff.Bytes())
	log.Print(base64Encoding)

	//img1.
	myfile, err := os.Create(new_png_file) // ... now lets save imag
	if err != nil {
		panic(err)
	}
	defer myfile.Close()
	png.Encode(myfile, img1) // output file /tmp/two_rectangles.png
}

func ToBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func makeBarcode() (barcode.Barcode, error) {
	scale := 6
	// Create the barcode
	//barcode.
	//qrCode, _ := qr.Encode("461", qr.M, qr.Auto)
	qrCode, _ := code128.Encode("4613563332343")

	// Scale the barcode to 200x200 pixels
	barCode, _ := barcode.Scale(qrCode, 125*scale, 40*scale)

	// create the output file
	file, _ := os.Create("barcode_new.png")
	defer file.Close()

	// encode the barcode as png
	return barCode, png.Encode(file, barCode)
}

func GenerateBarcode() error {

	width := 144
	height := 106
	img1 := image.NewRGBA(image.Rect(0, 0, width, height))
	addLabel(img1, 10, 15, "GULF UNION OZONE CO.", color.Black, 10, true)
	addLabel(img1, 10, 23, "KANA SCREW DRIVER", color.Black, 8, true)
	addLabel(img1, 10, 90, "SAR: 11.50 ", color.Black, 8, true)
	addLabel(img1, 10, 100, "(INCLUDES 15% VAT)", color.Black, 8, true)

	f, err := os.Create("./barcode.png")
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img1); err != nil {
		return err
	}

	_, err = makeBarcode()
	if err != nil {
		return err
	}

	imgFile2, err := os.Open("./barcode.png")
	if err != nil {
		fmt.Println(err)
	}

	img2, _, err := image.Decode(imgFile2)
	if err != nil {
		return err
	}

	//img2.Se
	//img2.Set(0, 0, color.White)

	offset := image.Pt(20, 20)
	b := img1.Bounds()
	//image3 := image.NewRGBA(b)
	image3 := image.NewRGBA(image.Rect(0, 0, width, height))

	//backGroundColor := image.Transparent

	draw.Draw(image3, b, img2, image.ZP, draw.Src)
	draw.Draw(image3, img1.Bounds().Add(offset), img1, image.Point{0, 0}, draw.Over)
	//draw.Draw(image3, img1.Bounds().Add(offset), backGroundColor, image.Point{0, 0}, draw.Src)
	image3.Set(0, 0, color.White)

	third, err := os.Create("barcode_finale.jpg")
	if err != nil {
		log.Fatalf("failed to create: %s", err)
	}
	jpeg.Encode(third, image3, &jpeg.Options{jpeg.DefaultQuality})
	defer third.Close()

	/*
		offset := image.Pt(30, 30)
		b := img1.Bounds()
		log.Print("b")
		log.Print(b.String())

		draw.Draw(img1, b.Add(offset), img2, image.ZP, draw.Src)
	*/
	/*

		offset := image.Pt(30, 30)
		b := img1.Bounds()
		image3 := image.NewRGBA(b)
		draw.Draw(image3, b, img2, image.ZP, draw.Src)
		draw.Draw(image3, img2.Bounds().Add(offset), img1, image.ZP, draw.Over)
	*/
	//foreGroundColor := image.NewUniform(color.Black)
	/*
		backGroundColor := image.Transparent
		backgroundWidth := 200
		backgroundHeight := 290
		background := image.NewRGBA(image.Rect(0, 0, backgroundWidth, backgroundHeight))

		draw.Draw(background, background.Bounds(), backGroundColor, image.ZP, draw.Src)

		third, err := os.Create("barcode_finale.jpg")
		if err != nil {
			log.Fatalf("failed to create: %s", err)
		}
		jpeg.Encode(third, background, &jpeg.Options{jpeg.DefaultQuality})
		defer third.Close()

	*/
	/*

	 */

	/*
		//img.Rect.Add()
		//starting position of the second image (bottom left)
		sp2 := image.Point{img.Bounds().Dx(), 0}

		//new rectangle for the second image
		r2 := image.Rectangle{sp2, sp2.Add(img2.Bounds().Size())}

		//rectangle for the big image
		r := image.Rectangle{image.Point{0, 0}, r2.Max}

		rgba := image.NewRGBA(r)
		draw.Draw(rgba, img.Bounds(), img, image.Point{0, 0}, draw.Src)
		draw.Draw(rgba, r2, img2, image.Point{0, 0}, draw.Src)

		out, err := os.Create("./barcode_final.jpg")
		if err != nil {
			fmt.Println(err)
		}

		var opt jpeg.Options
		opt.Quality = 80

		jpeg.Encode(out, rgba, &opt)
	*/
	return nil
}

func GeneratePDF() error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetTextColor(0, 0, 0) //black color
	pdf.SetFont("Arial", "", 12)
	x := float64(4)
	y := float64(10)
	pdf.Text(x, y, "GULF UNION OZONE CO.")
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(128, 128, 128) //grey color
	pdf.Text(x, y+7, "For Industrial Tools & Spare Parts")
	pdf.Text(x, y+12, "C.R / 5903506195")
	pdf.Text(x, y+17, "VAT / 302105134900003")

	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(0, 0, 0) //black color
	pdf.Text(x, y+37, "Quotation:")
	pdf.Text(x, y+42, "Quotation Date:")
	pdf.Text(x, y+47, "Customer:")
	pdf.Text(x, y+52, "VAT Number:")

	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(128, 128, 128)       //grey color
	pdf.Text(x+16, y+37, "#023095045562") //Quotation Number value
	pdf.Text(x+23, y+42, "Dec 05 2021")   //Quotation Date value
	pdf.Text(x+16, y+47, "Facebook")      // Customer Name Value
	pdf.Text(x+20, y+52, "96523875")      // Vat Number Value

	pdf.Image("./logo.png", x+91, y-5, 20, 20, false, "", 0, "") //total:210, center:105, image width:20, image center: 105-10=95

	//pdf.AddUTF8Font("Arial", "U", "")
	//tr := pdf.UnicodeTranslatorFromDescriptor("cp1251")
	//pdf.AddUTF8Font("HAFS Regular", "U", "./KFGQPC-Uthmanic-Script-HAFS-Regular.otf")
	//pdf.Font

	//pdf.AddFont("tajawal", "", "/home/sirin/go/src/github.com/sirinibin/pos-rest/scripts/Tajawal-Regular.ttf")

	pdf.SetTextColor(0, 0, 0)
	//black color

	pdf.SetFont("Arial", "", 12)
	pdf.Text(x+80, y+26, "QUOTATION /") //Quotation Number value //+tr("اقتباس")

	err := createArabicTextImage("اقتباس", "title_ar.png", 300, color.Black)
	if err != nil {
		return err
	}
	pdf.Image("./title_ar.png", x+104, y+19, 25, 10, false, "", 0, "")
	pdf.Line(84, 37.5, 127, 37.5)

	//store name
	err = createArabicTextImage("شركة اتحاد الخليج للأوزون", "store_name_ar.png", 900, color.Black)
	if err != nil {
		return err
	}
	pdf.Image("./store_name_ar.png", x+155, y-6, 55, 10, false, "", 0, "")

	//title
	err = createArabicTextImage("للادوات الصناعية وقطع الغيار", "store_title_ar.png", 900, color.Gray{Y: 120})
	if err != nil {
		return err
	}
	pdf.Image("./store_title_ar.png", x+175, y+4, 30, 6, false, "", 0, "")

	//c.r no.
	err = createArabicTextImage("رقم سجل الشركة / ٥۹۰۳٥۰٦٥", "store_cr_no_ar.png", 1000, color.Gray{Y: 120})
	if err != nil {
		return err
	}
	pdf.Image("./store_cr_no_ar.png", x+173, y+10, 30, 6, false, "", 0, "")

	//vat no.
	err = createArabicTextImage("الرقم الضريبي / ۳۰۲۱۰٥۱۳٤۰۰۰۰۳", "vat_no_ar.png", 1150, color.Gray{Y: 120})
	if err != nil {
		return err
	}
	pdf.Image("./vat_no_ar.png", x+163, y+15, 40, 6, false, "", 0, "")

	//pdf.RTL()
	//pdf.AddFont("Lateef", "", "./Lateef-Regular.ttf")
	//pdf.AddUTF8Font("Lateef", "", "./Lateef-Regular.ttf")
	//pdf.AddUTF8Font("Tajawal", "", "./Tajawal-Regular.ttf")
	//pdf.AddUTF8Font("Tajawal", "", "./Tajawal-Black.ttf")
	//pdf.SetFont("Lateef", "", 16)
	//pdf.SetFont("Tajawal", "", 16)
	//stream.showText()
	//text := "اقتباس"
	//log.Print(text)
	//log.Print(arabic.Shape((text)))

	//normalized := arabic.Normalize("سَنواتٌ")
	//fmt.Println(normalized)

	//tr := pdf.UnicodeTranslatorFromDescriptor("")
	//pdf.Text(x+124, y+26, "اقتباس") //Quotation Number value //+tr("اقتباس")
	//pdf.Line(110, 37.5, 129, 37.5)
	//pdf.Cell(200, 300, "اقتباس")
	//pdf.Text(x+85, y+37, "ok2")
	/*
		// pdf.SetFont("Times", "", 12)
		template := pdf.CreateTemplate(func(tpl *gofpdf.Tpl) {
			tpl.Image("./logo.png", 6, 6, 30, 0, false, "", 0, "")
			tpl.SetFont("Arial", "B", 16)
			tpl.Text(40, 20, "Template says hello")
			tpl.SetDrawColor(0, 100, 200)
			tpl.SetLineWidth(2.5)
			tpl.Line(95, 12, 105, 22)
		})
		_, tplSize := template.Size()
		// fmt.Println("Size:", tplSize)
		// fmt.Println("Scaled:", tplSize.ScaleBy(1.5))

		template2 := pdf.CreateTemplate(func(tpl *gofpdf.Tpl) {
			tpl.UseTemplate(template)
			subtemplate := tpl.CreateTemplate(func(tpl2 *gofpdf.Tpl) {
				tpl2.Image("./logo.png", 6, 86, 30, 0, false, "", 0, "")
				tpl2.SetFont("Arial", "B", 16)
				tpl2.Text(40, 100, "Subtemplate says hello")
				tpl2.SetDrawColor(0, 200, 100)
				tpl2.SetLineWidth(2.5)
				tpl2.Line(102, 92, 112, 102)
			})
			tpl.UseTemplate(subtemplate)
		})

		pdf.SetDrawColor(200, 100, 0)
		pdf.SetLineWidth(2.5)
		pdf.SetFont("Arial", "B", 16)

		// serialize and deserialize template
		b, _ := template2.Serialize()
		template3, _ := gofpdf.DeserializeTemplate(b)

		pdf.AddPage()
		pdf.UseTemplate(template3)
		pdf.UseTemplateScaled(template3, gofpdf.PointType{X: 0, Y: 30}, tplSize)
		pdf.Line(40, 210, 60, 210)
		pdf.Text(40, 200, "Template example page 1")

		pdf.AddPage()
		pdf.UseTemplate(template2)
		pdf.UseTemplateScaled(template3, gofpdf.PointType{X: 0, Y: 30}, tplSize.ScaleBy(1.4))
		pdf.Line(60, 210, 80, 210)
		pdf.Text(40, 200, "Template example page 2")

		//fileStr := example.Filename("Fpdf_CreateTemplate")
		//err := pdf.OutputFileAndClose(fileStr)
		//example.Summary(err, fileStr)
	*/
	filename := "./quotation.pdf"

	return pdf.OutputFileAndClose(filename)
}
func createArabicTextImage(arabicStr string, filename string, width int, color color.Color) error {
	img := image.NewRGBA(image.Rect(0, 0, width, 150))
	addLabel(img, 55, 95, arabic.Shape(arabicStr), color, 80, true)

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	return nil
}

/*
func addImage(img *image.RGBA, x, y int, newImg *image.RGBA) {
}
*/
func addLabel(img *image.RGBA, x, y int, label string, color color.Color, size float64, bold bool) int {
	//Load font file
	//You can download amiri font from this link: https://fonts.google.com/specimen/Amiri?preview.text=%D8%A8%D9%90%D8%A7%D9%84%D8%B9%D9%8E%D8%B1%D9%8E%D8%A8%D9%90%D9%91%D9%8A&preview.text_type=custom#standard-styles
	//b, err := ioutil.ReadFile("Amiri-Regular.ttf")
	//b, err := ioutil.ReadFile("Katibeh-Regular.ttf")
	var err error
	var b []byte
	if bold {
		b, err = ioutil.ReadFile("Amiri-Bold.ttf")
	} else {
		b, err = ioutil.ReadFile("Amiri-Regular.ttf")
	}

	//b, err := ioutil.ReadFile("mirza/Mirza-Regular.ttf")
	if err != nil {
		log.Println(err)
		return 0.0
	}

	ttf, err := opentype.Parse(b)
	if err != nil {
		log.Println(err)
		return 0.0
	}
	//Create Font.Face from font
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color),
		Face: face,
		Dot:  fixed.P(x, y),
	}

	d.DrawString(label)
	w := d.MeasureString(label)
	return w.Round()
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
