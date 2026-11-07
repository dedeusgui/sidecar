package tickets

// MaxActiveTickets caps how many tickets may be active at the same time.
const MaxActiveTickets = 10

// Queue holds the tickets that are currently active.
type Queue struct {
	active []string
}

// Admit adds a ticket while there is room under MaxActiveTickets.
func (q *Queue) Admit(id string) bool {
	if len(q.active) >= MaxActiveTickets {
		return false
	}
	q.active = append(q.active, id)
	return true
}
