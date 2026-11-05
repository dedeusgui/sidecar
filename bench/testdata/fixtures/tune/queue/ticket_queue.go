package queue

// TicketQueueService keeps the order of trucks waiting for admission.
type TicketQueueService struct {
	items []string
}

// Enqueue appends a truck to the waiting line.
func (s *TicketQueueService) Enqueue(plate string) {
	s.items = append(s.items, plate)
}

// ManualRelease advances the first truck in the line when the operator confirms it.
func (s *TicketQueueService) ManualRelease() string {
	if len(s.items) == 0 {
		return ""
	}
	first := s.items[0]
	s.items = s.items[1:]
	return first
}

// PeekNext returns the next truck without removing it from the line.
func (s *TicketQueueService) PeekNext() string {
	if len(s.items) == 0 {
		return ""
	}
	return s.items[0]
}

// Length reports how many trucks are waiting.
func (s *TicketQueueService) Length() int {
	return len(s.items)
}
