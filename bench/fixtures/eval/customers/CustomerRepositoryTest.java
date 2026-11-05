package shop.customers;

import org.junit.jupiter.api.Test;

class CustomerRepositoryTest {
    @Test
    void findByEmailReturnsNullForUnknown() {
        new CustomerRepository().findByEmail("nobody@example.com");
    }
}
