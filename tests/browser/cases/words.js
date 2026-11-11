// The view, and what happens to the chords when the words are edited.
const { test, edit, block, marks, sent, caret, file, save, assert, ORIGINAL } = require("../harness");

test("the sections are shown as words with their chords above", async (page) => {
	await edit(page);
	const got = await page.evaluate(() => ({
		blocks: document.querySelectorAll(".lyr").length,
		tabs: document.querySelectorAll(".tabbox").length,
		rawShown: [...document.querySelectorAll(".sectionbody")].filter((r) => r.offsetParent !== null).length,
		words: [...document.querySelectorAll(".lyrtext")].map((t) => t.value),
		notes: [...document.querySelectorAll(".chip.note")].map((c) => c.textContent),
		chords: document.querySelectorAll(".chips .chip:not(.note)").length,
		tab: document.querySelector(".tabtext").value,
		tag: document.querySelector(".tabtag").textContent,
		help: getComputedStyle(document.querySelector(".floating")).display + "/" + getComputedStyle(document.querySelector(".brackets")).display,
	}));
	assert.strictEqual(got.blocks, 5);
	assert.strictEqual(got.tabs, 1);
	assert.strictEqual(got.rawShown, 0, "the boxes with brackets are still on show");
	assert.ok(!got.words.join("\n").includes("["), "a bracket is showing among the words: " + got.words.join(" / "));
	assert.strictEqual(got.words[1].split("\n")[0], "Bajo la farola de mi calle ");
	assert.deepStrictEqual(got.notes, ["x2", "N.C."]);
	assert.strictEqual(got.chords, 20);
	assert.strictEqual(got.tab, "e|-----0-----|\nB|---1---1---|\nG|-2-------2-|");
	assert.strictEqual(got.tag, "tab");
	assert.strictEqual(got.help, "inline/none");
	assert.strictEqual(await marks(page, 1), "0:Am 10:F 21:C 27:G 31:Am 43:F 53:C 57:x2 134:E7");
});

test("a song nobody touched is saved byte for byte", async (page) => {
	await edit(page);
	for (let i = 0; i < 4; i++) assert.strictEqual(await sent(page, i), null, "section " + i + " would be sent as rows");
	await page.locator("#f-title").focus();
	await save(page);
	assert.strictEqual(await file(page), ORIGINAL);
});

test("typing before a chord pushes it along", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	assert.strictEqual(await marks(page, 1), "0:Am 14:F 25:C 31:G 35:Am 47:F 57:C 61:x2 138:E7");
	assert.strictEqual((await block(page, 1)).text.split("\n")[0], "Bajo muy la farola de mi calle ");
	// And now the section goes back as rows, with the box it arrived in left out.
	const rows = await sent(page, 1);
	assert.deepStrictEqual(rows[0], { words: "Bajo muy la farola de mi calle ", marks: [{ at: 0, text: "Am" }, { at: 14, text: "F" }, { at: 25, text: "C" }, { at: 31, text: "G" }] });
	assert.strictEqual(await sent(page, 0), null, "a section nobody touched would be sent as rows");
});

test("typing exactly where a chord stands leaves it over its letter", async (page) => {
	await edit(page);
	await caret(page, 1, 10);
	await page.keyboard.type("XY");
	assert.ok((await marks(page, 1)).startsWith("0:Am 12:F 23:C"), await marks(page, 1));
	assert.strictEqual((await block(page, 1)).text.slice(0, 16), "Bajo la faXYrola");
});

test("rubbing out the letter under a chord keeps the chord", async (page) => {
	await edit(page);
	await caret(page, 1, 21, 26);            // "calle"
	await page.keyboard.press("Backspace");
	assert.ok((await marks(page, 1)).startsWith("0:Am 10:F 21:C 22:G 26:Am"), await marks(page, 1));
	// Typed over, the chord is above what replaced it.
	await page.keyboard.type("barrio");
	assert.ok((await marks(page, 1)).startsWith("0:Am 10:F 21:C 28:G 32:Am"), await marks(page, 1));
	assert.strictEqual((await block(page, 1)).text.split("\n")[0], "Bajo la farola de mi barrio ");
});

test("replacing a selected word keeps its chord over the new one", async (page) => {
	await edit(page);
	await caret(page, 1, 10, 14);            // "rola", under F
	await page.keyboard.type("ro");
	assert.ok((await marks(page, 1)).startsWith("0:Am 10:F 19:C 25:G"), await marks(page, 1));
});

test("deleting whole lines takes their chords", async (page) => {
	await edit(page);
	await caret(page, 1, 0, 58);             // the first two lines and their line ends
	await page.keyboard.press("Delete");
	assert.strictEqual(await marks(page, 1), "76:E7");
	assert.ok((await block(page, 1)).text.startsWith("y la noche"));
});

test("joining two lines keeps the chords of both", async (page) => {
	await edit(page);
	await caret(page, 1, 28);                // the start of the second line
	await page.keyboard.press("Backspace");
	assert.ok((await marks(page, 1)).startsWith("0:Am 10:F 21:C 27:G 30:Am 42:F"), await marks(page, 1));
});

test("breaking a line takes the chords of the part cut off to the new one", async (page) => {
	await edit(page);
	await caret(page, 1, 8);                 // before "farola"
	await page.keyboard.press("Enter");
	const rows = await sent(page, 1);
	assert.deepStrictEqual(rows[0], { words: "Bajo la ", marks: [{ at: 0, text: "Am" }] });
	assert.deepStrictEqual(rows[1], { words: "farola de mi calle ", marks: [{ at: 2, text: "F" }, { at: 13, text: "C" }, { at: 19, text: "G" }] });
});

test("a new line after a chord at the end of a line leaves it there", async (page) => {
	await edit(page);
	await caret(page, 1, 27);                // the end of the first line, where G stands
	await page.keyboard.press("Enter");
	const rows = await sent(page, 1);
	assert.deepStrictEqual(rows[0].marks.map((m) => m.text), ["Am", "F", "C", "G"]);
	assert.deepStrictEqual(rows[1], { words: "" });
});

test("a line of chords alone stays a line of chords", async (page) => {
	await edit(page);
	assert.strictEqual(await marks(page, 0), "0:Am 1:F 2:C 3:G");
	// Spaces typed on a line of their own are a blank line.
	await caret(page, 4, 15);
	await page.keyboard.press("Enter");
	await page.keyboard.type("   ");
	const rows = await sent(page, 3);
	assert.deepStrictEqual(rows[rows.length - 1], { words: "" });
});

test("the words wrap like a page and never push it sideways", async (page) => {
	await edit(page);
	const got = await page.evaluate(() => {
		const b = document.querySelectorAll(".lyr")[1].block;
		return { box: b.box.scrollHeight, mirror: b.mirror.offsetHeight, shown: b.box.clientHeight,
			sideways: document.documentElement.scrollWidth - document.documentElement.clientWidth };
	});
	assert.ok(Math.abs(got.box - got.mirror) <= 1 && got.box === got.shown, JSON.stringify(got));
	assert.ok(got.sideways <= 0, "the page scrolls sideways by " + got.sideways);
}, { context: { viewport: { width: 360, height: 780 } } });
