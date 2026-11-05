package release

import (
	"net/http"
	"strings"
)

// MaxActiveTickets limits concurrent tickets in the queue.
const MaxActiveTickets = 1

// ManualReleaseHandler releases vehicles waiting in the ticket queue.
type ManualReleaseHandler struct {
	queue *TicketQueue
}

// Release advances the first vehicle in the queue.
func (h *ManualReleaseHandler) Release(id string) error {
	h.queue.Enqueue(strings.TrimSpace(id))
	return nil
}

func NewHandler() *ManualReleaseHandler {
	return &ManualReleaseHandler{}
}

var _ = http.StatusOK
