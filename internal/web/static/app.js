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

// Loading a file into the editor without a round trip.
//
// The form behind this posts the file and the server sends the editor back with
// the text in it, which is what happens with scripting off. When scripting is
// available there is no reason to make the file travel: it is already on this
// machine, so it is read here and dropped straight into the box.
//
// Nothing is saved either way. The reader still has to press Guardar, which is
// the point — a file whose contents you never saw before it landed is how a
// collection fills up with things nobody meant to add.
(function () {
	"use strict";

	var input = document.getElementById("loadfile");
	var box = document.getElementById("songtext");
	if (!input || !box || typeof FileReader === "undefined") return;

	var bar = input.closest("form");
	if (!bar) return;

	function note(text) {
		var el = bar.querySelector(".quiet");
		if (!el) {
			el = document.createElement("span");
			el.className = "quiet";
			var sep = bar.querySelector(".sep");
			bar.insertBefore(el, sep || null);
		}
		el.textContent = text;
	}

	input.addEventListener("change", function () {
		var file = input.files && input.files[0];
		if (!file) return;

		// Well past any real song, and small enough that reading it cannot
		// become the problem.
		if (file.size > 4 * 1024 * 1024) {
			note("Ese archivo es demasiado grande.");
			return;
		}

		var reader = new FileReader();
		reader.onload = function () {
			var text = String(reader.result || "").replace(/\r\n?/g, "\n");
			if (box.value.trim() !== "" && !window.confirm(
				"Ya hay texto en el editor. ¿Reemplazarlo con el archivo?")) {
				return;
			}
			box.value = text;
			note("Cargado: " + file.name);
			box.focus();
			box.setSelectionRange(0, 0);
			box.scrollTop = 0;
		};
		reader.onerror = function () {
			// Fall back to the form, which does the same thing server-side.
			bar.submit();
		};
		reader.readAsText(file, "utf-8");
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
 * The sound is Karplus-Strong: fill a short ring buffer the length of one
 * wavelength with noise, then read it round and round, blending each sample
 * with the next. The blend is a low-pass filter, so the high partials die
 * before the low ones and what comes out is a plucked string rather than a beep
 * — which matters, because the point is to recognise the chord you drew.
 *
 * Three guitars come out of four numbers each, because that is genuinely what
 * separates them: how fast the highs die, how long the note lasts, how sharp
 * the pluck is, and whether the whole thing then goes through an amplifier. */
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

	/* damp is how much of each sample's neighbour is mixed in: 0.5 is the
	 * darkest the filter goes, and less keeps the highs alive longer. fall is
	 * the loss per second on top of that. smooth is how many times the initial
	 * burst is rounded off before it is plucked, which is the attack. */
	/* Four numbers make a guitar, because these are what genuinely separate
	 * them: how bright the pluck is, how fast the highs die after it, how long
	 * the note lasts, and what it is played through.
	 *
	 * burst and tone are cutoffs in hertz — the first shapes the pluck itself,
	 * the second stands in for a body, a pickup or a speaker. damp is how much
	 * of its neighbour each sample takes on the way round the ring, and fall is
	 * the loss per second on top of that. */
	var tones = {
		/* Nylon: a soft pluck, a dark body, and it stops rather than rings. */
		nylon: { burst: 1100, tone: 1800, damp: 0.50, fall: 2.2, level: 0.26, seconds: 2.0, drive: 0 },
		/* Steel: a bright pluck that carries, which is most of the difference. */
		steel: { burst: 4000, tone: 5000, damp: 0.40, fall: 4.0, level: 0.22, seconds: 2.4, drive: 0 },
		/* An electric barely damps itself — the sustain is the instrument — and
		 * then it goes through a distorted amplifier, which is the rest of it. */
		electric: { burst: 2600, tone: 3200, damp: 0.22, fall: 16, level: 0.09, seconds: 3.2, drive: 8 }
	};
	var tone = "steel";

	var ctx = null;
	var plucks = {};

	function pluck(name, midi) {
		var key = name + ":" + midi;
		if (plucks[key]) return plucks[key];
		var t = tones[name];

		var freq = 440 * Math.pow(2, (midi - 69) / 12);
		var n = Math.max(2, Math.round(ctx.sampleRate / freq));
		var buf = ctx.createBuffer(1, Math.ceil(ctx.sampleRate * t.seconds), ctx.sampleRate);
		var out = buf.getChannelData(0);

		/* The ring is read once per period, not once per sample, so a fixed
		 * loop gain would silence a low string in a breath and let a high one
		 * ring for a minute. This gain evens that out, losing the same amount
		 * per second whatever the pitch.
		 *
		 * It sets the floor and not the whole decay: the blend throws away
		 * energy of its own, and for the two acoustic guitars that is what
		 * mostly decides how long a note lasts. */
		var gain = Math.pow(10, -3 / (t.fall * freq));

		/* Raw white noise starts at full scale on the first sample and arrives
		 * as a click. Rounding it off leaves a pluck, and how far it is rounded
		 * off is most of what separates a nylon string from a steel one: the
		 * loop filter alone barely tells them apart, because over the few turns
		 * that matter it treats them almost the same.
		 *
		 * Two passes round the ring, carrying the filter's state between them,
		 * so the burst has no seam where it wraps. */
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
		for (var j = 0; j < out.length; j++) {
			out[j] = line[p];
			var next = (p + 1) % n;
			line[p] = (line[p] * (1 - t.damp) + line[next] * t.damp) * gain;
			p = next;
		}

		/* Whatever is left when the buffer runs out would end on a step, and a
		 * step is a click. The last moments are faded instead — which the
		 * electric needs most, since it is still going. */
		var fadeAt = out.length - Math.floor(ctx.sampleRate * 0.08);
		for (var k = Math.max(0, fadeAt); k < out.length; k++) {
			out[k] *= (out.length - k) / (out.length - fadeAt);
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

	/* Where the strings are plugged in. Clean, that is one gain node; distorted,
	 * it is a small amplifier: lose the deep bass first or six strings turn to
	 * mud, drive it hard into the clip, then take the fizz off the top the way
	 * a speaker cabinet does. */
	function amplifier(name) {
		var t = tones[name];
		var input = ctx.createGain();
		if (!t.drive) {
			/* A body and a soundboard, standing in for everything that is not
			 * the string. Without it two clean guitars differ by less than
			 * they should. */
			var body = ctx.createBiquadFilter();
			body.type = "lowpass";
			body.frequency.value = t.tone;
			input.connect(body);
			body.connect(ctx.destination);
			return input;
		}
		var low = ctx.createBiquadFilter();
		low.type = "highpass";
		low.frequency.value = 130;

		var pre = ctx.createGain();
		pre.gain.value = 6;

		var clip = ctx.createWaveShaper();
		clip.curve = clipping(t.drive);
		clip.oversample = "4x";

		var cabinet = ctx.createBiquadFilter();
		cabinet.type = "lowpass";
		cabinet.frequency.value = t.tone;

		var out = ctx.createGain();
		out.gain.value = 0.42;

		input.connect(low);
		low.connect(pre);
		pre.connect(clip);
		clip.connect(cabinet);
		cabinet.connect(out);
		out.connect(ctx.destination);
		return input;
	}

	function strum(midis) {
		if (!ctx) ctx = new Audio();
		/* A browser may hand back a context that is asleep until a gesture
		 * resumes it, and this is one. */
		if (ctx.state === "suspended") ctx.resume();

		var t = tones[tone];
		var amp = amplifier(tone);
		var now = ctx.currentTime + 0.02;
		for (var i = 0; i < midis.length; i++) {
			var src = ctx.createBufferSource();
			src.buffer = pluck(tone, midis[i]);
			var gain = ctx.createGain();
			/* Quieter as the strings get higher, which is how a guitar sounds
			 * and what stops six of them at once from clipping the output. */
			gain.gain.value = t.level - i * (t.level * 0.07);
			src.connect(gain);
			gain.connect(amp);
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

	/* Delegated, and the state is read at the moment of the press: editing the
	 * neck replaces the button with a freshly rendered one carrying the notes
	 * that are now drawn. The row of guitars is outside that region and is not
	 * replaced, so the choice survives on its own. */
	document.addEventListener("click", function (e) {
		var pick = e.target.closest("[data-tone]");
		if (pick) {
			tone = pick.getAttribute("data-tone");
			var all = document.querySelectorAll("[data-tone]");
			for (var i = 0; i < all.length; i++) {
				var on = all[i] === pick;
				all[i].classList.toggle("on", on);
				all[i].setAttribute("aria-pressed", on ? "true" : "false");
			}
			/* Pressing a guitar plays the chord on it, which is the only way to
			 * compare two of them without hunting for the other button. */
			var near = document.querySelector("[data-sound]");
			if (near && notes(near).length) strum(notes(near));
			return;
		}

		var button = e.target.closest("[data-sound]");
		if (!button) return;
		var midis = notes(button);
		if (midis.length) strum(midis);
	});
})();
