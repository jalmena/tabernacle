/* Controls that only work with a script are hidden until this line runs.
 *
 * There are two of them — starting play mode and sounding a chord — and both
 * are marked .needsjs. Revealing them is one statement rather than each block
 * doing it, so a page with a script always shows the same set. A control whose
 * particular capability is missing takes itself away again: no sound card, no
 * sound button. */
document.body.classList.add("scripted");

// Tabernacle's only script.
//
// Its whole job is to make transposing feel instant without moving the reader's
// place on the page. Every control it touches is a real link that already works
// without it, so with scripting disabled nothing here is missed except the
// scroll position.
//
// There is deliberately no music theory in this file. Chords are parsed,
// transposed and spelled in Go, where it is tested; asking the server for the
// re-rendered sheet keeps one implementation of the rules instead of two that
// drift apart.
(function () {
	"use strict";

	var sheet = document.getElementById("sheet");
	if (!sheet) return;

	var controls = document.getElementById("controls");
	if (!controls) return;

	// Sheets already fetched, keyed by their query string. Walking up and down a
	// couple of semitones is then instant and works offline.
	var cache = new Map();

	function sheetURL(href) {
		var u = new URL(href, window.location.href);
		return u.pathname.replace(/\/$/, "") + "/sheet" + u.search;
	}

	function swap(html) {
		var doc = new DOMParser().parseFromString(html, "text/html");
		var fresh = doc.getElementById("sheet");
		var freshControls = doc.getElementById("controls");
		if (!fresh || !freshControls) return false;
		sheet.replaceWith(fresh);
		controls.replaceWith(freshControls);
		sheet = fresh;
		controls = freshControls;
		return true;
	}

	function go(href) {
		var target = sheetURL(href);
		var cached = cache.get(target);
		if (cached !== undefined) {
			if (swap(cached)) {
				history.replaceState(null, "", href);
			}
			return;
		}
		controls.setAttribute("aria-busy", "true");
		fetch(target, { headers: { "Accept": "text/html" } })
			.then(function (r) {
				if (!r.ok) throw new Error("status " + r.status);
				return r.text();
			})
			.then(function (html) {
				cache.set(target, html);
				if (swap(html)) {
					history.replaceState(null, "", href);
				} else {
					window.location.href = href;
				}
			})
			.catch(function () {
				// Whatever went wrong, the plain link still works.
				window.location.href = href;
			});
	}

	// One listener on the document, so it keeps working after the controls are
	// replaced.
	document.addEventListener("click", function (e) {
		if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
			return;
		}
		var link = e.target.closest ? e.target.closest("a[data-sheet]") : null;
		if (!link) return;
		e.preventDefault();
		go(link.getAttribute("href"));
	});

	// Keyboard: the same two controls, for anyone reading on a laptop.
	document.addEventListener("keydown", function (e) {
		if (e.metaKey || e.ctrlKey || e.altKey) return;
		var tag = (e.target && e.target.tagName) || "";
		if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;

		var want = null;
		if (e.key === "+" || e.key === "=") want = "up";
		else if (e.key === "-" || e.key === "_") want = "down";
		else if (e.key === "0") want = "reset";
		if (!want) return;

		var link = controls.querySelector('a[data-sheet="' + want + '"]');
		if (link) {
			e.preventDefault();
			go(link.getAttribute("href"));
		}
	});
})();

// The chord panel.
//
// Everything it shows was rendered by the server and already exists on the page,
// in the positions appendix at the foot of the sheet. This code clones a card,
// positions it and pages through the shapes. It does not draw a diagram, work out
// a fingering or transpose anything: all of that is Go, where it is tested, and
// duplicating any of it here is how the two would drift apart.
//
// Nothing here is required. Without it, a chord is a link to its card in the
// appendix, which is a perfectly good way to look up a shape.
(function () {
	"use strict";

	var pop = document.getElementById("chordpop");
	if (!pop || typeof pop.showPopover !== "function") return;

	var opener = null;

	// step moves through the shapes inside one card.
	function step(card, delta) {
		var shapes = card.querySelectorAll(".shape");
		if (shapes.length < 2) return;
		var current = 0;
		for (var i = 0; i < shapes.length; i++) {
			if (shapes[i].classList.contains("on")) current = i;
		}
		var next = (current + delta + shapes.length) % shapes.length;
		shapes[current].classList.remove("on");
		shapes[next].classList.add("on");
		var counter = card.querySelector(".cur");
		if (counter) counter.textContent = String(next + 1);
	}

	// place puts the panel next to what was clicked, clamped to the viewport.
	// Nothing scrolls: the reader's position on the page is the one thing this
	// feature must not cost them.
	function place(anchor) {
		if (window.matchMedia("(max-width: 34rem)").matches) return; // bottom sheet
		var r = anchor.getBoundingClientRect();
		var w = pop.offsetWidth || 240;
		var h = pop.offsetHeight || 200;
		var gap = 8;

		var left = r.left + r.width / 2 - w / 2;
		left = Math.max(gap, Math.min(left, window.innerWidth - w - gap));

		var top = r.bottom + gap;
		if (top + h > window.innerHeight - gap) {
			top = r.top - h - gap;           // above instead
			if (top < gap) top = Math.max(gap, window.innerHeight - h - gap);
		}
		pop.style.left = Math.round(left) + "px";
		pop.style.top = Math.round(top) + "px";
	}

	function open(anchor, id) {
		var source = document.getElementById("acorde-" + id);
		if (!source) return false;

		pop.replaceChildren(source.cloneNode(true));
		// The clone must not duplicate an id that is already in the document.
		var clone = pop.firstElementChild;
		if (clone) clone.removeAttribute("id");
		// Always start from the shape the card is showing, which is the easiest.
		var shapes = pop.querySelectorAll(".shape");
		for (var i = 0; i < shapes.length; i++) {
			shapes[i].classList.toggle("on", i === 0);
		}
		var counter = pop.querySelector(".cur");
		if (counter) counter.textContent = "1";

		opener = anchor;
		if (pop.matches(":popover-open")) pop.hidePopover();
		pop.showPopover();
		place(anchor);
		return true;
	}

	document.addEventListener("click", function (e) {
		if (!e.target.closest) return;

		// Paging, inside the popover or inside a card in the appendix.
		var nav = e.target.closest("[data-step]");
		if (nav) {
			var card = nav.closest(".chordcard");
			if (card) {
				e.preventDefault();
				step(card, parseInt(nav.getAttribute("data-step"), 10) || 1);
			}
			return;
		}

		var link = e.target.closest("a[data-chord]");
		if (!link) return;
		if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;

		// preventDefault is what keeps the page still: the link would otherwise
		// jump to the appendix and lose the reader's place.
		if (open(link, link.getAttribute("data-chord"))) {
			e.preventDefault();
		}
	});

	pop.addEventListener("toggle", function (e) {
		if (e.newState === "closed" && opener) {
			opener.focus({ preventScroll: true });
			opener = null;
		}
	});

	document.addEventListener("keydown", function (e) {
		if (!pop.matches(":popover-open")) return;
		var card = pop.querySelector(".chordcard");
		if (!card) return;
		if (e.key === "ArrowRight") {
			e.preventDefault();
			step(card, 1);
		} else if (e.key === "ArrowLeft") {
			e.preventDefault();
			step(card, -1);
		}
		// Escape and clicking outside are handled by the popover itself.
	});

	// Keep the panel attached to its chord when the window changes shape.
	window.addEventListener("resize", function () {
		if (pop.matches(":popover-open") && opener) place(opener);
	});
})();

// Loading a file into the editor.
//
// Choosing the file is enough: the form goes to the server, which reads it — a
// chord sheet included — and sends the editor back with the song in its fields.
// With scripting off the noscript button does the same. The page that comes back
// replaces whatever was typed, so that is asked about first.
//
// Nothing is saved either way. The reader still has to press Guardar, which is
// the point — a file whose contents you never saw before it landed is how a
// collection fills up with things nobody meant to add.
(function () {
	"use strict";

	var input = document.getElementById("loadfile");
	if (!input) return;
	var bar = input.closest("form");
	if (!bar) return;
	var editor = document.querySelector("form.editor");

	function typed() {
		if (!editor) return false;
		var boxes = editor.querySelectorAll("input:not([type=hidden]), textarea");
		for (var i = 0; i < boxes.length; i++) {
			if (boxes[i].value.trim() !== "") return true;
		}
		return false;
	}

	input.addEventListener("change", function () {
		if (!input.files || !input.files[0]) return;
		if (typed() && !window.confirm(bar.getAttribute("data-replace") || "")) {
			input.value = "";
			return;
		}
		// That was the question. Leaving the page is not asked about again.
		if (editor) editor.setAttribute("data-leaving", "");
		bar.submit();
	});
})();

// Asking before a song that is being written is left behind.
//
// Closing the tab, going back or following a link in the header throws away
// whatever has not been saved, and none of them asks. This does, with the
// browser's own question, once there is something to lose: a field typed in, a
// chord moved, or a page that came back from the server holding a change that
// is in no file yet. Saving, cancelling and the editor's own buttons are not
// leaving, and are not asked about.
//
// The question is only hung on the page while there is something to lose. A
// page that listens for it is one a browser will not keep whole to come back
// to, and most visits to the editor change nothing.
(function () {
	"use strict";

	var form = document.querySelector("form.editor");
	if (!form) return;

	var asking = false;

	function ask(e) {
		if (form.hasAttribute("data-leaving")) return;
		// The first is what the standard says; the second is what the browsers
		// that came before it listen for.
		e.preventDefault();
		e.returnValue = "";
	}

	function unsaved() {
		if (asking) return;
		asking = true;
		window.addEventListener("beforeunload", ask);
	}

	function changed(e) {
		// Ticking a section to join it, or choosing which kind to add, changes
		// nothing in the song.
		if (e.target.matches(".sectionpick") || e.target.matches("select[name=add]")) return;
		unsaved();
	}

	if (form.hasAttribute("data-unsaved")) unsaved();
	form.addEventListener("input", changed);
	form.addEventListener("change", changed);

	// Every button in the form goes to the server and comes back with the song,
	// and Cancel is somebody saying they do not want it.
	form.addEventListener("submit", function (e) {
		if (!e.defaultPrevented) form.setAttribute("data-leaving", "");
	});
	form.addEventListener("click", function (e) {
		if (e.target.closest && e.target.closest("[data-discard]")) form.setAttribute("data-leaving", "");
	});
})();

// The editor's keyboard, and the one button that needs to know where the caret is.
//
// Every action is still a button that goes to the server; this only presses
// them, so the keyboard and the mouse cannot disagree about what happened.
// Dividing a section needs the line the caret is on, which only a script can
// tell, so that button stays disabled without one.
(function () {
	"use strict";

	var form = document.querySelector("form.editor");
	if (!form) return;
	var at = form.querySelector("input[name=at]");
	var after = form.querySelector("input[name=after]");
	var merge = form.querySelector("[data-merge]");
	var count = form.querySelector("[data-count]");
	var current = null;

	// The keys are worth showing only to somebody with a keyboard: the labels
	// on the buttons, and the button that lists the rest. A Mac's keys are
	// called what its keyboard calls them.
	if (window.matchMedia("(hover: hover) and (pointer: fine)").matches) {
		document.documentElement.classList.add("haskeys");
		var keys = form.querySelector("[data-keys]");
		if (keys) keys.hidden = false;
		if (/Mac|iPhone|iPad/.test(navigator.platform || "")) {
			[".hint", ".keys kbd"].forEach(function (sel) {
				var labels = document.querySelectorAll(sel);
				for (var k = 0; k < labels.length; k++) {
					labels[k].textContent = labels[k].textContent
						.replace(/\bAlt\b/, "⌥").replace(/\bCtrl\b/, "⌘");
				}
			});
		}
	}

	function picks() { return form.querySelectorAll(".sectionpick:checked"); }

	function press(button) {
		if (!button) return;
		button.disabled = false;
		if (form.requestSubmit) form.requestSubmit(button);
		else button.click();
	}

	// The line the second half starts on, counted the way the server counts it:
	// past any blank lines at the top. A caret after the last word of a line
	// divides after that line; anywhere else, before it.
	function lineAt(box) {
		var v = box.value, pos = box.selectionStart;
		var lead = v.length - v.replace(/^\n+/, "").length;
		var line = v.slice(lead, Math.max(pos, lead)).split("\n").length - 1;
		var start = v.lastIndexOf("\n", pos - 1) + 1;
		if ((pos >= v.length || v.charAt(pos) === "\n") && pos > start) line++;
		return line;
	}

	// The same line for whatever the caret is in, and null for anything that is
	// not somewhere lines are written. The box a section arrives in counts its
	// own; one that shows only part of a section says, since only what made it
	// knows what stands before it.
	function lineIn(el) {
		if (!el || !el.matches) return null;
		if (el.matches(".sectionbody")) return lineAt(el);
		return typeof el.lineInSection === "function" ? el.lineInSection() : null;
	}

	function refresh() {
		var splits = form.querySelectorAll("[data-split]");
		for (var i = 0; i < splits.length; i++) {
			splits[i].disabled = splits[i].closest(".songsection") !== current;
		}
		var n = picks().length;
		if (merge) merge.disabled = n < 2;
		if (count) count.textContent = n >= 2 ? "(" + n + ")" : "";
	}

	// Where the caret is, remembered as it moves, so that a button can be
	// pressed after the caret has left the box to press it: the line to divide
	// at, and the section a new one goes after.
	function track(e) {
		var box = e.target.closest ? e.target.closest(".songsection") : null;
		if (box && after) {
			after.value = Array.prototype.indexOf.call(form.querySelectorAll(".songsection"), box);
		}
		var line = lineIn(e.target);
		if (line === null) return;
		current = box;
		if (at) at.value = line;
		refresh();
	}
	["focusin", "keyup", "click", "select"].forEach(function (type) {
		form.addEventListener(type, track);
	});
	form.addEventListener("change", function (e) {
		if (e.target.matches(".sectionpick")) refresh();
	});
	// A section's box grows with what is typed into it, as it was drawn to fit
	// what it held.
	form.addEventListener("input", function (e) {
		var box = e.target;
		if (!box.matches(".sectionbody")) return;
		while (box.scrollHeight > box.clientHeight && box.rows < 40) box.rows++;
	});
	// The page may have come back with the caret already in a section, put
	// there before this script was listening.
	track({ target: document.activeElement || form });
	refresh();

	form.addEventListener("keydown", function (e) {
		var target = e.target;
		if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key === "Enter") {
			e.preventDefault();
			press(form.querySelector("[data-save]"));
			return;
		}
		// Enter in a one-line field is not a reason to send the form. In a
		// section's name it means "done naming", so it goes on to the lines.
		if (e.key === "Enter" && !e.altKey && target.tagName === "INPUT" && target.type !== "checkbox") {
			e.preventDefault();
			var section = target.closest(".songsection");
			var lines = section && section.querySelector(".sectionbody");
			if (lines && target.matches(".sectionlabel")) lines.focus();
			return;
		}
		if (!e.altKey || e.ctrlKey || e.metaKey) return;

		var box = (target.closest && target.closest(".songsection")) || current;
		var button = null;
		switch (e.code) {
		case "Enter":
		case "NumpadEnter":
			var line = box ? lineIn(target) : null;
			if (line !== null) {
				if (at) at.value = line;
				button = box.querySelector("[data-split]");
			}
			break;
		case "ArrowUp":
			button = box && box.querySelector("[data-up]");
			break;
		case "ArrowDown":
			button = box && box.querySelector("[data-down]");
			break;
		case "KeyX":
			if (box) {
				var pick = box.querySelector(".sectionpick");
				pick.checked = !pick.checked;
				refresh();
			}
			e.preventDefault();
			return;
		case "KeyM":
			// The ticked ones, or failing that this one and the next.
			if (picks().length < 2 && box) {
				var next = box.nextElementSibling;
				if (next && next.matches(".songsection")) {
					box.querySelector(".sectionpick").checked = true;
					next.querySelector(".sectionpick").checked = true;
				}
			}
			if (picks().length >= 2) button = merge;
			break;
		case "KeyN":
			button = form.querySelector("button[name=addnow]");
			break;
		default:
			return;
		}
		e.preventDefault();
		press(button);
	});
})();

// The words of a section with its chords above them.
//
// A section reaches the page as a box of text with its chords in brackets,
// which is what the form posts and what there is to edit with no script. This
// takes each one over and shows it the way a sheet is read: the words in a box
// of their own and every chord floating over the syllable it falls on, where it
// can be dragged along the line, renamed or thrown away.
//
// It knows nothing about the format. The server hands each section over already
// taken apart, as its words and the place in them each bracket stood, and that
// is what goes back; the brackets are written in Go, by the code that writes
// every other line of a song file. What a typed name means is asked of the
// server too. All that is decided here is where a chord goes when the words
// under it change, which is a question about editing and not about music.
(function () {
	"use strict";

	var form = document.querySelector("form.editor");
	var kit = document.getElementById("chordkit");
	var pick = document.getElementById("chordpick");
	if (!form || !kit || !kit.content || !pick || typeof pick.showPopover !== "function") return;

	var name = document.getElementById("pickname");
	var offer = document.getElementById("pickbody");
	var used = pick.querySelector(".pickused");
	var usedList = pick.querySelector(".pickchords");
	var narrow = window.matchMedia("(max-width: 34rem)");
	var fine = window.matchMedia("(hover: hover) and (pointer: fine)");
	var probe = document.createRange();

	// What words are called on the clipboard when their chords go with them.
	var CLIP = "application/x-tabernacle-words";

	var sections = [];
	// What the server has said about a name: true for a chord, false for
	// anything else that goes in brackets.
	var known = new Map();
	var asked = new Map();

	var editing = null;   // the chord the chooser is open on
	var showing = null;   // the name the chooser's choices are for
	var lookup = 0;
	var drag = null;      // the chord being held
	var armed = false;    // a press that began in the room above a line
	var shut = false;     // a finger's tap that closes the chooser, and is for nothing else
	var skip = false;     // the click that ends a drag is not a press
	var hovered = null;   // the block the pointer is offering a chord in
	// A Mac's Option and an arrow walk the words a word at a time, which is not
	// to be taken from it.
	var mac = /Mac|iPhone|iPad/.test(navigator.platform || "");

	document.documentElement.classList.add("chordsfloat");

	Array.prototype.forEach.call(form.querySelectorAll(".songsection"), function (el) {
		var sec = take(el);
		if (sec) sections.push(sec);
	});
	if (!sections.length) return;

	function piece(selector) {
		return kit.content.querySelector(selector).cloneNode(true);
	}

	// ------------------------------------------------- sections and blocks ---

	// take replaces the box a section arrived in with the blocks it is made of:
	// a box of words for each run of lines, a fixed-width one for each fenced
	// block, and a gap, which is not drawn, where only blank lines lie between
	// two of those. A section the server could not take apart has no rows, and
	// is left as it is.
	function take(el) {
		var raw = el.querySelector(".sectionbody");
		var store = el.querySelector(".sectionrows");
		if (!raw || !store) return null;
		var rows;
		try {
			rows = JSON.parse(store.value);
		} catch (e) {
			return null;
		}
		if (!Array.isArray(rows)) return null;

		var sec = { el: el, raw: raw, store: store, blocks: [] };
		var runs = [];
		rows.forEach(function (row) {
			var last = runs[runs.length - 1];
			if (row.fenced) runs.push({ fenced: row });
			else if (last && last.rows) last.rows.push(row);
			else runs.push({ rows: [row] });
		});
		runs.forEach(function (run, i) {
			if (run.fenced) {
				sec.blocks.push(fence(sec, run.fenced));
				return;
			}
			// Blank lines between words and a fenced block are a gap and
			// nothing more. They are kept, and sent back as they came, but not
			// given a line of their own to be typed on; and where there is
			// nothing but a gap between two blocks, no box at all.
			var lines = run.rows, lead = 0, trail = 0;
			if (i > 0) {
				while (lines.length && blank(lines[0])) {
					lines = lines.slice(1);
					lead++;
				}
			}
			if (i < runs.length - 1) {
				while (lines.length && blank(lines[lines.length - 1])) {
					lines = lines.slice(0, -1);
					trail++;
				}
			}
			if (!lines.length) {
				sec.blocks.push({ sec: sec, gap: lead + trail });
				return;
			}
			var b = words(sec);
			b.lead = lead;
			b.trail = trail;
			lines.forEach(function (row, k) {
				if (k) b.text += "\n";
				var start = b.text.length, line = row.words || "";
				b.text += line;
				(row.marks || []).forEach(function (m) {
					// A bracket with nothing in it is not kept, here or there.
					if (!m.text || !m.text.trim()) return;
					// Only that it is a chord is remembered. That it is not one,
					// as a file is read, says nothing of what the same letters
					// mean when they are typed where a chord goes.
					if (m.chord) known.set(m.text, true);
					b.marks.push({
						pos: start + Math.max(0, Math.min(m.at, line.length)),
						text: m.text, chord: !!m.chord, el: null
					});
				});
			});
			sec.blocks.push(b);
		});
		sec.blocks.forEach(function (block) {
			if (!block.el) return;
			raw.parentNode.insertBefore(block.el, raw);
			if (block.mirror) block.box.value = block.text;
		});
		raw.hidden = true;
		return sec;
	}

	function blank(row) {
		return !row.words && !(row.marks && row.marks.length);
	}

	// A block of words: the box they are typed in, the copy of them that can be
	// measured, and the chords standing in them, in the order they are read.
	function words(sec) {
		var el = piece(".lyr");
		var b = {
			sec: sec, fenced: false, el: el,
			box: el.querySelector(".lyrtext"),
			mirror: el.querySelector(".lyrmirror"),
			layer: el.querySelector(".chips"),
			line: el.querySelector(".chipmark"),
			ghost: el.querySelector(".chipghost"),
			text: "", marks: [], lead: 0, trail: 0,
			drawn: null, lines: [], starts: [], cells: [],
			caret: 0, turn: null, pointed: null,
			undo: [], redo: [], last: "", lastAt: 0
		};
		el.block = b;
		b.box.lineInSection = function () { return lineOf(b, b.box.selectionStart); };
		return b;
	}

	// A fenced block: tablature, which is columns and stays as it is written.
	function fence(sec, row) {
		var el = piece(".tabbox");
		var b = { sec: sec, fenced: true, el: el, box: el.querySelector(".tabtext"), info: row.info || "" };
		el.block = b;
		el.querySelector(".tabtag").textContent = b.info;
		b.box.value = row.words || "";
		b.box.lineInSection = function () { return lineOf(b, 0); };
		fit(b);
		return b;
	}

	function fit(b) {
		b.box.rows = Math.min(Math.max(b.box.value.split("\n").length, 2), 40);
	}

	function wordsOf(el) {
		return el && el.matches && el.matches(".lyrtext") ? el.parentNode.block || null : null;
	}

	function chipOf(el) {
		var chip = el && el.closest ? el.closest(".chip") : null;
		return chip && chip.mark ? chip : null;
	}

	// rowsOf is a section the way it goes back to the server: a row for every
	// line of words, with the place of each chord in it, and one for every
	// fenced block.
	function rowsOf(sec) {
		var rows = [];
		function gap(n) {
			while (n-- > 0) rows.push({ words: "" });
		}
		sec.blocks.forEach(function (b) {
			if (b.fenced) {
				var inside = b.box.value.replace(/\n+$/, "");
				if (inside.trim() !== "") rows.push({ words: inside, fenced: true, info: b.info });
				return;
			}
			if (!b.el) {
				gap(b.gap);
				return;
			}
			gap(b.lead);
			b.first = rows.length;
			// Each chord goes to the line it stands on, which is only found by
			// walking them in order.
			var all = b.marks.slice().sort(byPlace);
			var start = 0, k = 0;
			b.text.split("\n").forEach(function (line) {
				var row = { words: line }, marks = [];
				for (; k < all.length && all[k].pos <= start + line.length; k++) {
					if (!all[k].text) continue;
					var mark = { at: all[k].pos - start, text: all[k].text };
					// A name nobody has read yet is read by the server as it
					// writes the line, so a song saved in a hurry is saved right.
					if (unread(all[k])) mark.typed = true;
					marks.push(mark);
				}
				if (marks.length) row.marks = marks;
				// Spaces alone on a line are a blank line. Nobody can see them
				// to have meant them.
				else if (line.trim() === "") row.words = "";
				rows.push(row);
				start += line.length + 1;
			});
			gap(b.trail);
		});
		return rows;
	}

	function byPlace(a, z) {
		return a.pos - z.pos;
	}

	// unread says a chord's name is one somebody typed that the server has not
	// said the meaning of yet.
	function unread(m) {
		return !!m.text && m.chord !== false && !known.has(m.text);
	}

	// touch says a section has been changed here, so it goes back as rows and
	// the text it arrived as is no longer sent. Until this is called a section
	// goes back as exactly the text it was.
	function touch(sec) {
		sec.store.value = JSON.stringify(rowsOf(sec));
		sec.store.disabled = false;
		sec.raw.disabled = true;
		// Said the way a field says it, for whatever else on the page wants to
		// know the song has changed.
		sec.store.dispatchEvent(new Event("change", { bubbles: true }));
	}

	// lineOf is the line of the whole section a caret is on, counted the way
	// the server counts once it has written the section out: one for each line
	// of words, and for a fenced block its lines and its two fences. Blank
	// lines at the top do not count, because they are not kept. A caret after
	// the last word of a line divides after it and anywhere else before it; in
	// a fenced block it divides before the block, which cannot be cut in two.
	function lineOf(b, caret) {
		var rows = rowsOf(b.sec);
		var row = b.first;
		if (b.fenced) {
			// The rows before it are those of every block before it.
			var mine = b.sec.blocks.indexOf(b);
			row = rowsOf({ blocks: b.sec.blocks.slice(0, mine) }).length;
		} else {
			var before = b.text.slice(0, caret);
			var start = before.lastIndexOf("\n") + 1;
			row += before.split("\n").length - 1;
			if (caret > start && (caret >= b.text.length || b.text.charAt(caret) === "\n")) row++;
		}
		var skip = 0;
		while (skip < rows.length && !rows[skip].fenced && blank(rows[skip])) skip++;
		var line = 0;
		for (var i = skip; i < row && i < rows.length; i++) {
			line += rows[i].fenced ? rows[i].words.split("\n").length + 2 : 1;
		}
		return line;
	}

	// ---------------------------------------------------------- measuring ---

	// reflect copies the words into the mirror: the same words laid out by the
	// same rules in the same place, where they can be measured, since a text
	// box will not say where one of its letters is. Each line is an element of
	// its own and ends in something that takes no room, so that an empty line
	// still has a height and the end of every line is somewhere.
	function reflect(b) {
		if (b.drawn === b.text) return;
		b.lines = b.text.split("\n");
		b.starts = [];
		b.cells = [];
		b.mirror.textContent = "";
		var at = 0;
		b.lines.forEach(function (line) {
			var row = document.createElement("div");
			var end = document.createElement("span");
			end.textContent = "​";
			row.appendChild(document.createTextNode(line));
			row.appendChild(end);
			b.mirror.appendChild(row);
			b.starts.push(at);
			at += line.length + 1;
		});
		b.drawn = b.text;
	}

	// lineAt is the line a place in the words is on.
	function lineAt(b, pos) {
		var lo = 0, hi = b.starts.length - 1;
		while (lo < hi) {
			var mid = (lo + hi + 1) >> 1;
			if (b.starts[mid] <= pos) lo = mid;
			else hi = mid - 1;
		}
		return lo;
	}

	// letter is where on the screen one letter of a line is drawn, or the end
	// of the line when there is no such letter.
	function letter(b, i, col) {
		var row = b.mirror.children[i];
		if (col < b.lines[i].length) {
			probe.setStart(row.firstChild, col);
			probe.setEnd(row.firstChild, col + 1);
			var rect = probe.getClientRects()[0];
			if (rect) return rect;
		}
		return row.lastChild.getBoundingClientRect();
	}

	// places lists everywhere on a line a chord could stand, before each of its
	// letters and at its end, with where in the block that is. It is kept until
	// the words or the width of the block change.
	function places(b, i, frame) {
		if (b.cells[i]) return b.cells[i];
		var line = b.lines[i], out = [];
		for (var col = 0; col <= line.length; col++) {
			// Never between the two halves of one character.
			var code = line.charCodeAt(col);
			if (code >= 0xDC00 && code <= 0xDFFF) continue;
			var rect = letter(b, i, col);
			out.push({
				pos: b.starts[i] + col,
				x: rect.left - frame.left,
				top: rect.top - frame.top,
				bottom: rect.bottom - frame.top
			});
		}
		return (b.cells[i] = out);
	}

	// near is the place in the words nearest a point on the screen, and whether
	// the point is over the words or in the room above them where their chords
	// are.
	//
	// A line owns the room above it. Letters sit in the middle of a line's
	// height, so that room is the gap between the letters of one line and those
	// of the next, and a point belongs to the first line whose letters end
	// below it.
	function near(b, x, y) {
		reflect(b);
		var frame = b.layer.getBoundingClientRect();
		x -= frame.left;
		y -= frame.top;

		var rows = b.mirror.children;
		// How far above its own letters a line begins, and so how far above
		// that its room for chords does.
		var rise = places(b, 0, frame)[0].top - rows[0].offsetTop;
		var lo = 0, hi = rows.length - 1;
		while (lo < hi) {
			var mid = (lo + hi + 1) >> 1;
			if (rows[mid].offsetTop - rise <= y) lo = mid;
			else hi = mid - 1;
		}

		// A long line is drawn as several, each with room above it too.
		var all = places(b, lo, frame), top = all[0].top, best = null;
		all.forEach(function (c) {
			if (c.top - 2 * rise <= y) top = c.top;
		});
		all.forEach(function (c) {
			if (Math.abs(c.top - top) > 3) return;
			if (!best || Math.abs(c.x - x) < Math.abs(best.x - x)) best = c;
		});
		return { pos: best.pos, x: best.x, top: best.top, bottom: best.bottom, above: y < best.top };
	}

	// ------------------------------------------------------------ drawing ---

	// draw puts every chord of a block where it stands.
	function draw(b) {
		reflect(b);
		b.marks.forEach(function (m) {
			if (!m.el) {
				m.el = piece(".chip");
				m.el.mark = m;
				m.el.block = b;
			}
			var label = m.text || "…";
			if (m.el.textContent !== label) m.el.textContent = label;
			m.el.classList.toggle("note", m.chord === false);
			m.el.classList.toggle("blank", !m.text);
		});
		if (!drag) order(b);

		var frame = b.layer.getBoundingClientRect();
		var at = b.marks.map(function (m) {
			var i = lineAt(b, m.pos);
			var rect = letter(b, i, m.pos - b.starts[i]);
			return {
				m: m, x: rect.left - frame.left, top: rect.top - frame.top, bottom: rect.bottom - frame.top,
				w: m.el.offsetWidth, h: m.el.offsetHeight,
				// A line of chords with no words under it has no syllables for
				// one to have been moved away from.
				bare: b.lines[i].trim() === ""
			};
		});

		// A chord starts where its syllable does, unless the one before it on
		// the line has not ended yet. Then it stands after that one, and is
		// marked as belonging further back. Worked from the left, and then
		// from the right so that none is pushed off the edge.
		var edge = {}, wall = {};
		at.forEach(function (p) {
			var row = Math.round(p.top);
			p.left = Math.max(p.x, row in edge ? edge[row] + (p.bare ? 8 : 3) : 0);
			edge[row] = p.left + p.w;
		});
		for (var k = at.length - 1; k >= 0; k--) {
			var row = Math.round(at[k].top);
			var most = (row in wall ? wall[row] - 3 : frame.width) - at[k].w;
			if (at[k].left > most) at[k].left = Math.max(most, 0);
			wall[row] = at[k].left;
		}
		at.forEach(function (p) {
			var m = p.m;
			m.el.style.left = Math.round(p.left) + "px";
			m.el.style.top = Math.round(p.top - p.h) + "px";
			m.el.classList.toggle("pushed", !p.bare && p.left - p.x > 6);
			m.x = p.x;
			m.top = p.top;
			m.bottom = p.bottom;
			m.mid = p.left + p.w / 2;
		});

		// One chord of a block takes Tab, and the arrows go on from it.
		if (b.marks.indexOf(b.turn) < 0) b.turn = b.marks[0] || null;
		b.marks.forEach(function (m) { m.el.tabIndex = m === b.turn ? 0 : -1; });

		if (b.marks.indexOf(b.pointed) < 0) b.pointed = null;
		point(b, b.pointed);
	}

	// order puts the chords on the page in the order they are read, which is
	// the order the keyboard walks them in. Moving one about on the page costs
	// it the keyboard, so nothing is moved unless the order changed, and never
	// while one is held.
	function order(b) {
		var kids = b.layer.children, same = kids.length === b.marks.length;
		for (var i = 0; same && i < kids.length; i++) same = kids[i] === b.marks[i].el;
		if (same) return;
		var focused = document.activeElement;
		while (b.layer.firstChild) b.layer.removeChild(b.layer.firstChild);
		b.marks.forEach(function (m) { b.layer.appendChild(m.el); });
		if (focused && focused.mark && b.marks.indexOf(focused.mark) >= 0) focused.focus({ preventScroll: true });
	}

	// point shows where a chord stands, to the letter: a line through the words
	// at the place it is at, or would be put.
	function point(b, where) {
		b.line.classList.toggle("on", !!where);
		if (!where) return;
		b.line.style.left = Math.round(where.x) + "px";
		b.line.style.top = Math.round(where.top) + "px";
		b.line.style.height = Math.round(where.bottom - where.top) + "px";
	}

	// seat puts a chord among the others in the order they are read: by where
	// it stands and, among those standing in one place, by how far along it was
	// dropped. With nowhere said it goes after them.
	function seat(b, m, x) {
		var i = b.marks.indexOf(m);
		if (i >= 0) b.marks.splice(i, 1);
		var at = 0;
		while (at < b.marks.length && (b.marks[at].pos < m.pos ||
			(b.marks[at].pos === m.pos && !(x < middle(b.marks[at]))))) at++;
		b.marks.splice(at, 0, m);
	}

	// A chord that has not been drawn yet has no middle, and anything that is
	// to go before the chords at its place goes before it too.
	function middle(m) {
		return m.mid === undefined ? Infinity : m.mid;
	}

	// ------------------------------------------------------------ history ---

	function snap(b) {
		return {
			text: b.text, caret: b.caret,
			marks: b.marks.map(function (m) {
				return { pos: m.pos, text: m.text, chord: m.chord, loose: m.loose, stray: m.stray };
			})
		};
	}

	function keep(b, was) {
		b.undo.push(was);
		if (b.undo.length > 200) b.undo.shift();
		b.redo = [];
		b.last = "";
	}

	// remember keeps how a block stood before a change. Letters typed one after
	// another are one change, and so are letters rubbed out, or undoing a
	// sentence would take a keystroke for each of its letters.
	function remember(b, kind) {
		var now = Date.now();
		var joins = (kind === "type" || kind === "erase") && kind === b.last && now - b.lastAt < 1000;
		b.lastAt = now;
		if (joins) return;
		keep(b, snap(b));
		b.last = kind;
	}

	// back takes a block to how it stood, out of one pile of changes and into
	// the other.
	function back(b, from, to) {
		if (!from.length) return;
		// Whatever was in hand is let go of first: it belongs to how the block
		// stands now, and that is about to be replaced.
		if (drag && drag.b === b) {
			drag.el.classList.remove("held");
			drag = null;
			skip = true;
		}
		if (editing && editing.b === b) {
			editing.dropped = true;
			pick.hidePopover();
		}
		var was = from.pop();
		to.push(snap(b));
		b.last = "";
		b.text = was.text;
		b.marks = was.marks.map(function (m) {
			return { pos: m.pos, text: m.text, chord: m.chord, loose: m.loose, stray: m.stray, el: null };
		});
		b.box.value = b.text;
		b.box.setSelectionRange(was.caret, was.caret);
		b.caret = was.caret;
		// A name that was still out when this was kept is asked for again:
		// the answer went to the chord as it then was.
		b.marks.forEach(function (m) { if (unread(m)) read(b, m); });
		draw(b);
		touch(b.sec);
		if (!b.el.contains(document.activeElement)) b.box.focus();
	}

	// -------------------------------------------------------------- words ---

	// typed is called when the words have changed, however that happened, and
	// moves the chords to suit.
	function typed(b, e) {
		var now = b.box.value, old = b.text;
		if (now === old) return;
		// What changed lies between what the two share at the start and what
		// they share at the end. The end is measured first and no further back
		// than the caret, which is where typing leaves it: otherwise a letter
		// typed beside the same letter could be either of the two.
		var caret = b.box.selectionEnd, tail = 0;
		while (tail < now.length - caret && tail < old.length &&
			old.charCodeAt(old.length - 1 - tail) === now.charCodeAt(now.length - 1 - tail)) tail++;
		var from = 0, most = Math.min(old.length, now.length) - tail;
		while (from < most && old.charCodeAt(from) === now.charCodeAt(from)) from++;
		// Not through the middle of a character that takes two units, such as
		// most emoji: two different ones share their first half.
		if (from > 0 && (half(old.charCodeAt(from)) || half(now.charCodeAt(from)))) from--;
		if (tail > 0 && (half(old.charCodeAt(old.length - tail)) || half(now.charCodeAt(now.length - tail)))) tail--;
		var to = old.length - tail, put = now.slice(from, now.length - tail);

		// A bracket that arrived as a letter, because the keyboard gave no
		// chance to stop it. It is taken back out and treated as what it is.
		if (put === "[" && from === to && !(e && e.isComposing)) {
			b.box.setRangeText("", from, from + 1, "start");
			bracket(b, from);
			return;
		}

		// A letter put together from several keys, an accent and then its
		// vowel, is one change and was kept when it began.
		if (!b.composing) remember(b, !put ? "erase" : from === to && put.indexOf("\n") < 0 ? "type" : "edit");
		follow(b, from, to, put);
		b.text = now;
		b.caret = b.box.selectionStart;
		draw(b);
		touch(b.sec);
	}

	// half says a unit is the second of the two some characters take.
	function half(code) {
		return code >= 0xDC00 && code <= 0xDFFF;
	}

	// follow moves the chords when the letters from one place to another are
	// replaced by something else.
	//
	// A chord belongs to the letter it stands over and goes where that letter
	// goes. Typing in front of a chord therefore pushes it along, and so does
	// typing exactly where it stands, since that is in front of its letter. At
	// the end of a line it has no letter, and stays on its line when the line
	// is broken there.
	//
	// When its letter is rubbed out, the chord is kept, because a chord is more
	// work to put back than a word. It waits at the gap with no letter of its
	// own, and what is typed there next goes under it: rub a word out, type
	// another, and the chord is over the new one. A chord just put in from the
	// keyboard waits the same way for the syllable it is for.
	//
	// When whole lines go their chords go with them: nobody deleting a verse
	// wants its chords left in a heap on the line above. That takes in the
	// chord at the end of a line all of which went. It also takes in a line
	// emptied first and joined to the one before after, which is how a line is
	// usually deleted: the chords left waiting on it go when it does, and so
	// does the one that was at its end.
	function follow(b, from, to, put) {
		var old = b.text;
		var across = old.slice(from, to).indexOf("\n") >= 0;
		var grown = put.length - (to - from);
		var ends = to === old.length || old.charAt(to) === "\n";
		var begins = old.lastIndexOf("\n", to - 1) + 1;
		// The line the change ends on has gone whole if the change ends where
		// the line does and began before it, taking more than a line break.
		var whole = across && ends && to - from > 1 && begins > from;
		// Or it has lost every letter it had and is still there, empty.
		var emptied = !across && ends && !put && to > from && begins === from;
		b.marks = b.marks.filter(function (m) {
			var p = m.pos, waited = m.loose, left = m.stray;
			m.loose = m.stray = false;
			if (p < from) return true;
			if (p > to || (p === to && to > from)) {
				if (p === to && across && (whole || waited || left)) return false;
				m.pos = p + grown;
				// Still waiting, if the letters between it and the caret went.
				m.loose = waited && !put && m.pos === from;
				// Left at the end of a line with nothing on it.
				m.stray = !put && m.pos === from && (left || (p === to && emptied));
				return true;
			}
			if (from === to) {
				// Typed where it stands. It has no letter at the end of a line,
				// and stays on its line when the line is broken there.
				var last = p === old.length || old.charAt(p) === "\n";
				if (!waited && !(last && put.charAt(0) === "\n")) m.pos = p + put.length;
				return true;
			}
			if (across && !(p === from && old.charAt(from) === "\n")) return false;
			m.pos = from;
			m.loose = !put;
			return true;
		});
		// A chord that waited where the others were pushed along is before
		// them now. Those that share a place keep the order they were in.
		b.marks.sort(byPlace);
	}

	// A bracket opens a chord, as it does in a song file. After a backslash it
	// is only a bracket, as it is there too.
	function bracket(b, at) {
		if (at > 0 && b.text.charAt(at - 1) === "\\") {
			b.box.setRangeText("[", at - 1, at, "end");
			typed(b);
			return;
		}
		begin(b, at, "words");
	}

	// ------------------------------------------------------------- chords ---

	// begin stands a new chord in the words and asks what it is. It has no name
	// until the chooser gives it one, and is gone again if it never gets one.
	function begin(b, pos, from, x) {
		var before = snap(b);
		var m = { pos: pos, text: "", chord: true, el: null };
		seat(b, m, x);
		draw(b);
		choose(b, m, from, before);
	}

	function remove(b, m) {
		var i = b.marks.indexOf(m);
		if (i < 0) return;
		remember(b, "chord");
		b.marks.splice(i, 1);
		var next = b.marks[i] || b.marks[i - 1];
		b.turn = next || null;
		draw(b);
		touch(b.sec);
		if (next) {
			next.el.focus();
		} else {
			b.box.focus();
			b.box.setSelectionRange(m.pos, m.pos);
		}
	}

	// nudge moves a chord along its line by a letter, or to where the next word
	// or the one before it starts.
	function nudge(b, m, step, byWord) {
		var i = lineAt(b, m.pos), line = b.lines[i], col = m.pos - b.starts[i];
		if (byWord && step > 0) {
			var ahead = line.slice(col).search(/\s\S/);
			col = ahead < 0 ? line.length : col + ahead + 1;
		} else if (byWord) {
			var behind = line.slice(0, col).search(/\S+\s*$/);
			col = behind < 0 ? 0 : behind;
		} else {
			col += step;
			// Not to between the two halves of one character.
			var code = line.charCodeAt(col);
			if (code >= 0xDC00 && code <= 0xDFFF) col += step;
		}
		col = Math.max(0, Math.min(line.length, col));
		if (b.starts[i] + col === m.pos) return;
		remember(b, "chord");
		m.pos = b.starts[i] + col;
		// It joins any chords already there from the side it came from.
		seat(b, m, step > 0 ? -Infinity : Infinity);
		settled(b, m);
	}

	// hop moves a chord to the line above or below, as near as it can to
	// straight up or down.
	function hop(b, m, step) {
		var i = lineAt(b, m.pos) + step;
		if (i < 0 || i >= b.lines.length) return;
		var all = places(b, i, b.layer.getBoundingClientRect());
		var top = step > 0 ? all[0].top : all[all.length - 1].top, best = null;
		all.forEach(function (c) {
			if (Math.abs(c.top - top) > 3) return;
			if (!best || Math.abs(c.x - m.x) < Math.abs(best.x - m.x)) best = c;
		});
		remember(b, "chord");
		m.pos = best.pos;
		seat(b, m, m.x);
		settled(b, m);
	}

	function settled(b, m) {
		b.pointed = m;
		draw(b);
		m.el.focus({ preventScroll: true });
		touch(b.sec);
	}

	// ------------------------------------------------------------ chooser ---

	// choose opens the chooser on a chord. A new one comes with how the block
	// stood before it was put there, which is what undoing it goes back to.
	function choose(b, m, from, before) {
		if (pick.matches(":popover-open")) pick.hidePopover();
		if (b.marks.indexOf(m) < 0) return;
		editing = {
			b: b, m: m, from: from, was: m.text, fresh: !!before, before: before || snap(b),
			caret: b.box.selectionStart, dropped: false, applied: false
		};
		name.value = m.text;
		listUsed();
		show(m.text);
		hush();
		b.pointed = m;
		point(b, m);
		m.el.classList.add("on");
		pick.showPopover();
		settle();
		reveal();
		// Somebody with a keyboard types the name. On a phone the keyboard
		// would cover the choices, so it waits to be asked for.
		if (fine.matches || from === "words" || from === "keys") {
			name.focus();
			name.select();
		} else {
			pick.focus();
		}
	}

	// done is called as the chooser closes, however it was closed. What the
	// name box says is what the chord becomes; Escape leaves it as it was.
	function done() {
		var e = editing;
		if (!e) return;
		editing = null;
		showing = null;
		clearTimeout(lookup);
		var b = e.b, m = e.m, at = b.marks.indexOf(m);
		if (m.el) m.el.classList.remove("on");
		// Gone already, if the block was taken back to how it stood before.
		if (at < 0) return;
		var text = e.dropped ? e.was : clean(name.value);
		if (text !== m.text) {
			m.text = text;
			if (text) read(b, m);
		}
		if (!m.text) b.marks.splice(at, 1);
		var changed = e.fresh ? !!m.text : m.text !== e.was;
		if (changed) keep(b, e.before);
		b.pointed = null;
		if (m.text) b.turn = m;
		draw(b);
		if (changed || e.applied) touch(b.sec);

		// The keyboard goes back to the song, unless the chooser was closed by
		// pressing somewhere else, which is then where it already is.
		if (!pick.contains(document.activeElement)) return;
		if (e.from === "words") {
			// Back to the words, where the chord waits for what is typed next:
			// the syllable it is for.
			var caret = m.text ? m.pos : e.caret;
			b.box.focus();
			b.box.setSelectionRange(caret, caret);
			b.caret = caret;
			m.loose = true;
		} else if (m.text) {
			m.el.focus({ preventScroll: true });
		} else if (fine.matches) {
			b.box.focus();
		}
	}

	// clean is a name as it can be written between brackets.
	function clean(text) {
		return text.replace(/[\]\r\n]/g, "").replace(/^\s*\[/, "").trim();
	}

	// apply writes one of the chooser's own choices, which leaves it open: a
	// note is often followed by a kind.
	function apply(value) {
		var e = editing;
		if (!e) return;
		e.applied = true;
		known.set(value, true);
		name.value = value;
		e.m.text = value;
		e.m.chord = true;
		draw(e.b);
		touch(e.b.sec);
		show(value);
	}

	// ask puts a name to the server, which answers with what it means and what
	// it could be changed into. Each name is asked about once.
	function ask(text) {
		var answer = asked.get(text);
		if (answer) return answer;
		answer = fetch("/chords/pick?c=" + encodeURIComponent(text), { headers: { "Accept": "text/html" } })
			.then(function (r) { return r.ok ? r.text() : Promise.reject(r.status); })
			.then(function (html) {
				var root = new DOMParser().parseFromString(html, "text/html").querySelector(".pickbody");
				return { html: html, symbol: (root && root.getAttribute("data-symbol")) || "" };
			});
		asked.set(text, answer);
		answer.catch(function () { asked.delete(text); });
		return answer;
	}

	// read finds out what a typed name means and writes the chord the way the
	// server spells it: "lam" becomes Am, and "x2" stays what it is. Until the
	// answer comes the name stands as it was typed. A song saved before then
	// loses nothing by it: the name goes marked as unread, and the server reads
	// it the same way as it writes the line.
	function read(b, m) {
		var text = m.text;
		if (known.has(text)) {
			m.chord = known.get(text);
			return;
		}
		m.chord = true;
		ask(text).then(function (got) {
			if (got.symbol) known.set(got.symbol, true);
			else known.set(text, false);
			if (m.text !== text || b.marks.indexOf(m) < 0) return;
			m.chord = !!got.symbol;
			if (got.symbol) m.text = got.symbol;
			// If the chooser has been opened on it meanwhile, it still holds
			// the name as it was typed, in its box and as what to go back to,
			// and closing it either way would type that again.
			if (editing && editing.m === m) {
				if (editing.was === text) editing.was = m.text;
				if (clean(name.value) === text) name.value = m.text;
			}
			draw(b);
			touch(b.sec);
		}, function () {
			// No answer. The name stays as typed, and is read when it is saved.
		});
	}

	// show fills the chooser with what a name could be changed into.
	//
	// The note or the list that was just pressed is among what is replaced, and
	// would take the keyboard away with it, so the one that takes its place is
	// handed it.
	function show(text) {
		showing = text;
		ask(text).then(function (got) {
			if (showing !== text) return;
			var had = offer.contains(document.activeElement) ? document.activeElement : null;
			offer.innerHTML = got.html;
			if (had) {
				var next = had.id ? document.getElementById(had.id) : offer.querySelector(".rootlink.on");
				(next || name).focus();
			}
			settle();
			reveal();
		}, function () {});
	}

	// listUsed offers the chords the song already has, the commonest first:
	// the next chord of a song is nearly always one of them.
	function listUsed() {
		var count = new Map();
		sections.forEach(function (sec) {
			sec.blocks.forEach(function (b) {
				if (!b.mirror) return;
				b.marks.forEach(function (m) {
					if (m.chord && m.text) count.set(m.text, (count.get(m.text) || 0) + 1);
				});
			});
		});
		var names = Array.from(count.keys()).sort(function (a, z) { return count.get(z) - count.get(a); });
		usedList.textContent = "";
		names.slice(0, 12).forEach(function (text) {
			var button = document.createElement("button");
			button.type = "button";
			button.className = "chordref";
			button.setAttribute("data-c", text);
			button.textContent = text;
			usedList.appendChild(button);
		});
		used.hidden = !names.length;
	}

	// settle puts the chooser by its chord: above it, where it does not cover
	// the words the chord is over, or under those words when there is no room
	// above. On a phone the stylesheet makes it a sheet at the foot of the
	// screen, and nothing is set here.
	function settle() {
		var e = editing;
		if (!e || !e.m.el || !pick.matches(":popover-open")) return;
		var style = pick.style;
		if (narrow.matches) {
			style.left = style.top = style.bottom = style.maxHeight = "";
			return;
		}
		var chip = e.m.el.getBoundingClientRect();
		var under = e.b.layer.getBoundingClientRect().top + e.m.bottom;
		var gap = 8, tall = 340;
		var above = chip.top - gap, below = window.innerHeight - under - gap;
		style.left = Math.round(Math.max(gap, Math.min(chip.left, window.innerWidth - pick.offsetWidth - gap))) + "px";
		if (above >= tall || above >= below) {
			style.top = "";
			style.bottom = Math.round(window.innerHeight - chip.top + gap) + "px";
			style.maxHeight = Math.round(above - gap) + "px";
		} else {
			style.bottom = "";
			style.top = Math.round(under + gap) + "px";
			style.maxHeight = Math.round(below - gap) + "px";
		}
	}

	// reveal brings the chord out from behind the chooser on a phone, where the
	// chooser is a sheet over the foot of the screen: the page is moved up
	// until the chord and its words clear it. Once as the sheet opens and again
	// if it grows, and never while the page is being scrolled, which is the
	// reader looking at something else.
	function reveal() {
		var e = editing;
		if (!e || !e.m.el || !narrow.matches) return;
		var under = e.b.layer.getBoundingClientRect().top + e.m.bottom + 12;
		var over = under - (window.innerHeight - pick.offsetHeight);
		if (over > 0) window.scrollBy(0, over);
	}

	pick.addEventListener("beforetoggle", function (e) {
		if (e.newState === "closed") done();
	});

	// Escape closes the chooser and keeps nothing. The browser does the
	// closing, wherever the keyboard is; this only says what it meant, and
	// anything done in the chooser after an Escape that did not close it, one
	// that only shut a list, takes that back.
	document.addEventListener("keydown", function (e) {
		if (editing) editing.dropped = e.key === "Escape";
	}, true);
	["click", "change", "input"].forEach(function (type) {
		pick.addEventListener(type, function () {
			if (editing) editing.dropped = false;
		}, true);
	});

	pick.addEventListener("click", function (e) {
		if (!editing || !e.target.closest) return;
		var choice = e.target.closest("[data-c]");
		if (choice && choice.closest(".pickchords")) {
			// A chord the song already has is a whole answer.
			name.value = choice.getAttribute("data-c");
			pick.hidePopover();
		} else if (choice) {
			apply(choice.getAttribute("data-c"));
		} else if (e.target.closest("[data-remove]")) {
			name.value = "";
			pick.hidePopover();
		}
	});

	pick.addEventListener("change", function (e) {
		if (editing && e.target.matches("select")) apply(e.target.value);
	});

	pick.addEventListener("keydown", function (e) {
		if (e.target !== name || e.isComposing) return;
		if (e.key === "Enter" || e.key === "]") {
			e.preventDefault();
			pick.hidePopover();
		}
	});

	// As a name is typed, the choices follow it.
	name.addEventListener("input", function () {
		clearTimeout(lookup);
		lookup = setTimeout(function () {
			if (editing) show(clean(name.value));
		}, 200);
	});

	// ------------------------------------------------------------ pointer ---

	form.addEventListener("pointerdown", function (e) {
		skip = false;
		var chip = chipOf(e.target);
		// One with no name yet is still being put in, and stays where it is.
		if (!chip || e.button !== 0 || !e.isPrimary || !chip.mark.text) return;
		var b = chip.block, m = chip.mark;
		drag = {
			b: b, m: m, el: chip, id: e.pointerId, x: e.clientX, y: e.clientY,
			grab: e.clientX - b.layer.getBoundingClientRect().left - m.x,
			from: m.pos, at: b.marks.indexOf(m), was: snap(b), moved: false, lifted: false
		};
		try {
			chip.setPointerCapture(e.pointerId);
		} catch (x) {
			// Without it the chord still follows while the pointer is over the form.
		}
		// A finger that starts on a chord and goes up or down is scrolling the
		// page, and the browser takes it from here. One that rests on the chord
		// first has picked it up, and may then carry it anywhere.
		if (e.pointerType !== "touch") return;
		var held = drag;
		setTimeout(function () {
			if (drag !== held || held.moved) return;
			held.lifted = true;
			chip.classList.add("held");
		}, 350);
	});

	// Once a finger has picked a chord up, the page stays still under it.
	form.addEventListener("touchmove", function (e) {
		if (drag && drag.lifted && e.cancelable) e.preventDefault();
	}, { passive: false });

	document.addEventListener("pointermove", function (e) {
		if (!drag) {
			hover(e);
		} else if (e.pointerId === drag.id) {
			// A button let go of where nothing heard of it, in another window,
			// leaves a chord following a pointer that is no longer holding it.
			if (e.pointerType === "mouse" && !e.buttons) letGo(e, true);
			else pull(e);
		}
	});

	// pull moves the held chord to the place nearest the pointer. It does not
	// start until the pointer has gone far enough to mean it: a press that
	// trembles is still a press.
	function pull(e) {
		var d = drag, b = d.b, m = d.m;
		if (!d.moved) {
			if (Math.abs(e.clientX - d.x) + Math.abs(e.clientY - d.y) < 5) return;
			d.moved = true;
			d.el.classList.add("held");
			hush();
		}
		var at = near(b, e.clientX - d.grab, e.clientY);
		m.pos = at.pos;
		seat(b, m, e.clientX - b.layer.getBoundingClientRect().left);
		b.pointed = m;
		draw(b);
	}

	// letGo ends a drag: the chord stays where it was dropped, or with Alt or
	// Ctrl held a copy of it does, or it goes back if the drag was called off.
	function letGo(e, calledOff) {
		var d = drag, b = d.b, m = d.m;
		drag = null;
		try {
			d.el.releasePointerCapture(d.id);
		} catch (x) {
			// It had already been let go of.
		}
		d.el.classList.remove("held");
		if (!d.moved || b.marks.indexOf(m) < 0) return;
		skip = true;
		var stays = m;
		if (calledOff || e.altKey || e.ctrlKey || e.metaKey) {
			var where = m.pos, x = m.mid;
			m.pos = d.from;
			b.marks.splice(b.marks.indexOf(m), 1);
			b.marks.splice(d.at, 0, m);
			if (!calledOff) {
				stays = { pos: where, text: m.text, chord: m.chord, el: null };
				seat(b, stays, x);
				if (unread(stays)) read(b, stays);
			}
		}
		if (stays !== m || m.pos !== d.from || b.marks.indexOf(m) !== d.at) {
			keep(b, d.was);
			touch(b.sec);
		}
		b.turn = stays;
		b.pointed = null;
		draw(b);
		stays.el.focus({ preventScroll: true });
	}

	form.addEventListener("pointerup", function (e) {
		if (drag && e.pointerId === drag.id) letGo(e, false);
	});
	form.addEventListener("pointercancel", function (e) {
		if (drag && e.pointerId === drag.id) letGo(e, true);
	});
	document.addEventListener("keydown", function (e) {
		if (drag && drag.moved && e.key === "Escape") {
			e.preventDefault();
			letGo(e, true);
		}
	}, true);

	// hover offers a chord wherever the pointer is in the room above a line.
	function hover(e) {
		var b = wordsOf(e.target);
		if (!b || e.pointerType !== "mouse" || e.buttons) {
			hush();
			return;
		}
		var at = near(b, e.clientX, e.clientY);
		if (!at.above) {
			hush();
			return;
		}
		if (hovered !== b) hush();
		hovered = b;
		b.el.classList.add("reach");
		b.ghost.classList.add("on");
		b.ghost.style.left = Math.round(at.x) + "px";
		b.ghost.style.top = Math.round(at.top - b.ghost.offsetHeight) + "px";
		point(b, at);
	}

	function hush() {
		if (!hovered) return;
		hovered.el.classList.remove("reach");
		hovered.ghost.classList.remove("on");
		point(hovered, hovered.pointed);
		hovered = null;
	}
	document.documentElement.addEventListener("pointerleave", hush);

	// On a phone the chooser is a sheet over half the screen, and the way out
	// of it is a tap anywhere else. That tap is only that. A finger has nothing
	// to show it what it is about to press, half of every box of words is room
	// for a chord, and beside the words are the buttons that move a section or
	// remove it: none of them is what somebody closing a sheet meant. A pointer
	// is shown where its press will land, and gets what it was shown.
	document.addEventListener("pointerdown", function (e) {
		shut = !!editing && e.pointerType !== "mouse" && !pick.contains(e.target);
		// Nor is a press ever an Escape, whatever key came before it.
		if (editing) editing.dropped = false;
	}, true);
	document.addEventListener("pointercancel", function () {
		// It went on to scroll the page, and closed nothing.
		shut = false;
	}, true);
	document.addEventListener("mousedown", function (e) {
		// No caret either, which would bring the keyboard up.
		if (shut) e.preventDefault();
	}, true);
	document.addEventListener("click", function (e) {
		if (!shut) return;
		shut = false;
		// One made by a key is not that tap, and is let through.
		if (!e.detail) return;
		e.preventDefault();
		e.stopPropagation();
	}, true);

	// A press in the room above a line is for a chord. It must not move the
	// caret, nor call up a phone's keyboard.
	form.addEventListener("mousedown", function (e) {
		var b = wordsOf(e.target);
		armed = !shut && !!b && e.button === 0 && near(b, e.clientX, e.clientY).above;
		if (armed) e.preventDefault();
	});

	form.addEventListener("click", function (e) {
		var chip = chipOf(e.target);
		if (chip) {
			if (skip) {
				skip = false;
				if (e.detail) return;
			}
			// A click with no pointer behind it came from the keyboard.
			choose(chip.block, chip.mark, e.detail ? "chip" : "keys");
			return;
		}
		var b = wordsOf(e.target);
		if (!b || !armed) return;
		armed = false;
		var at = near(b, e.clientX, e.clientY);
		if (at.above) begin(b, at.pos, "lane", at.x);
	});

	// The line under a chord shows while the pointer or the keyboard is on it,
	// and the chord the keyboard is on is the one Tab comes back to.
	function mind(e, on) {
		var chip = chipOf(e.target);
		if (!chip || drag || editing) return;
		var b = chip.block;
		b.pointed = on ? chip.mark : null;
		point(b, b.pointed);
		if (!on || e.type !== "focusin") return;
		b.turn = chip.mark;
		b.marks.forEach(function (m) { if (m.el) m.el.tabIndex = m === b.turn ? 0 : -1; });
	}
	form.addEventListener("pointerover", function (e) { mind(e, true); });
	form.addEventListener("pointerout", function (e) { mind(e, false); });
	form.addEventListener("focusin", function (e) { mind(e, true); });
	form.addEventListener("focusout", function (e) { mind(e, false); });

	// ----------------------------------------------------------- keyboard ---

	// Before the editor's own keys, which are heard on the same form: on a
	// chord, Alt and an arrow move the chord and not the section it is in.
	form.addEventListener("keydown", function (e) {
		var chip = chipOf(e.target);
		var b = chip ? chip.block : wordsOf(e.target);
		if (!b) return;
		var key = e.key.length === 1 ? e.key.toLowerCase() : e.key;

		if ((e.ctrlKey || e.metaKey) && !e.altKey && (key === "z" || key === "y")) {
			e.preventDefault();
			e.stopPropagation();
			if (key === "y" || e.shiftKey) back(b, b.redo, b.undo);
			else back(b, b.undo, b.redo);
			return;
		}
		var step = key === "ArrowLeft" || key === "ArrowUp" ? -1 : 1;
		if (!chip) {
			// With the caret in the words, Alt and a sideways arrow are the
			// browser's way back and forward on most keyboards, one keystroke
			// from the keys that move a chord, and a song half written is not
			// to be left that way. Here they move the chord standing at the
			// caret, the one just put in if there are several, and nothing
			// when there is none.
			if (mac || !e.altKey || e.ctrlKey || e.metaKey || (key !== "ArrowLeft" && key !== "ArrowRight")) return;
			e.preventDefault();
			e.stopPropagation();
			var here = null;
			b.marks.forEach(function (mark) {
				if (mark.pos === b.box.selectionStart && (!here || mark.loose || !here.loose)) here = mark;
			});
			if (here) nudge(b, here, step, e.shiftKey);
			return;
		}
		if (e.ctrlKey || e.metaKey) return;

		var m = chip.mark;
		switch (key) {
		case "ArrowLeft":
		case "ArrowRight":
			if (e.altKey) {
				nudge(b, m, step, e.shiftKey);
			} else {
				var to = b.marks[b.marks.indexOf(m) + step];
				if (to) to.el.focus();
			}
			break;
		case "ArrowUp":
		case "ArrowDown":
			if (!e.altKey) return;
			hop(b, m, step);
			break;
		case "Delete":
		case "Backspace":
			remove(b, m);
			break;
		case "Escape":
			b.box.focus();
			b.box.setSelectionRange(m.pos, m.pos);
			break;
		default:
			// A letter typed on a chord starts its new name.
			if (e.altKey || e.key.length !== 1 || e.key === " ") return;
			choose(b, m, "keys");
			name.value = "";
			return;
		}
		e.preventDefault();
		e.stopPropagation();
	}, true);

	form.addEventListener("beforeinput", function (e) {
		var b = wordsOf(e.target);
		if (!b) return;
		if (e.inputType === "historyUndo" || e.inputType === "historyRedo") {
			e.preventDefault();
			if (e.inputType === "historyUndo") back(b, b.undo, b.redo);
			else back(b, b.redo, b.undo);
		} else if (e.inputType === "insertText" && e.data === "[") {
			e.preventDefault();
			bracket(b, b.box.selectionStart);
		}
	});

	// A letter put together from several keys is kept as one change when it
	// begins. Some browsers say it has ended before the last of it arrives, so
	// the end is believed a moment late.
	form.addEventListener("compositionstart", function (e) {
		var b = wordsOf(e.target);
		if (!b) return;
		remember(b, "edit");
		b.composing = true;
	});
	form.addEventListener("compositionend", function (e) {
		var b = wordsOf(e.target);
		if (b) setTimeout(function () { b.composing = false; }, 0);
	});

	// Words picked up and dropped elsewhere in their box are taken out and put
	// in, and a browser that only says so once both are done leaves no way to
	// tell where each chord went. They are moved by cutting and pasting, which
	// takes their chords along.
	form.addEventListener("dragstart", function (e) {
		if (wordsOf(e.target)) e.preventDefault();
	});

	form.addEventListener("input", function (e) {
		var b = wordsOf(e.target);
		if (b) {
			typed(b, e);
			return;
		}
		var block = e.target.matches(".tabtext") ? e.target.parentNode.block : null;
		if (block) {
			fit(block);
			touch(block.sec);
		}
	});

	// Where the caret is, kept as it moves: undoing goes back to it, the chord
	// that takes Tab is the one after it, and a chord waiting for a letter
	// stops waiting once the caret has left its side.
	["keyup", "click", "select", "focusin"].forEach(function (type) {
		form.addEventListener(type, function (e) {
			var b = wordsOf(e.target);
			if (!b) return;
			b.caret = b.box.selectionStart;
			var next = null;
			b.marks.forEach(function (m) {
				if (m.pos !== b.caret) m.loose = m.stray = false;
				if (!next && m.pos >= b.caret) next = m;
			});
			next = next || b.marks[b.marks.length - 1] || null;
			if (next !== b.turn) {
				b.turn = next;
				b.marks.forEach(function (m) { if (m.el) m.el.tabIndex = m === next ? 0 : -1; });
			}
		});
	});

	// ---------------------------------------------------------- clipboard ---

	// Words copied or cut out of a block take their chords with them, in a form
	// only this editor reads; anything else is given the words alone.
	function lift(e) {
		var b = wordsOf(e.target);
		if (!b || !e.clipboardData) return;
		var from = b.box.selectionStart, to = b.box.selectionEnd;
		var ends = to === b.text.length || b.text.charAt(to) === "\n";
		var taken = b.marks.filter(function (m) {
			return m.text && m.pos >= from && (m.pos < to || (ends && m.pos === to));
		});
		if (from === to || !taken.length) return;

		var text = b.text.slice(from, to);
		e.clipboardData.setData("text/plain", text);
		e.clipboardData.setData(CLIP, JSON.stringify({
			words: text,
			marks: taken.map(function (m) { return { at: m.pos - from, text: m.text, chord: m.chord }; })
		}));
		e.preventDefault();
		if (e.type !== "cut") return;
		remember(b, "edit");
		b.marks = b.marks.filter(function (m) { return taken.indexOf(m) < 0; });
		b.box.setRangeText("", from, to, "start");
		follow(b, from, to, "");
		settleWords(b);
	}
	form.addEventListener("copy", lift);
	form.addEventListener("cut", lift);

	form.addEventListener("paste", function (e) {
		var b = wordsOf(e.target);
		if (!b || !e.clipboardData) return;
		var clip;
		try {
			clip = JSON.parse(e.clipboardData.getData(CLIP));
		} catch (x) {
			return;
		}
		if (!clip || typeof clip.words !== "string" || !Array.isArray(clip.marks)) return;
		// Only if they are the words that are about to be pasted: something
		// copied since, somewhere that leaves this behind, is another matter.
		if (clip.words !== e.clipboardData.getData("text/plain").replace(/\r\n?/g, "\n")) return;
		e.preventDefault();
		var from = b.box.selectionStart, to = b.box.selectionEnd;
		remember(b, "edit");
		b.box.setRangeText(clip.words, from, to, "end");
		follow(b, from, to, clip.words);
		// Last first, so that those sharing a place end in the order they were in.
		clip.marks.slice().reverse().forEach(function (m) {
			if (typeof m.text !== "string" || !m.text.trim()) return;
			var mark = {
				pos: from + Math.max(0, Math.min(m.at | 0, clip.words.length)),
				text: m.text, chord: m.chord !== false, el: null
			};
			seat(b, mark, -Infinity);
			if (unread(mark)) read(b, mark);
		});
		settleWords(b);
	});

	// settleWords finishes a change to the words that was made here and not by
	// typing, so that nothing was heard of it.
	function settleWords(b) {
		b.text = b.box.value;
		b.caret = b.box.selectionStart;
		draw(b);
		touch(b.sec);
	}

	// --------------------------------------------------------- everything ---

	// A section is divided at a line counted the way this script counts them,
	// and the text a section arrived as may count otherwise: a fenced block
	// with a blank line at its end has one more. So a section about to be
	// divided is sent as it is held here, changed or not, and the server cuts
	// what was counted.
	function dividing(button) {
		if (!button || !button.hasAttribute || !button.hasAttribute("data-split")) return;
		sections.forEach(function (sec) {
			if (sec.el.contains(button)) touch(sec);
		});
	}
	form.addEventListener("submit", function (e) { dividing(e.submitter); });
	form.addEventListener("click", function (e) {
		dividing(e.target.closest ? e.target.closest("[data-split]") : null);
	}, true);

	// A block is drawn again when its size changes, since every letter in it may
	// have moved, and all of them when the window's does.
	var stale = new Set(), due = 0;
	function redraw(b) {
		stale.add(b);
		cancelAnimationFrame(due);
		due = requestAnimationFrame(function () {
			stale.forEach(function (block) {
				block.cells = [];
				draw(block);
			});
			stale.clear();
			settle();
		});
	}
	function everyBlock(fn) {
		sections.forEach(function (sec) {
			sec.blocks.forEach(function (b) { if (b.mirror) fn(b); });
		});
	}
	window.addEventListener("resize", function () { everyBlock(redraw); });
	window.addEventListener("scroll", settle, { passive: true });
	if (window.ResizeObserver) {
		var watch = new ResizeObserver(function (entries) {
			entries.forEach(function (entry) { redraw(entry.target.block); });
		});
		everyBlock(function (b) { watch.observe(b.el); });
	}

	sections.forEach(function (sec) {
		sec.blocks.forEach(function (b) { if (b.mirror) draw(b); });
		// The caret goes where the server asked for it, which was the box this
		// section arrived in.
		if (!sec.raw.autofocus) return;
		for (var i = 0; i < sec.blocks.length; i++) {
			if (sec.blocks[i].box) {
				sec.blocks[i].box.focus();
				return;
			}
		}
	});
})();

// Saying, as soon as files are chosen for import, which of them the collection
// already has and by whom, with a button that writes that artist in.
//
// Without a script the import itself leaves those songs out and the report
// says why; this only says so before anything is sent.
(function () {
	"use strict";

	var picker = document.querySelector(".filepick");
	var known = document.getElementById("importknown");
	var artist = document.getElementById("importartist");
	if (!picker || !known || !artist) return;

	picker.addEventListener("change", function () {
		var params = new URLSearchParams();
		Array.prototype.forEach.call(picker.files || [], function (file) {
			if (/\.(song|zip)$/i.test(file.name)) return;
			params.append("t", file.name.replace(/\.[^.]*$/, ""));
		});
		if (!params.toString()) {
			known.innerHTML = "";
			return;
		}
		fetch("/import/known?" + params.toString(), { headers: { "Accept": "text/html" } })
			.then(function (r) { return r.ok ? r.text() : ""; })
			.then(function (html) { known.innerHTML = html; })
			.catch(function () { known.innerHTML = ""; });
	});

	known.addEventListener("click", function (e) {
		var button = e.target.closest("[data-artist]");
		if (!button) return;
		artist.value = button.getAttribute("data-artist");
		artist.focus();
	});
})();

/* The login box.
 *
 * Same shape as the chord popover: it anchors on one element, feature-detects the
 * Popover API, and returns early if either is missing. Without it the button
 * beside the box is an ordinary link to /login, which carries the same form. */
(function () {
	"use strict";

	var pop = document.getElementById("loginpop");
	if (!pop || typeof pop.showPopover !== "function") return;

	var opener = null;
	var narrow = window.matchMedia("(max-width: 34rem)");

	/* Under the button and aligned with its right edge, which is where the eye
	 * already is. On a phone the stylesheet turns the box into a sheet at the
	 * bottom, so leave it alone there. */
	function place() {
		if (!opener || narrow.matches) return;
		var box = opener.getBoundingClientRect();
		var width = pop.offsetWidth;
		var left = Math.min(box.right - width, window.innerWidth - width - 8);
		pop.style.left = Math.max(8, left) + "px";
		pop.style.top = box.bottom + 6 + "px";
	}

	document.addEventListener("click", function (e) {
		if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
		var link = e.target.closest("a[data-login]");
		if (!link || link.getAttribute("data-login") !== "loginpop") return;

		e.preventDefault();
		opener = link;
		pop.showPopover();
		place();
		var first = pop.querySelector("input[name=user]");
		if (first) first.focus();
	});

	window.addEventListener("resize", function () {
		if (pop.matches(":popover-open")) place();
	});
})();

/* Marking every song in the songbook, or none.
 *
 * Ticking boxes by hand works without this; it only saves the sixtieth click. */
(function () {
	"use strict";

	var form = document.getElementById("songbookform");
	if (!form) return;

	document.addEventListener("click", function (e) {
		var button = e.target.closest("[data-pick]");
		if (!button || !form.contains(button)) return;

		var want = button.getAttribute("data-pick") === "all";
		var boxes = form.querySelectorAll('input[type="checkbox"][name="s"]');
		for (var i = 0; i < boxes.length; i++) boxes[i].checked = want;
	});
})();

/* Closing a notice.
 *
 * The message is one-shot anyway — the server clears it as it renders — so
 * without scripting it simply goes away on the next page. This is for the reader
 * who wants it gone now. */
(function () {
	"use strict";

	document.addEventListener("click", function (e) {
		var button = e.target.closest("[data-dismiss]");
		if (!button) return;
		var notice = button.closest(".flash");
		if (notice) notice.remove();
	});
})();

/* Printing the songbook.
 *
 * The keyboard shortcut does the same thing; this is a visible way to reach it,
 * and on every browser worth naming "print" includes saving as a PDF. */
(function () {
	"use strict";

	document.addEventListener("click", function (e) {
		if (!e.target.closest("[data-print]")) return;
		window.print();
	});
})();

/* The chord page.
 *
 * Everything here works without it: the neck is a GET form, the chooser is a
 * GET form, and every shape is a link. What the script adds is that none of
 * that navigates. It fetches the same region the server renders and swaps it
 * in, so trying chords is a continuous thing rather than a series of pages, and
 * it leaves the address bar alone — the URL a reader arrived on is still the one
 * they can share, and it does not turn into a log of every dot they tried. */
(function () {
	"use strict";

	var lab = document.getElementById("chordlab");
	if (!lab || !window.fetch || !window.AbortController) return;

	/* Some controls only exist for the reader who has no script: a button to
	 * identify what is already identified, a button to apply a choice already
	 * applied. Saying so here rather than leaving them out of the markup is what
	 * keeps the page whole without one. The class outlives every swap, because
	 * only the contents of #chordlab are replaced. */
	lab.classList.add("live");

	var pending = null;
	var timer = null;
	/* What each string was set to before the current click, which is the only way
	 * to know that a radio was clicked while already chosen. */
	var was = {};

	function board() { return document.getElementById("fretboard"); }

	function remember() {
		was = {};
		var b = board();
		if (!b) return;
		b.querySelectorAll(".boardcell input:checked").forEach(function (el) {
			was[el.name] = el.value;
		});
	}
	remember();

	function fromBoard(over) {
		var b = board();
		var p = b ? new URLSearchParams(new FormData(b)) : new URLSearchParams();
		for (var k in over) {
			if (over[k] === null) p.delete(k);
			else p.set(k, over[k]);
		}
		return p;
	}

	/* A chord chosen by name arrives with no shape, which is what tells the
	 * server to put the chord's easiest shape on the neck. */
	function forChord(symbol) {
		var p = new URLSearchParams();
		p.set("c", symbol);
		var b = board();
		if (b && b.elements.all && b.elements.all.value) p.set("all", "1");
		return p;
	}

	function show(params, focus) {
		if (pending) pending.abort();
		pending = new AbortController();
		fetch("/chords/fragment?" + params.toString(), {
			signal: pending.signal,
			headers: { "Accept": "text/html" },
		})
			.then(function (r) { return r.ok ? r.text() : Promise.reject(r.status); })
			.then(function (html) {
				lab.innerHTML = html;
				remember();
				/* Everything in here is new, including any control another
				 * block is keeping state for. Saying so once is cheaper than
				 * each of them watching the document for changes. */
				document.dispatchEvent(new CustomEvent("labswapped"));
				if (focus) {
					var el = lab.querySelector(focus);
					if (el) el.focus();
				}
			})
			/* A failed refresh leaves the last good answer on screen, and every
			 * control is still a link or a form that works on its own. */
			.catch(function () {});
	}

	function later(params, focus) {
		clearTimeout(timer);
		/* Long enough that running down a string is one request rather than five,
		 * short enough to feel like the page is keeping up. */
		timer = setTimeout(function () { show(params, focus); }, 120);
	}

	function selector(input) {
		return '.boardcell input[name="' + input.name + '"][value="' + input.value + '"]';
	}

	lab.addEventListener("click", function (e) {
		var cell = e.target.closest(".boardcell input");
		if (cell) {
			/* Pressing the dot that is already there takes the finger off, which
			 * leaves the string ringing open. Only the × silences it: a string
			 * you have stopped holding is not a string you have muted. */
			if (cell.closest(".boardcell").classList.contains("fret") &&
				was[cell.name] === cell.value) {
				var open = board().querySelector(
					'.boardcell.open input[name="' + cell.name + '"]');
				if (open) open.checked = true;
			}
			was[cell.name] = board().querySelector(
				'.boardcell input[name="' + cell.name + '"]:checked').value;
			later(fromBoard({}), selector(cell));
			return;
		}

		var move = e.target.closest(".boardmove[data-start]");
		if (move) {
			e.preventDefault();
			show(fromBoard({ start: move.dataset.start }),
				'.boardmove[data-start="' + move.dataset.start + '"]');
			return;
		}

		var shape = e.target.closest("[data-grip]");
		if (shape) {
			e.preventDefault();
			var p = fromBoard({ start: shape.dataset.start });
			/* The shape's own grip replaces what is on the neck, so the neck's
			 * strings must not also be sent: the server prefers them. */
			for (var i = 0; i < 8; i++) p.delete("s" + i);
			p.set("grip", shape.dataset.grip);
			/* A near miss carries a chord as well as a shape: it is both the
			 * answer and the way to hold it. */
			if (shape.dataset.c) p.set("c", shape.dataset.c);
			show(p, shape.classList.contains("labshape") ? ".labshape.on" : ".boardclear");
			return;
		}

		/* One of the other names for what is drawn. The neck does not change —
		 * the notes are the same notes — so only the chord travels, and the
		 * shape on the board goes with the request untouched. */
		var reading = e.target.closest(".readings [data-c]");
		if (reading) {
			e.preventDefault();
			show(fromBoard({ c: reading.dataset.c }), ".readings a.on");
			return;
		}

		var pick = e.target.closest("[data-c]");
		if (pick) {
			e.preventDefault();
			show(forChord(pick.dataset.c), "#csym");
			return;
		}

		var more = e.target.closest("[data-more]");
		if (more) {
			e.preventDefault();
			show(fromBoard({ all: "1" }), ".labmore a");
		}
	});

	/* Arrow keys move between radios without a click, so the neck listens for
	 * both. The two agree because they ask for the same thing. */
	lab.addEventListener("change", function (e) {
		var input = e.target.closest(".boardcell input");
		if (!input) return;
		was[input.name] = input.value;
		later(fromBoard({}), selector(input));
	});

	lab.addEventListener("submit", function (e) {
		var form = e.target.closest("form");
		if (!form) return;
		e.preventDefault();
		if (form.id === "fretboard") {
			show(fromBoard({}), null);
			return;
		}
		var field = form.querySelector("[name=c]");
		show(forChord(field ? field.value : ""), "#csym");
	});

	lab.addEventListener("change", function (e) {
		if (e.target.id !== "cqual") return;
		show(forChord(e.target.value), "#cqual");
	});
})();

/* Play mode.
 *
 * The one thing on the sheet that scripting has to do rather than merely make
 * nicer: a page cannot scroll itself without it. So the button that starts it
 * is hidden until this runs, and hiding it is this code's first act — a control
 * that does nothing is worse than one that is not there.
 *
 * There is no URL state and nothing is remembered. Play mode is what you are
 * doing right now, not a way the song is; the way a song is belongs in the
 * .song file, which is where a per-song speed would go if it is ever wanted. */
(function () {
	"use strict";

	var bar = document.getElementById("playbar");
	if (!bar || !document.getElementById("playstart")) return;
	if (typeof window.requestAnimationFrame !== "function") return;

	/* Transposing replaces the whole of #controls with a freshly rendered one,
	 * and the button lives inside it. A listener held on the element found at
	 * load would be left attached to a node no longer on the page, so the
	 * button would stop working the first time anybody changed key. Hence the
	 * lookup happens every time, and the click is delegated. */
	function startButton() { return document.getElementById("playstart"); }

	/* Pixels a second, one entry per notch. Slow enough at the bottom to hold a
	 * long verse on screen, fast enough at the top for a sheet you already know.
	 * Steps rather than a slider because this is operated with one finger,
	 * looking somewhere else. */
	var speeds = [8, 14, 22, 32, 46, 64, 88, 120];
	var notch = 2;
	var sizes = [1.125, 1.3125, 1.5, 1.75, 2, 2.375];
	var size = 2;

	var running = false;
	var last = 0;
	var carry = 0;
	var frame = 0;
	var lock = null;

	var toggle = bar.querySelector('[data-play="toggle"]');
	var readout = bar.querySelector(".playspeed");

	function show() {
		readout.textContent = String(notch + 1);
		toggle.textContent = running ? "⏸" : "▶";
		toggle.setAttribute("aria-label",
			toggle.getAttribute(running ? "data-label-pause" : "data-label-start"));
	}

	/* Fractions of a pixel are kept in carry rather than thrown away, or the
	 * slowest speeds would round to nothing every frame and never move. */
	function step(now) {
		if (!running) return;
		if (last) {
			carry += speeds[notch] * (now - last) / 1000;
			var whole = Math.floor(carry);
			if (whole > 0) {
				carry -= whole;
				var before = window.scrollY;
				window.scrollBy(0, whole);
				if (window.scrollY === before) return pause();  // the foot of the page
			}
		}
		last = now;
		frame = window.requestAnimationFrame(step);
	}

	/* The screen going dark halfway through a verse is the whole reason this
	 * mode exists, so ask for it to stay on. Not every browser has this, and a
	 * refusal is not worth mentioning: the mode still works. */
	function keepAwake() {
		if (!navigator.wakeLock || lock) return;
		navigator.wakeLock.request("screen").then(function (got) {
			lock = got;
			got.addEventListener("release", function () { lock = null; });
		}).catch(function () {});
	}

	function play() {
		if (running) return;
		running = true;
		last = 0;
		carry = 0;
		frame = window.requestAnimationFrame(step);
		keepAwake();
		show();
	}

	function pause() {
		running = false;
		if (frame) window.cancelAnimationFrame(frame);
		frame = 0;
		show();
	}

	function enter() {
		document.body.classList.add("playing");
		bar.hidden = false;
		play();
		toggle.focus();
	}

	function leave() {
		pause();
		document.body.classList.remove("playing");
		bar.hidden = true;
		if (lock) { lock.release(); lock = null; }
		var button = startButton();
		if (button) button.focus();
	}

	function resize(by) {
		size = Math.min(sizes.length - 1, Math.max(0, size + by));
		document.body.style.setProperty("--play-size", sizes[size] + "rem");
	}

	document.addEventListener("click", function (e) {
		if (e.target.closest("#playstart")) enter();
	});

	bar.addEventListener("click", function (e) {
		var button = e.target.closest("[data-play]");
		if (!button) return;
		switch (button.getAttribute("data-play")) {
		case "toggle":  running ? pause() : play(); break;
		case "slower":  notch = Math.max(0, notch - 1); show(); break;
		case "faster":  notch = Math.min(speeds.length - 1, notch + 1); show(); break;
		case "smaller": resize(-1); break;
		case "bigger":  resize(1); break;
		case "exit":    leave(); break;
		}
	});

	/* Reaching for the page to look ahead should not be a fight with the
	 * scrolling. Touching it pauses; the bar starts it again. */
	function interrupted() { if (running) pause(); }
	window.addEventListener("wheel", interrupted, { passive: true });
	window.addEventListener("touchstart", interrupted, { passive: true });

	document.addEventListener("keydown", function (e) {
		if (!document.body.classList.contains("playing")) return;
		if (e.target.closest("input, textarea, select")) return;
		switch (e.key) {
		case " ":         running ? pause() : play(); break;
		case "Escape":    leave(); break;
		case "ArrowUp":   notch = Math.min(speeds.length - 1, notch + 1); show(); break;
		case "ArrowDown": notch = Math.max(0, notch - 1); show(); break;
		default: return;
		}
		e.preventDefault();
	});

	/* A wake lock is dropped when the tab is hidden and is not given back on its
	 * own, so ask again on the way back. */
	document.addEventListener("visibilitychange", function () {
		if (!document.hidden && running) keepAwake();
	});
})();

/* Hearing the shape on the neck.
 *
 * The notes arrive from the server on the button, as MIDI numbers, because
 * which pitch a fret on a string makes is music and this file decides no music.
 * All that happens here is turning numbers into air.
 *
 * A string is Karplus-Strong: fill a ring buffer one wavelength long with
 * noise, then read it round and round, blending each sample with the next. The
 * blend is a low-pass filter, so the high partials die before the low ones and
 * a plucked string comes out rather than a beep.
 *
 * A string on its own still sounds like a synthesiser, and two things are why.
 * Half of a guitar is the box, so the string is followed by the resonances of
 * one — the air inside, the top plate — and those are most of what makes one
 * guitar recognisably not another. And a pluck is not only a string: there is
 * the sound of a nail or a pick crossing it, which arrives before the note and
 * is a large part of hearing a guitar rather than a tone. */
(function () {
	"use strict";

	var Audio = window.AudioContext || window.webkitAudioContext;
	if (!Audio) {
		/* No sound card, no sound controls. They are only ever visible because
		 * the script said so, and the script now knows better. */
		var dead = document.querySelectorAll("[data-sound], .tonepick");
		for (var i = 0; i < dead.length; i++) dead[i].remove();
		return;
	}

	/* burst is the cutoff of the filter over the pluck itself, in hertz, and
	 * carries most of the difference between nylon and steel: the loop filter
	 * alone barely tells them apart over the few turns that matter.
	 *
	 * damp is how much of its neighbour each sample takes on the way round the
	 * ring, and fall the loss per second on top of that. body is the box, as
	 * [hertz, Q, decibels], tilt is where the top is rolled off, and pick is how
	 * loud the nail is against the note. */
	var packs = {
		nylon: {
			burst: 900, damp: 0.50, fall: 2.6, seconds: 2.4, level: 0.26, pick: 0.05,
			/* A classical guitar's air resonance sits low and its top plate
			 * just above; both are strong, which is the warmth. */
			body: [[105, 3.0, 5], [215, 2.5, 3.5], [400, 2.0, 2]],
			tilt: 2400
		},
		steel: {
			burst: 4200, damp: 0.40, fall: 4.6, seconds: 2.8, level: 0.22, pick: 0.11,
			/* A steel-string is louder and brighter: the same two resonances
			 * sit a little higher, and there is a presence peak that is why a
			 * strummed dreadnought cuts through a room. */
			body: [[100, 3.0, 4], [250, 2.0, 3], [2600, 1.0, 4]],
			tilt: 6500
		}
	};
	/* The electric is the steel guitar with an amplifier after it. */
	var stringOf = { nylon: "nylon", steel: "steel", electric: "steel" };
	var drive = 5;

	var tone = "steel";
	var ctx = null;
	var plucks = {};

	function pluck(name, midi) {
		var key = name + ":" + midi;
		if (plucks[key]) return plucks[key];
		var t = packs[name];

		var freq = 440 * Math.pow(2, (midi - 69) / 12);
		var buf = ctx.createBuffer(1, Math.ceil(ctx.sampleRate * t.seconds), ctx.sampleRate);
		var out = buf.getChannelData(0);

		/* A ring of whole samples can only play the notes whose wavelength
		 * happens to be a whole number of them, and everything else comes out
		 * as much as five cents off — which across six strings is a chord that
		 * beats against itself. The blend already delays the loop by damp
		 * samples; an allpass supplies whatever fraction is still missing. */
		var wanted = ctx.sampleRate / freq;
		var n = Math.max(2, Math.floor(wanted - t.damp));
		var missing = wanted - n - t.damp;
		var apC = (1 - missing) / (1 + missing);

		/* The ring is read once per period, not once per sample, so a fixed
		 * loop gain would silence a low string in a breath and let a high one
		 * ring for a minute. This gain evens that out, losing the same amount
		 * per second whatever the pitch. It sets the floor and not the whole
		 * decay: the blend throws away energy of its own, and that is what
		 * mostly decides how long a note lasts. */
		var gain = Math.pow(10, -3 / (t.fall * freq));

		/* Raw white noise starts at full scale on the first sample and arrives
		 * as a click. Two passes round the ring, carrying the filter's state
		 * between them so the burst has no seam where it wraps. */
		var line = new Float32Array(n);
		for (var i = 0; i < n; i++) line[i] = Math.random() * 2 - 1;
		var a = Math.exp((-2 * Math.PI * t.burst) / ctx.sampleRate);
		var prev = 0;
		for (var pass = 0; pass < 2; pass++) {
			for (var i = 0; i < n; i++) {
				prev = (1 - a) * line[i] + a * prev;
				line[i] = prev;
			}
		}
		/* Filtering it took the level with it, and a quiet string is not the
		 * point of a dark one. */
		var loudest = 0;
		for (var i = 0; i < n; i++) loudest = Math.max(loudest, Math.abs(line[i]));
		if (loudest > 0) for (var i = 0; i < n; i++) line[i] /= loudest;

		var p = 0;
		var apIn = 0;
		var apOut = 0;
		for (var j = 0; j < out.length; j++) {
			var here = line[p];
			var next = line[(p + 1) % n];
			out[j] = here;
			var blended = (here * (1 - t.damp) + next * t.damp) * gain;
			apOut = apC * (blended - apOut) + apIn;
			apIn = blended;
			line[p] = apOut;
			p = (p + 1) % n;
		}

		/* The nail or the pick crossing the string, which happens before the
		 * note and is a large part of hearing a guitar rather than a tone. It
		 * is not part of the string, so it is not in the loop: a few
		 * milliseconds of noise on top, gone before the note has settled. */
		var attack = Math.floor(ctx.sampleRate * 0.012);
		var edge = 0;
		for (var k = 0; k < attack && k < out.length; k++) {
			var white = Math.random() * 2 - 1;
			/* Bright: the sound is the crossing, not the string. */
			edge = 0.6 * white + 0.4 * edge;
			out[k] += edge * t.pick * (1 - k / attack);
		}

		/* Whatever is left when the buffer runs out would end on a step, and a
		 * step is a click. The last moments are faded instead. */
		var fadeAt = out.length - Math.floor(ctx.sampleRate * 0.08);
		for (var m = Math.max(0, fadeAt); m < out.length; m++) {
			out[m] *= (out.length - m) / (out.length - fadeAt);
		}

		plucks[key] = buf;
		return buf;
	}

	/* A soft clip. Distortion is not the strings being loud, it is the
	 * amplifier refusing to go any louder and folding the tops over, which is
	 * what adds the harmonics a clean string does not have. */
	function clipping(amount) {
		var n = 1024;
		var curve = new Float32Array(n);
		for (var i = 0; i < n; i++) {
			var x = (i * 2) / n - 1;
			curve[i] = Math.tanh(x * amount);
		}
		return curve;
	}

	function link(nodes) {
		for (var i = 0; i < nodes.length - 1; i++) nodes[i].connect(nodes[i + 1]);
		return nodes[0];
	}

	/* Where the strings are plugged in: the box first, then an amplifier if one
	 * was asked for. */
	function instrument(name) {
		var t = packs[stringOf[name]];
		var chain = [];

		var input = ctx.createGain();
		chain.push(input);

		for (var i = 0; i < t.body.length; i++) {
			var peak = ctx.createBiquadFilter();
			peak.type = "peaking";
			peak.frequency.value = t.body[i][0];
			peak.Q.value = t.body[i][1];
			peak.gain.value = t.body[i][2];
			chain.push(peak);
		}

		var top = ctx.createBiquadFilter();
		top.type = "lowpass";
		top.frequency.value = t.tilt;
		chain.push(top);

		if (name === "electric") {
			/* Lose the deep bass first or six distorted strings turn to mud. */
			var low = ctx.createBiquadFilter();
			low.type = "highpass";
			low.frequency.value = 130;

			var pre = ctx.createGain();
			pre.gain.value = 1 + drive * 1.6;

			var clip = ctx.createWaveShaper();
			clip.curve = clipping(1 + drive * 2);
			clip.oversample = "4x";

			/* A speaker cabinet, which is why an amplifier sounds like music
			 * and a clipped signal on its own sounds like a fault. */
			var cabinet = ctx.createBiquadFilter();
			cabinet.type = "lowpass";
			cabinet.frequency.value = 3200;

			/* Clipping raises the average level a long way; this puts it back. */
			var back = ctx.createGain();
			back.gain.value = 0.5 / Math.sqrt(1 + drive);

			chain.push(low, pre, clip, cabinet, back);
		}

		chain.push(ctx.destination);
		link(chain);
		return input;
	}

	function strum(midis) {
		if (!ctx) ctx = new Audio();
		/* A browser may hand back a context that is asleep until a gesture
		 * resumes it, and this is one. */
		if (ctx.state === "suspended") ctx.resume();

		var t = packs[stringOf[tone]];
		var into = instrument(tone);
		var now = ctx.currentTime + 0.02;
		for (var i = 0; i < midis.length; i++) {
			var src = ctx.createBufferSource();
			src.buffer = pluck(stringOf[tone], midis[i]);
			var gain = ctx.createGain();
			/* Quieter as the strings get higher, which is how a guitar sounds
			 * and what stops six of them at once from clipping the output. */
			gain.gain.value = t.level - i * (t.level * 0.07);
			src.connect(gain);
			gain.connect(into);
			/* 35ms apart, low to high: a strum rather than a piano chord. */
			src.start(now + i * 0.035);
		}
	}

	function notes(button) {
		var raw = (button.getAttribute("data-sound") || "").split(",");
		var out = [];
		for (var i = 0; i < raw.length; i++) {
			var n = parseInt(raw[i], 10);
			if (n >= 0 && n <= 127) out.push(n);
		}
		return out;
	}

	function playDrawn() {
		var button = document.querySelector("[data-sound]");
		if (button && notes(button).length) strum(notes(button));
	}

	/* The chooser lives inside the region the neck's own script replaces on
	 * every edit, so its state is held here and put back afterwards rather than
	 * read off the page. */
	function restore() {
		var pick = document.querySelector(".tonepick");
		if (pick) pick.value = tone;
	}
	restore();
	document.addEventListener("labswapped", restore);

	document.addEventListener("change", function (e) {
		if (!e.target.closest(".tonepick")) return;
		tone = e.target.value;
		/* Choosing a guitar plays the chord on it, which is the only way to
		 * compare two of them without reaching for another control. */
		playDrawn();
	});

	/* Delegated, and the notes are read at the moment of the press: editing the
	 * neck replaces the button with a freshly rendered one carrying whatever is
	 * drawn now. */
	document.addEventListener("click", function (e) {
		if (e.target.closest("[data-sound]")) playDrawn();
	});
})();
