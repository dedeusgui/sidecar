MAX_ACTIVE_TICKETS = 1


class GateController:
    """Opens the barrier for trucks that were admitted at the gate."""

    def __init__(self):
        self.active = []

    def admit_vehicle(self, plate):
        if len(self.active) >= MAX_ACTIVE_TICKETS:
            return False
        self.active.append(plate)
        return True

    def open_barrier(self, plate):
        """Raise the barrier so the admitted truck can pass."""
        return plate in self.active
