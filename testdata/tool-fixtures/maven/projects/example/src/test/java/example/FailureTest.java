package example;

import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;

public class FailureTest {
    @Test
    void test() {
        Assertions.assertTrue(false);
    }
}
