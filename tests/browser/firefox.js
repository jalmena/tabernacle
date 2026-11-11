// The editor in Firefox: is every chord over its letter, and do the main
// gestures do what they do in Chromium? Exits with 3 when there is no Firefox.
const path = require("path");
const fs = require("fs");
const { start, stop, SHOTS, SLUG } = require("./harness");
const { launch, there } = require("./bidi");

const START = "0:Am 10:F 21:C 27:G 31:Am 43:F 53:C 57:x2 134:E7";

(async () => {
	if (!there()) {
		console.log("SKIPPED: no Firefox on this machine (name one in FIREFOX)");
		process.exit(3);
	}
	const url = (await start()) + "/songs/" + SLUG + "/edit";
	fs.mkdirSync(SHOTS, { recursive: true });
	const page = await launch();
	const marks = (n) => page.evaluate((n) => document.querySelectorAll(".lyr")[n].block.marks.map((m) => m.pos + ":" + m.text).join(" "), n);
	const isOpen = () => page.evaluate(() => document.getElementById("chordpick").matches(":popover-open"));
	const caret = (n, pos) => page.evaluate(([n, pos]) => { const b = document.querySelectorAll(".lyr")[n].block.box; b.focus(); b.setSelectionRange(pos, pos); b.dispatchEvent(new Event("select", { bubbles: true })); }, [n, pos]);
	const where = (n, pos) => page.evaluate(([n, pos]) => {
		const b = document.querySelectorAll(".lyr")[n].block; let i = 0;
		while (i + 1 < b.starts.length && b.starts[i + 1] <= pos) i++;
		const row = b.mirror.children[i], col = pos - b.starts[i]; let r;
		if (col < b.lines[i].length) { const range = document.createRange(); range.setStart(row.firstChild, col); range.setEnd(row.firstChild, col + 1); r = range.getClientRects()[0]; }
		else r = row.lastChild.getBoundingClientRect();
		return { x: r.left, top: r.top, bottom: r.bottom };
	}, [n, pos]);
	const chip = (n, k) => page.evaluate(([n, k]) => { const b = document.querySelectorAll(".lyr")[n].block, m = b.marks[k], r = m.el.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2, left: r.left, top: r.top, bottom: r.bottom, anchor: b.layer.getBoundingClientRect().left + m.x }; }, [n, k]);
	const into = (n) => page.evaluate((n) => document.querySelectorAll(".lyr")[n].scrollIntoView({ block: "center" }), n);
	let checks = 0;
	const check = (name, ok, detail) => { checks++; console.log((ok ? "ok    " : "FAIL  ") + name + (ok ? "" : "\n      " + detail)); if (!ok) process.exitCode = 1; };

	for (const [label, w, h] of [["desktop", 1280, 900], ["narrow", 380, 800]]) {
		await page.setViewport(w, h);
		await page.goto(url);
		await page.sleep(400);
		const report = await page.evaluate(() => {
			const out = { boxes: [], off: [] };
			document.querySelectorAll(".lyr").forEach((lyr, bi) => {
				const b = lyr.block, box = b.box, mirror = b.mirror;
				const rec = { block: bi, boxHeight: box.scrollHeight, mirrorHeight: mirror.offsetHeight, shown: box.clientHeight };
				box.style.whiteSpace = "pre"; mirror.style.whiteSpace = "pre"; box.style.width = "30px"; mirror.style.width = "30px"; box.style.overflow = "auto"; mirror.style.overflow = "auto";
				rec.boxWide = box.scrollWidth; rec.mirrorWide = mirror.scrollWidth;
				box.style.whiteSpace = mirror.style.whiteSpace = box.style.width = mirror.style.width = box.style.overflow = mirror.style.overflow = "";
				out.boxes.push(rec);
				const frame = b.layer.getBoundingClientRect();
				b.marks.forEach((m) => {
					let i = 0; while (i + 1 < b.starts.length && b.starts[i + 1] <= m.pos) i++;
					if (b.lines[i].trim() === "" || m.el.classList.contains("pushed")) return;
					const r = m.el.getBoundingClientRect(), cs = getComputedStyle(m.el);
					const name = r.left - frame.left + parseFloat(cs.borderLeftWidth) + parseFloat(cs.paddingLeft);
					if (Math.abs(name - m.x) > 1.5 || r.bottom - frame.top > m.top + 4.5) out.off.push(`${bi}:${m.text} name ${name.toFixed(1)} letter ${m.x.toFixed(1)} bottom ${(r.bottom - frame.top).toFixed(1)} top ${m.top.toFixed(1)}`);
				});
			});
			out.sideways = document.documentElement.scrollWidth - document.documentElement.clientWidth;
			return out;
		});
		check(`${label}: the words box and its measured copy wrap alike`, report.boxes.every((b) => Math.abs(b.boxHeight - b.mirrorHeight) <= 1 && b.shown >= b.boxHeight - 1), JSON.stringify(report.boxes));
		check(`${label}: their letters are the same width`, report.boxes.every((b) => Math.abs(b.boxWide - b.mirrorWide) <= 1), JSON.stringify(report.boxes));
		check(`${label}: every chord is over its letter`, report.off.length === 0, report.off.join(" | "));
		check(`${label}: the page does not scroll sideways`, report.sideways <= 0, String(report.sideways));
		await page.evaluate(() => { document.querySelectorAll(".lyrmirror").forEach((m) => { m.style.visibility = "visible"; m.style.color = "red"; m.style.opacity = "0.55"; }); document.querySelectorAll(".lyr")[1].scrollIntoView({ block: "center" }); });
		await page.sleep(100);
		await page.screenshot(path.join(SHOTS, `firefox-${label}.png`));
	}

	await page.setViewport(1280, 900);
	await page.goto(url);
	await page.sleep(400);
	await into(1);

	await caret(1, 5);
	await page.type("muy ");
	check("typing before a chord pushes it along", (await marks(1)) === "0:Am 14:F 25:C 31:G 35:Am 47:F 57:C 61:x2 138:E7", await marks(1));
	await page.press("z", ["Control"]);
	check("undo puts words and chords back", (await marks(1)) === START, await marks(1));

	await caret(1, 148);
	await page.type("[");
	check("a bracket opens the chooser", await isOpen(), "closed");
	await page.type("lam");
	await page.press("Enter");
	await page.sleep(300);
	check("a typed name is read and the caret goes back to the words", (await marks(1)) === START + " 148:Am" && await page.evaluate(() => document.activeElement.className) === "lyrtext", await marks(1));
	await page.type("los ");
	check("what is typed next goes under the new chord", (await marks(1)).endsWith("148:Am") && (await page.evaluate(() => document.querySelectorAll(".lyr")[1].block.text.split("\n")[4])) === "sin los acordes esta línea", await marks(1));

	let c = await chip(1, 1);
	const to = await where(1, 5);
	await page.drag(c.x, c.y, c.x + (to.x - c.anchor), c.y);
	await page.sleep(150);
	check("a chord drags to another letter", (await marks(1)).startsWith("0:Am 5:F 21:C"), await marks(1));
	check("and letting go does not open the chooser", !(await isOpen()), "open");

	c = await chip(1, 4);
	await page.click(c.x, c.y);
	await page.sleep(400);
	const seen = await page.evaluate(() => { const r = document.getElementById("chordpick").getBoundingClientRect(); return { open: document.getElementById("chordpick").matches(":popover-open"), name: document.getElementById("pickname").value, kind: (document.getElementById("pickkind") || {}).value, top: r.top, bottom: r.bottom, left: r.left, right: r.right, w: innerWidth, h: innerHeight }; });
	check("pressing a chord opens the chooser on it, on the screen", seen.open && seen.name === "Am" && seen.kind === "Am" && seen.top >= 0 && seen.bottom <= seen.h && seen.left >= 0 && seen.right <= seen.w && (seen.bottom <= c.top || seen.top >= c.bottom), JSON.stringify(seen));
	await page.screenshot(path.join(SHOTS, "firefox-chooser.png"));
	const d = await page.evaluate(() => { const r = document.querySelector('#pickbody [data-c="Dm"]').getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; });
	await page.click(d.x, d.y);
	await page.sleep(300);
	check("a note changes the chord and leaves the chooser open", (await marks(1)).includes("31:Dm") && await isOpen(), await marks(1));
	await page.press("Escape");
	await page.sleep(150);
	check("Escape puts it back", (await marks(1)).includes("31:Am") && !(await isOpen()), await marks(1));

	const lane = await where(1, 58);
	await page.move(lane.x + 4, lane.top - 8);
	await page.sleep(100);
	check("the room above a line offers a chord", await page.evaluate(() => !!document.querySelector(".chipghost.on")), "no offer");
	await page.click(lane.x + 4, lane.top - 8);
	await page.sleep(300);
	const g = await page.evaluate(() => { const b = document.querySelector('#chordpick .pickchords [data-c="G"]'); if (!b) return null; const r = b.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; });
	check("pressing there opens the chooser with the song's chords", !!g, "no list");
	if (g) await page.click(g.x, g.y);
	await page.sleep(200);
	check("and one of them is put there", (await marks(1)).includes("58:G"), await marks(1));

	await page.evaluate(() => document.querySelectorAll(".lyr")[1].block.marks[0].el.focus());
	await page.press("ArrowRight", ["Alt"]);
	check("Alt and an arrow move a chord, not the browser's history", (await marks(1)).startsWith("1:Am") && (await page.evaluate(() => location.pathname)).endsWith("/edit"), await marks(1));
	await page.press("Delete");
	check("Delete takes it out", (await marks(1)).startsWith("5:F "), await marks(1));

	await page.evaluate(() => document.querySelectorAll(".lyr")[1].block.box.focus());
	await page.press("Enter", ["Control"]);
	await page.sleep(800);
	const saved = await page.evaluate(async (slug) => ({ path: location.pathname, raw: await (await fetch("/songs/" + slug + "/raw")).text() }), SLUG);
	const want = "## Estrofa\nBajo [F]la farola de mi [C]calle [G]\ncan[Am]ción que no [F]quiere aca[C]bar [x2]\n[G]y la noche se alarga mientras suena una guitarra vieja que nadie sabe ya de [E7]quién es\n\nsin [Am]los acordes esta línea\n";
	check("saving writes the file somebody would have typed", saved.path === "/songs/" + SLUG && saved.raw.includes(want), saved.raw);

	const errors = page.events.filter((e) => e.method === "log.entryAdded" && (e.params.level === "error" || /Content Security Policy/i.test(e.params.text || "")));
	check("nothing was logged as an error", errors.length === 0, errors.map((e) => e.params.text).join(" | "));
	await page.close();
	await stop();
	console.log(process.exitCode ? "FAILED" : `all ${checks} checks ok in Firefox, screenshots in ${SHOTS}`);
})().catch((e) => { console.error(e); process.exit(2); });
