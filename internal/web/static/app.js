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
