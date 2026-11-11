// Every song of a collection opened in the editor and saved, first untouched and
// then with every block of it touched. What is saved must be the file that was
// there, and the sheet it draws the same sheet.
//
// The collection is the example songs and the one made up for these tests, or a
// copy of the directory named in SONGS. The directory itself is never written to.
const { start, stop, launch, open, edit } = require("./harness");

(async () => {
	await start({ songs: process.env.SONGS });
	const browser = await launch();
	const page = await open(browser);
	const get = async (path) => (await page.context().request.get(path)).text();

	await page.goto("/");
	const slugs = await page.evaluate(() => [...new Set([...document.querySelectorAll('a[href^="/songs/"]')].map((a) => a.getAttribute("href").split("/")[2]).filter((s) => s && s !== "new"))]);
	const health = JSON.parse(await get("/healthz"));
	console.log(slugs.length, "songs");

	let bad = health.songs === slugs.length ? 0 : 1;
	if (bad) console.log("FAIL  the collection has", health.songs, "songs and the front page links", slugs.length);
	if (health.broken) console.log("      ", health.broken, "files could not be read as songs and are left out");

	for (const slug of slugs) {
		const before = await get(`/songs/${slug}/raw`), sheetBefore = (await get(`/songs/${slug}/sheet`)).replace(/\s+/g, " ");
		await edit(page, slug);
		const shape = await page.evaluate(() => ({
			sections: document.querySelectorAll(".songsection").length,
			laidOut: document.querySelectorAll(".sectionbody[hidden]").length,
			blocks: document.querySelectorAll(".lyr").length, tabs: document.querySelectorAll(".tabbox").length,
			chords: document.querySelectorAll(".chips .chip").length,
			sideways: document.documentElement.scrollWidth - document.documentElement.clientWidth,
			overlap: [...document.querySelectorAll(".lyr")].some((l) => {
				const r = [...l.querySelectorAll(".chip")].map((c) => c.getBoundingClientRect());
				return r.some((a, i) => r.some((b, j) => j > i && Math.round(a.top) === Math.round(b.top) && a.right > b.left + 0.5 && b.right > a.left + 0.5));
			}),
		}));
		await page.locator("#f-title").focus();
		await Promise.all([page.waitForNavigation(), page.keyboard.press("Control+Enter")]);
		const untouched = await get(`/songs/${slug}/raw`);

		// Every block touched: a letter typed and rubbed out again.
		await edit(page, slug);
		const boxes = await page.evaluate(() => document.querySelectorAll(".lyrtext, .tabtext").length);
		for (let i = 0; i < boxes; i++) {
			await page.evaluate((i) => { const b = document.querySelectorAll(".lyrtext, .tabtext")[i]; b.focus(); b.setSelectionRange(b.value.length, b.value.length); }, i);
			await page.keyboard.type("x");
			await page.keyboard.press("Backspace");
		}
		const asRows = await page.evaluate(() => [...document.querySelectorAll(".sectionrows")].filter((s) => !s.disabled).length);
		await Promise.all([page.waitForNavigation(), page.keyboard.press("Control+Enter")]);
		const touched = await get(`/songs/${slug}/raw`), sheetAfter = (await get(`/songs/${slug}/sheet`)).replace(/\s+/g, " ");

		const wrong = [
			untouched === before ? "" : "saved untouched, it is another file",
			touched === before ? "" : "saved after every block was touched, it is another file",
			sheetAfter === sheetBefore ? "" : "its sheet changed",
			shape.laidOut === shape.sections ? "" : "a section was left as a box of text",
			asRows === shape.sections || !boxes ? "" : "a section touched was not sent as rows",
			shape.sideways <= 0 ? "" : "the editor scrolls sideways",
			shape.overlap ? "two chords overlap" : "",
		].concat(page.problems).filter(Boolean);
		if (wrong.length) bad++;
		console.log(wrong.length ? "FAIL " : "ok   ", slug, `${shape.sections} sections, ${shape.blocks} blocks of words, ${shape.tabs} of tablature, ${shape.chords} chords`, wrong.join("; "));
		if (touched !== before) {
			const a = before.split("\n"), b = touched.split("\n");
			for (let i = 0; i < Math.max(a.length, b.length); i++) {
				if (a[i] !== b[i]) { console.log("      line", i + 1, "was", JSON.stringify(a[i]), "and is", JSON.stringify(b[i])); break; }
			}
		}
		page.problems.length = 0;
	}
	await browser.close();
	await stop();
	console.log(bad ? `FAILED (${bad})` : "every song comes back as it was");
	process.exit(bad ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(2); });
