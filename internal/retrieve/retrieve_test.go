package retrieve

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dedeusgui/sidecar/internal/index"
)

// project is a small, realistic fixture with English identifiers, a Portuguese prompt
// vocabulary target, one unrelated module and a constant with a literal value.
var project = map[string]string{
	"release/manual_release_handler.go": `package release

// ManualReleaseHandler releases vehicles that wait in the ticket queue.
type ManualReleaseHandler struct {
	queue *TicketQueueService
}

// Release advances the first vehicle in the queue when the operator confirms it.
func (h *ManualReleaseHandler) Release(vehicleID string) error {
	h.queue.Enqueue(vehicleID)
	return nil
}
`,
	"queue/ticket_queue_service.go": `package queue

// MaxActiveTickets limits how many tickets may be open at the same time.
const MaxActiveTickets = 1

// TicketQueueService keeps the order of vehicles waiting for admission.
type TicketQueueService struct {
	items []string
}

// Enqueue appends a vehicle to the waiting line.
func (s *TicketQueueService) Enqueue(vehicleID string) {
	s.items = append(s.items, vehicleID)
}
`,
	"vehicle/vehicle_admission.go": `package vehicle

// VehicleAdmission decides whether a truck can enter the weighing area.
type VehicleAdmission struct{}

// Admit checks the vehicle before it enters the scale.
func (v VehicleAdmission) Admit(plate string) bool {
	return plate != ""
}
`,
	"billing/invoice_renderer.go": `package billing

// InvoiceRenderer prints a fiscal document for a customer.
type InvoiceRenderer struct{}

// Render returns the printable invoice text.
func (r InvoiceRenderer) Render(total int) string {
	return "total"
}
`,
	"queue/ticket_queue_service_test.go": `package queue

import "testing"

func TestEnqueueKeepsOrder(t *testing.T) {
	s := &TicketQueueService{}
	s.Enqueue("a")
}
`,
}

func buildIndex(t *testing.T) *index.Index {
	t.Helper()
	root := t.TempDir()
	for rel, body := range project {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ix, _, err := index.Open(root, filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if _, err := ix.Sync(context.Background(), index.Options{}); err != nil {
		t.Fatal(err)
	}
	return ix
}

func names(ev []Evidence) []string {
	out := make([]string, len(ev))
	for i, e := range ev {
		out[i] = e.Hit.Name
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestPortugueseQueryFindsEnglishIdentifiers(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(),
		"Adiantar um caminhão na fila sem afetar a liberação automática", Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := names(res.Evidence)
	if !contains(got, "ManualReleaseHandler") && !contains(got, "Release") {
		t.Fatalf("release handler not retrieved for PT prompt: %v", got)
	}
	if !contains(got, "TicketQueueService") {
		t.Fatalf("queue service not retrieved for 'fila': %v", got)
	}
	if contains(got, "InvoiceRenderer") || contains(got, "Render") {
		t.Fatalf("unrelated billing code leaked into evidence: %v", got)
	}
	if qs := res.Trace.Translations["fila"]; !contains(qs, "queue") {
		t.Fatalf("expected fila→queue translation to be used because queue exists in the repo, trace: %+v", res.Trace.Translations)
	}
}

func TestExactIdentifierIsVerifiedWithLiteralValue(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "o MAX_ACTIVE_TICKETS limita a fila?", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range res.Evidence {
		if e.Hit.Name == "MaxActiveTickets" || e.Hit.Name == "MAX_ACTIVE_TICKETS" {
			found = true
			if e.Verdict != "VERIFIED" {
				t.Errorf("exact identifier verdict = %s, want VERIFIED", e.Verdict)
			}
			if e.Hit.Value != "1" {
				t.Errorf("constant value = %q, want literal 1", e.Hit.Value)
			}
		}
	}
	if !found {
		t.Fatalf("constant not retrieved: %v", names(res.Evidence))
	}
}

func TestUnrelatedGreetingHasNoEvidence(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "bom dia, tudo certo por aí?", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence) != 0 {
		t.Fatalf("greeting produced evidence: %v", names(res.Evidence))
	}
}

func TestStructuralRelationsAddSameFileCallees(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "liberação manual do veículo", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var sawRelation bool
	for _, e := range res.Evidence {
		for _, r := range e.Relations {
			if strings.HasPrefix(r, "calls:") || strings.HasPrefix(r, "called_by:") || strings.HasPrefix(r, "member_of:") || strings.HasPrefix(r, "contains:") {
				sawRelation = true
			}
		}
	}
	if !sawRelation {
		t.Fatalf("no structural relation reported for release handler: %+v", res.Evidence)
	}
}

func TestTestFilesAreReportedAsContracts(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "fila de tickets enqueue", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var sawTest bool
	for _, e := range res.Evidence {
		if e.Hit.Path == "queue/ticket_queue_service_test.go" {
			sawTest = true
			if e.Section != "EXISTING CONTRACTS" {
				t.Errorf("test file section = %q, want EXISTING CONTRACTS", e.Section)
			}
		}
	}
	if !sawTest {
		t.Fatalf("related test not reported: %v", names(res.Evidence))
	}
}

func TestDiversificationLimitsItemsPerFile(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "queue ticket service enqueue constant", Options{MaxItems: 10, MaxPerFile: 2})
	if err != nil {
		t.Fatal(err)
	}
	perFile := map[string]int{}
	for _, e := range res.Evidence {
		perFile[e.Hit.Path]++
	}
	for p, n := range perFile {
		if n > 2 {
			t.Fatalf("file %s contributed %d items, limit 2", p, n)
		}
	}
}

func TestTraceExplainsFactorsAndTimings(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(), "liberação de veículo na fila", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Trace.Terms) == 0 || res.Trace.Lexical == 0 {
		t.Fatalf("trace missing terms or candidate counts: %+v", res.Trace)
	}
	if len(res.Trace.Candidates) == 0 {
		t.Fatal("trace has no candidate factors")
	}
	// A fast stage can read as 0 on a coarse clock (Windows), so only presence and sign matter.
	if d, ok := res.Trace.Timings["lexical"]; !ok || d < 0 {
		t.Fatalf("lexical timing not recorded: %+v", res.Trace.Timings)
	}
	for _, c := range res.Trace.Candidates {
		if c.Name == "" || c.Reason == "" {
			t.Fatalf("candidate without explanation: %+v", c)
		}
	}
}

func TestBudgetAdaptsToEvidenceSize(t *testing.T) {
	ix := buildIndex(t)
	small, _ := Search(context.Background(), ix.Store(), "MAX_ACTIVE_TICKETS", Options{})
	broad, _ := Search(context.Background(), ix.Store(), "fila veículo liberação ticket pesagem entrada", Options{})
	if small.Trace.Budget >= broad.Trace.Budget {
		t.Fatalf("budget did not grow with complexity: small=%d broad=%d", small.Trace.Budget, broad.Trace.Budget)
	}
	if broad.Trace.Budget > 1500 || small.Trace.Budget < 100 {
		t.Fatalf("budget out of documented range: small=%d broad=%d", small.Trace.Budget, broad.Trace.Budget)
	}
}

func TestPortugueseVerbAndAdverbReachEnglishMethod(t *testing.T) {
	ix := buildIndex(t)
	res, err := Search(context.Background(), ix.Store(),
		"Qual método libera manualmente o primeiro veículo da fila?", Options{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range res.Evidence {
		if e.Hit.Name == "Release" && e.Hit.Kind == "method" {
			found = true
		}
	}
	if !found {
		t.Fatalf("method Release not retrieved for 'libera manualmente ... fila': %v (trace %v)", names(res.Evidence), res.Trace.Translations)
	}
}

func TestOnlyCodeLikeTokensCountAsExactIdentifiers(t *testing.T) {
	keys := identifierKeys("quais owners e Owner no MAX_ACTIVE_TICKETS e processManualRelease?")
	has := func(k string) bool {
		for _, x := range keys {
			if x == k {
				return true
			}
		}
		return false
	}
	if has("owners") || has("owner") {
		t.Fatalf("plain English words must not be exact identifiers: %v", keys)
	}
	if !has("maxactivetickets") || !has("processmanualrelease") {
		t.Fatalf("code identifiers missing: %v", keys)
	}
}
