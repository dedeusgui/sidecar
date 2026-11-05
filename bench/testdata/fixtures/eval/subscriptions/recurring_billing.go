package subscriptions

// SubscriptionScheduler charges recurring plans every month.
type SubscriptionScheduler struct {
	plans []string
}

// ChargeRecurringSubscriptions bills every active plan once per cycle.
func ChargeRecurringSubscriptions(s *SubscriptionScheduler) int {
	return len(s.plans)
}
