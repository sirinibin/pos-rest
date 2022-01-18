package main

import (
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"log"
	"os"

	arabic "github.com/abdullahdiaa/garabic"
	"github.com/jung-kurt/gofpdf"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func main() {
	log.Print("Cool")

	GeneratePDF()
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
	addLabel(img, 55, 95, arabic.Shape(arabicStr), color)

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

func addLabel(img *image.RGBA, x, y int, label string, color color.Color) {
	//Load font file
	//You can download amiri font from this link: https://fonts.google.com/specimen/Amiri?preview.text=%D8%A8%D9%90%D8%A7%D9%84%D8%B9%D9%8E%D8%B1%D9%8E%D8%A8%D9%90%D9%91%D9%8A&preview.text_type=custom#standard-styles
	//b, err := ioutil.ReadFile("Amiri-Regular.ttf")
	//b, err := ioutil.ReadFile("Katibeh-Regular.ttf")
	b, err := ioutil.ReadFile("Amiri-Regular.ttf")
	//b, err := ioutil.ReadFile("mirza/Mirza-Regular.ttf")
	if err != nil {
		log.Println(err)
		return
	}

	ttf, err := opentype.Parse(b)
	if err != nil {
		log.Println(err)
		return
	}
	//Create Font.Face from font
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    80,
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
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
