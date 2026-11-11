// A Tabernacle to try things on, a browser to try them with, a small runner,
// and the hands the cases share.
const { spawn } = require("child_process");
const assert = require("assert");
const fs = require("fs");
const net = require("net");
const os = require("os");
const path = require("path");

const ROOT = path.join(__dirname, "..", "..");
const BIN = process.env.TABERNACLE_BIN || "tabernacle";
const SHOTS = process.env.SHOTS || path.join(os.tmpdir(), "tabernacle-browser-shots");
const WORK = fs.mkdtempSync(path.join(os.tmpdir(), "tabernacle-browser-"));
const DATA = path.join(WORK, "data");

// The song most cases edit: made up for them, so that every place in it is known.
const SLUG = "inventadas-la-farola";
const ORIGINAL = fs.readFileSync(path.join(__dirname, "songs", "Inventadas", "La farola.song"), "utf8");

let server = null, port = 0, base = "";

function freePort() {
	return new Promise((ok, no) => {
		const s = net.createServer();
		s.once("error", no);
		s.listen(0, "127.0.0.1", () => { const p = s.address().port; s.close(() => ok(p)); });
	});
}

function stop() {
	const was = server;
	server = null;
	if (!was || was.exitCode !== null || was.signalCode !== null) return Promise.resolve();
	return new Promise((ok) => { was.once("exit", ok); was.kill("SIGTERM"); });
}

// start puts a fresh instance up and answers with its address. Its collection is
// the example songs and the one made up for these tests, or a copy of opts.songs;
// whatever a case did to the last instance is gone.
async function start(opts = {}) {
	await stop();
	fs.rmSync(DATA, { recursive: true, force: true });
	if (opts.songs) fs.cpSync(opts.songs, path.join(DATA, "songs"), { recursive: true });
	else {
		fs.cpSync(path.join(ROOT, "examples", "songs"), path.join(DATA, "songs"), { recursive: true });
		fs.cpSync(path.join(__dirname, "songs"), path.join(DATA, "songs"), { recursive: true });
	}
	if (!port) port = await freePort();
	base = "http://127.0.0.1:" + port;
	const log = fs.openSync(path.join(WORK, "server.log"), "w");
	const env = Object.assign({}, process.env, { TABERNACLE_DATA_DIR: DATA, TABERNACLE_ADDR: "127.0.0.1:" + port, TABERNACLE_BASE_URL: base, TABERNACLE_LANG: opts.lang || "es" });
	const child = spawn(BIN, [], { env, stdio: ["ignore", log, log] });
	fs.closeSync(log);
	let gone = null;
	child.once("error", (e) => { gone = e.code === "ENOENT" ? `no Tabernacle to run at "${BIN}": build one and name it in TABERNACLE_BIN` : String(e); });
	child.once("exit", (code) => { gone = gone || "Tabernacle stopped as it started (" + code + "):\n" + fs.readFileSync(path.join(WORK, "server.log"), "utf8").slice(-1500); });
	server = child;
	for (let i = 0; i < 100 && !gone; i++) {
		try { if ((await fetch(base + "/healthz")).ok) return base; } catch (e) { /* not listening yet */ }
		await new Promise((r) => setTimeout(r, 50));
	}
	server = null;
	child.kill("SIGKILL");
	throw new Error(gone || "Tabernacle did not answer on " + base);
}

process.on("exit", () => {
	if (server) server.kill("SIGKILL");
	fs.rmSync(WORK, { recursive: true, force: true });
});
process.on("SIGINT", () => process.exit(130));

function playwright() {
	try {
		return require(process.env.PLAYWRIGHT_CORE || "playwright-core");
	} catch (e) {
		console.error("playwright-core was not found: point PLAYWRIGHT_CORE at one (see tests/browser/README.md)");
		process.exit(2);
	}
}

function launch() {
	return playwright().chromium.launch({ headless: true, executablePath: process.env.CHROMIUM || undefined });
}

// A page on the running instance. It keeps every script error, console error and
// refused style or script in page.problems, and a dialog nobody was expecting too.
async function open(browser, context = {}, how = {}) {
	fs.mkdirSync(SHOTS, { recursive: true });
	const ctx = await browser.newContext(Object.assign({ baseURL: base, viewport: { width: 1280, height: 900 }, locale: "es-ES" }, context));
	const page = await ctx.newPage();
	page.problems = [];
	page.on("pageerror", (e) => page.problems.push("pageerror: " + e.message));
	page.on("console", (m) => {
		if (m.type() === "error" || /Content Security Policy|Refused to/i.test(m.text())) page.problems.push("console: " + m.text());
	});
	// A case that expects to be asked something answers for itself.
	if (!how.dialogs) page.on("dialog", (d) => { page.problems.push("dialog: " + d.message()); d.dismiss(); });
	return page;
}

const tests = [];
let group = "";

// test adds a case. opts.context is passed to the browser, opts.lang to the
// instance; opts.dialogs leaves dialogs to the case, and opts.noisy lets it log
// errors it provokes on purpose.
function test(name, fn, opts) { tests.push({ name, fn, group, opts: opts || {} }); }

// load reads a file of cases, which a word given to run can then name.
function load(file) {
	group = path.basename(file, ".js");
	require(file);
}

// run tries every case whose name or file holds the word given, each on an
// instance of its own, and answers with how many failed.
async function run(word) {
	const browser = await launch();
	let failed = 0, ran = 0;
	for (const t of tests) {
		if (word && !t.name.includes(word) && t.group !== word) continue;
		await start({ lang: t.opts.lang });
		const page = await open(browser, t.opts.context, t.opts);
		let err = null;
		try {
			await t.fn(page);
			if (page.problems.length && !t.opts.noisy) throw new Error("the page complained: " + page.problems.join(" | "));
		} catch (e) {
			err = e;
			try { await page.screenshot({ path: path.join(SHOTS, "fail-" + t.name.replace(/[^a-z0-9]+/gi, "-").slice(0, 60) + ".png") }); } catch (x) { /* the page is gone */ }
		}
		await page.context().close();
		ran++;
		if (err) {
			failed++;
			console.log(`FAIL  ${t.name}\n      ${String(err.message || err).split("\n").slice(0, 12).join("\n      ")}`);
		} else console.log(`ok    ${t.name}`);
	}
	await browser.close();
	await stop();
	console.log(`\n${ran - failed}/${ran} passed` + (failed ? ", screenshots of the failures in " + SHOTS : ""));
	return failed;
}

// edit opens a song in the editor and waits for the words to be laid out.
async function edit(page, slug) {
	await page.goto("/songs/" + (slug || SLUG) + "/edit");
	await page.waitForFunction(() => document.documentElement.classList.contains("chordsfloat") || !window.HTMLTemplateElement);
	await page.waitForTimeout(80);
}

// The state of one block of words: its text and its chords as [place, name, isChord].
function block(page, n) {
	return page.evaluate((n) => {
		const b = document.querySelectorAll(".lyr")[n].block;
		return { text: b.text, marks: b.marks.map((m) => [m.pos, m.text, m.chord]) };
	}, n);
}
async function marks(page, n) { return (await block(page, n)).marks.map((m) => m[0] + ":" + m[1]).join(" "); }

// What a section would send: its rows when it has been touched, null when it has not.
function sent(page, i) {
	return page.evaluate((i) => {
		const store = document.querySelector('[name="rows' + i + '"]'), raw = document.querySelector('[name="body' + i + '"]');
		if (store.disabled !== raw.disabled) return store.disabled ? null : JSON.parse(store.value);
		return "BOTH " + store.disabled;
	}, i);
}

async function caret(page, n, from, to) {
	await page.evaluate(([n, from, to]) => {
		const box = document.querySelectorAll(".lyr")[n].block.box;
		box.focus();
		box.setSelectionRange(from, to === undefined ? from : to);
		box.dispatchEvent(new Event("select", { bubbles: true }));
	}, [n, from, to]);
}

// Where on the screen a place in a block's words is: the left of its letter, and the top and bottom of the letters there.
function where(page, n, pos) {
	return page.evaluate(([n, pos]) => {
		const b = document.querySelectorAll(".lyr")[n].block;
		let i = 0;
		while (i + 1 < b.starts.length && b.starts[i + 1] <= pos) i++;
		const row = b.mirror.children[i], col = pos - b.starts[i];
		let r;
		if (col < b.lines[i].length) {
			const range = document.createRange();
			range.setStart(row.firstChild, col); range.setEnd(row.firstChild, col + 1);
			r = range.getClientRects()[0];
		} else r = row.lastChild.getBoundingClientRect();
		return { x: r.left, top: r.top, bottom: r.bottom };
	}, [n, pos]);
}

// The middle of a chord on the screen.
function chip(page, n, k) {
	return page.evaluate(([n, k]) => {
		const r = document.querySelectorAll(".lyr")[n].block.marks[k].el.getBoundingClientRect();
		return { x: r.left + r.width / 2, y: r.top + r.height / 2, left: r.left, right: r.right, top: r.top, bottom: r.bottom };
	}, [n, k]);
}

// dragChip carries a chord with the mouse to over a place in the words.
// opts.hold leaves the button down, opts.before runs with the chord in hand,
// opts.modifier is a key held while it is let go, opts.dx misses by that much.
async function dragChip(page, n, k, toPos, opts) {
	opts = opts || {};
	const from = await chip(page, n, k);
	const anchor = await page.evaluate(([n, k]) => {
		const b = document.querySelectorAll(".lyr")[n].block, m = b.marks[k];
		return b.layer.getBoundingClientRect().left + m.x;
	}, [n, k]);
	const to = await where(page, n, toPos);
	await page.mouse.move(from.x, from.y);
	await page.mouse.down();
	const tx = from.x + (to.x - anchor) + (opts.dx || 0), ty = to.top - 8;
	await page.mouse.move((from.x + tx) / 2, (from.y + ty) / 2, { steps: 4 });
	await page.mouse.move(tx, ty, { steps: 4 });
	if (opts.before) await opts.before();
	if (opts.modifier) await page.keyboard.down(opts.modifier);
	if (!opts.hold) await page.mouse.up();
	if (opts.modifier) await page.keyboard.up(opts.modifier);
}

const isOpen = (page) => page.evaluate(() => document.getElementById("chordpick").matches(":popover-open"));
const focusIs = (page) => page.evaluate(() => {
	const a = document.activeElement;
	return a ? (a.id ? "#" + a.id : a.className ? "." + String(a.className).split(" ")[0] : a.tagName) : "";
});

// The file of a song as it is on disk.
async function file(page, slug) {
	return page.evaluate(async (u) => (await fetch(u)).text(), "/songs/" + (slug || SLUG) + "/raw");
}

// A song put into the collection as the text of its file, and the name it can then be opened by.
async function make(page, text) {
	await page.goto("/songs/new");
	return page.evaluate(async (text) => {
		const res = await fetch("/songs", { method: "POST", body: new URLSearchParams({ text }) });
		return res.url.split("/songs/")[1];
	}, text);
}

async function save(page) {
	await Promise.all([page.waitForNavigation(), page.keyboard.press("Control+Enter")]);
}

module.exports = {
	start, stop, launch, open, test, load, run,
	edit, block, marks, sent, caret, where, chip, dragChip, isOpen, focusIs, file, make, save,
	assert, SHOTS, SLUG, ORIGINAL,
};
