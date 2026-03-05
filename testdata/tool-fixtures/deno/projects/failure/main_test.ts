Deno.test("fails intentionally", () => {
  const left = 1;
  const right = 2;
  if (left !== right) {
    throw new Error(`intentional failure: ${left} != ${right}`);
  }
});
