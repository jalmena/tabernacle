// Everything here, one after another, and what came of each.
const { spawnSync } = require("child_process");
const path = require("path");

const CHECKS = ["run.js", "geometry.js", "roundtrip.js", "random.js", "firefox.js"];

const results = CHECKS.map((script) => {
	console.log("\n== " + script);
	const code = spawnSync(process.execPath, [path.join(__dirname, script)], { stdio: "inherit" }).status;
	// firefox.js answers 3 when there is no Firefox to try.
	return [script, code === 0 ? "passed" : script === "firefox.js" && code === 3 ? "skipped" : "FAILED"];
});
console.log("\n" + results.map(([script, how]) => how.padEnd(8) + script).join("\n"));
process.exit(results.some(([, how]) => how === "FAILED") ? 1 : 0);
