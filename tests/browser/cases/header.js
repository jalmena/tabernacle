// The header of a song: the key and the tuning chosen from a list, the artist and the genre typed or chosen.
const { test, edit, focusIs, file, save, assert, SHOTS } = require("../harness");

const shown = (page, sel) => page.evaluate((sel) => getComputedStyle(document.querySelector(sel)).display !== "none", sel);

test("the tuning is the usual one until another is chosen, and a box appears for one the list does not hold", async (page) => {
	await edit(page);
	assert.strictEqual(await page.evaluate(() => { const s = document.getElementById("f-tuning"); return s.options[s.selectedIndex].text; }), "Estándar");
	assert.ok(!(await shown(page, ".tuningother")), "the box for another tuning is showing");

	await page.selectOption("#f-tuning", "custom");
	assert.ok(await shown(page, ".tuningother"), "choosing another tuning brought up no box");
	assert.strictEqual(await focusIs(page), ".tuningother");
	await page.keyboard.type("CGCGCE");
	await save(page);
	assert.ok((await file(page)).includes("\ncapo: 2\ntuning: CGCGCE\n"), await file(page));

	await edit(page);
	assert.strictEqual(await page.inputValue("#f-tuning"), "custom");
	assert.ok(await shown(page, ".tuningother"));
	await page.selectOption("#f-tuning", "Drop D");
	assert.ok(!(await shown(page, ".tuningother")));
	await page.locator("#f-title").focus();
	await save(page);
	assert.ok((await file(page)).includes("\ntuning: Drop D\n"), await file(page));

	await edit(page);
	await page.selectOption("#f-tuning", "");
	await page.locator("#f-title").focus();
	await save(page);
	assert.ok(!(await file(page)).includes("tuning:"), await file(page));
});

test("the key is chosen from the keys there are", async (page) => {
	await edit(page);
	assert.strictEqual(await page.inputValue("#f-key"), "Am");
	const keys = await page.evaluate(() => [...document.querySelectorAll("#f-key option")].map((o) => o.value).join(" "));
	assert.strictEqual(keys, " C Db D Eb E F F# G Ab A Bb B Cm C#m Dm Ebm Em Fm F#m Gm G#m Am Bbm Bm");
	await page.selectOption("#f-key", "Em");
	await page.locator("#f-title").focus();
	await save(page);
	assert.ok((await file(page)).includes("\nkey: Em\n"), await file(page));
});

test("the artist and the genre are typed, or chosen from what is on offer", async (page) => {
	await edit(page);
	const got = await page.evaluate(() => ({
		artist: document.getElementById("f-artist").list && [...document.getElementById("f-artist").list.options].map((o) => o.value),
		genre: document.getElementById("f-genre").list && [...document.getElementById("f-genre").list.options].map((o) => o.value),
	}));
	assert.deepStrictEqual(got.artist, ["Inventadas", "Traditional"]);
	// The collection's own three first, then the usual ones in order.
	assert.ok(got.genre.length > 40 && got.genre.slice(0, 3).sort().join() === "hymn,rumba,traditional", got.genre.slice(0, 5).join());
	assert.strictEqual(got.genre[3], "bachata");
	assert.ok(got.genre.includes("flamenco") && got.genre.includes("villancico"));
	await page.fill("#f-genre", "flamenco");
	await page.fill("#f-artist", "Otra Gente");
	await page.screenshot({ path: SHOTS + "/header-desktop.png" });
	await save(page);
	const saved = await file(page, page.url().split("/songs/")[1]);
	assert.ok(saved.includes("\nartist: Otra Gente\n") && saved.includes("\ngenre: flamenco\n"), saved);
});

test("the header fits a phone", async (page) => {
	await edit(page);
	await page.selectOption("#f-tuning", "custom");
	await page.keyboard.type("CGCGCE");
	const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
	assert.ok(wide <= 0, "the page scrolls sideways by " + wide);
	await page.evaluate(() => document.querySelector(".songfields").scrollIntoView({ block: "start" }));
	await page.evaluate(() => scrollBy(0, -70));
	await page.screenshot({ path: SHOTS + "/header-phone.png" });
}, { context: { viewport: { width: 360, height: 760 }, hasTouch: true, isMobile: true, deviceScaleFactor: 2 } });
