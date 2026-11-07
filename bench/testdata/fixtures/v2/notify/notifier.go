package notify

// Notifier sends a message to a recipient through one channel.
type Notifier interface {
	Send(to, message string) error
}
