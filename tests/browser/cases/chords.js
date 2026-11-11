// Putting chords in, moving them, changing them and taking them out.
const { test, edit, block, marks, sent, caret, where, chip, dragChip, isOpen, focusIs, assert } = require("../harness");

const START = "0:Am 10:F 21:C 27:G 31:Am 43:F 53:C 57:x2 134:E7";

// Bring a block to the middle of the window, where the pointer can reach it.
async function into(page, n) {
	await page.evaluate((n) => document.querySelectorAll(".lyr")[n].scrollIntoView({ block: "center" }), n);
	await page.waitForTimeout(30);
}
const has = async (page, n, mark) => (await marks(page, n)).split(" ").includes(mark);
const waitMark = (page, n, mark) => page.waitForFunction(([n, mark]) => {
	const b = document.querySelectorAll(".lyr")[n].block;
	return b.marks.some((m) => m.pos + ":" + m.text === mark);
}, [n, mark], { timeout: 3000 });

test("pressing above a syllable offers a chord and puts one there", async (page) => {
	await edit(page);
	await into(page, 1);
	const at = await where(page, 1, 148);                       // over "acordes", on a line with no chords
	await page.mouse.move(at.x + 3, at.top - 8);
	const offered = await page.evaluate(() => {
		const lyr = document.querySelectorAll(".lyr")[1], g = lyr.querySelector(".chipghost").getBoundingClientRect();
		return { ghost: lyr.querySelector(".chipghost").classList.contains("on"), reach: lyr.classList.contains("reach"),
			line: lyr.querySelector(".chipmark").classList.contains("on"), left: g.left, bottom: g.bottom,
			cursor: getComputedStyle(lyr.querySelector(".lyrtext")).cursor };
	});
	assert.ok(offered.ghost && offered.reach && offered.line, JSON.stringify(offered));
	assert.strictEqual(offered.cursor, "copy");
	assert.ok(Math.abs(offered.bottom - at.top) < 5, "the mark offered is not just above the letters: " + JSON.stringify(offered));

	await page.mouse.click(at.x + 3, at.top - 8);
	assert.ok(await isOpen(page), "the chooser did not open");
	assert.strictEqual(await focusIs(page), "#pickname");
	const usedNames = await page.evaluate(() => [...document.querySelectorAll(".pickchords .chordref")].map((b) => b.textContent));
	assert.deepStrictEqual(usedNames, ["Am", "F", "G", "C", "E7", "Dm"]);
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.blank").length), 1);

	await page.click('#chordpick .pickchords [data-c="E7"]');
	assert.ok(!(await isOpen(page)), "the chooser stayed open");
	assert.ok(await has(page, 1, "148:E7"), await marks(page, 1));
	assert.strictEqual(await focusIs(page), ".chip");
	const rows = await sent(page, 1);
	assert.deepStrictEqual(rows[4], { words: "sin acordes esta línea", marks: [{ at: 4, text: "E7" }] });
});

test("pressing among the words is only a caret", async (page) => {
	await edit(page);
	await into(page, 1);
	const at = await where(page, 1, 148);
	await page.mouse.move(at.x + 3, (at.top + at.bottom) / 2);
	assert.ok(!(await page.evaluate(() => document.querySelector(".chipghost.on"))), "a chord is offered over the words themselves");
	await page.mouse.click(at.x + 3, (at.top + at.bottom) / 2);
	assert.ok(!(await isOpen(page)));
	assert.strictEqual(await focusIs(page), ".lyrtext");
	assert.strictEqual(await marks(page, 1), START);
	assert.strictEqual(await sent(page, 1), null);
});

test("a bracket puts a chord at the caret and typing goes on under it", async (page) => {
	await edit(page);
	await caret(page, 1, 148);
	await page.keyboard.type("[");
	assert.ok(await isOpen(page), "the chooser did not open");
	assert.strictEqual(await focusIs(page), "#pickname");
	await page.keyboard.type("lam");
	await page.keyboard.press("Enter");
	assert.ok(!(await isOpen(page)));
	assert.strictEqual(await focusIs(page), ".lyrtext");
	await page.keyboard.type("otros ");
	await waitMark(page, 1, "148:Am");
	assert.strictEqual((await block(page, 1)).text.split("\n")[4], "sin otros acordes esta línea");

	// The closing bracket ends it too, as it does in a file.
	await page.keyboard.type("[g7]mas");
	await waitMark(page, 1, "154:G7");
	assert.strictEqual((await block(page, 1)).text.split("\n")[4], "sin otros masacordes esta línea");
	const rows = await sent(page, 1);
	assert.deepStrictEqual(rows[4].marks, [{ at: 4, text: "Am" }, { at: 10, text: "G7" }]);
});

test("a bracket after a backslash is only a bracket", async (page) => {
	await edit(page);
	await caret(page, 1, 148);
	await page.keyboard.type("\\[sic] ");
	assert.ok(!(await isOpen(page)));
	assert.strictEqual((await block(page, 1)).text.split("\n")[4], "sin [sic] acordes esta línea");
	assert.strictEqual(await marks(page, 1), START);
});

test("a chord given no name is no chord", async (page) => {
	await edit(page);
	await caret(page, 1, 148);
	await page.keyboard.type("[");
	await page.keyboard.press("Enter");
	assert.strictEqual(await marks(page, 1), START);
	await page.keyboard.type("[");
	await page.keyboard.type("Dm");
	await page.keyboard.press("Escape");
	assert.ok(!(await isOpen(page)));
	assert.strictEqual(await marks(page, 1), START);
	assert.strictEqual(await focusIs(page), ".lyrtext");
	assert.strictEqual(await sent(page, 1), null, "the section counts as changed");
});

test("dragging a chord moves it along the line, letter by letter", async (page) => {
	await edit(page);
	await into(page, 1);
	await dragChip(page, 1, 1, 5, { hold: true });               // F, from over "rola" to over "la"
	const during = await page.evaluate(() => {
		const lyr = document.querySelectorAll(".lyr")[1];
		return { held: !!lyr.querySelector(".chip.held"), line: lyr.querySelector(".chipmark").classList.contains("on") };
	});
	assert.ok(during.held && during.line, JSON.stringify(during));
	await page.mouse.up();
	assert.strictEqual(await marks(page, 1), START.replace("10:F", "5:F"));
	assert.ok(!(await isOpen(page)), "letting go of a dragged chord opened the chooser");
	assert.strictEqual(await focusIs(page), ".chip");
	const rows = await sent(page, 1);
	assert.deepStrictEqual(rows[0].marks, [{ at: 0, text: "Am" }, { at: 5, text: "F" }, { at: 21, text: "C" }, { at: 27, text: "G" }]);
});

test("a chord can be dragged to another line", async (page) => {
	await edit(page);
	await into(page, 1);
	await dragChip(page, 1, 1, 148);
	assert.strictEqual(await marks(page, 1), "0:Am 21:C 27:G 31:Am 43:F 53:C 57:x2 134:E7 148:F");
	// And past the end of a line it stands at the end of it.
	await dragChip(page, 1, 8, 166, { dx: 300 });
	assert.ok(await has(page, 1, "166:F"), await marks(page, 1));
});

test("Escape calls a drag off", async (page) => {
	await edit(page);
	await into(page, 1);
	await dragChip(page, 1, 1, 5, { hold: true, before: () => page.keyboard.press("Escape") });
	await page.mouse.up();
	assert.strictEqual(await marks(page, 1), START);
	assert.ok(!(await isOpen(page)));
	assert.strictEqual(await sent(page, 1), null);
});

test("dragged with Ctrl held, a chord leaves a copy", async (page) => {
	await edit(page);
	await into(page, 1);
	await dragChip(page, 1, 1, 148, { modifier: "Control" });
	assert.strictEqual(await marks(page, 1), START + " 148:F");
});

test("pressing a chord opens the chooser above it, and its choices change it", async (page) => {
	await edit(page);
	await into(page, 1);
	const c = await chip(page, 1, 4);                             // Am over "ción"
	await page.mouse.click(c.x, c.y);
	assert.ok(await isOpen(page));
	await page.waitForFunction(() => document.querySelector("#pickbody .pickbody").getAttribute("data-symbol") === "Am");
	const seen = await page.evaluate(() => {
		const r = document.getElementById("chordpick").getBoundingClientRect();
		return { name: document.getElementById("pickname").value, on: document.querySelector(".pickbody .rootlink.on").textContent,
			kind: document.getElementById("pickkind").value, bass: document.getElementById("pickbass").value,
			left: r.left, right: r.right, top: r.top, bottom: r.bottom, wide: innerWidth, tall: innerHeight };
	});
	assert.deepStrictEqual([seen.name, seen.on, seen.kind, seen.bass], ["Am", "A", "Am", "Am"]);
	assert.ok(seen.bottom <= c.top && seen.top >= 0 && seen.left >= 0 && seen.right <= seen.wide, "not above the chord, or off the screen: " + JSON.stringify(seen));
	assert.ok(Math.abs(seen.left - c.left) < 2, "not lined up with the chord");

	await page.click('#pickbody [data-c="Dm"]');
	await page.waitForFunction(() => document.querySelector("#pickbody .rootlink.on").textContent === "D");
	assert.ok(await has(page, 1, "31:Dm"));
	assert.ok(await isOpen(page), "choosing a note closed the chooser");
	assert.strictEqual(await page.evaluate(() => document.activeElement.textContent), "D", "the keyboard was not handed on to the note chosen");
	await page.focus("#pickkind");
	await page.selectOption("#pickkind", "Dm7");
	await page.waitForFunction(() => document.getElementById("pickkind").value === "Dm7" && document.querySelector("#pickbody .pickbody").getAttribute("data-symbol") === "Dm7");
	await page.focus("#pickbass");
	await page.selectOption("#pickbass", "Dm7/C");
	await waitMark(page, 1, "31:Dm7/C");
	assert.strictEqual(await page.inputValue("#pickname"), "Dm7/C");

	// Each choice hands the keyboard on to what replaced it.
	assert.strictEqual(await focusIs(page), "#pickbass");

	// Escape leaves the chord as it was before the chooser opened.
	await page.keyboard.press("Escape");
	assert.ok(!(await isOpen(page)));
	assert.strictEqual(await marks(page, 1), START);

	// Enter keeps what was chosen.
	await page.mouse.click(c.x, c.y);
	await page.waitForFunction(() => document.querySelector("#pickbody .pickbody").getAttribute("data-symbol") === "Am");
	await page.click('#pickbody [data-c="Em"]');
	await page.locator("#pickname").press("Enter");
	assert.strictEqual(await marks(page, 1), START.replace("31:Am", "31:Em"));
	assert.strictEqual(await focusIs(page), ".chip");
});

test("the chooser goes under the words when there is no room above", async (page) => {
	await edit(page);
	await page.evaluate(() => { const c = document.querySelectorAll(".lyr")[1].block.marks[0].el; scrollBy(0, c.getBoundingClientRect().top - 60); });
	await page.waitForTimeout(30);
	const c = await chip(page, 1, 0);
	await page.mouse.click(c.x, c.y);
	const r = await page.evaluate(() => { const r = document.getElementById("chordpick").getBoundingClientRect(); return { top: r.top, bottom: r.bottom, tall: innerHeight }; });
	const line = await where(page, 1, 0);
	assert.ok(r.top >= line.bottom && r.bottom <= r.tall, JSON.stringify({ r, line }));
});

test("what is typed for a chord is read generously", async (page) => {
	await edit(page);
	await into(page, 1);
	for (const [typed, want, note] of [["sol7", "G7", false], ["am/g", "Am/G", false], ["x3", "x3", true], ["[Bb]", "Bb", false], ["let ring", "let ring", true]]) {
		const c = await chip(page, 1, 0);
		await page.mouse.click(c.x, c.y);
		await page.keyboard.type(typed);
		await page.keyboard.press("Enter");
		await page.waitForFunction(([want, note]) => {
			const m = document.querySelectorAll(".lyr")[1].block.marks[0];
			return m.text === want && m.el.textContent === want && m.el.classList.contains("note") === note;
		}, [want, note], { timeout: 3000 }).catch(async () => { throw new Error(`${typed} became ${await marks(page, 1)}`); });
	}
	assert.strictEqual((await sent(page, 1))[0].marks[0].text, "let ring");
});

test("a chord is taken out with the key, the button or an empty name", async (page) => {
	await edit(page);
	await into(page, 1);
	await page.evaluate(() => document.querySelectorAll(".lyr")[1].block.marks[1].el.focus());
	await page.keyboard.press("Delete");
	assert.strictEqual(await marks(page, 1), START.replace("10:F ", ""));
	assert.strictEqual(await focusIs(page), ".chip");

	let c = await chip(page, 1, 1);                               // C now
	await page.mouse.click(c.x, c.y);
	await page.click("#chordpick [data-remove]");
	assert.strictEqual(await marks(page, 1), START.replace("10:F ", "").replace("21:C ", ""));

	c = await chip(page, 1, 1);                                   // G now
	await page.mouse.click(c.x, c.y);
	await page.keyboard.press("Backspace");
	await page.keyboard.press("Enter");
	assert.strictEqual(await marks(page, 1), "0:Am 31:Am 43:F 53:C 57:x2 134:E7");
	assert.deepStrictEqual((await sent(page, 1))[0], { words: "Bajo la farola de mi calle ", marks: [{ at: 0, text: "Am" }] });
});

test("the keyboard reaches the chords, walks them and moves them", async (page) => {
	await edit(page);
	const url = page.url();
	await caret(page, 1, 9);
	await page.keyboard.press("Tab");                             // to the chord after the caret
	assert.strictEqual(await page.evaluate(() => document.activeElement.textContent + "@" + document.activeElement.mark.pos), "F@10");
	assert.ok(await page.evaluate(() => document.querySelectorAll(".lyr")[1].querySelector(".chipmark").classList.contains("on")), "nothing shows where the focused chord stands");
	await page.keyboard.press("ArrowLeft");
	assert.strictEqual(await page.evaluate(() => document.activeElement.mark.pos), 0);
	await page.keyboard.press("ArrowRight");
	await page.keyboard.press("Alt+ArrowRight");
	assert.strictEqual(await marks(page, 1), START.replace("10:F", "11:F"));
	await page.keyboard.press("Alt+Shift+ArrowRight");           // to the start of "de"
	assert.strictEqual(await marks(page, 1), START.replace("10:F", "15:F"));
	await page.keyboard.press("Alt+Shift+ArrowLeft");            // back to the start of "farola"
	assert.strictEqual(await marks(page, 1), START.replace("10:F", "8:F"));
	await page.keyboard.press("Alt+ArrowDown");                  // to the line below, straight down
	const below = await block(page, 1);
	const moved = below.marks.filter((m) => m[1] === "F").map((m) => m[0]);
	assert.ok(moved[0] >= 34 && moved[0] <= 38, "went to " + moved);
	assert.strictEqual(await focusIs(page), ".chip");
	// None of that was the section moving, which the same keys do elsewhere.
	assert.strictEqual(page.url(), url);
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".sectionlabel")[1].value), "Estrofa");

	// A letter typed on a chord starts its new name.
	await page.keyboard.type("g");
	assert.ok(await isOpen(page));
	await page.keyboard.press("Enter");
	await page.waitForFunction(() => document.querySelectorAll(".lyr")[1].block.marks.some((m) => m.text === "G" && m.pos > 30 && m.pos < 40));
	// And Escape goes back to the words, at the chord.
	await page.keyboard.press("Escape");
	assert.strictEqual(await focusIs(page), ".lyrtext");
});

test("only one chord of a block takes Tab", async (page) => {
	await edit(page);
	const stops = await page.evaluate(() => [...document.querySelectorAll(".lyr")].map((l) => l.querySelectorAll('.chip[tabindex="0"]').length));
	assert.deepStrictEqual(stops, [1, 1, 1, 1, 1]);
});

test("undoing and redoing cover words and chords alike", async (page) => {
	await edit(page);
	await into(page, 1);
	const before = await block(page, 1);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	await dragChip(page, 1, 1, 0, { dx: 40 });
	await page.keyboard.press("Delete");
	const after = await block(page, 1);
	assert.notDeepStrictEqual(after, before);

	await page.keyboard.press("Control+z");
	await page.keyboard.press("Control+z");
	assert.strictEqual((await block(page, 1)).text, after.text, "two steps back should still have the words typed");
	await page.keyboard.press("Control+z");
	assert.deepStrictEqual(await block(page, 1), before);
	await page.keyboard.press("Control+z");                       // nothing left to undo
	assert.deepStrictEqual(await block(page, 1), before);

	await page.keyboard.press("Control+y");
	await page.keyboard.press("Control+Shift+z");
	await page.keyboard.press("Control+y");
	assert.deepStrictEqual(await block(page, 1), after);
	assert.deepStrictEqual((await sent(page, 1))[0].words, "Bajo muy la farola de mi calle ");
});

test("chords too close together stand side by side", async (page) => {
	await edit(page);
	const rows = await page.evaluate(() => [0, 2].map((n) => {
		const b = document.querySelectorAll(".lyr")[n].block;
		return b.marks.map((m) => { const r = m.el.getBoundingClientRect(); return { t: m.text, left: r.left, right: r.right, top: Math.round(r.top), pushed: m.el.classList.contains("pushed") }; });
	}));
	for (const row of rows) {
		for (let i = 1; i < row.length; i++) {
			if (row[i].top === row[i - 1].top) assert.ok(row[i].left >= row[i - 1].right, `${row[i - 1].t} and ${row[i].t} overlap`);
		}
	}
	assert.deepStrictEqual(rows[0].map((c) => c.pushed), [false, false, false, false], "chords with no words under them are marked as moved along");
	// The ones moved along say so, and only those.
	const moved = await page.evaluate(() => document.querySelectorAll(".lyr")[2].block.marks.map((m) => {
		const frame = m.el.closest(".lyr").block.layer.getBoundingClientRect(), r = m.el.getBoundingClientRect(), cs = getComputedStyle(m.el);
		const name = r.left - frame.left + parseFloat(cs.borderLeftWidth) + parseFloat(cs.paddingLeft);
		return { t: m.text, by: Math.round(name - m.x), pushed: m.el.classList.contains("pushed") };
	}));
	for (const c of moved) assert.strictEqual(c.pushed, c.by > 6, JSON.stringify(moved));
	assert.ok(moved.filter((c) => c.pushed).map((c) => c.t).join(" ").startsWith("E7 Am"), JSON.stringify(moved));
});

test("with the chooser open, the pointer is still shown where a press would put a chord", async (page) => {
	await edit(page);
	await into(page, 1);
	const c = await chip(page, 1, 0);
	await page.mouse.click(c.x, c.y);
	assert.ok(await isOpen(page));
	assert.ok(await page.evaluate(() => document.querySelectorAll(".lyr")[1].block.marks[0].el.classList.contains("on")), "nothing marks the chord being changed");
	const at = await where(page, 1, 128);                         // far along a long line, clear of the chooser
	const covered = await page.evaluate(([x, y]) => document.getElementById("chordpick").contains(document.elementFromPoint(x, y)), [at.x + 3, at.top - 8]);
	assert.ok(!covered, "the chooser is over the place this test presses");
	await page.mouse.move(at.x + 3, at.top - 8);
	assert.ok(await page.evaluate(() => !!document.querySelector(".chipghost.on")), "no chord is offered while the chooser is open");
	await page.mouse.click(at.x + 3, at.top - 8);
	assert.ok(await isOpen(page), "the press it was shown did not put a chord there");
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.blank").length), 1);
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.on").length), 1);
	assert.ok((await marks(page, 1)).startsWith("0:Am 10:F"), "the chord that was open changed");
	await page.keyboard.press("Escape");
	assert.strictEqual(await marks(page, 1), START);
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.on").length), 0);
});

test("a tap that closes the chooser on a phone does nothing else, and the chord is never behind the sheet", async (page) => {
	await edit(page);
	// The last chord of the song, with the foot of the page at the foot of the screen.
	await page.evaluate(() => scrollTo(0, document.documentElement.scrollHeight));
	await page.waitForTimeout(80);
	const last = await chip(page, 4, 0);
	await page.touchscreen.tap(last.x, last.y);
	await page.waitForFunction(() => document.querySelector("#pickbody .pickbody").getAttribute("data-symbol") === "G");
	await page.waitForTimeout(150);
	const seen = await page.evaluate(() => {
		const sheet = document.getElementById("chordpick").getBoundingClientRect(), b = document.querySelectorAll(".lyr")[4].block, m = b.marks[0];
		return { sheetTop: sheet.top, chordBottom: m.el.getBoundingClientRect().bottom, wordsBottom: b.layer.getBoundingClientRect().top + m.bottom };
	});
	assert.ok(seen.wordsBottom <= seen.sheetTop, "the chord and its words are behind the sheet: " + JSON.stringify(seen));

	// A tap in the room above a line closes the sheet and is not also a new chord.
	await page.evaluate(() => { document.querySelectorAll(".lyr")[1].scrollIntoView({ block: "start" }); scrollBy(0, -90); });
	await page.waitForTimeout(80);
	const at = await where(page, 1, 34);                           // over "ción", well above the sheet
	assert.ok(await isOpen(page));
	assert.ok(at.top < (await page.evaluate(() => document.getElementById("chordpick").getBoundingClientRect().top)) - 30, "the sheet is over the place this test taps");
	await page.touchscreen.tap(at.x + 40, at.top - 12);
	await page.waitForTimeout(150);
	assert.ok(!(await isOpen(page)), "the sheet stayed, or another opened");
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.blank").length), 0);
	assert.notStrictEqual(await focusIs(page), ".lyrtext", "the closing tap put the caret in the words");
	assert.strictEqual(await marks(page, 1), START);
	// The next one is for a chord.
	await page.touchscreen.tap(at.x + 40, at.top - 12);
	await page.waitForTimeout(150);
	assert.ok(await isOpen(page), "the tap after it did not start a chord");
	await page.touchscreen.tap(at.x + 40, at.top - 12);
	await page.waitForTimeout(150);

	// Nor is it a press on whatever was under the finger: here, the button that throws a section away.
	const sections = await page.evaluate(() => document.querySelectorAll(".songsection").length);
	const c = await chip(page, 1, 0);
	await page.touchscreen.tap(c.x, c.y);
	assert.ok(await isOpen(page));
	const bin = await page.evaluate(() => { const r = document.querySelector('button[name="del1"]').getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; });
	await page.touchscreen.tap(bin.x, bin.y);
	await page.waitForTimeout(300);
	assert.ok(!(await isOpen(page)));
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".songsection").length), sections, "closing the sheet threw a section away");
}, { context: { viewport: { width: 360, height: 640 }, hasTouch: true, isMobile: true, deviceScaleFactor: 2 } });

test("copying, cutting and pasting words takes their chords", async (page) => {
	await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
	await edit(page);
	await caret(page, 1, 0, 27);
	await page.keyboard.press("Control+c");
	await caret(page, 1, 166);
	await page.keyboard.press("Control+v");
	assert.strictEqual(await marks(page, 1), START + " 166:Am 176:F 187:C 193:G");
	assert.strictEqual((await block(page, 1)).text.split("\n")[4], "sin acordes esta líneaBajo la farola de mi calle ");

	// Cut takes them away from where they were.
	await caret(page, 1, 0, 5);
	await page.keyboard.press("Control+x");
	assert.ok((await marks(page, 1)).startsWith("5:F 16:C 22:G"), await marks(page, 1));
	await caret(page, 1, 3);
	await page.keyboard.press("Control+v");
	assert.ok((await marks(page, 1)).startsWith("3:Am 10:F"), await marks(page, 1));
	// What anything else is given is the words alone.
	assert.strictEqual(await page.evaluate(() => navigator.clipboard.readText()), "Bajo ");
});

test("on a phone the chooser is a sheet, a finger drags and a tap puts a chord", async (page) => {
	await edit(page);
	await into(page, 1);
	assert.ok(await page.evaluate(() => matchMedia("(pointer: coarse)").matches), "this is not a phone");
	const cdp = await page.context().newCDPSession(page);
	const touch = (type, x, y) => cdp.send("Input.dispatchTouchEvent", { type, touchPoints: type === "touchEnd" ? [] : [{ x, y }] });

	const c = await chip(page, 1, 1), to = await where(page, 1, 5);
	const anchor = await page.evaluate(() => { const b = document.querySelectorAll(".lyr")[1].block; return b.layer.getBoundingClientRect().left + b.marks[1].x; });
	await touch("touchStart", c.x, c.y);
	// At the pace of a finger: moved in one instant it is a flick, and the tap that follows a flick is swallowed.
	for (let i = 1; i <= 5; i++) { await page.waitForTimeout(60); await touch("touchMove", c.x + (to.x - anchor) * i / 5, c.y); }
	await page.waitForTimeout(250);
	await touch("touchEnd");
	assert.strictEqual(await marks(page, 1), START.replace("10:F", "5:F"));
	assert.ok(!(await isOpen(page)));

	const c2 = await chip(page, 1, 0);
	await page.touchscreen.tap(c2.x, c2.y);
	assert.ok(await isOpen(page));
	const sheet = await page.evaluate(() => { const r = document.getElementById("chordpick").getBoundingClientRect(); return { left: r.left, right: r.right, bottom: r.bottom, wide: innerWidth, tall: innerHeight, focus: document.activeElement.id }; });
	assert.deepStrictEqual([sheet.left, sheet.right, sheet.bottom], [0, sheet.wide, sheet.tall], JSON.stringify(sheet));
	assert.strictEqual(sheet.focus, "chordpick", "the name box took the focus, and the keyboard with it");
	await page.touchscreen.tap(180, 60);                          // outside it
	assert.ok(!(await isOpen(page)));

	const at = await where(page, 1, 148);
	await page.touchscreen.tap(at.x + 3, at.top - 10);
	assert.ok(await isOpen(page), "a tap above a line did not open the chooser");
	assert.notStrictEqual(await focusIs(page), ".lyrtext", "the tap put the caret in the words, and the keyboard up");
	await page.tap('#chordpick .pickchords [data-c="G"]');
	assert.ok(await has(page, 1, "148:G"), await marks(page, 1));
	assert.ok((await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)) <= 0, "the page scrolls sideways");
}, { context: { viewport: { width: 360, height: 780 }, hasTouch: true, isMobile: true, deviceScaleFactor: 2 } });

test("a bracket the keyboard gave no chance to stop is taken back out", async (page) => {
	await edit(page);
	await caret(page, 1, 148);
	// What a phone's keyboard does: the letter is already in the box when the page hears of it.
	await page.evaluate(() => {
		const box = document.querySelectorAll(".lyr")[1].block.box;
		box.setRangeText("[", 148, 148, "end");
		box.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText", data: "[" }));
	});
	assert.ok(await isOpen(page), "the chooser did not open");
	assert.strictEqual((await block(page, 1)).text.split("\n")[4], "sin acordes esta línea");
	await page.keyboard.type("C");
	await page.keyboard.press("Enter");
	assert.ok((await marks(page, 1)).endsWith("148:C"), await marks(page, 1));
});

test("undo from the browser's own menu is the same undo", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	const cancelled = await page.evaluate(() => {
		const box = document.querySelectorAll(".lyr")[1].block.box;
		return !box.dispatchEvent(new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType: "historyUndo" }));
	});
	assert.ok(cancelled, "the browser's own undo was let through");
	assert.strictEqual(await marks(page, 1), START);
	assert.strictEqual((await block(page, 1)).text.split("\n")[0], "Bajo la farola de mi calle ");
});

test("typing in one burst is one step back, and a pause starts another", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("uno ");
	await page.waitForTimeout(1100);
	await page.keyboard.type("dos ");
	await page.keyboard.press("Control+z");
	assert.strictEqual((await block(page, 1)).text.split("\n")[0], "Bajo uno la farola de mi calle ");
	await page.keyboard.press("Control+z");
	assert.strictEqual((await block(page, 1)).text.split("\n")[0], "Bajo la farola de mi calle ");
});

test("a finger going up or down over a chord scrolls the page, and one that rests on it first carries it", async (page) => {
	await edit(page);
	await into(page, 1);
	const cdp = await page.context().newCDPSession(page);
	const touch = (type, x, y) => cdp.send("Input.dispatchTouchEvent", { type, touchPoints: type === "touchEnd" ? [] : [{ x, y }] });
	const paced = async (x1, y1, x2, y2, rest) => {
		await touch("touchStart", x1, y1);
		await page.waitForTimeout(rest);
		for (let i = 1; i <= 6; i++) { await page.waitForTimeout(50); await touch("touchMove", x1 + (x2 - x1) * i / 6, y1 + (y2 - y1) * i / 6); }
		await page.waitForTimeout(250);
		await touch("touchEnd");
		await page.waitForTimeout(150);
	};

	// A swipe upwards that happens to start on a chord.
	let c = await chip(page, 1, 4);
	const before = await page.evaluate(() => scrollY);
	await paced(c.x, c.y, c.x + 4, c.y - 160, 0);
	const after = await page.evaluate(() => scrollY);
	assert.ok(after - before > 80, `the page did not scroll: ${before} -> ${after}`);
	assert.strictEqual(await marks(page, 1), START, "the swipe moved a chord");
	assert.ok(!(await isOpen(page)));

	// Resting on it first picks it up, and then it goes down a line.
	await into(page, 1);
	c = await chip(page, 1, 1);                                    // F on the first line
	const to = await where(page, 1, 28 + 8);                       // "que", on the second
	const stay = await page.evaluate(() => scrollY);
	await paced(c.x, c.y, to.x + 6, to.top - 10, 450);
	assert.strictEqual(await page.evaluate(() => scrollY), stay, "the page scrolled under a chord that had been picked up");
	const got = await marks(page, 1);
	assert.ok(!got.includes("10:F") && /\b3[4-9]:F\b/.test(got), got);
}, { context: { viewport: { width: 360, height: 780 }, hasTouch: true, isMobile: true, deviceScaleFactor: 2 } });
