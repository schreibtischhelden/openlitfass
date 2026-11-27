/*!
 * markdown-pages.js - simple Markdown-based static sites, without the generator
 *
 * Loads README.md (or the page selected by the query string), header.md and
 * footer.md, converts them with Showdown and sanitizes the result with
 * DOMPurify before it is inserted into <main>, <header> and <footer>.
 *
 * After each region is rendered a "mdp:rendered" event is dispatched on
 * document (detail: { region: "main" | "header" | "footer", page: "..." }).
 */
(function (global) {
	'use strict';

	var MarkdownPages = {};

	// ---------------------------------------------------------------------
	// Page resolution (pure, unit tested)
	// ---------------------------------------------------------------------

	// one path segment: letters, digits, "_", "-", "." - no leading dot,
	// so "..", ".git" etc. are rejected
	var SEGMENT = /^[A-Za-z0-9_-][A-Za-z0-9_.-]*$/;

	function isSafePath(path) {
		if (typeof path !== 'string' || path.length === 0 || path.length > 200) {
			return false;
		}
		return path.split('/').every(function (segment) {
			return SEGMENT.test(segment);
		});
	}

	/**
	 * Maps the query string to a Markdown file relative to index.html.
	 *   (none)            -> README.md
	 *   ?page=foo         -> pages/foo.md
	 *   ?foo              -> foo.md
	 *   ?docs=foo         -> docs/foo.md
	 * Returns null when the requested path is not allowed. Only relative,
	 * same-directory paths are produced: no "//host", "https:", "..", etc.
	 */
	function resolvePage(search) {
		var params = new URLSearchParams(search || '');
		var first = params.entries().next().value;
		if (!first) {
			return 'README.md';
		}
		var key = first[0];
		var value = first[1];
		if (value && value.slice(-3) === '.md') {
			value = value.slice(0, -3);
		}
		if (key.slice(-3) === '.md') {
			key = key.slice(0, -3);
		}

		var path;
		if (key && !value) {
			path = key;
		} else if (key === 'page' && value) {
			path = 'pages/' + value;
		} else if (key && value) {
			path = key + '/' + value;
		} else {
			return 'README.md';
		}
		return isSafePath(path) ? path + '.md' : null;
	}

	MarkdownPages.isSafePath = isSafePath;
	MarkdownPages.resolvePage = resolvePage;

	// ---------------------------------------------------------------------
	// Conversion + sanitizing
	// ---------------------------------------------------------------------

	MarkdownPages.showdownOptions = {
		tables: true,
		emoji: true,
		ghCompatibleHeaderId: true,
		simpleLineBreaks: true,
		strikethrough: true,
		tasklists: true,
		parseImgDimensions: true,
		// keep "..." as typed - Showdown would also turn it into "…" inside link URLs
		ellipsis: false,
	};

	// Showdown extensions (by name) registered by add-ons, e.g. forms
	MarkdownPages.extensions = [];

	// extra DOMPurify settings, add-ons may extend these
	MarkdownPages.sanitizeConfig = {
		ADD_ATTR: ['target'],
		FORBID_TAGS: ['style'],
	};

	var purifyConfigured = false;

	function configurePurify(DOMPurify) {
		if (purifyConfigured) {
			return;
		}
		purifyConfigured = true;
		// links opening a new window must not get access to window.opener
		DOMPurify.addHook('afterSanitizeAttributes', function (node) {
			if (node.tagName === 'A' && node.getAttribute('target')) {
				node.setAttribute('rel', 'noopener noreferrer');
			}
		});
		// DOMPurify drops ids that equal a document property (DOM clobbering
		// protection), e.g. "## Fonts" -> id="fonts". Anchor links to such
		// headings would break, so heading ids are kept when they are safe.
		DOMPurify.addHook('uponSanitizeAttribute', function (node, data) {
			if (data.attrName !== 'id') {
				return;
			}
			if (RESERVED_IDS.indexOf(String(data.attrValue).toLowerCase()) >= 0) {
				// never let content shadow the library globals (any element)
				data.keepAttr = false;
			} else if (/^H[1-6]$/.test(node.nodeName) && isSafeHeadingId(data.attrValue)) {
				data.forceKeepAttr = true;
			}
		});
	}

	// globals of markdown-pages.js and the example scripts that are checked
	// with "if (window.x)" before their script has loaded - an element with
	// such an id would be found via window named access instead
	var RESERVED_IDS = ['hljs', 'showdown', 'showdownforms', 'dompurify', 'markdownpages'];

	/**
	 * A heading id may be kept if it looks like a generated header id and is
	 * not reserved. Headings are not form/img/embed/object elements, so they
	 * cannot clobber document properties, and window named access never
	 * shadows existing window properties (e.g. "location", "name", "top").
	 */
	function isSafeHeadingId(id) {
		return typeof id === 'string'
			&& /^[a-z0-9][a-z0-9_-]{0,99}$/i.test(id)
			&& RESERVED_IDS.indexOf(id.toLowerCase()) < 0;
	}

	MarkdownPages.isSafeHeadingId = isSafeHeadingId;

	function toHtml(markdown) {
		var converter = new global.showdown.Converter({ extensions: MarkdownPages.extensions });
		Object.keys(MarkdownPages.showdownOptions).forEach(function (key) {
			converter.setOption(key, MarkdownPages.showdownOptions[key]);
		});
		var html = converter.makeHtml(markdown);
		configurePurify(global.DOMPurify);
		return global.DOMPurify.sanitize(html, MarkdownPages.sanitizeConfig);
	}

	MarkdownPages.toHtml = toHtml;

	// ---------------------------------------------------------------------
	// Forms - provided by @schreibtischhelden/showdown-forms (vendored)
	// ---------------------------------------------------------------------
	//
	// :::form action="https://example.com/submit" method="post"
	// Name* = ___
	// [[ Send ]]
	// :::
	//
	// The options keep the attributes markdown-pages.js used before:
	// data-mdp-form="local", data-mdp-name, ids "mdp-form-<n>" and the
	// "mdp:form-submit" event for forms without action.

	MarkdownPages.formsOptions = { idPrefix: 'mdp-form', dataPrefix: 'mdp' };
	MarkdownPages.formsExtension = null;

	/** registers the forms extension once showdown-forms is loaded */
	function setupForms(showdownForms) {
		if (MarkdownPages.formsExtension || !showdownForms) {
			return;
		}
		// one instance for the page, so form ids keep counting across regions
		MarkdownPages.formsExtension = showdownForms(MarkdownPages.formsOptions);
		Array.prototype.push.apply(MarkdownPages.extensions, MarkdownPages.formsExtension);
		if (typeof document !== 'undefined') {
			showdownForms.listen(document, MarkdownPages.formsOptions);
		}
	}

	function restoreFormNames(root) {
		if (global.showdownForms) {
			global.showdownForms.restoreNames(root, MarkdownPages.formsOptions);
		}
	}

	MarkdownPages.setupForms = setupForms;

	// ---------------------------------------------------------------------
	// Menu (menu.json) - pure part, unit tested
	// ---------------------------------------------------------------------
	//
	// menu.json:
	// { "main": [
	//     { "title": "Home", "href": "." },
	//     { "title": "Docs", "id": "docs", "page": "docs", "children": [
	//         { "title": "Install", "page": "install" } ] },
	//     { "title": "GitHub", "href": "https://github.com/..." } ] }
	//
	// Placeholders (in any Markdown file, also header.md / footer.md):
	//   {{menu:main}}       top-level items of the main menu
	//   {{menu:tree}}       the whole menu with all levels (e.g. grouped docs navigation)
	//   {{menu:sub:<id>}}   children of the main menu item with that id
	//   {{menu:current}}    children of the main menu item that contains the current page
	//   {{toc}}             table of contents (h2/h3) of the current page
	//
	// Elements outside the rendered regions (e.g. a sidebar in index.html)
	// can use <nav data-mdp-menu="tree"></nav> and <nav data-mdp-toc></nav>.

	MarkdownPages.menuUrl = 'menu.json';

	var PLACEHOLDER = /\{\{\s*(?:menu:(main|tree|current|sub:([A-Za-z0-9_-]+))|(toc))\s*\}\}/g;
	var SAFE_HREF = /^(https?:|mailto:|[^:]*$)/i; // absolute http(s)/mailto, or no scheme at all
	var MAX_DEPTH = 5;

	function slug(text) {
		var result = String(text).toLowerCase().trim().replace(/[^a-z0-9]+/g, '-');
		// trim dashes without a regex (avoids polynomial backtracking on "----...")
		var start = 0;
		var end = result.length;
		while (start < end && result.charAt(start) === '-') {
			start++;
		}
		while (end > start && result.charAt(end - 1) === '-') {
			end--;
		}
		return result.slice(start, end);
	}

	/** href of a menu item, or null if it has no valid target */
	function menuItemHref(item) {
		if (typeof item.page === 'string') {
			var page = item.page.replace(/\.md$/, '');
			return isSafePath(page) ? '?page=' + page : null;
		}
		if (typeof item.href === 'string') {
			var href = item.href.trim();
			// "//host" would be protocol-relative, control characters are never valid
			if (/^\/\//.test(href) || /[\u0000-\u001f\u007f]/.test(href)) {
				return null;
			}
			return SAFE_HREF.test(href) ? href : null;
		}
		return null;
	}

	/** Markdown file a menu item points to (for "current page"), or null */
	function menuItemFile(item) {
		var href = menuItemHref(item);
		if (href === null) {
			return null;
		}
		if (href === '.' || href === './' || href === '' || href === 'index.html' || href === './index.html') {
			return 'README.md';
		}
		if (href.charAt(0) === '?') {
			return resolvePage(href);
		}
		return null;
	}

	/**
	 * Validates and normalises the parsed menu.json. Invalid entries are
	 * dropped instead of breaking the whole menu.
	 */
	function normalizeMenu(data) {
		function items(list, depth) {
			if (!Array.isArray(list) || depth > MAX_DEPTH) {
				return [];
			}
			return list.reduce(function (result, raw) {
				if (!raw || typeof raw !== 'object' || typeof raw.title !== 'string' || !raw.title.trim()) {
					return result;
				}
				var item = {
					title: raw.title.trim(),
					id: typeof raw.id === 'string' && /^[A-Za-z0-9_-]+$/.test(raw.id)
						? raw.id
						: slug(typeof raw.page === 'string' ? raw.page : raw.title),
					href: menuItemHref(raw),
					file: menuItemFile(raw),
					children: items(raw.children, depth + 1),
				};
				if (item.href !== null || item.children.length) {
					result.push(item);
				}
				return result;
			}, []);
		}
		return { main: items(data && data.main, 0) };
	}

	function containsFile(item, file) {
		return item.file === file || item.children.some(function (child) {
			return containsFile(child, file);
		});
	}

	/**
	 * Returns the list of items to show for a placeholder:
	 *   "main", "tree", "current" or "sub:<id>"
	 */
	function menuItemsFor(menu, kind, currentFile) {
		if (kind === 'main' || kind === 'tree') {
			return menu.main;
		}
		var parent = null;
		if (kind === 'current') {
			parent = menu.main.filter(function (item) {
				return containsFile(item, currentFile);
			})[0];
		} else if (kind.indexOf('sub:') === 0) {
			var id = kind.slice(4);
			parent = menu.main.filter(function (item) {
				return item.id === id;
			})[0];
		}
		return parent ? parent.children : [];
	}

	/**
	 * Parses the text of menu.json. Throws if it is not valid JSON or has no
	 * "main" array, so the caller can fall back to the hand-written navigation.
	 */
	function parseMenu(text) {
		var data = JSON.parse(text);
		if (!data || typeof data !== 'object' || !Array.isArray(data.main)) {
			throw new Error('"main" must be an array');
		}
		return normalizeMenu(data);
	}

	MarkdownPages.normalizeMenu = normalizeMenu;
	MarkdownPages.parseMenu = parseMenu;
	MarkdownPages.menuItemsFor = menuItemsFor;

	/**
	 * All items with a link in menu order (depth first) - used for
	 * previous/next navigation and search. "section" is the title of the
	 * top-level item an entry belongs to (null for top-level items).
	 */
	function flattenMenu(menu) {
		var result = [];
		(function walk(items, section) {
			items.forEach(function (item) {
				if (item.href !== null) {
					result.push({ title: item.title, href: item.href, file: item.file, section: section });
				}
				walk(item.children, section === null ? item.title : section);
			});
		})(menu && menu.main ? menu.main : [], null);
		return result;
	}

	MarkdownPages.flattenMenu = flattenMenu;

	// ---------------------------------------------------------------------
	// Menu - DOM part
	// ---------------------------------------------------------------------

	var menuPromise = null;

	function loadMenu() {
		if (!menuPromise) {
			// menu.json is optional: any failure (missing file, network error,
			// invalid JSON or structure) resolves to null -> fallback navigation
			menuPromise = Promise.resolve().then(function () {
				return fetchText(MarkdownPages.menuUrl);
			}).then(function (text) {
				if (text === null) {
					throw new Error(MarkdownPages.menuUrl + ' not found');
				}
				return parseMenu(text);
			}).catch(function (error) {
				console.info('markdown-pages.js: no menu, using fallback navigation (' + error.message + ')');
				return null;
			});
		}
		return menuPromise;
	}

	/** builds <ul> with DOM APIs only - titles are never parsed as HTML */
	function buildList(items, currentFile, nested) {
		var ul = document.createElement('ul');
		items.forEach(function (item) {
			var li = document.createElement('li');
			var label;
			if (item.href !== null) {
				label = document.createElement('a');
				label.setAttribute('href', item.href);
				if (/^https?:/i.test(item.href)) {
					label.setAttribute('rel', 'noopener noreferrer');
				}
			} else {
				label = document.createElement('span');
			}
			label.textContent = item.title;
			if (item.file !== null && item.file === currentFile) {
				label.setAttribute('aria-current', 'page');
				li.className = 'mdp-menu-item-current';
			} else if (containsFile(item, currentFile)) {
				li.className = 'mdp-menu-item-active';
			}
			li.appendChild(label);
			if (nested && item.children.length) {
				li.appendChild(buildList(item.children, currentFile, nested));
			}
			ul.appendChild(li);
		});
		return ul;
	}

	/**
	 * Replaces {{menu:...}} placeholders in text nodes (outside of code) with
	 * <nav data-mdp-menu="..."> elements, then fills all such elements.
	 */
	function applyMenus(root) {
		var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
			acceptNode: function (node) {
				return node.parentNode.closest('code, pre, script, style, textarea')
					? NodeFilter.FILTER_REJECT
					: NodeFilter.FILTER_ACCEPT;
			},
		});
		var textNodes = [];
		while (walker.nextNode()) {
			PLACEHOLDER.lastIndex = 0;
			if (PLACEHOLDER.test(walker.currentNode.nodeValue)) {
				textNodes.push(walker.currentNode);
			}
		}

		textNodes.forEach(function (node) {
			var fragment = document.createDocumentFragment();
			var text = node.nodeValue;
			var last = 0;
			text.replace(PLACEHOLDER, function (match, kind, id, toc, offset) {
				fragment.appendChild(document.createTextNode(text.slice(last, offset)));
				var nav = document.createElement('nav');
				nav.setAttribute(toc ? 'data-mdp-toc' : 'data-mdp-menu', toc ? '' : kind);
				fragment.appendChild(nav);
				last = offset + match.length;
				return match;
			});
			fragment.appendChild(document.createTextNode(text.slice(last)));
			var parent = node.parentNode;
			// a paragraph that only holds the placeholder is replaced entirely
			if (parent.tagName === 'P' && parent.childNodes.length === 1 && !text.replace(PLACEHOLDER, '').trim()) {
				parent.replaceWith(fragment);
			} else {
				parent.replaceChild(fragment, node);
			}
		});

		return fillMenus(root.querySelectorAll('[data-mdp-menu]'), root);
	}

	/** fills the given <nav data-mdp-menu> elements from menu.json */
	function fillMenus(navs, root) {
		if (!navs.length) {
			return Promise.resolve();
		}
		return loadMenu().then(function (menu) {
			if (menu === null) {
				// no usable menu.json: drop the placeholders, keep the hand-written navigation
				navs.forEach(function (nav) {
					nav.remove();
				});
				return;
			}
			// menu.json loaded: hand-written navigation marked as fallback is replaced
			root.querySelectorAll('[data-mdp-menu-fallback]').forEach(function (element) {
				element.remove();
			});
			var currentFile = resolvePage(global.location.search);
			navs.forEach(function (nav) {
				var kind = nav.getAttribute('data-mdp-menu');
				var items = menuItemsFor(menu, kind, currentFile);
				nav.className = 'mdp-menu mdp-menu-' + kind.split(':')[0];
				if (!nav.hasAttribute('aria-label')) {
					nav.setAttribute('aria-label', kind === 'main' || kind === 'tree' ? 'Main menu' : 'Submenu');
				}
				nav.textContent = '';
				if (items.length) {
					nav.appendChild(buildList(items, currentFile, kind !== 'main'));
				} else {
					nav.hidden = true;
				}
			});
		});
	}

	MarkdownPages.applyMenus = applyMenus;
	MarkdownPages.loadMenu = loadMenu;

	/** menus placed directly in index.html, outside header/main/footer */
	function applyStaticMenus() {
		var navs = Array.prototype.filter.call(document.querySelectorAll('[data-mdp-menu]'), function (nav) {
			return !nav.closest('header, main, footer');
		});
		var sidebar = document.createDocumentFragment(); // no fallback elements to remove here
		return fillMenus(navs, sidebar).catch(function (error) {
			console.error('markdown-pages.js: menu', error);
		});
	}

	/**
	 * Fills <nav data-mdp-toc> elements with the h2/h3 headings of <main>.
	 * Built with DOM APIs only; headings without id are skipped.
	 */
	function buildToc() {
		var main = document.querySelector('main');
		var headings = main ? main.querySelectorAll('h2[id], h3[id]') : [];
		document.querySelectorAll('[data-mdp-toc]').forEach(function (nav) {
			nav.textContent = '';
			nav.className = 'mdp-toc';
			if (!nav.hasAttribute('aria-label')) {
				nav.setAttribute('aria-label', 'Table of contents');
			}
			var list = document.createElement('ul');
			var current = null;
			headings.forEach(function (heading) {
				var li = document.createElement('li');
				var a = document.createElement('a');
				a.setAttribute('href', '#' + encodeURIComponent(heading.id));
				a.textContent = heading.textContent;
				li.appendChild(a);
				if (heading.tagName === 'H3' && current) {
					var sub = current.querySelector('ul') || current.appendChild(document.createElement('ul'));
					sub.appendChild(li);
				} else {
					list.appendChild(li);
					current = li;
				}
			});
			nav.hidden = !headings.length;
			nav.appendChild(list);
		});
	}

	MarkdownPages.buildToc = buildToc;

	// ---------------------------------------------------------------------
	// Browser runtime
	// ---------------------------------------------------------------------

	function loadScript(src) {
		return new Promise(function (resolve, reject) {
			var script = document.createElement('script');
			script.src = src;
			script.onload = resolve;
			script.onerror = function () {
				reject(new Error('could not load ' + src));
			};
			document.head.appendChild(script);
		});
	}

	function fetchText(path) {
		// resolve against the page and refuse anything that leaves the origin
		var url = new URL(path, global.location.href);
		if (url.origin !== global.location.origin) {
			return Promise.resolve(null);
		}
		return fetch(url.href, { credentials: 'same-origin' }).then(function (response) {
			return response.ok ? response.text() : null;
		});
	}

	function render(selector, markdown, page) {
		var element = document.querySelector(selector);
		if (!element) {
			return;
		}
		element.innerHTML = toHtml(markdown);
		restoreFormNames(element);
		return applyMenus(element).catch(function (error) {
			console.error('markdown-pages.js: menu', error);
		}).then(function () {
			if (selector === 'main') {
				buildToc();
			}
			document.dispatchEvent(new CustomEvent('mdp:rendered', {
				detail: { region: selector, page: page },
			}));
		});
	}

	function renderMain(page) {
		var request = page ? fetchText(page) : Promise.resolve(null);
		return request.then(function (markdown) {
			if (markdown !== null) {
				return markdown;
			}
			page = '404.md';
			return fetchText('404.md').then(function (notFound) {
				return notFound !== null ? notFound : '# Page not found\n\nThe requested page does not exist.';
			});
		}).then(function (markdown) {
			var rendered = render('main', markdown, page);
			var h1 = document.querySelector('main h1');
			if (h1) {
				document.title = h1.textContent + ' | ' + document.title;
			}
			var hash = global.location.hash.substring(1);
			try {
				hash = decodeURIComponent(hash);
			} catch (e) {
				// keep the raw hash if it is not valid percent-encoding
			}
			var target = hash && document.getElementById(hash);
			if (target) {
				target.scrollIntoView();
			}
			return rendered;
		});
	}

	function renderRegion(file, selector) {
		return fetchText(file).then(function (markdown) {
			if (markdown) {
				render(selector, markdown, file);
			}
		});
	}

	function start() {
		// resolve vendored libraries relative to this file, so it also works
		// when markdown-pages.js is loaded from another directory or a CDN
		var base = new URL('.', document.currentScript.src);
		var vendor = function (path) {
			return new URL('assets/vendor/' + path, base).href;
		};
		MarkdownPages.baseUrl = base.href;

		MarkdownPages.ready = Promise.all([
			loadScript(vendor('showdown/showdown.min.js')),
			loadScript(vendor('dompurify/purify.min.js')),
			loadScript(vendor('showdown-forms/showdown-forms.js')),
		]).then(function () {
			setupForms(global.showdownForms);
			var onDom = document.readyState === 'loading'
				? new Promise(function (resolve) { document.addEventListener('DOMContentLoaded', resolve); })
				: Promise.resolve();
			return onDom;
		}).then(function () {
			return Promise.all([
				renderMain(resolvePage(global.location.search)),
				renderRegion('header.md', 'header'),
				renderRegion('footer.md', 'footer'),
				applyStaticMenus(),
			]);
		}).catch(function (error) {
			console.error('markdown-pages.js:', error);
		});
	}

	global.MarkdownPages = MarkdownPages;

	if (typeof document !== 'undefined' && document.currentScript) {
		start();
	}
})(typeof window !== 'undefined' ? window : globalThis);
