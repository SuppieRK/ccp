function rootCause() {
  throw new Error("root-cause");
}

function fail() {
  try {
    rootCause();
  } catch (err) {
    const wrapped = new Error("boom");
    wrapped.cause = err;
    throw wrapped;
  }
}

fail();
