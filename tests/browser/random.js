// Random editing. A session types, rubs out, puts chords in, drags them, renames
// them, moves them with the keys, copies and pastes, undoes and redoes, in
// whatever order its seed gives. After every step the page is asked whether
//
//   - the chords of each block are in the order they stand in,
//   - each stands at a whole place inside its words, and never inside a character,
//   - the words the editor holds are the words in the box,
//   - what is drawn is what it holds, name for name,
//   - what a section would send says what its words and chords say on the screen,
//   - nothing was left in hand: no chooser open, no chord being carried.
//
// A session ends by undoing everything and redoing it, which must bring the same
// words and chords back, and by saving the song and opening it again, where
// every line must have its chords over the same letters.
//
//   node random.js [sessions] [steps] [first seed]
const { start, stop, launch, open, edit, make, caret, where, dragChip, isOpen, save } = require("./harness");

const SONGS = [
	["[G]la la\n[C]lo [D]lo lo [x2]"],
	["A[G]mazing grace how [C]sweet [x2]\n\nthe [Am]sound\n```tab\ne|-0-|\n```\n\n[F]that saved"],
	["uno [C][x2]dos\ntres", "[Am] [F]  [C]"],
	["😀[C]la 😁[D]lo\nla"],
];
const NAMES = ["Am", "G", "x2", "lam", "D7", "sol", "C/E"];
const WORDS = ["a", "b", " ", " ", "\n", "la ", "é", "😀", "ab\ncd", "x"];
const STEPS = ["type", "type", "type", "erase", "erase", "bracket", "bracket", "above", "keys", "keys", "drag", "drag", "undo", "undo", "redo", "clipboard", "rename", "pause"];
const KEYS = ["Alt+ArrowLeft", "Alt+ArrowRight", "Alt+Shift+ArrowLeft", "Alt+Shift+ArrowRight", "Alt+ArrowUp", "Alt+ArrowDown", "Delete", "ArrowRight", "Escape"];

function rng(seed) {
	let s = seed >>> 0;
	return () => (s = (s * 1664525 + 1013904223) >>> 0) / 4294967296;
}

// look runs in the page. It answers with what breaks a rule, the blocks of words
// as the editor holds them, and the lines of the song with the chords over them.
function look() {
	const problems = [], blocks = [], lines = [];
	if (window.sameVisit !== true) return { problems: ["the page was left or loaded again"], blocks, lines };
	if (document.getElementById("chordpick").matches(":popover-open")) problems.push("the chooser was left open");
	if (document.querySelector(".chip.held")) problems.push("a chord was left in hand");
	document.querySelectorAll(".songsection").forEach((sec, si) => {
		const shown = [];
		sec.querySelectorAll(".lyr, .tabbox").forEach((el) => {
			const b = el.block;
			if (b.fenced) {
				const inside = b.box.value.replace(/\n+$/, "");
				if (inside.trim() !== "") shown.push("```" + b.info + "\n" + inside);
				return;
			}
			const say = (what) => problems.push("section " + si + ", block " + blocks.length + ": " + what);
			const places = b.marks.map((m) => m.text + "@" + m.pos).join(" ");
			if (!b.marks.every((m, i) => !i || b.marks[i - 1].pos <= m.pos)) say("the chords are out of order: " + places);
			b.marks.forEach((m) => {
				if (!(Number.isInteger(m.pos) && m.pos >= 0 && m.pos <= b.text.length)) say(m.text + " stands at " + m.pos + " of " + b.text.length);
				const unit = b.text.charCodeAt(m.pos);
				if (unit >= 0xDC00 && unit <= 0xDFFF) say(m.text + "@" + m.pos + " stands inside a character");
				if (!m.text) say("a chord with no name was left at " + m.pos);
				else if (!m.el || m.el.textContent !== m.text) say("the chord drawn says " + (m.el && m.el.textContent) + " and the one held says " + m.text);
			});
			if (b.text !== b.box.value) say("the words held are " + JSON.stringify(b.text) + " and the box has " + JSON.stringify(b.box.value));
			const drawn = [...b.layer.children];
			if (drawn.length !== b.marks.length || !drawn.every((chip, i) => chip === b.marks[i].el && chip.mark === b.marks[i])) say("drawn [" + drawn.map((c) => c.textContent).join(" ") + "], held [" + places + "]");
			if (b.marks.length && el.querySelectorAll('.chip[tabindex="0"]').length !== 1) say("not exactly one chord takes Tab");

			blocks.push({ text: b.text, marks: b.marks.map((m) => [m.pos, m.text]), undo: b.undo.length, redo: b.redo.length });
			let begin = 0;
			b.text.split("\n").forEach((line) => {
				const over = b.marks.filter((m) => m.text && m.pos >= begin && m.pos <= begin + line.length).map((m) => (m.pos - begin) + ":" + m.text);
				if (over.length || line.trim() !== "") shown.push(line + " | " + over.join(" "));
				begin += line.length + 1;
			});
		});
		const store = sec.querySelector(".sectionrows");
		if (store && !store.disabled) {
			const sent = JSON.parse(store.value).map((row) => {
				if (row.fenced) return "```" + (row.info || "") + "\n" + row.words;
				const over = (row.marks || []).map((m) => m.at + ":" + m.text);
				return over.length || row.words !== "" ? row.words + " | " + over.join(" ") : null;
			}).filter((line) => line !== null);
			if (JSON.stringify(sent) !== JSON.stringify(shown)) problems.push("section " + si + " would send " + JSON.stringify(sent) + " and shows " + JSON.stringify(shown));
		}
		lines.push(...shown);
	});
	return { problems, blocks, lines };
}

const into = async (page, n) => { await page.evaluate((n) => document.querySelectorAll(".lyr")[n].scrollIntoView({ block: "center" }), n); await page.waitForTimeout(30); };
const focus = (page, n, k) => page.evaluate(([n, k]) => { const b = document.querySelectorAll(".lyr")[n].block; (k < 0 ? b.box : b.marks[k].el).focus(); }, [n, k]);

// Every name typed has been read by the server, so none goes marked as unread.
const answered = (page) => page.waitForFunction(() => ![...document.querySelectorAll(".sectionrows")].some((s) => !s.disabled && s.value.includes('"typed"')), null, { timeout: 5000 });

// Typed a key at a time, or all at once the way a paste or a dictation arrives.
async function put(page, text) {
	if (text.length > 2) return page.keyboard.insertText(text);
	for (const ch of text) {
		if (ch === "\n") await page.keyboard.press("Enter"); else await page.keyboard.type(ch);
	}
}

// A press where there is nothing to press.
async function elsewhere(page) {
	const at = await page.evaluate(() => {
		for (const [x, y] of [[innerWidth - 6, innerHeight / 2], [6, innerHeight / 2], [innerWidth - 6, 120]]) {
			const el = document.elementFromPoint(x, y);
			if (el && !el.closest("a, button, input, textarea, select, label, summary, .lyr, #chordpick")) return [x, y];
		}
		return null;
	});
	if (!at) throw new Error("there is nowhere on the page to press that is nothing");
	await page.mouse.click(at[0], at[1]);
}

async function session(browser, seed, steps) {
	const r = rng(seed), pick = (a) => a[Math.floor(r() * a.length)], int = (n) => Math.floor(r() * n);
	const page = await open(browser);
	await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
	const which = int(SONGS.length);
	const slug = await make(page, "---\ntitle: Al azar " + seed + "\n---\n\n" + SONGS[which].map((body, i) => "## Parte " + (i + 1) + "\n" + body + "\n").join("\n"));
	await edit(page, slug);
	await page.evaluate(() => { window.sameVisit = true; return navigator.clipboard.writeText(""); });

	const trail = [];
	const broke = (problems) => ({ seed, song: which, trail, problems });
	let seen = await page.evaluate(look);
	if (seen.problems.length) return broke(seen.problems);

	for (let i = 0; i < steps; i++) {
		const n = int(seen.blocks.length), b = seen.blocks[n], size = b.text.length;
		// A caret never stands between the two halves of one character.
		const whole = (text, at) => (at > 0 && at < text.length && text.charCodeAt(at) >= 0xDC00 && text.charCodeAt(at) <= 0xDFFF ? at - 1 : at);
		const place = () => whole(b.text, int(size + 1));
		const step = pick(STEPS);
		let label = step;
		try {
			if (step === "type") {
				const from = place(), to = r() < 0.25 ? whole(b.text, Math.min(size, from + int(6))) : from, text = pick(WORDS);
				await caret(page, n, from, to);
				await put(page, text);
				label = `type ${JSON.stringify(text)} over ${from}..${to}`;
			} else if (step === "erase") {
				const from = place(), to = r() < 0.4 ? whole(b.text, Math.min(size, from + int(8))) : from, key = r() < 0.4 ? "Delete" : "Backspace";
				await caret(page, n, from, to);
				await page.keyboard.press(key);
				label = `${key} at ${from}..${to}`;
			} else if (step === "bracket" || step === "above") {
				const at = place(), name = pick(NAMES), how = pick(["enter", "enter", "enter", "escape", "elsewhere", "empty"]);
				label = `${step === "bracket" ? "a bracket" : "a press above"} at ${at}, ${how === "empty" ? "no name" : name}, ${how}`;
				if (step === "bracket") {
					await caret(page, n, at);
					await page.keyboard.type("[");
				} else {
					await into(page, n);
					const spot = await where(page, n, at);
					await page.mouse.click(spot.x + 3, spot.top - 8);
				}
				if (await isOpen(page)) {
					if (how !== "empty") await page.keyboard.type(name);
					if (how === "escape") await page.keyboard.press("Escape");
					else if (how === "elsewhere") await elsewhere(page);
					else await page.keyboard.press("Enter");
					if (step === "bracket" && how === "enter" && r() < 0.8) {
						const text = pick(["do", "re mi", "x\n", "\n"]);
						await put(page, text);
						label += `, then ${JSON.stringify(text)}`;
					}
				}
			} else if (step === "keys") {
				if (!b.marks.length) continue;
				const k = int(b.marks.length), key = pick(KEYS);
				await focus(page, n, k);
				await page.keyboard.press(key);
				label = `${key} on ${b.marks[k][1]}@${b.marks[k][0]}`;
			} else if (step === "drag") {
				if (!b.marks.length) continue;
				const k = int(b.marks.length), to = place(), how = pick(["drop", "drop", "drop", "copy", "escape"]);
				await into(page, n);
				await dragChip(page, n, k, to, how === "copy" ? { modifier: "Control" } : how === "escape" ? { hold: true, before: () => page.keyboard.press("Escape") } : {});
				if (how === "escape") await page.mouse.up();
				// Let go where it was picked up, a drag is a press, and a press opens the chooser.
				if (await isOpen(page)) await page.keyboard.press("Escape");
				label = `drag ${b.marks[k][1]}@${b.marks[k][0]} to ${to}, ${how}`;
			} else if (step === "undo" || step === "redo") {
				await focus(page, n, b.marks.length && r() < 0.3 ? int(b.marks.length) : -1);
				await page.keyboard.press(step === "undo" ? "Control+z" : "Control+y");
			} else if (step === "clipboard") {
				const from = place(), to = whole(b.text, Math.min(size, from + int(10))), key = r() < 0.5 ? "Control+x" : "Control+c";
				await caret(page, n, from, to);
				await page.keyboard.press(key);
				const now = (await page.evaluate(look)).blocks, m = int(now.length), text = now[m].text;
				const at = whole(text, int(text.length + 1)), end = r() < 0.3 ? whole(text, Math.min(text.length, at + int(5))) : at;
				await caret(page, m, at, end);
				await page.keyboard.press("Control+v");
				label = `${key} of ${from}..${to}, pasted in block ${m} over ${at}..${end}`;
			} else if (step === "rename") {
				if (!b.marks.length) continue;
				const k = int(b.marks.length), how = pick(["enter", "escape", "elsewhere", "choose", "choose and escape", "remove"]);
				label = `rename ${b.marks[k][1]}@${b.marks[k][0]}, ${how}`;
				await focus(page, n, k);
				await page.keyboard.press("Enter");
				if (!(await isOpen(page))) label += " (the chooser did not open)";
				else if (how === "remove") await page.click("#chordpick [data-remove]");
				else if (how.startsWith("choose")) {
					const offered = page.locator("#pickbody [data-c]");
					await offered.first().waitFor({ timeout: 1500 }).catch(() => {});
					const count = await offered.count();
					if (count) await offered.nth(int(count)).click();
					if (how === "choose") await elsewhere(page); else await page.keyboard.press("Escape");
					// A note that is not a chord is offered nothing, and Escape took the keyboard back to it.
					if (await isOpen(page)) await page.keyboard.press("Escape");
				} else {
					const name = pick(NAMES);
					await page.keyboard.press("Control+a");
					await page.keyboard.type(name);
					if (how === "escape") await page.keyboard.press("Escape");
					else if (how === "elsewhere") await elsewhere(page);
					else await page.keyboard.press("Enter");
					label += ` as ${name}`;
				}
			} else await page.waitForTimeout(40);
		} catch (err) {
			trail.push(label);
			return broke(["the step threw: " + String(err && err.message || err).split("\n")[0]]);
		}
		trail.push(label);
		seen = await page.evaluate(look);
		if (seen.problems.length) return broke(seen.problems);
	}

	// Everything undone and done again.
	await answered(page);
	const held = (blocks) => JSON.stringify(blocks.map((b) => [b.text, b.marks]));
	const before = await page.evaluate(look);
	for (let n = 0; n < before.blocks.length; n++) {
		await focus(page, n, -1);
		for (let i = 0; i < before.blocks[n].undo; i++) await page.keyboard.press("Control+z");
		await answered(page);
		const back = (await page.evaluate(look)).blocks[n];
		if (back.undo) return broke([`block ${n}: ${back.undo} steps were left to undo after ${before.blocks[n].undo} undos`]);
		// As many as were undone, and no more: what a session undid itself stays undone.
		for (let i = 0; i < before.blocks[n].undo; i++) await page.keyboard.press("Control+y");
		await answered(page);
	}
	const after = await page.evaluate(look);
	trail.push("everything undone and redone");
	if (after.problems.length) return broke(after.problems);
	if (held(after.blocks) !== held(before.blocks)) return broke(["undone and redone, it is " + held(after.blocks) + " and it was " + held(before.blocks)]);

	// Saved and opened again.
	await page.locator("#f-title").focus();
	await save(page);
	await edit(page, slug);
	await page.evaluate(() => { window.sameVisit = true; });
	const again = await page.evaluate(look);
	trail.push("saved and opened again");
	if (again.problems.length) return broke(again.problems);
	if (JSON.stringify(again.lines) !== JSON.stringify(after.lines)) return broke(["saved and opened again, it is " + JSON.stringify(again.lines) + " and it was " + JSON.stringify(after.lines)]);
	if (page.problems.length) return broke(["the page complained: " + page.problems.join(" | ")]);
	await page.context().close();
	return null;
}

(async () => {
	const sessions = +process.argv[2] || 30, steps = +process.argv[3] || 40, first = +process.argv[4] || 1;
	await start();
	const browser = await launch();
	let bad = 0;
	for (let seed = first; seed < first + sessions; seed++) {
		const out = await session(browser, seed, steps);
		if (!out) { console.log(`ok    seed ${seed}`); continue; }
		bad++;
		console.log(`FAIL  seed ${seed}, on song ${out.song}, after ${out.trail.length} steps\n      the last of them: ${out.trail.slice(-4).join(" | ")}\n      ${out.problems.slice(0, 6).join("\n      ")}`);
	}
	await browser.close();
	await stop();
	console.log(bad ? `\n${bad} of ${sessions} sessions broke a rule: node random.js 1 ${steps} <seed> runs one again` : `\n${sessions} sessions of ${steps} steps broke no rule`);
	process.exit(bad ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(2); });
