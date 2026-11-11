// Every case in cases/, in Chromium. A word narrows it to the cases whose name
// holds it, or to the file of that name: node run.js chords, node run.js "a bracket".
const fs = require("fs");
const path = require("path");
const { load, run } = require("./harness");

const dir = path.join(__dirname, "cases");
for (const f of fs.readdirSync(dir).filter((f) => f.endsWith(".js")).sort()) load(path.join(dir, f));
run(process.argv[2]).then((failed) => process.exit(failed ? 1 : 0), (e) => { console.error(e); process.exit(2); });
