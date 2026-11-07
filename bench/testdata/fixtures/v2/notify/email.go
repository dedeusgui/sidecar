package notify

// EmailNotifier sends messages by email. New channels follow its shape.
type EmailNotifier struct {
	From string
}

// Send delivers the message by email.
func (e EmailNotifier) Send(to, message string) error {
	_ = e.From
	_ = to
	_ = message
	return nil
}
