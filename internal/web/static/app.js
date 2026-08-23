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
 * The neck is a plain GET form and works without any of this: pressing
 * Identificar reloads the page with the shape in the URL. What the script adds
 * is that moving a finger answers immediately, by fetching the part of the page
 * that depends on the shape and swapping it in.
 *
 * The neck itself is never replaced, which is what keeps the keyboard focus on
 * the radio button the reader just moved to. */
(function () {
	"use strict";

	var board = document.getElementById("fretboard");
	var result = document.getElementById("chordresult");
	if (!board || !result || !window.fetch || !window.history) return;

	var timer = null;
	var pending = null;

	function state() {
		var params = new URLSearchParams(new FormData(board));
		return params.toString();
	}

	function refresh() {
		var query = state();
		if (pending) pending.abort();
		pending = new AbortController();
		fetch("/chords/fragment?" + query, {
			signal: pending.signal,
			headers: { "Accept": "text/html" },
		})
			.then(function (r) { return r.ok ? r.text() : Promise.reject(r.status); })
			.then(function (html) {
				result.innerHTML = html;
				history.replaceState(null, "", "/chords?" + query);
			})
			/* A failed refresh leaves the last good answer on screen. The form
			 * still submits the ordinary way, so nothing is unreachable. */
			.catch(function () {});
	}

	board.addEventListener("change", function (e) {
		if (!e.target.matches(".boardcell input")) return;
		clearTimeout(timer);
		/* Long enough that dragging across a string is one request rather than
		 * six, short enough to feel like the page is keeping up. */
		timer = setTimeout(refresh, 120);
	});
})();
