// Trigger a real unhandled rejection path.
Promise.reject(new Error("boom"));

// Keep process alive long enough for rejection event to surface.
setTimeout(() => {}, 25);
