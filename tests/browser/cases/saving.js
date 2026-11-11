// What is saved, and the sections themselves: dividing, joining and moving them, the file behind them, a new song.
const { test, edit, block, marks, sent, caret, chip, dragChip, focusIs, file, save, assert, SLUG, ORIGINAL } = require("../harness");

const into = async (page, n) => { await page.evaluate((n) => document.querySelectorAll(".lyr")[n].scrollIntoView({ block: "center" }), n); await page.waitForTimeout(30); };
const bodies = (page) => page.evaluate(() => [...document.querySelectorAll(".songsection")].map((s) => s.querySelector(".sectionlabel").value + "|" + s.querySelector(".sectionbody").value));
const waitMark = (page, n, mark) => page.waitForFunction(([n, mark]) => document.querySelectorAll(".lyr")[n].block.marks.some((m) => m.pos + ":" + m.text === mark), [n, mark], { timeout: 3000 });

test("what is saved is the file somebody would have typed", async (page) => {
	await edit(page);
	await into(page, 1);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	await dragChip(page, 1, 1, 5);                                // F, to over "muy"
	await caret(page, 1, 152);
	await page.keyboard.type("[dm");
	await page.keyboard.press("Enter");
	await waitMark(page, 1, "152:Dm");
	const x2 = await chip(page, 1, 7);
	await page.mouse.click(x2.x, x2.y);
	await page.keyboard.type("x3");
	await page.keyboard.press("Enter");
	await save(page);

	const want = ORIGINAL
		.replace("[Am]Bajo la fa[F]rola de mi [C]calle [G]", "[Am]Bajo [F]muy la farola de mi [C]calle [G]")
		.replace("aca[C]bar [x2]", "aca[C]bar [x3]")
		.replace("sin acordes esta línea", "sin [Dm]acordes esta línea");
	assert.notStrictEqual(want, ORIGINAL);
	assert.strictEqual(await file(page), want);
	assert.ok(page.url().endsWith("/songs/" + SLUG), "saving did not go back to the song");
});

test("a name still being read when the song is saved is read as it is written", async (page) => {
	await page.route("**/chords/pick?c=lam", async (route) => { await new Promise((r) => setTimeout(r, 1500)); await route.continue().catch(() => {}); });
	await edit(page);
	await caret(page, 1, 148);
	await page.keyboard.type("[lam");
	await page.keyboard.press("Enter");
	assert.strictEqual((await sent(page, 1))[4].marks[0].typed, true, "the name does not go marked as unread");
	const began = Date.now();
	await save(page);
	assert.ok(Date.now() - began < 1200, "saving waited for the answer");
	assert.ok((await file(page)).includes("sin [Am]acordes esta línea"), await file(page));
});

test("and so is one the server never answered about", async (page) => {
	await edit(page);
	await page.route("**/chords/pick?c=mim", (route) => route.abort());
	await caret(page, 1, 148);
	await page.keyboard.type("[mim");
	await page.keyboard.press("Enter");
	await page.waitForTimeout(100);
	await save(page);
	assert.ok((await file(page)).includes("sin [Em]acordes esta línea"), await file(page));
}, { noisy: true });

test("tablature is edited in its own box and saved between its fences", async (page) => {
	await edit(page);
	const box = page.locator(".tabtext");
	await box.focus();
	await page.keyboard.press("Control+End");
	await page.keyboard.type("\nD|-----------|");
	assert.strictEqual(await page.evaluate(() => document.querySelector(".tabtext").rows), 4);
	await save(page);
	const want = ORIGINAL.replace("G|-2-------2-|\n```", "G|-2-------2-|\nD|-----------|\n```");
	assert.strictEqual(await file(page), want);

	// Emptied, the block is gone and the words either side are left.
	await edit(page);
	await page.locator(".tabtext").fill("");
	await save(page);
	assert.ok((await file(page)).endsWith("## Puente\n[Am]Antes del solo\ny [G]después de él\n"), await file(page));
});

test("a section divides at the caret, from the words", async (page) => {
	await edit(page);
	await caret(page, 1, 58);                                     // the start of the third line
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll("[data-split]")[1].disabled), false, "the divide button is not offered");
	await Promise.all([page.waitForNavigation(), page.keyboard.press("Alt+Enter")]);
	const got = await bodies(page);
	assert.strictEqual(got.length, 5);
	assert.strictEqual(got[1], "Estrofa|[Am]Bajo la fa[F]rola de mi [C]calle [G]\ncan[Am]ción que no [F]quiere aca[C]bar [x2]");
	assert.ok(got[2].startsWith("Estrofa|y la noche"), got[2]);
	// The caret is in the second half, in its words.
	assert.strictEqual(await focusIs(page), ".lyrtext");
	assert.ok(await page.evaluate(() => document.activeElement.closest(".songsection") === document.querySelectorAll(".songsection")[2]));
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".lyr").length), 6);
});

test("it divides in the right place after a fenced block and after a change", async (page) => {
	await edit(page);
	await caret(page, 3, 5);
	await page.keyboard.type("XX");                               // the section is now sent as rows
	await caret(page, 4, 0);                                      // the line after the tablature
	await Promise.all([page.waitForNavigation(), page.keyboard.press("Alt+Enter")]);
	const got = await bodies(page);
	assert.strictEqual(got[3], "Puente|[Am]AntesXX del solo\n```tab\ne|-----0-----|\nB|---1---1---|\nG|-2-------2-|\n```");
	assert.strictEqual(got[4], "Puente|y [G]después de él");
});

test("moving, joining, adding and removing sections keep what was changed", async (page) => {
	await edit(page);
	await caret(page, 2, 0);
	await page.keyboard.type("Oh ");
	await Promise.all([page.waitForNavigation(), page.keyboard.press("Alt+ArrowUp")]);
	let got = await bodies(page);
	assert.strictEqual(got[1], "Estribillo|Oh [F]Ay, [G]ay, [Am]ay\n[Dm]lu[E7]na [Am]llena [N.C.]");
	assert.ok(got[2].startsWith("Estrofa|"));
	assert.strictEqual(await focusIs(page), ".lyrtext");

	await page.keyboard.press("Alt+x");
	await page.evaluate(() => document.querySelectorAll(".lyr")[2].block.box.focus());
	await page.keyboard.press("Alt+x");
	await Promise.all([page.waitForNavigation(), page.keyboard.press("Alt+m")]);
	got = await bodies(page);
	assert.strictEqual(got.length, 3);
	assert.ok(got[1].startsWith("Estribillo|Oh [F]Ay") && got[1].includes("\n\n[Am]Bajo la fa[F]rola"), got[1]);

	await Promise.all([page.waitForNavigation(), page.keyboard.press("Alt+n")]);
	got = await bodies(page);
	assert.strictEqual(got.length, 4);
	assert.strictEqual(await focusIs(page), ".lyrtext");
	assert.strictEqual(await page.evaluate(() => document.activeElement.value + "|" + document.activeElement.placeholder), "|Escribe aquí la letra");
	await page.keyboard.type("nueva [c");
	await page.keyboard.press("Enter");
	await page.keyboard.type("letra");
	await page.waitForFunction(() => document.activeElement.parentNode.block.marks.some((m) => m.text === "C"));

	await Promise.all([page.waitForNavigation(), page.locator('button[name="del0"]').click()]);
	got = await bodies(page);
	assert.strictEqual(got.length, 3);
	assert.ok(got.some((b) => b.endsWith("|nueva [C]letra")), JSON.stringify(got));
});

test("the file is still there behind the words, and back again", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	await Promise.all([page.waitForNavigation(), page.locator('button[name="plainer"]').click()]);
	const text = await page.inputValue("#songtext");
	assert.ok(text.includes("[Am]Bajo muy la fa[F]rola de mi [C]calle [G]"), text);
	assert.strictEqual(await page.evaluate(() => document.querySelectorAll(".lyr").length), 0);
	await Promise.all([page.waitForNavigation(), page.locator('button[name="fielder"]').click()]);
	assert.strictEqual((await block(page, 1)).text.split("\n")[0], "Bajo muy la farola de mi calle ");
	assert.strictEqual(await marks(page, 0), "0:Am 1:F 2:C 3:G");
});

test("a section that cannot be taken apart stays a box of text", async (page) => {
	await page.goto("/songs/new");
	await Promise.all([page.waitForNavigation(), page.evaluate(() => {
		const f = document.createElement("form");
		f.method = "post"; f.action = "/songs";
		for (const [k, v] of [["title", "Rota"], ["label0", "Bien"], ["body0", "la [C]la"], ["label1", "Mal"], ["body1", "la [C la"], ["stay", "1"]]) {
			const i = document.createElement("textarea"); i.name = k; i.value = v; f.appendChild(i);
		}
		document.body.appendChild(f); f.submit();
	})]);
	const got = await page.evaluate(() => [...document.querySelectorAll(".songsection")].map((s) => ({
		words: s.querySelectorAll(".lyr").length, box: s.querySelector(".sectionbody").offsetParent !== null })));
	assert.deepStrictEqual(got, [{ words: 1, box: false }, { words: 0, box: true }]);
});

test("with no script the editor is boxes of text", async (page) => {
	await page.goto("/songs/" + SLUG + "/edit");
	const got = await page.evaluate(() => ({
		boxes: [...document.querySelectorAll(".sectionbody")].filter((b) => b.offsetParent !== null).length,
		words: document.querySelectorAll(".lyr").length,
		help: getComputedStyle(document.querySelector(".floating")).display + "/" + getComputedStyle(document.querySelector(".brackets")).display,
		chooser: document.getElementById("chordpick").offsetParent !== null || document.getElementById("chordpick").getClientRects().length > 0,
	}));
	assert.deepStrictEqual(got, { boxes: 4, words: 0, help: "none/inline", chooser: false });
	await page.locator('[name="body0"]').fill("[Am] [F] [C] [E7]");
	await Promise.all([page.waitForNavigation(), page.locator("button[data-save]").click()]);
	assert.ok((await (await page.context().request.get("/songs/" + SLUG + "/raw")).text()).includes("## Intro\n[Am] [F] [C] [E7]\n"));
}, { context: { javaScriptEnabled: false } });

test("the list of keys names the ones for chords", async (page) => {
	await edit(page);
	await page.locator("[data-keys]").click();
	const text = await page.evaluate(() => document.getElementById("keys").innerText.replace(/\s+/g, " "));
	for (const want of ["[ poner un acorde en el cursor", "Alt ← → mover el acorde enfocado", "Supr quitar el acorde enfocado", "Ctrl Z deshacer"]) {
		assert.ok(text.includes(want), `no "${want}" in: ${text}`);
	}
});

test("a new song is written from nothing", async (page) => {
	await page.goto("/songs/new");
	await page.fill("#f-title", "Recién hecha");
	await page.selectOption("#addkind", "verse");
	await Promise.all([page.waitForNavigation(), page.locator('button[name="addnow"]').click()]);
	assert.strictEqual(await focusIs(page), ".lyrtext");
	await page.keyboard.type("[g");
	await page.keyboard.press("Enter");
	await page.keyboard.type("Una letra [d");
	await page.keyboard.press("Enter");
	await page.keyboard.type("nueva");
	await page.keyboard.press("Enter");
	await page.keyboard.type("y otra línea [x2");
	await page.keyboard.press("Enter");
	await page.waitForFunction(() => document.querySelector(".lyr").block.marks.map((m) => m.text).join(" ") === "G D x2");
	await save(page);
	const raw = await file(page, "recien-hecha");
	assert.strictEqual(raw, "---\ntitle: Recién hecha\n---\n\n## Estrofa\n[G]Una letra [D]nueva\ny otra línea [x2]\n");
});

test("a chord sheet loaded from a file arrives with its chords above the words", async (page) => {
	await page.goto("/songs/new");
	const sheet = "Intro: LAm  FA\n\n   LAm          FA\nUna hoja con los acordes encima\n        DO        SOL\nque se carga desde un archivo\n";
	await Promise.all([page.waitForNavigation(), page.setInputFiles("#loadfile", { name: "Hoja de prueba.txt", mimeType: "text/plain", buffer: Buffer.from(sheet) })]);
	const got = await page.evaluate(() => [...document.querySelectorAll(".lyr")].map((l) => l.block.text + " :: " + l.block.marks.map((m) => m.pos + ":" + m.text).join(" ")));
	assert.strictEqual(got.length, 2, JSON.stringify(got));
	assert.ok(got[1].startsWith("Una hoja con los acordes encima\nque se carga desde un archivo :: 4:Am 17:F"), got[1]);
	assert.strictEqual(await page.inputValue("#f-title"), "Hoja de prueba");
});
