pub fn subtract(a: i32, b: i32) -> i32 {
	a - b
}

#[cfg(test)]
mod tests {
	use super::subtract;

	#[test]
	fn subtracts_positive_numbers() {
		assert_eq!(subtract(5, 3), 2);
	}

	#[test]
	fn failing_case() {
		assert_eq!(subtract(10, 4), 7);
	}
}
