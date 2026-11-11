// A small WebDriver BiDi client for a Firefox already on the machine, so that
// what depends on how a browser lays text out is tried in a second engine.
// It needs nothing installed: the socket is Node's own.
const { spawn, spawnSync } = require("child_process");
const fs = require("fs");
const net = require("net");
const os = require("os");
const path = require("path");

const FIREFOX = process.env.FIREFOX || "firefox";

// The keys WebDriver has its own codes for.
const KEYS = { Enter: "\uE007", Escape: "\uE00C", Backspace: "\uE003", Delete: "\uE017", Tab: "\uE004", ArrowRight: "\uE014", ArrowLeft: "\uE012", Control: "\uE009", Alt: "\uE00A", Shift: "\uE008" };

function there() {
	const tried = spawnSync(FIREFOX, ["--version"], { stdio: "ignore" });
	return !tried.error && tried.status === 0;
}

function freePort() {
	return new Promise((ok, no) => {
		const s = net.createServer();
		s.once("error", no);
		s.listen(0, "127.0.0.1", () => { const p = s.address().port; s.close(() => ok(p)); });
	});
}

async function launch() {
	const profile = fs.mkdtempSync(path.join(os.tmpdir(), "tabernacle-firefox-"));
	fs.writeFileSync(path.join(profile, "user.js"), [
		'user_pref("browser.shell.checkDefaultBrowser", false);',
		'user_pref("datareporting.policy.dataSubmissionEnabled", false);',
		'user_pref("toolkit.telemetry.reportingpolicy.firstRun", false);',
		'user_pref("browser.startup.homepage_override.mstone", "ignore");',
		'user_pref("remote.active-protocols", 1);',
		'user_pref("ui.prefersReducedMotion", 1);',
	].join("\n"));
	const port = await freePort();
	const proc = spawn(FIREFOX, ["--headless", "--no-remote", "--profile", profile, "--remote-debugging-port", String(port), "--width", "1280", "--height", "900", "about:blank"], { stdio: ["ignore", "pipe", "pipe"] });
	const gone = () => { proc.kill(); fs.rmSync(profile, { recursive: true, force: true }); };
	process.on("exit", gone);
	let log = "";
	proc.stderr.on("data", (d) => { log += d; });
	proc.stdout.on("data", (d) => { log += d; });
	for (let i = 0; i < 150 && !/WebDriver BiDi listening/.test(log); i++) await new Promise((r) => setTimeout(r, 100));
	if (!/WebDriver BiDi listening/.test(log)) { gone(); throw new Error("Firefox did not start WebDriver BiDi:\n" + log.slice(-2000)); }

	const ws = new WebSocket(`ws://127.0.0.1:${port}/session`);
	await new Promise((ok, no) => { ws.onopen = ok; ws.onerror = (e) => no(new Error("websocket: " + (e.message || e.type))); });
	let next = 1;
	const waiting = new Map();
	const events = [];
	ws.onmessage = (msg) => {
		const data = JSON.parse(msg.data);
		if (data.id && waiting.has(data.id)) {
			const { ok, no } = waiting.get(data.id);
			waiting.delete(data.id);
			if (data.type === "error") no(new Error(data.error + ": " + data.message)); else ok(data.result);
		} else if (data.method) events.push(data);
	};
	const send = (method, params) => new Promise((ok, no) => { const id = next++; waiting.set(id, { ok, no }); ws.send(JSON.stringify({ id, method, params: params || {} })); });

	await send("session.new", { capabilities: {} });
	await send("session.subscribe", { events: ["log.entryAdded"] });
	const context = (await send("browsingContext.getTree", {})).contexts[0].context;
	const mouse = (actions) => send("input.performActions", { context, actions: [{ type: "pointer", id: "mouse", parameters: { pointerType: "mouse" }, actions }] });
	const keys = (actions) => send("input.performActions", { context, actions: [{ type: "key", id: "keys", actions }] });

	return {
		// What the browser logged, errors among it.
		events,
		goto: (url) => send("browsingContext.navigate", { context, url, wait: "complete" }),
		// evaluate runs a function in the page and gives back what it returns, as JSON carries it.
		async evaluate(fn, arg) {
			const r = await send("script.callFunction", {
				functionDeclaration: `async (arg) => JSON.stringify(await (${fn.toString()})(JSON.parse(arg)))`,
				arguments: [{ type: "string", value: JSON.stringify(arg === undefined ? null : arg) }],
				target: { context }, awaitPromise: true,
			});
			if (r.type === "exception") throw new Error("in the page: " + r.exceptionDetails.text);
			return r.result.type === "string" ? JSON.parse(r.result.value) : undefined;
		},
		async screenshot(file) {
			const r = await send("browsingContext.captureScreenshot", { context });
			fs.writeFileSync(file, Buffer.from(r.data, "base64"));
		},
		setViewport: (width, height) => send("browsingContext.setViewport", { context, viewport: { width, height } }),
		drag: (x1, y1, x2, y2) => mouse([
			{ type: "pointerMove", x: Math.round(x1), y: Math.round(y1) }, { type: "pointerDown", button: 0 },
			{ type: "pointerMove", x: Math.round((x1 + x2) / 2), y: Math.round((y1 + y2) / 2), duration: 80 },
			{ type: "pointerMove", x: Math.round(x2), y: Math.round(y2), duration: 80 }, { type: "pause", duration: 60 }, { type: "pointerUp", button: 0 },
		]),
		click: (x, y) => mouse([{ type: "pointerMove", x: Math.round(x), y: Math.round(y) }, { type: "pointerDown", button: 0 }, { type: "pause", duration: 40 }, { type: "pointerUp", button: 0 }]),
		move: (x, y) => mouse([{ type: "pointerMove", x: Math.round(x), y: Math.round(y) }]),
		type: (text) => keys([...text].flatMap((ch) => [{ type: "keyDown", value: ch }, { type: "keyUp", value: ch }])),
		// press sends one key, named as the page would name it, with the modifiers held.
		press(key, mods) {
			const held = (mods || []).map((m) => KEYS[m]), k = KEYS[key] || key;
			return keys(held.map((value) => ({ type: "keyDown", value })).concat([{ type: "keyDown", value: k }, { type: "keyUp", value: k }], held.reverse().map((value) => ({ type: "keyUp", value }))));
		},
		sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
		async close() {
			try { await send("browser.close", {}); } catch (e) { /* it is going anyway */ }
			ws.close();
			gone();
		},
	};
}

module.exports = { launch, there };
