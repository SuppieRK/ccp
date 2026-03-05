// Emit real runtime warnings through Node's warning channel.
process.emitWarning("x", { type: "ExperimentalWarning" });
process.emitWarning("x", { type: "ExperimentalWarning" });
process.stdout.write("application payload\n");
