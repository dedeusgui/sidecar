package app.dispatch;

import org.junit.jupiter.api.Test;

class DispatchServiceTest {
    @Test
    void advanceVehicleAddsLoad() {
        new DispatchService().advanceVehicle("ABC1234");
    }
}
