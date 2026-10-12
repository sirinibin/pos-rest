package models

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// withStorePushRecorder swaps the sender for a recorder for one test.
// sent(ids...) returns the events for those stores only: in the integration
// run, writes from other tests' background work can land in the recorder.
func withStorePushRecorder(t *testing.T, delay time.Duration) (sent func(ids ...primitive.ObjectID) []primitive.ObjectID) {
	t.Helper()
	var mu sync.Mutex
	var got []primitive.ObjectID
	oldSend, oldDelay := sendStoreUpdated, StorePushDelay
	sendStoreUpdated = func(id primitive.ObjectID) {
		mu.Lock()
		got = append(got, id)
		mu.Unlock()
	}
	StorePushDelay = delay
	t.Cleanup(func() {
		time.Sleep(3 * delay) // let pending timers fire before restoring
		sendStoreUpdated, StorePushDelay = oldSend, oldDelay
	})
	return func(ids ...primitive.ObjectID) []primitive.ObjectID {
		mu.Lock()
		defer mu.Unlock()
		var out []primitive.ObjectID
		for _, g := range got {
			for _, id := range ids {
				if g == id {
					out = append(out, g)
					break
				}
			}
		}
		return out
	}
}

func TestNotifyStoreUpdated_BurstSendsOneEvent(t *testing.T) {
	sent := withStorePushRecorder(t, 30*time.Millisecond)
	id := primitive.NewObjectID()
	for i := 0; i < 50; i++ {
		NotifyStoreUpdated(id)
	}
	time.Sleep(120 * time.Millisecond)
	got := sent(id)
	if len(got) != 1 || got[0] != id {
		t.Fatalf("want one event for %s, got %v", id.Hex(), got)
	}
}

func TestNotifyStoreUpdated_LaterWriteSendsAgain(t *testing.T) {
	sent := withStorePushRecorder(t, 20*time.Millisecond)
	id := primitive.NewObjectID()
	NotifyStoreUpdated(id)
	time.Sleep(80 * time.Millisecond)
	NotifyStoreUpdated(id)
	time.Sleep(80 * time.Millisecond)
	if got := sent(id); len(got) != 2 {
		t.Fatalf("want an event after each separated write, got %d", len(got))
	}
}

func TestNotifyStoreUpdated_StoresAreIndependent(t *testing.T) {
	sent := withStorePushRecorder(t, 20*time.Millisecond)
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	NotifyStoresUpdated([]primitive.ObjectID{a, b, a, b})
	time.Sleep(80 * time.Millisecond)
	got := sent(a, b)
	if len(got) != 2 {
		t.Fatalf("want one event per store, got %v", got)
	}
	seen := map[primitive.ObjectID]bool{got[0]: true, got[1]: true}
	if !seen[a] || !seen[b] {
		t.Fatalf("want events for both stores, got %v", got)
	}
}

func TestNotifyStoreUpdated_ZeroIDIgnored(t *testing.T) {
	sent := withStorePushRecorder(t, 10*time.Millisecond)
	NotifyStoreUpdated(primitive.NilObjectID)
	time.Sleep(40 * time.Millisecond)
	if got := sent(primitive.NilObjectID); len(got) != 0 {
		t.Fatalf("zero id must not send, got %v", got)
	}
}

func TestNotifyStoreUpdated_ReturnsImmediately(t *testing.T) {
	var calls int32
	oldSend, oldDelay := sendStoreUpdated, StorePushDelay
	sendStoreUpdated = func(primitive.ObjectID) { atomic.AddInt32(&calls, 1) }
	StorePushDelay = time.Hour
	defer func() { sendStoreUpdated, StorePushDelay = oldSend, oldDelay }()
	start := time.Now()
	NotifyStoreUpdated(primitive.NewObjectID())
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("NotifyStoreUpdated must not block the write path")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("event must wait for the bundling delay")
	}
}

// A write landing while the event is being sent still gets its own event:
// nothing written is left unannounced.
func TestNotifyStoreUpdated_ConcurrentWritersAlwaysFollowedByEvent(t *testing.T) {
	sent := withStorePushRecorder(t, 5*time.Millisecond)
	id := primitive.NewObjectID()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				NotifyStoreUpdated(id)
				time.Sleep(time.Millisecond)
			}
		}()
	}
	wg.Wait()
	last := len(sent(id))
	time.Sleep(40 * time.Millisecond)
	if len(sent(id)) == 0 {
		t.Fatal("no event sent")
	}
	storePushMu.Lock()
	pending := storePushPending[StoreUpdatedEvent+"|"+id.Hex()]
	storePushMu.Unlock()
	if pending {
		t.Fatal("a write was left pending with no event")
	}
	if len(sent(id)) < last {
		t.Fatal("events went missing")
	}
	if len(sent(id)) >= 8*40 {
		t.Fatalf("writes were not bundled: %d events for %d writes", len(sent(id)), 8*40)
	}
}

func TestUserMayUseStore(t *testing.T) {
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	cases := []struct {
		name  string
		role  string
		ids   []*primitive.ObjectID
		store primitive.ObjectID
		want  bool
	}{
		{"admin any store", "Admin", []*primitive.ObjectID{&b}, a, true},
		{"no assigned stores means every store", "User", nil, a, true},
		{"assigned store", "User", []*primitive.ObjectID{&b, &a}, a, true},
		{"other store", "Manager", []*primitive.ObjectID{&b}, a, false},
		{"nil entries skipped", "User", []*primitive.ObjectID{nil}, a, false},
	}
	for _, c := range cases {
		if got := UserMayUseStore(c.role, c.ids, c.store); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// TestStoreWritesNotify fails when a function writes to the store collection
// without calling NotifyStoreUpdated/NotifyStoresUpdated, so browsers keeping
// one copy of the store per page would keep showing the old store. Inserts
// are not checked: no browser holds a store that did not exist yet.
func TestStoreWritesNotify(t *testing.T) {
	writeOps := `(UpdateOne|UpdateMany|ReplaceOne|DeleteOne|DeleteMany|FindOneAndUpdate|FindOneAndReplace|FindOneAndDelete|BulkWrite)\(`
	directWrite := regexp.MustCompile(`Collection\("store"\)\s*\.\s*` + writeOps)
	bind := regexp.MustCompile(`(?m)(\w+)\s*:?=\s*[^\n]*Collection\("store"\)\s*$`)
	notify := regexp.MustCompile(`NotifyStores?Updated\(`)

	var offenders []string
	checked := 0
	for _, dir := range []string{".", "../controller"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				body := string(src[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
				if !strings.Contains(body, `Collection("store")`) {
					continue
				}
				writes := directWrite.MatchString(body)
				for _, m := range bind.FindAllStringSubmatch(body, -1) {
					if regexp.MustCompile(`\b` + regexp.QuoteMeta(m[1]) + `\s*\.\s*` + writeOps).MatchString(body) {
						writes = true
					}
				}
				if !writes {
					continue
				}
				checked++
				// A write no screen shows may opt out with a "store-push: silent"
				// comment that says why.
				if !notify.MatchString(body) && !strings.Contains(body, "store-push: silent") {
					offenders = append(offenders, path+": "+fn.Name.Name)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("found no store writes at all; the scan is broken")
	}
	if len(offenders) > 0 {
		t.Fatalf("these functions write the store without NotifyStoreUpdated:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestNotifyProcurementMessagesChanged_BundledPerType(t *testing.T) {
	var mu sync.Mutex
	got := map[string]int{}
	id := primitive.NewObjectID()
	oldSend, oldDelay := sendProcurementMessagesChanged, StorePushDelay
	sendProcurementMessagesChanged = func(storeID primitive.ObjectID, msgType string) {
		if storeID != id {
			return // another test's background work
		}
		mu.Lock()
		got[msgType]++
		mu.Unlock()
	}
	StorePushDelay = 20 * time.Millisecond
	defer func() {
		time.Sleep(60 * time.Millisecond)
		sendProcurementMessagesChanged, StorePushDelay = oldSend, oldDelay
	}()
	for i := 0; i < 50; i++ {
		NotifyProcurementMessagesChanged(id, "email")
	}
	NotifyProcurementMessagesChanged(id, "whatsapp")
	NotifyProcurementMessagesChanged(primitive.NilObjectID, "email")
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if got["email"] != 1 || got["whatsapp"] != 1 || len(got) != 2 {
		t.Fatalf("want one event per type, got %v", got)
	}
}

// A store change and a message change at the same moment are separate events.
func TestCoalesce_TopicsDoNotSwallowEachOther(t *testing.T) {
	sent := withStorePushRecorder(t, 20*time.Millisecond)
	var msgs int32
	oldSend := sendProcurementMessagesChanged
	id := primitive.NewObjectID()
	sendProcurementMessagesChanged = func(storeID primitive.ObjectID, _ string) {
		if storeID == id {
			atomic.AddInt32(&msgs, 1)
		}
	}
	defer func() { sendProcurementMessagesChanged = oldSend }()
	NotifyStoreUpdated(id)
	NotifyProcurementMessagesChanged(id, "whatsapp")
	time.Sleep(80 * time.Millisecond)
	if len(sent(id)) != 1 || atomic.LoadInt32(&msgs) != 1 {
		t.Fatalf("store events %d, message events %d; want 1 and 1", len(sent(id)), msgs)
	}
}

func TestPushJobProgress_LatestSnapshotToOwner(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]interface{}
	var owners []string
	oldSend, oldDelay := sendJobProgress, StorePushDelay
	sendJobProgress = func(owner string, data map[string]interface{}) {
		mu.Lock()
		owners = append(owners, owner)
		got = append(got, data)
		mu.Unlock()
	}
	StorePushDelay = 20 * time.Millisecond
	defer func() {
		time.Sleep(60 * time.Millisecond)
		sendJobProgress, StorePushDelay = oldSend, oldDelay
	}()
	var step int32
	snap := func() interface{} { return atomic.LoadInt32(&step) }
	for i := 1; i <= 30; i++ {
		atomic.StoreInt32(&step, int32(i))
		PushJobProgress("u1", "backup", "job-1", snap)
	}
	PushJobProgress("", "backup", "job-2", snap)
	PushJobProgress("u1", "backup", "", snap)
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("want one bundled event, got %d", len(got))
	}
	if owners[0] != "u1" || got[0]["job_id"] != "job-1" || got[0]["kind"] != "backup" || got[0]["progress"] != int32(30) {
		t.Fatalf("want the latest snapshot for the owner, got %v to %v", got[0], owners[0])
	}
}

func TestNotifyFrontendDeployed_OncePerGap(t *testing.T) {
	oldSend, oldGap := sendFrontendDeployed, FrontendDeployedMinGap
	storePushMu.Lock()
	oldLast := lastFrontendDeployed
	lastFrontendDeployed = time.Time{}
	storePushMu.Unlock()
	t.Cleanup(func() {
		sendFrontendDeployed, FrontendDeployedMinGap = oldSend, oldGap
		storePushMu.Lock()
		lastFrontendDeployed = oldLast
		storePushMu.Unlock()
	})

	var mu sync.Mutex
	sent := 0
	sendFrontendDeployed = func() { mu.Lock(); sent++; mu.Unlock() }
	FrontendDeployedMinGap = 100 * time.Millisecond

	results := make(chan bool, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- NotifyFrontendDeployed() }()
	}
	wg.Wait()
	close(results)
	announced := 0
	for r := range results {
		if r {
			announced++
		}
	}
	if announced != 1 {
		t.Fatalf("%d of 20 simultaneous calls announced, want 1", announced)
	}

	time.Sleep(150 * time.Millisecond)
	if !NotifyFrontendDeployed() {
		t.Fatal("a call after the gap must announce again")
	}
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if sent != 2 {
		t.Fatalf("sent %d times, want 2", sent)
	}
}

func TestNotifyDashboardRecomputed_OncePerBurstPerStore(t *testing.T) {
	oldSend, oldDelay := sendDashboardRecomputed, StorePushDelay
	t.Cleanup(func() { sendDashboardRecomputed, StorePushDelay = oldSend, oldDelay })
	StorePushDelay = 20 * time.Millisecond

	var mu sync.Mutex
	got := map[primitive.ObjectID]int{}
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	sendDashboardRecomputed = func(id primitive.ObjectID) {
		if id != a && id != b && !id.IsZero() {
			return // another test's background work
		}
		mu.Lock()
		got[id]++
		mu.Unlock()
	}

	NotifyDashboardRecomputed(a)
	NotifyDashboardRecomputed(a)
	NotifyDashboardRecomputed(b)
	NotifyDashboardRecomputed(primitive.NilObjectID)
	time.Sleep(80 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if got[a] != 1 || got[b] != 1 || len(got) != 2 {
		t.Fatalf("sent %v, want one event each for the two stores", got)
	}
}
