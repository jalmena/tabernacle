// The dark theme, an interface written right to left, and the focus being visible.
const { test, edit, marks, caret, chip, dragChip, isOpen, focusIs, assert, SHOTS } = require("../harness");

// Is every chord of every block over its own letter?
const aligned = (page) => page.evaluate(() => {
	const off = [];
	document.querySelectorAll(".lyr").forEach((lyr, n) => {
		const b = lyr.block, frame = b.layer.getBoundingClientRect();
		b.marks.forEach((m) => {
			let i = 0;
			while (i + 1 < b.starts.length && b.starts[i + 1] <= m.pos) i++;
			if (b.lines[i].trim() === "" || m.el.classList.contains("pushed")) return;
			const r = m.el.getBoundingClientRect(), cs = getComputedStyle(m.el);
			const name = r.left - frame.left + parseFloat(cs.borderLeftWidth) + parseFloat(cs.paddingLeft);
			if (Math.abs(name - m.x) > 1.5 || r.bottom - frame.top > m.top + 4.5) off.push(`${n}:${m.text} name ${name.toFixed(1)} letter ${m.x.toFixed(1)}`);
		});
	});
	return off;
});

test("the dark theme keeps every part readable", async (page) => {
	await edit(page);
	const got = await page.evaluate(() => {
		const px = (c) => c.match(/\d+/g).slice(0, 3).map(Number);
		const lum = (c) => { const [r, g, b] = px(c).map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
		const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); };
		const lyr = document.querySelectorAll(".lyr")[1];
		const bg = getComputedStyle(lyr).backgroundColor;
		return {
			paper: bg,
			words: ratio(getComputedStyle(lyr.querySelector(".lyrtext")).color, bg),
			chord: ratio(getComputedStyle(lyr.querySelector(".chip:not(.note)")).color, bg),
			note: ratio(getComputedStyle(lyr.querySelector(".chip.note")).color, bg),
		};
	});
	assert.ok(got.words >= 7 && got.chord >= 4.5 && got.note >= 4.5, JSON.stringify(got));
	assert.deepStrictEqual(await aligned(page), []);
}, { context: { colorScheme: "dark" } });

test("the light theme too", async (page) => {
	await edit(page);
	const got = await page.evaluate(() => {
		const px = (c) => c.match(/\d+/g).slice(0, 3).map(Number);
		const lum = (c) => { const [r, g, b] = px(c).map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }); return 0.2126 * r + 0.7152 * g + 0.0722 * b; };
		const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05); };
		const lyr = document.querySelectorAll(".lyr")[1];
		const bg = getComputedStyle(lyr).backgroundColor;
		return { chord: ratio(getComputedStyle(lyr.querySelector(".chip:not(.note)")).color, bg), note: ratio(getComputedStyle(lyr.querySelector(".chip.note")).color, bg) };
	});
	assert.ok(got.chord >= 4.5 && got.note >= 4.5, JSON.stringify(got));
});

test("in an interface written right to left the words still read left to right", async (page) => {
	await edit(page);
	const got = await page.evaluate(() => ({
		page: document.documentElement.dir,
		words: getComputedStyle(document.querySelector(".lyr")).direction,
		tab: getComputedStyle(document.querySelector(".tabbox")).direction,
		box: document.querySelectorAll(".lyr")[1].block.box.getBoundingClientRect().left,
		first: (() => { const b = document.querySelectorAll(".lyr")[1].block; return b.layer.getBoundingClientRect().left + b.marks[0].x; })(),
		sideways: Math.abs(document.documentElement.scrollWidth - document.documentElement.clientWidth),
	}));
	assert.deepStrictEqual([got.page, got.words, got.tab], ["rtl", "ltr", "ltr"]);
	assert.ok(got.first - got.box < 20, "the first letter is not at the left of its box: " + JSON.stringify(got));
	assert.deepStrictEqual(await aligned(page), []);
	// And a chord still drags to the letter it is dropped on.
	await page.evaluate(() => document.querySelectorAll(".lyr")[1].scrollIntoView({ block: "center" }));
	await dragChip(page, 1, 1, 5);
	assert.ok((await marks(page, 1)).startsWith("0:Am 5:F 21:C"), await marks(page, 1));
	const c = await chip(page, 1, 4);
	await page.mouse.click(c.x, c.y);
	assert.ok(await isOpen(page));
	assert.strictEqual(await page.evaluate(() => getComputedStyle(document.querySelector(".pickname")).direction), "ltr");
	await page.screenshot({ path: SHOTS + "/rtl-chooser.png" });
}, { lang: "ar" });

test("whatever the keyboard is on can be seen", async (page) => {
	await edit(page);
	await caret(page, 1, 0);
	const ring = () => page.evaluate(() => { const a = document.activeElement, own = getComputedStyle(a), box = a.closest(".lyr") ? getComputedStyle(a.closest(".lyr")) : null;
		return { own: own.outlineStyle + " " + own.outlineWidth, box: box ? box.outlineStyle + " " + box.outlineWidth : "" }; });
	await page.keyboard.press("ArrowRight");
	assert.strictEqual((await ring()).box, "solid 2px", "the words box shows no ring");
	await page.keyboard.press("Tab");
	assert.strictEqual(await focusIs(page), ".chip");
	assert.strictEqual((await ring()).own, "solid 2px", "the chord shows no ring");
	await page.keyboard.press("Enter");
	assert.ok(await isOpen(page));
	assert.strictEqual(await focusIs(page), "#pickname");
	assert.strictEqual((await ring()).own, "solid 2px", "the name box shows no ring");
	// Tab walks the chooser: the name, taking the chord out, the song's chords, the notes, the two lists.
	const stops = [];
	for (let i = 0; i < 22; i++) { await page.keyboard.press("Tab"); stops.push(await page.evaluate(() => { const a = document.activeElement; return a.id || a.getAttribute("data-c") || (a.hasAttribute("data-remove") ? "remove" : a.tagName); })); }
	assert.deepStrictEqual(stops.slice(0, 7), ["remove", "Am", "F", "G", "C", "E7", "Dm"]);
	assert.ok(stops.includes("pickkind") && stops.includes("pickbass"), stops.join(" "));
});
