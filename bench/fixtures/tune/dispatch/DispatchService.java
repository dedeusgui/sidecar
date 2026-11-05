package app.dispatch;

public class DispatchService {
    static final int MAX_CONCURRENT_LOADS = 2;

    public void advanceVehicle(String plate) {
        loads.add(plate);
    }
}
