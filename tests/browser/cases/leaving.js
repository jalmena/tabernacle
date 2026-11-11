// Being asked before a song that has been changed is left, and not otherwise.
const { test, edit, caret, dragChip, file, save, assert, SLUG } = require("../harness");

// Follow the first link in the header and say whether the browser asked first. The answer given is "stay".
async function leave(page, answer) {
	let asked = null;
	const onDialog = async (d) => { asked = d.type(); if (answer === "go") await d.accept(); else await d.dismiss(); };
	page.on("dialog", onDialog);
	await page.locator("header a.brand").click({ noWaitAfter: true });
	await page.waitForTimeout(400);
	page.off("dialog", onDialog);
	return { asked, left: !(await page.evaluate(() => !!document.querySelector("form.editor"))) };
}

test("a song that was only opened is left without a word", async (page) => {
	await edit(page);
	await page.locator(".sectionpick").first().check();           // ticking a section changes nothing in the song
	await page.selectOption("#addkind", "chorus");
	assert.deepStrictEqual(await leave(page), { asked: null, left: true });
}, { dialogs: true });

test("words typed are asked about, and staying keeps them", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	assert.deepStrictEqual(await leave(page), { asked: "beforeunload", left: false });
	assert.ok((await page.evaluate(() => document.querySelectorAll(".lyr")[1].block.text)).startsWith("Bajo muy la"));
	assert.deepStrictEqual(await leave(page, "go"), { asked: "beforeunload", left: true });
}, { dialogs: true });

test("so is a chord that was only moved", async (page) => {
	await edit(page);
	await page.evaluate(() => document.querySelectorAll(".lyr")[1].scrollIntoView({ block: "center" }));
	await dragChip(page, 1, 1, 5);
	await page.evaluate(() => scrollTo(0, 0));
	assert.deepStrictEqual(await leave(page), { asked: "beforeunload", left: false });
}, { dialogs: true });

test("and a field of the header", async (page) => {
	await edit(page);
	await page.fill("#f-genre", "copla");
	assert.deepStrictEqual(await leave(page), { asked: "beforeunload", left: false });
}, { dialogs: true });

test("saving is not leaving", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	let asked = false;
	page.on("dialog", (d) => { asked = true; d.dismiss(); });
	await save(page);
	assert.ok(!asked, "saving asked whether to leave");
	assert.ok((await file(page)).includes("[Am]Bajo muy la fa[F]rola"));
}, { dialogs: true });

test("nor is cancelling", async (page) => {
	await edit(page);
	await caret(page, 1, 5);
	await page.keyboard.type("muy ");
	let asked = false;
	page.on("dialog", (d) => { asked = true; d.dismiss(); });
	await Promise.all([page.waitForNavigation(), page.locator("[data-discard]").click()]);
	assert.ok(!asked, "cancelling asked whether to leave");
	assert.ok(page.url().endsWith("/songs/" + SLUG));
}, { dialogs: true });

test("a page that came back with a section added is asked about though nothing was typed on it", async (page) => {
	await edit(page);
	let asked = false;
	const quiet = (d) => { asked = true; d.dismiss(); };
	page.on("dialog", quiet);
	await Promise.all([page.waitForNavigation(), page.locator('button[name="addnow"]').click()]);
	assert.ok(!asked, "the editor's own button asked whether to leave");
	page.off("dialog", quiet);
	await page.keyboard.press("Tab");                             // a key, so that the browser will ask at all
	assert.deepStrictEqual(await leave(page), { asked: "beforeunload", left: false });
}, { dialogs: true });

test("loading a file over what was typed asks once, not twice", async (page) => {
	await page.goto("/songs/new");
	await page.fill("#f-title", "Algo");
	const asked = [];
	page.on("dialog", (d) => { asked.push(d.type()); d.accept(); });
	await Promise.all([page.waitForNavigation(), page.setInputFiles("#loadfile", { name: "Hoja.txt", mimeType: "text/plain", buffer: Buffer.from("LAm\nuna hoja\n") })]);
	assert.deepStrictEqual(asked, ["confirm"]);
	assert.strictEqual(await page.inputValue("#f-title"), "Hoja");
}, { dialogs: true });
