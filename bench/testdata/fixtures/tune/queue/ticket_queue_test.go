package queue

import "testing"

func TestManualReleaseAdvancesTheLine(t *testing.T) {
	q := &TicketQueueService{}
	q.Enqueue("ABC1234")
	if got := q.ManualRelease(); got != "ABC1234" {
		t.Fatalf("released %q", got)
	}
}
