package models

import (
	"strings"
	"sync"
	"testing"
)

// Documents of different stores are signed at the same time: one document's
// whole-amount setting ("50" instead of "50.00") must never reach another's XML.
func TestMarshalZatcaXML_NoLeakBetweenDocuments(t *testing.T) {
	doc := struct {
		XMLName struct{}       `xml:"Doc"`
		Amount  MonetaryAmount `xml:"Amount"`
	}{Amount: MonetaryAmount{Value: 50, CurrencyID: "SAR"}}

	var wg sync.WaitGroup
	errs := make(chan string, 400)
	for i := 0; i < 200; i++ {
		raw := i%2 == 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := marshalZatcaXML(doc, raw)
			if err != nil {
				errs <- err.Error()
				return
			}
			want := ">50.00<"
			if raw {
				want = ">50<"
			}
			if !strings.Contains(string(out), want) {
				errs <- "raw=" + map[bool]string{true: "true", false: "false"}[raw] + ": " + string(out)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if zatcaRawWholeAmounts {
		t.Error("whole-amount setting left on after marshaling")
	}
}
