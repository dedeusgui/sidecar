package shop.customers;

public class CustomerRepository {
    static final int PASSWORD_MIN_LENGTH = 8;

    public Customer findByEmail(String email) {
        return byEmail.get(email);
    }
}
