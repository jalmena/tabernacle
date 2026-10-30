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
		bar.submit();
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
		if (!e.target.matches || !e.target.matches(".sectionbody")) return;
		current = box;
		if (at) at.value = lineAt(e.target);
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
			if (box && target.matches(".sectionbody")) {
				if (at) at.value = lineAt(target);
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
