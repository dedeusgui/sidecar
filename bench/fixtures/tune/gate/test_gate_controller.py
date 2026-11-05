from gate_controller import GateController


def test_admit_vehicle_respects_limit():
    gate = GateController()
    assert gate.admit_vehicle("AAA1111")
    assert not gate.admit_vehicle("BBB2222")
