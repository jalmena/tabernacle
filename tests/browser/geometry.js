// Is every chord drawn where its letter is, and does the measured copy of the
// words agree with the box they are typed in? At the sizes of a desktop, a phone
// and a tablet, and at the zooms where a letter no longer starts on a whole pixel.
const path = require("path");
const { start, stop, launch, open, edit, SHOTS } = require("./harness");

(async () => {
	await start();
	const browser = await launch();
	let failed = 0;
	const zooms = [1.1, 1.25, 1.5, 1.75].map((z) => ["zoom " + z, { viewport: { width: Math.round(1280 / z), height: Math.round(900 / z) }, deviceScaleFactor: z }]);
	const sizes = [["desktop", {}], ["phone", { viewport: { width: 360, height: 780 } }], ["tablet", { viewport: { width: 820, height: 1100 }, hasTouch: true, isMobile: true }]];
	for (const [label, context] of sizes.concat(zooms)) {
		const page = await open(browser, Object.assign({ deviceScaleFactor: 2 }, context));
		await edit(page);
		await page.waitForTimeout(200);

		const report = await page.evaluate(() => {
			const out = { chips: [], boxes: [] };
			document.querySelectorAll(".lyr").forEach((lyr, bi) => {
				const b = lyr.block, box = b.box, mirror = b.mirror;
				// The two must wrap alike: the same height once laid out.
				const seen = { block: bi, boxHeight: box.scrollHeight, mirrorHeight: mirror.offsetHeight };
				// And their letters must be as wide. Both are squeezed narrower than
				// any line, so that what is measured is the words and not the box.
				box.style.whiteSpace = "pre"; mirror.style.whiteSpace = "pre"; box.style.width = "30px"; mirror.style.width = "30px";
				box.style.overflow = "auto"; mirror.style.overflow = "auto";
				seen.boxWide = box.scrollWidth;
				seen.mirrorWide = mirror.scrollWidth;
				box.style.whiteSpace = mirror.style.whiteSpace = box.style.width = mirror.style.width = box.style.overflow = mirror.style.overflow = "";
				out.boxes.push(seen);

				const frame = b.layer.getBoundingClientRect(), cs = getComputedStyle(box);
				b.marks.forEach((m, k) => {
					const i = b.starts.filter((s) => s <= m.pos).length - 1;
					// On a line that fits in one row, the letter is as far along as the words before it are wide.
					let plain = null;
					if (mirror.children[i].getBoundingClientRect().height < 1.5 * parseFloat(cs.lineHeight)) {
						const span = document.createElement("span");
						span.style.whiteSpace = "pre"; span.style.position = "absolute"; span.style.visibility = "hidden";
						span.textContent = b.lines[i].slice(0, m.pos - b.starts[i]);
						lyr.appendChild(span);
						plain = parseFloat(cs.paddingLeft) + span.getBoundingClientRect().width;
						span.remove();
					}
					// A chord that only just does not fit after the one before it is moved along by a hair, with no sign on it.
					const before = k ? b.marks[k - 1].el.getBoundingClientRect() : null, own = m.el.getBoundingClientRect();
					const nudged = !!before && Math.round(before.top) === Math.round(own.top) && own.left - before.right <= 3.5;
					const style = getComputedStyle(m.el);
					out.chips.push({ block: bi, text: m.text, line: i, letter: m.x, plain, name: own.left - frame.left + parseFloat(style.borderLeftWidth) + parseFloat(style.paddingLeft),
						bare: b.lines[i].trim() === "", nudged, pushed: m.el.classList.contains("pushed"), bottom: own.bottom - frame.top, lettersTop: m.top });
				});
			});
			return out;
		});

		for (const bx of report.boxes) {
			if (Math.abs(bx.boxHeight - bx.mirrorHeight) > 1 || Math.abs(bx.boxWide - bx.mirrorWide) > 1) { failed++; console.log(label, "the copy and the box disagree", JSON.stringify(bx)); }
		}
		for (const c of report.chips) {
			const over = c.pushed || c.bare || (c.nudged && c.name - c.letter <= 7) || Math.abs(c.name - c.letter) <= 1.5;
			// A chord is let down a fifth of its own type size towards its words.
			const above = c.bottom <= c.lettersTop + 4.5;
			if (!over || !above) { failed++; console.log(label, "a chord is off its letter", JSON.stringify(c)); }
			if (c.plain !== null && Math.abs(c.letter - c.plain) > 0.75) { failed++; console.log(label, "a letter was measured where it is not", JSON.stringify(c)); }
		}
		if (page.problems.length) { failed++; console.log(label, "the page complained:", page.problems.join(" | ")); }
		console.log(label + ":", report.boxes.length, "blocks,", report.chips.length, "chords,", report.chips.filter((c) => c.plain !== null).length, "of them on lines of one row");

		// The copy shown in red over the box: where they agree the words read as one.
		await page.evaluate(() => document.querySelectorAll(".lyrmirror").forEach((m) => { m.style.visibility = "visible"; m.style.color = "red"; m.style.opacity = "0.55"; }));
		await page.locator(".songsection").nth(1).screenshot({ path: path.join(SHOTS, "geometry-" + label.replace(" ", "-") + ".png") });
		await page.context().close();
	}
	await browser.close();
	await stop();
	console.log(failed ? `FAILED (${failed})` : "every chord is over its letter, screenshots in " + SHOTS);
	process.exit(failed ? 1 : 0);
})().catch((e) => { console.error(e); process.exit(2); });
