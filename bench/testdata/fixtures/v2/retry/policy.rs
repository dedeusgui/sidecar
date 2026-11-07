/// Maximum number of times a declined payment is retried.
pub const MAX_RETRY_ATTEMPTS: u32 = 5;

/// Delay in seconds before the given retry attempt.
pub fn next_delay(attempt: u32) -> u64 {
    2u64.pow(attempt.min(MAX_RETRY_ATTEMPTS))
}
