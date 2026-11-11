// The corners: the edge of a line, of a drag, of an undo and of a name not yet read, each as the keys that reach it.
const { test, edit, block, marks, sent, caret, where, chip, dragChip, isOpen, focusIs, file, save, make, assert } = require("../harness");

const song = (body) => "---\ntitle: Caso\n---\n\n## Estrofa\n" + body + "\n";
const into = async (page, n) => { await page.evaluate((n) => document.querySelectorAll(".lyr")[n].scrollIntoView({ block: "center" }), n); await page.waitForTimeout(30); };
const waitMark = (page, n, mark) => page.waitForFunction(([n, mark]) => document.querySelectorAll(".lyr")[n].block.marks.some((m) => m.pos + ":" + m.text === mark), [n, mark], { timeout: 3000 });
// Every name typed so far has been read by the server, so none goes marked as unread.
const answered = (page, i) => page.waitForFunction((i) => !document.querySelector('[name="rows' + i + '"]').value.includes('"typed"'), i, { timeout: 3000 });
const bodies = (page) => page.evaluate(() => [...document.querySelectorAll(".songsection")].map((s) => s.querySelector(".sectionbody").value));

test("a chord put in where another stands, and then its word, is saved on its own line", async (page) => {
	const slug = await make(page, song("[G]la la"));
	await edit(page, slug);
	await caret(page, 0, 0);
	await page.keyboard.type("[D");
	await page.keyboard.press("Enter");
	await page.keyboard.type("new words");
	assert.strictEqual(await marks(page, 0), "0:D 9:G");
	await page.keyboard.press("Enter");
	await answered(page, 0);
	assert.deepStrictEqual(await sent(page, 0), [
		{ words: "new words", marks: [{ at: 0, text: "D" }] },
		{ words: "la la", marks: [{ at: 0, text: "G" }] },
	]);
	await save(page);
	assert.ok((await file(page, slug)).endsWith("## Estrofa\n[D]new words\n[G]la la\n"), await file(page, slug));
});

test("the same in the middle of a line", async (page) => {
	const slug = await make(page, song("Amazing [C]grace"));
	await edit(page, slug);
	await caret(page, 0, 8);
	await page.keyboard.type("[G");
	await page.keyboard.press("Enter");
	await page.keyboard.type("sweet ");
	assert.strictEqual(await marks(page, 0), "8:G 14:C");
	await answered(page, 0);
	assert.deepStrictEqual(await sent(page, 0), [{ words: "Amazing sweet grace", marks: [{ at: 8, text: "G" }, { at: 14, text: "C" }] }]);
});

test("undoing does not forget that a chord was waiting for its word", async (page) => {
	const slug = await make(page, song("la [C]sweet lo"));
	await edit(page, slug);
	await caret(page, 0, 3, 8);
	await page.keyboard.press("Delete");
	await page.waitForTimeout(1100);
	await page.keyboard.type("soft");
	await page.keyboard.press("Control+z");
	await page.keyboard.type("kind");
	assert.deepStrictEqual(await sent(page, 0), [{ words: "la kind lo", marks: [{ at: 3, text: "C" }] }]);
});

test("letters the file holds as an annotation are still a chord when typed as one", async (page) => {
	const slug = await make(page, song("la [am]la"));
	await edit(page, slug);
	assert.ok(await page.evaluate(() => document.querySelector(".chip").classList.contains("note")), "the one in the file is not shown as an annotation");
	await caret(page, 0, 0);
	await page.keyboard.type("[am");
	await page.keyboard.press("Enter");
	await waitMark(page, 0, "0:Am");
	assert.strictEqual(await marks(page, 0), "0:Am 3:am");
});

test("a section nobody touched divides after a fenced block whose text ends in a blank line", async (page) => {
	const slug = await make(page, song("la\n```tab\ne|-0-|\n\n```\nlo\nli"));
	await edit(page, slug);
	assert.strictEqual(await sent(page, 0), null);
	await caret(page, 1, 0);
	await Promise.all([page.waitForNavigation(), page.keyboard.press("Alt+Enter")]);
	assert.deepStrictEqual(await bodies(page), ["la\n```tab\ne|-0-|\n```", "lo\nli"]);
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".sectionbody[hidden]").length), 2, "a half came back as a box of text");
});

test("Alt and a sideways arrow in the words move the chord at the caret and never leave the page", async (page) => {
	await page.goto("/");
	await edit(page);
	const url = page.url();
	await caret(page, 1, 148);
	await page.keyboard.type("[Am");
	await page.keyboard.press("Enter");
	await waitMark(page, 1, "148:Am");
	assert.strictEqual(await focusIs(page), ".lyrtext");
	await page.keyboard.press("Alt+ArrowRight");
	assert.ok((await marks(page, 1)).endsWith("149:Am"), await marks(page, 1));
	// With no chord there, nothing happens at all.
	await caret(page, 1, 100);
	await page.keyboard.press("Alt+ArrowLeft");
	await page.keyboard.press("Alt+ArrowRight");
	await page.waitForTimeout(200);
	assert.strictEqual(page.url(), url, "the page was left");
	assert.ok((await marks(page, 1)).endsWith("149:Am"));
});

test("an Escape that closed nothing does not make the next press throw the name away", async (page) => {
	await edit(page);
	await into(page, 1);
	const c = await chip(page, 1, 0);
	await page.mouse.click(c.x, c.y);
	await page.keyboard.type("D");
	// What shutting a list or calling off an accent looks like to the page: the key, and the chooser still open.
	await page.evaluate(() => document.activeElement.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
	assert.ok(await isOpen(page));
	await page.mouse.click(40, 300);
	assert.ok(!(await isOpen(page)));
	assert.ok((await marks(page, 1)).startsWith("0:D "), await marks(page, 1));
});

test("a line emptied and then joined away takes its chords, the last one too", async (page) => {
	await edit(page);
	await caret(page, 1, 28, 57);                                 // the whole of the second line, not its line end
	await page.keyboard.press("Delete");
	assert.ok((await marks(page, 1)).includes("28:Am 28:F 28:C 28:x2"), await marks(page, 1));
	await page.keyboard.press("Backspace");
	assert.strictEqual(await marks(page, 1), "0:Am 10:F 21:C 27:G 104:E7");
	assert.strictEqual((await block(page, 1)).text.split("\n").length, 4);
});

test("but typed over, the emptied line keeps them, the last one at its end", async (page) => {
	await edit(page);
	await caret(page, 1, 28, 57);
	await page.keyboard.press("Delete");
	await page.keyboard.type("otra");
	assert.ok((await marks(page, 1)).includes("28:Am 28:F 28:C 32:x2"), await marks(page, 1));
});

test("a selection that runs to the end of a line takes the chord at its end", async (page) => {
	await edit(page);
	await caret(page, 1, 27, 57);                                 // from the end of the first line to the end of the second
	await page.keyboard.press("Delete");
	assert.strictEqual(await marks(page, 1), "0:Am 10:F 21:C 27:G 104:E7");
});

test("a line of chords alone joins the line above with its chords", async (page) => {
	const slug = await make(page, song("uno\n[Am][F]"));
	await edit(page, slug);
	await caret(page, 0, 4);
	await page.keyboard.press("Backspace");
	assert.strictEqual(await marks(page, 0), "3:Am 3:F");
	assert.strictEqual((await block(page, 0)).text, "uno");
});

test("chords that share a place are pasted in the order they were in", async (page) => {
	await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
	const slug = await make(page, song("la [C][x2]lo"));
	await edit(page, slug);
	await caret(page, 0, 0, 5);
	await page.keyboard.press("Control+c");
	await caret(page, 0, 5);
	await page.keyboard.press("Control+v");
	assert.strictEqual(await marks(page, 0), "3:C 3:x2 8:C 8:x2");
});

test("an emoji typed over one never leaves a chord inside a character", async (page) => {
	const slug = await make(page, song("😀a [C]la"));
	await edit(page, slug);
	assert.strictEqual(await marks(page, 0), "4:C");
	await caret(page, 0, 0, 6);
	await page.keyboard.insertText("😁");
	assert.strictEqual(await marks(page, 0), "0:C");
	await save(page);
	assert.ok((await file(page, slug)).endsWith("## Estrofa\n[C]😁\n"), await file(page, slug));
});

test("after a drag that was called off, a key still opens the chooser", async (page) => {
	await edit(page);
	await into(page, 1);
	await dragChip(page, 1, 1, 5, { hold: true, before: () => page.keyboard.press("Escape") });
	await page.mouse.move(40, 300);
	await page.mouse.up();
	await page.evaluate(() => document.querySelectorAll(".lyr")[1].block.marks[1].el.focus());
	await page.keyboard.press("Enter");
	assert.ok(await isOpen(page), "the key was swallowed");
});

test("a chord with no name yet cannot be picked up", async (page) => {
	await edit(page);
	await into(page, 1);
	const at = await where(page, 1, 148);
	await page.mouse.click(at.x + 3, at.top - 8);
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.blank").length), 1);
	const blank = await page.evaluate(() => { const r = document.querySelector(".chip.blank").getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; });
	await page.mouse.move(blank.x, blank.y);
	await page.mouse.down({ });
	await page.mouse.move(blank.x + 80, blank.y, { steps: 4 });
	await page.mouse.up();
	await page.keyboard.press("Escape");
	assert.strictEqual(await marks(page, 1), "0:Am 10:F 21:C 27:G 31:Am 43:F 53:C 57:x2 134:E7");
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.blank").length), 0);
});

test("words cannot be dragged about inside their box", async (page) => {
	await edit(page);
	const stopped = await page.evaluate(() => !document.querySelectorAll(".lyr")[1].block.box.dispatchEvent(new DragEvent("dragstart", { bubbles: true, cancelable: true })));
	assert.ok(stopped);
});

test("a chord whose pointer let go where nothing heard of it goes back", async (page) => {
	await edit(page);
	await into(page, 1);
	await dragChip(page, 1, 1, 5, { hold: true });
	assert.ok((await marks(page, 1)).startsWith("0:Am 5:F"));
	// The move a browser sends once the pointer is back with no button down.
	await page.evaluate(() => { const el = document.querySelector(".chip.held"); el.dispatchEvent(new PointerEvent("pointermove", { bubbles: true, pointerId: 1, pointerType: "mouse", buttons: 0, clientX: 300, clientY: 300 })); });
	assert.strictEqual(await marks(page, 1), "0:Am 10:F 21:C 27:G 31:Am 43:F 53:C 57:x2 134:E7");
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".chip.held").length), 0);
	await page.mouse.up();
});

test("a letter made of an accent and a vowel is one step back", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("can ");
	await page.waitForTimeout(1100);
	const cdp = await page.context().newCDPSession(page);
	await cdp.send("Input.imeSetComposition", { text: "´", selectionStart: 1, selectionEnd: 1 });
	await cdp.send("Input.insertText", { text: "ó" });
	await page.keyboard.type("n");
	assert.strictEqual((await block(page, 1)).text.split("\n")[0], "Bajo can ónla farola de mi calle ");
	await page.keyboard.press("Control+z");
	const once = (await block(page, 1)).text.split("\n")[0];
	assert.ok(once === "Bajo can la farola de mi calle " || once === "Bajo can óla farola de mi calle ", "one step back gave: " + once);
	assert.ok(!once.includes("´"), "half a letter is a place undo stops at");
});
