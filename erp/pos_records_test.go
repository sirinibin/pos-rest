package erp

import (
	"strings"
	"sync"
	"testing"
)

func TestPosRecordValidate(t *testing.T) {
	big := M{"blob": strings.Repeat("x", maxPosDataBytes+1)}
	cases := []struct {
		name string
		rec  M
		prev M
		errs []string
	}{
		{"ok", M{"terminal": "restaurant", "kind": "tables", "data": M{"T1": M{"covers": 2}}}, nil, nil},
		{"ok list data", M{"terminal": "salon", "kind": "appointments", "data": []interface{}{M{"at": "10:00"}}}, nil, nil},
		{"ok key+status", M{"terminal": "thobe", "kind": "measurements", "key": "cus_123", "status": "open"}, nil, nil},
		{"indian demo terminal ok", M{"terminal": "cafein", "kind": "board"}, nil, nil},
		{"unknown terminal", M{"terminal": "casino", "kind": "tables"}, nil, []string{"terminal"}},
		{"missing terminal", M{"kind": "tables"}, nil, []string{"terminal"}},
		{"bad kind", M{"terminal": "barber", "kind": "queue list"}, nil, []string{"kind"}},
		{"missing kind", M{"terminal": "barber"}, nil, []string{"kind"}},
		{"key not text", M{"terminal": "barber", "kind": "stamps", "key": 5}, nil, []string{"key"}},
		{"key too long", M{"terminal": "barber", "kind": "stamps", "key": strings.Repeat("k", 121)}, nil, []string{"key"}},
		{"bad status", M{"terminal": "coffee", "kind": "queue", "status": "in progress!"}, nil, []string{"status"}},
		{"data scalar", M{"terminal": "coffee", "kind": "queue", "data": "x"}, nil, []string{"data"}},
		{"data too large", M{"terminal": "coffee", "kind": "queue", "data": big}, nil, []string{"data"}},
		{"terminal changed", M{"terminal": "coffee", "kind": "queue"}, M{"terminal": "cafesa", "kind": "queue"}, []string{"terminal"}},
		{"kind changed", M{"terminal": "coffee", "kind": "board"}, M{"terminal": "coffee", "kind": "queue"}, []string{"kind"}},
		{"same on update", M{"terminal": "coffee", "kind": "queue"}, M{"terminal": "coffee", "kind": "queue"}, nil},
	}
	for _, c := range cases {
		e := posRecordValidate(nil, c.rec, c.prev)
		if len(e) != len(c.errs) {
			t.Errorf("%s: errors %v, want keys %v", c.name, e, c.errs)
			continue
		}
		for _, k := range c.errs {
			if _, ok := e[k]; !ok {
				t.Errorf("%s: missing error %s in %v", c.name, k, e)
			}
		}
	}
}

func TestPosCounterKey(t *testing.T) {
	if id, e := posCounterKey("restaurant", "kot"); e != nil || id != "restaurant:kot" {
		t.Fatalf("restaurant kot: %q %v", id, e)
	}
	for _, c := range [][2]string{{"casino", "kot"}, {"restaurant", ""}, {"restaurant", "a b"}, {"", ""}} {
		if _, e := posCounterKey(c[0], c[1]); e == nil {
			t.Errorf("%v should be rejected", c)
		}
	}
}

func TestPosRecordsResourceRegistered(t *testing.T) {
	for _, r := range allResources() {
		if r.Path == "pos-records" {
			b, ok := r.Backend.(*nativeBackend)
			if r.Name != "posRecords" || r.Scope != "store" || r.Module != "sales" || !ok || b.coll != posRecordsColl {
				t.Fatalf("pos-records resource: %+v", r)
			}
			for _, k := range []string{"terminal", "kind", "key", "status"} {
				if !b.whereKeys[k] {
					t.Errorf("pos-records should filter by %s", k)
				}
			}
			return
		}
	}
	t.Fatal("pos-records resource missing")
}

func TestPosNextNumber_Unauthenticated(t *testing.T) {
	r := call(t, "POST", "/pos-records/next-number", "", M{"storeId": "x", "terminal": "restaurant", "key": "kot"})
	if r.Code != 401 {
		t.Fatalf("want 401, got %d %s", r.Code, r.Raw)
	}
}

func TestAPI_PosRecords_FilterAndNumbers(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sA := fx.StoreA.Hex()
	key := uniq("k")
	for i, rec := range []M{
		{"storeId": sA, "terminal": "barber", "kind": "queue", "key": key, "data": M{"token": 1}},
		{"storeId": sA, "terminal": "barber", "kind": "stamps", "key": key, "data": M{"n": 3}},
		{"storeId": sA, "terminal": "salon", "kind": "queue", "key": key, "data": M{"token": 2}},
	} {
		if r := call(t, "POST", "/pos-records", tok, rec, "Idempotency-Key", uniq("op")+itoa(i)); r.Code != 201 {
			t.Fatalf("create: %d %s", r.Code, r.Raw)
		}
	}
	l := call(t, "GET", "/pos-records?storeId="+sA+"&where.terminal=barber&where.key="+key+"&limit=50", tok, nil)
	if l.Code != 200 || len(arr(l.Body["data"])) != 2 {
		t.Fatalf("barber records: %d %s", l.Code, l.Raw)
	}
	l = call(t, "GET", "/pos-records?storeId="+sA+"&where.kind=queue&where.terminal=barber,salon&where.key="+key, tok, nil)
	if l.Code != 200 || len(arr(l.Body["data"])) != 2 {
		t.Fatalf("queue records: %d %s", l.Code, l.Raw)
	}
	if bad := call(t, "GET", "/pos-records?storeId="+sA+"&where.data=x", tok, nil); bad.Code != 400 {
		t.Fatalf("filter by data should be refused: %d", bad.Code)
	}
	if bad := call(t, "POST", "/pos-records", tok, M{"storeId": sA, "terminal": "nope", "kind": "x"}, "Idempotency-Key", uniq("op")); bad.Code != 400 {
		t.Fatalf("unknown terminal should be refused: %d", bad.Code)
	}

	// numbers: start on first use, never repeat, even when asked at once
	ck := strings.ReplaceAll(uniq("n"), ".", "_")
	first := call(t, "POST", "/pos-records/next-number", tok, M{"storeId": sA, "terminal": "thobe", "key": ck, "start": 5102})
	if first.Code != 200 || first.Body["number"] != 5102.0 {
		t.Fatalf("first number: %d %s", first.Code, first.Raw)
	}
	var mu sync.Mutex
	seen := map[float64]bool{5102: true}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := call(t, "POST", "/pos-records/next-number", tok, M{"storeId": sA, "terminal": "thobe", "key": ck, "start": 5102})
			n, _ := r.Body["number"].(float64)
			mu.Lock()
			defer mu.Unlock()
			if r.Code != 200 || seen[n] {
				t.Errorf("number %v repeated or failed: %d %s", n, r.Code, r.Raw)
			}
			seen[n] = true
		}()
	}
	wg.Wait()
	if len(seen) != 9 || !seen[5110] {
		t.Fatalf("want 5102..5110, got %v", seen)
	}
	if bad := call(t, "POST", "/pos-records/next-number", tok, M{"storeId": sA, "terminal": "thobe", "key": "a b"}); bad.Code != 400 {
		t.Fatalf("bad key should be refused: %d", bad.Code)
	}
	if bad := call(t, "POST", "/pos-records/next-number", tok, M{"terminal": "thobe", "key": "x"}); bad.Code != 400 {
		t.Fatalf("missing store should be refused: %d", bad.Code)
	}
}

func TestValidatePosSettings(t *testing.T) {
	cases := []struct {
		name string
		rec  M
		errs []string
	}{
		{"absent", M{}, nil},
		{"nil", M{"posSettings": nil}, nil},
		{"ok", M{"posSettings": M{"restaurant": M{"tables": []interface{}{M{"name": "Hall", "tables": "T1:2"}}}, "barber": M{"chairs": 4.0}}}, nil},
		{"not object", M{"posSettings": "x"}, []string{"posSettings"}},
		{"unknown terminal", M{"posSettings": M{"casino": M{}}}, []string{"posSettings.casino"}},
		{"terminal not object", M{"posSettings": M{"barber": 4.0}}, []string{"posSettings.barber"}},
		{"too large", M{"posSettings": M{"barber": M{"x": strings.Repeat("y", maxPosSettingsBytes)}}}, []string{"posSettings"}},
		{"print format ok", M{"posSettings": M{"grocery": M{"printFormat": "A4", "autoPrint": true}}}, nil},
		{"every print format", M{"posSettings": M{"thobe": M{"printFormat": "c95x55"}, "salon": M{"printFormat": "r58"}}}, nil},
		{"unknown print format", M{"posSettings": M{"grocery": M{"printFormat": "A9"}}}, []string{"posSettings.grocery.printFormat"}},
		{"print format not text", M{"posSettings": M{"grocery": M{"printFormat": 80.0}}}, []string{"posSettings.grocery.printFormat"}},
		{"auto print not bool", M{"posSettings": M{"grocery": M{"autoPrint": "yes"}}}, []string{"posSettings.grocery.autoPrint"}},
	}
	for _, c := range cases {
		e := map[string]string{}
		validatePosSettings(c.rec, e)
		if len(e) != len(c.errs) {
			t.Errorf("%s: errors %v, want keys %v", c.name, e, c.errs)
			continue
		}
		for _, k := range c.errs {
			if _, ok := e[k]; !ok {
				t.Errorf("%s: missing error %s in %v", c.name, k, e)
			}
		}
	}
	// stores run it
	if e := storeValidate(nil, M{"nameEn": "S", "posSettings": "x"}, nil); e["posSettings"] == "" {
		t.Errorf("storeValidate should check posSettings: %v", e)
	}
}
