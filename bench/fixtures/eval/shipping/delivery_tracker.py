class DeliveryTracker:
    """Follows parcels from the carrier until they reach the customer."""

    def __init__(self):
        self.status = {}

    def track_shipment(self, tracking_code):
        return self.status.get(tracking_code, "unknown")

    def mark_delivered(self, tracking_code):
        self.status[tracking_code] = "delivered"
