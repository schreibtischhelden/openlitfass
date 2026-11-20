/* ---------------------------------------------------------
   openLitfass – Frontend-Logik
   Vanilla JS, keine Abhängigkeiten, keine Build-Pipeline.

   Daten (alle von fetch / build-site erzeugt, siehe docs/backend.md):
     data/events.json      Termine
     data/categories.json  Kategorie -> Piktogramm (Reihenfolge = Menü)
     data/sources.json     Quellen für "Quelle: …" und die Über-Seite
     data/site.json        instance.json der Instanz (Name, Texte, Links)

   Ansichten (Hash-Routing, damit Links teilbar sind):
     #            Liste
     #gemerkt     gemerkte Termine
     #ueber       Über-Seite
     #<event-id>  Detailansicht eines Termins
--------------------------------------------------------- */
(() => {
  "use strict";

  // ---- Texte ----
  // Alle sichtbaren Texte an einer Stelle, damit Mehrsprachigkeit später
  // ohne Umbau ergänzt werden kann.
  const STR = {
    today: "Heute",
    tomorrow: "Morgen",
    all: "Alle",
    weekend: "Wochenende",
    allDay: "ganztägig",
    free: "Kostenlos",
    ticket: "Ticket nötig",
    registration: "Anmeldung nötig",
    family: "Familie & Kinder",
    resultOne: "1 Termin",
    resultMany: (n) => `${n} Termine`,
    savedHeading: "Gemerkt",
    emptyNoData: ["Gerade stehen keine Termine an.", "Schau bald wieder vorbei."],
    emptyFiltered: ["Keine Termine für diese Auswahl.", ""],
    emptySaved: ["Noch nichts gemerkt.", "Tipp auf den Stern in einem Termin, um ihn hier zu sammeln."],
    resetFilters: "Filter zurücksetzen",
    loadError: "Die Termine konnten nicht geladen werden. Bitte später erneut versuchen.",
    back: "Zurück",
    save: "Merken",
    unsave: "Nicht mehr merken",
    addToCalendar: "In Kalender",
    share: "Teilen",
    toEvent: "Zur Veranstaltung",
    openMap: "Karte öffnen",
    noRegistration: "keine Anmeldung nötig",
    source: "Quelle",
    updated: "aktualisiert",
    linkCopied: "Link kopiert",
    about: "Über",
    aboutSources: "Woher die Termine kommen",
    aboutSubmit: "Termin fehlt?",
    aboutSubmitBtn: "Termin melden",
    imprint: "Impressum",
    privacy: "Datenschutz",
    poweredBy: "Erstellt mit openLitfass",
    inbox: "Von Hand eingetragen"
  };

  const WEEKDAYS = ["Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"];
  const WEEKDAYS_SHORT = ["So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"];
  const MONTHS = ["Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"];
  const MONTHS_SHORT = ["Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"];

  // ---- Icons (Tabler Icons, MIT, outline, 24x24) ----
  const ICON = {
    fallback: ["M6.5 7.5a1 1 0 1 0 2 0a1 1 0 1 0 -2 0", "M3 6v5.172a2 2 0 0 0 .586 1.414l7.71 7.71a2.41 2.41 0 0 0 3.408 0l5.592 -5.592a2.41 2.41 0 0 0 0 -3.408l-7.71 -7.71a2 2 0 0 0 -1.414 -.586h-5.172a3 3 0 0 0 -3 3"],
    free: ["M3 9a1 1 0 0 1 1 -1h16a1 1 0 0 1 1 1v2a1 1 0 0 1 -1 1h-16a1 1 0 0 1 -1 -1l0 -2", "M12 8l0 13", "M19 12v7a2 2 0 0 1 -2 2h-10a2 2 0 0 1 -2 -2v-7", "M7.5 8a2.5 2.5 0 0 1 0 -5a4.8 8 0 0 1 4.5 5a4.8 8 0 0 1 4.5 -5a2.5 2.5 0 0 1 0 5"],
    ticket: ["M15 5l0 2", "M15 11l0 2", "M15 17l0 2", "M5 5h14a2 2 0 0 1 2 2v3a2 2 0 0 0 0 4v3a2 2 0 0 1 -2 2h-14a2 2 0 0 1 -2 -2v-3a2 2 0 0 0 0 -4v-3a2 2 0 0 1 2 -2"],
    registration: ["M8 7a4 4 0 1 0 8 0a4 4 0 0 0 -8 0", "M6 21v-2a4 4 0 0 1 4 -4h4", "M15 19l2 2l4 -4"],
    family: ["M6 19a2 2 0 1 0 4 0a2 2 0 1 0 -4 0", "M16 19a2 2 0 1 0 4 0a2 2 0 1 0 -4 0", "M2 5h2.5l1.632 4.897a6 6 0 0 0 5.693 4.103h2.675a5.5 5.5 0 0 0 0 -11h-.5v6", "M6 9h14", "M9 17l1 -3", "M16 14l1 3"],
    pin: ["M9 11a3 3 0 1 0 6 0a3 3 0 0 0 -6 0", "M17.657 16.657l-4.243 4.243a2 2 0 0 1 -2.827 0l-4.244 -4.243a8 8 0 1 1 11.314 0"],
    calendar: ["M4 7a2 2 0 0 1 2 -2h12a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-12a2 2 0 0 1 -2 -2v-12", "M16 3v4", "M8 3v4", "M4 11h16"],
    calendarPlus: ["M12.5 21h-6.5a2 2 0 0 1 -2 -2v-12a2 2 0 0 1 2 -2h12a2 2 0 0 1 2 2v5", "M16 3v4", "M8 3v4", "M4 11h16", "M16 19h6", "M19 16v6"],
    clock: ["M3 12a9 9 0 1 0 18 0a9 9 0 0 0 -18 0", "M12 7v5l3 3"],
    users: ["M9 7m-4 0a4 4 0 1 0 8 0a4 4 0 1 0 -8 0", "M3 21v-2a4 4 0 0 1 4 -4h4a4 4 0 0 1 4 4v2", "M16 3.13a4 4 0 0 1 0 7.75", "M21 21v-2a4 4 0 0 0 -3 -3.85"],
    coin: ["M3 12a9 9 0 1 0 18 0a9 9 0 1 0 -18 0", "M14.8 9a2 2 0 0 0 -1.8 -1h-2a2 2 0 1 0 0 4h2a2 2 0 1 1 0 4h-2a2 2 0 0 1 -1.8 -1", "M12 7v10"],
    share: ["M6 12m-3 0a3 3 0 1 0 6 0a3 3 0 1 0 -6 0", "M18 6m-3 0a3 3 0 1 0 6 0a3 3 0 1 0 -6 0", "M18 18m-3 0a3 3 0 1 0 6 0a3 3 0 1 0 -6 0", "M8.7 10.7l6.6 -3.4", "M8.7 13.3l6.6 3.4"],
    external: ["M12 6h-6a2 2 0 0 0 -2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2 -2v-6", "M11 13l9 -9", "M15 4h5v5"],
    star: ["M12 17.75l-6.172 3.245l1.179 -6.873l-5 -4.867l6.9 -1l3.086 -6.253l3.086 6.253l6.9 1l-5 4.867l1.179 6.873l-6.158 -3.245"],
    back: ["M15 6l-6 6l6 6"]
  };

  function svg(paths, cls) {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" ` +
      `stroke-linejoin="round" aria-hidden="true"${cls ? ` class="${cls}"` : ""}>${paths.map((d) => `<path d="${d}"/>`).join("")}</svg>`;
  }

  const ATTRS = [
    { key: "free", label: STR.free, icon: ICON.free, test: (ev) => ev.price && ev.price.free },
    { key: "family", label: STR.family, icon: ICON.family, test: (ev) => ev.familyFriendly },
    { key: "ticket", label: STR.ticket, icon: ICON.ticket, test: (ev) => ev.ticketRequired },
    { key: "registration", label: STR.registration, icon: ICON.registration, test: (ev) => ev.registrationRequired }
  ];
  const WHEN = [
    { key: "all", label: STR.all },
    { key: "today", label: STR.today },
    { key: "weekend", label: STR.weekend }
  ];

  // ---- State ----
  const state = {
    events: [],
    byId: new Map(),
    generatedAt: null,
    categories: [],
    categoryIcons: new Map(),
    sources: new Map(),
    site: {},
    search: "",
    categoriesOn: new Set(),
    attrsOn: new Set(),
    when: "all",
    view: "list",
    cameFromList: false
  };

  const $ = (id) => document.getElementById(id);
  const el = {
    header: document.querySelector(".app-header"),
    searchToggle: $("searchToggle"),
    searchBar: $("searchBar"),
    searchInput: $("searchInput"),
    categoryMenu: $("categoryMenu"),
    quickFilters: $("quickFilters"),
    main: $("main"),
    viewList: $("viewList"),
    viewDetail: $("viewDetail"),
    viewAbout: $("viewAbout"),
    resultCount: $("resultCount"),
    groups: $("eventGroups"),
    empty: $("emptyState"),
    footer: $("siteFooter"),
    savedCount: $("savedCount"),
    toast: $("toast"),
    navItems: document.querySelectorAll(".bottom-nav__item")
  };

  // ---- Gemerkt (nur lokal im Browser) ----
  const SAVED_KEY = "openlitfass:saved:" + location.pathname;
  const saved = new Set(readSaved());

  function readSaved() {
    try {
      const v = JSON.parse(localStorage.getItem(SAVED_KEY) || "[]");
      return Array.isArray(v) ? v : [];
    } catch (_) {
      return [];
    }
  }
  function writeSaved() {
    try { localStorage.setItem(SAVED_KEY, JSON.stringify([...saved])); } catch (_) { /* privat/gesperrt: nur für diese Sitzung */ }
  }
  function toggleSaved(id) {
    if (saved.has(id)) saved.delete(id); else saved.add(id);
    writeSaved();
    updateSavedCount();
  }
  function updateSavedCount() {
    const n = [...saved].filter((id) => state.byId.has(id)).length;
    el.savedCount.hidden = n === 0;
    el.savedCount.textContent = String(n);
  }

  // ---- Datum ----
  // Bewusst lokale Datumsteile statt toISOString() (das rechnet in UTC um
  // und springt nahe Mitternacht auf das falsche Datum).
  function iso(d) {
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
  }
  function parseDate(s) {
    const [y, m, d] = s.split("-").map(Number);
    return new Date(y, m - 1, d);
  }
  function addDays(d, n) {
    const r = new Date(d);
    r.setDate(r.getDate() + n);
    return r;
  }
  function todayISO() { return iso(new Date()); }

  function dayHeading(dateStr) {
    const d = parseDate(dateStr);
    const base = `${WEEKDAYS_SHORT[d.getDay()]} ${d.getDate()}. ${MONTHS_SHORT[d.getMonth()]}`;
    if (dateStr === todayISO()) return `${STR.today} · ${base}`;
    if (dateStr === iso(addDays(new Date(), 1))) return `${STR.tomorrow} · ${base}`;
    return base;
  }
  function longDate(dateStr) {
    const d = parseDate(dateStr);
    return `${WEEKDAYS[d.getDay()]}, ${d.getDate()}. ${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
  }
  function weekendRange() {
    const now = new Date();
    const dow = now.getDay();
    if (dow === 0) return [todayISO(), todayISO()];
    const sat = addDays(now, 6 - dow);
    return [iso(sat), iso(addDays(sat, 1))];
  }
  function timeText(ev) {
    if (!ev.startTime) return STR.allDay;
    return ev.endTime ? `${ev.startTime} – ${ev.endTime} Uhr` : `${ev.startTime} Uhr`;
  }
  function relativeTime(date) {
    const mins = Math.round((Date.now() - date.getTime()) / 60000);
    if (mins < 60) return `vor ${Math.max(mins, 1)} Min.`;
    const h = Math.round(mins / 60);
    if (h < 48) return `vor ${h} Std.`;
    return `am ${date.getDate()}. ${MONTHS_SHORT[date.getMonth()]}`;
  }

  // ---- Preis ----
  const euro = new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR", minimumFractionDigits: 0, maximumFractionDigits: 2 });
  function priceText(ev) {
    if (!ev.price) return "";
    if (ev.price.free) return STR.free;
    if (typeof ev.price.amount === "number") return euro.format(ev.price.amount);
    return "";
  }

  // ---- Laden ----
  function getJSON(path) {
    return fetch(path).then((res) => {
      if (!res.ok) throw new Error(`${path}: ${res.status}`);
      return res.json();
    });
  }

  Promise.allSettled([
    getJSON("data/events.json"),
    getJSON("data/categories.json"),
    getJSON("data/sources.json"),
    getJSON("data/site.json")
  ]).then(([events, cats, sources, site]) => {
    if (site.status === "fulfilled") state.site = site.value || {};
    if (sources.status === "fulfilled") {
      (sources.value.sources || []).forEach((s) => state.sources.set(s.name, s));
    }
    if (cats.status === "fulfilled") {
      state.categories = cats.value.categories || [];
      state.categoryIcons = new Map(state.categories.map((c) => [c.name, c.icon]));
    } else {
      console.warn("categories.json fehlt, Kategorie-Menü bleibt leer", cats.reason);
    }

    if (events.status !== "fulfilled") {
      console.error(events.reason);
      showLoadError();
      return;
    }
    state.generatedAt = events.value.generatedAt ? new Date(events.value.generatedAt) : null;
    state.events = (events.value.events || [])
      .slice()
      .sort((a, b) => (a.date + (a.startTime || "")).localeCompare(b.date + (b.startTime || "")));
    state.byId = new Map(state.events.map((ev) => [ev.id, ev]));

    buildCategoryMenu();
    buildQuickFilters();
    renderFooter();
    updateSavedCount();
    bindEvents();
    route();
  });

  function showLoadError() {
    el.empty.hidden = false;
    el.empty.innerHTML = `<strong>${escapeHtml(STR.loadError)}</strong>`;
  }

  // ---- Aufbau ----
  function buildCategoryMenu() {
    el.categoryMenu.innerHTML = state.categories.map((c) =>
      `<button type="button" class="cm-item" data-category="${escapeHtml(c.name)}" aria-pressed="false" ` +
      `aria-label="${escapeHtml(c.name)}" title="${escapeHtml(c.name)}">${svg(c.icon, "cm-icon")}</button>`
    ).join("");
    el.categoryMenu.hidden = state.categories.length === 0;
  }

  function buildQuickFilters() {
    const when = WHEN.map((w) =>
      `<button type="button" class="chip" data-when="${w.key}" aria-pressed="false">${escapeHtml(w.label)}</button>`
    ).join("");
    const attrs = ATTRS.map((a) =>
      `<button type="button" class="chip chip--icon" data-attr="${a.key}" aria-pressed="false" ` +
      `aria-label="${escapeHtml(a.label)}" title="${escapeHtml(a.label)}">${svg(a.icon)}</button>`
    ).join("");
    el.quickFilters.innerHTML = when + `<span class="quick-filters__sep" aria-hidden="true"></span>` + attrs;
    syncFilterButtons();
  }

  function syncFilterButtons() {
    el.categoryMenu.querySelectorAll(".cm-item").forEach((b) => setPressed(b, state.categoriesOn.has(b.dataset.category)));
    el.quickFilters.querySelectorAll("[data-when]").forEach((b) => setPressed(b, b.dataset.when === state.when));
    el.quickFilters.querySelectorAll("[data-attr]").forEach((b) => setPressed(b, state.attrsOn.has(b.dataset.attr)));
  }
  function setPressed(btn, on) {
    btn.classList.toggle("is-active", on);
    btn.setAttribute("aria-pressed", String(on));
  }

  function renderFooter() {
    const s = state.site;
    const links = [];
    if (s.imprintUrl) links.push(`<a href="${escapeHtml(safeUrl(s.imprintUrl))}">${STR.imprint}</a>`);
    if (s.privacyUrl) links.push(`<a href="${escapeHtml(safeUrl(s.privacyUrl))}">${STR.privacy}</a>`);
    links.push(`<a href="#ueber">${STR.about}</a>`);
    el.footer.innerHTML = links.join("");
  }

  // ---- Interaktion ----
  function bindEvents() {
    el.searchToggle.addEventListener("click", () => {
      const open = el.searchBar.hidden;
      el.searchBar.hidden = !open;
      el.searchToggle.setAttribute("aria-expanded", String(open));
      if (open) {
        if (state.view !== "list" && state.view !== "saved") location.hash = "";
        el.searchInput.focus();
      } else if (state.search) {
        state.search = "";
        el.searchInput.value = "";
        renderList();
      }
      measureSticky();
    });
    el.searchInput.addEventListener("input", (e) => {
      state.search = e.target.value.trim().toLowerCase();
      renderList();
    });

    el.categoryMenu.addEventListener("click", (e) => {
      const b = e.target.closest(".cm-item");
      if (!b) return;
      toggleIn(state.categoriesOn, b.dataset.category);
      afterFilterChange();
    });
    el.quickFilters.addEventListener("click", (e) => {
      const b = e.target.closest(".chip");
      if (!b) return;
      if (b.dataset.when) state.when = b.dataset.when;
      if (b.dataset.attr) toggleIn(state.attrsOn, b.dataset.attr);
      afterFilterChange();
    });

    // Klick auf eine Karte merkt sich, dass "Zurück" in die Liste führt
    // (dann history.back() statt Hash-Sprung, damit die Scrollposition bleibt).
    el.groups.addEventListener("click", (e) => {
      if (e.target.closest(".event-card")) state.cameFromList = true;
    });

    window.addEventListener("hashchange", route);
    let t = null;
    window.addEventListener("resize", () => { clearTimeout(t); t = setTimeout(measureSticky, 120); });
  }

  function toggleIn(set, v) { if (set.has(v)) set.delete(v); else set.add(v); }
  function afterFilterChange() {
    syncFilterButtons();
    renderList();
    window.scrollTo({ top: 0 });
  }
  function resetFilters() {
    state.search = "";
    el.searchInput.value = "";
    state.categoriesOn.clear();
    state.attrsOn.clear();
    state.when = "all";
    afterFilterChange();
  }

  // ---- Routing ----
  function route() {
    const hash = decodeURIComponent(location.hash.replace(/^#/, ""));
    let view = "list";
    if (hash === "gemerkt") view = "saved";
    else if (hash === "ueber") view = "about";
    else if (hash && state.byId.has(hash)) view = "detail";

    const fromView = state.view;
    state.view = view;
    el.viewList.hidden = !(view === "list" || view === "saved");
    el.viewDetail.hidden = view !== "detail";
    el.viewAbout.hidden = view !== "about";
    const filtersVisible = view === "list";
    el.categoryMenu.hidden = !filtersVisible || state.categories.length === 0;
    el.quickFilters.hidden = !filtersVisible;
    el.navItems.forEach((a) => a.classList.toggle("is-active", a.dataset.view === view || (view === "detail" && a.dataset.view === "list")));

    if (view === "list" || view === "saved") {
      renderList();
      if (fromView !== "detail") window.scrollTo({ top: 0 });
    } else if (view === "detail") {
      renderDetail(state.byId.get(hash));
      window.scrollTo({ top: 0 });
    } else {
      renderAbout();
      window.scrollTo({ top: 0 });
    }
    if (view !== "detail") state.cameFromList = false;
    measureSticky();
  }

  // Sticky-Abstände an die tatsächlich gerenderten Höhen anpassen (Header
  // mit/ohne Suche, ein- oder mehrzeiliges Kategorie-Menü).
  function measureSticky() {
    const root = document.documentElement.style;
    root.setProperty("--hdr", `${el.header.offsetHeight}px`);
    root.setProperty("--cm", `${el.categoryMenu.hidden ? 0 : el.categoryMenu.offsetHeight}px`);
    root.setProperty("--qf", `${el.quickFilters.hidden ? 0 : el.quickFilters.offsetHeight}px`);
  }

  // ---- Liste ----
  function filteredEvents() {
    const today = todayISO();
    const [wkStart, wkEnd] = weekendRange();
    const savedOnly = state.view === "saved";

    return state.events.filter((ev) => {
      if (ev.date < today) return false;
      if (savedOnly) return saved.has(ev.id);
      if (state.when === "today" && ev.date !== today) return false;
      if (state.when === "weekend" && (ev.date < wkStart || ev.date > wkEnd)) return false;
      if (state.categoriesOn.size && !state.categoriesOn.has(ev.category)) return false;
      for (const a of ATTRS) {
        if (state.attrsOn.has(a.key) && !a.test(ev)) return false;
      }
      if (state.search) {
        const hay = [ev.title, ev.description, ev.category, ev.organizer, ev.location && ev.location.name]
          .filter(Boolean).join(" ").toLowerCase();
        if (!hay.includes(state.search)) return false;
      }
      return true;
    });
  }

  function hasActiveFilters() {
    return state.search || state.categoriesOn.size || state.attrsOn.size || state.when !== "all";
  }

  function renderList() {
    if (state.view !== "list" && state.view !== "saved") return;
    const results = filteredEvents();
    const savedView = state.view === "saved";

    el.resultCount.textContent = (savedView ? `${STR.savedHeading} · ` : "") +
      (results.length === 1 ? STR.resultOne : STR.resultMany(results.length));
    el.resultCount.hidden = results.length === 0;

    const groups = new Map();
    results.forEach((ev) => {
      if (!groups.has(ev.date)) groups.set(ev.date, []);
      groups.get(ev.date).push(ev);
    });
    el.groups.innerHTML = [...groups].map(([date, evs]) =>
      `<section class="day-group" aria-label="${escapeHtml(longDate(date))}">` +
      `<h2 class="day-heading">${escapeHtml(dayHeading(date))}</h2>` +
      `<ul class="event-list">${evs.map(cardHtml).join("")}</ul></section>`
    ).join("");

    el.empty.hidden = results.length > 0;
    if (results.length === 0) {
      let msg = STR.emptyNoData;
      if (savedView) msg = STR.emptySaved;
      else if (hasActiveFilters()) msg = STR.emptyFiltered;
      el.empty.innerHTML = `<strong>${escapeHtml(msg[0])}</strong>${msg[1] ? `<span>${escapeHtml(msg[1])}</span>` : ""}` +
        (!savedView && hasActiveFilters() ? `<br><button type="button" class="btn" id="emptyReset">${STR.resetFilters}</button>` : "");
      const btn = $("emptyReset");
      if (btn) btn.addEventListener("click", resetFilters);
    }
  }

  function categoryLabelHtml(cat) {
    if (!cat) return "";
    return `<span class="category-label">${svg(state.categoryIcons.get(cat) || ICON.fallback)}${escapeHtml(cat)}</span>`;
  }

  function cardHtml(ev) {
    const time = ev.startTime
      ? `${escapeHtml(ev.startTime)}${ev.endTime ? `<small>– ${escapeHtml(ev.endTime)}</small>` : ""}`
      : `<small>${STR.allDay}</small>`;
    const meta = [];
    if (ev.location && ev.location.name) meta.push(`<span>${svg(ICON.pin)} ${escapeHtml(ev.location.name)}</span>`);
    const price = priceText(ev);
    if (price) meta.push(`<span>${escapeHtml(price)}</span>`);
    const attrIcons = ATTRS.filter((a) => a.key !== "free" && a.test(ev))
      .map((a) => `<span title="${escapeHtml(a.label)}" aria-label="${escapeHtml(a.label)}">${svg(a.icon)}</span>`).join("");
    if (attrIcons) meta.push(attrIcons);

    return `<li><a class="event-card" href="#${encodeURIComponent(ev.id)}">` +
      `<div class="event-card__time">${time}</div>` +
      `<div class="event-card__body">${categoryLabelHtml(ev.category)}` +
      `<h3 class="event-card__title">${escapeHtml(ev.title)}</h3>` +
      `<p class="event-card__meta">${meta.join("")}</p></div>` +
      (saved.has(ev.id) ? `<span class="event-card__saved" aria-label="${STR.savedHeading}">${svg(ICON.star)}</span>` : "") +
      `</a></li>`;
  }

  // ---- Detail ----
  function renderDetail(ev) {
    const facts = [];
    facts.push(fact(ICON.calendar, escapeHtml(longDate(ev.date))));
    facts.push(fact(ICON.clock, escapeHtml(timeText(ev))));
    if (ev.location && (ev.location.name || ev.location.address)) {
      const q = [ev.location.name, ev.location.address].filter(Boolean).join(", ");
      const mapUrl = `https://www.openstreetmap.org/search?query=${encodeURIComponent(q)}`;
      facts.push(fact(ICON.pin, `${escapeHtml(ev.location.name || ev.location.address)}` +
        `<small>${ev.location.name && ev.location.address ? escapeHtml(ev.location.address) + " · " : ""}` +
        `<a href="${mapUrl}" target="_blank" rel="noopener">${STR.openMap}</a></small>`));
    }
    const priceBits = [priceText(ev)];
    if (ev.ticketRequired) priceBits.push(STR.ticket);
    priceBits.push(ev.registrationRequired ? STR.registration : STR.noRegistration);
    facts.push(fact(ICON.coin, escapeHtml(priceBits.filter(Boolean).join(" · "))));
    if (ev.familyFriendly) facts.push(fact(ICON.family, escapeHtml(STR.family)));
    if (ev.organizer) facts.push(fact(ICON.users, escapeHtml(ev.organizer)));

    const external = ev.url && !isSelfLink(ev.url)
      ? `<a class="btn btn--primary" href="${escapeHtml(safeUrl(ev.url))}" target="_blank" rel="noopener">${STR.toEvent} ${svg(ICON.external)}</a>`
      : "";

    const src = state.sources.get(ev.source);
    const srcLabel = ev.source === "inbox" ? STR.inbox : (src ? src.label : ev.source);
    const sourceLine = [srcLabel ? `${STR.source}: ${escapeHtml(srcLabel)}` : "",
      state.generatedAt ? `${STR.updated} ${relativeTime(state.generatedAt)}` : ""].filter(Boolean).join(" · ");

    const isSaved = saved.has(ev.id);
    el.viewDetail.innerHTML =
      `<div class="detail__bar">` +
      `<button type="button" class="detail__back" id="detailBack">${svg(ICON.back)} ${STR.back}</button>` +
      `<button type="button" class="icon-btn star-btn${isSaved ? " is-active" : ""}" id="detailStar" aria-pressed="${isSaved}" ` +
      `aria-label="${isSaved ? STR.unsave : STR.save}" title="${isSaved ? STR.unsave : STR.save}">${svg(ICON.star)}</button>` +
      `</div>` +
      categoryLabelHtml(ev.category) +
      `<h1 class="detail__title">${escapeHtml(ev.title)}</h1>` +
      `<ul class="facts">${facts.join("")}</ul>` +
      (ev.description ? `<p class="detail__desc">${escapeHtml(ev.description)}</p>` : "") +
      `<div class="btn-row">` +
      `<button type="button" class="btn" id="detailIcs">${svg(ICON.calendarPlus)} ${STR.addToCalendar}</button>` +
      `<button type="button" class="btn" id="detailShare">${svg(ICON.share)} ${STR.share}</button>` +
      `</div>` +
      (external ? `<div class="btn-row" style="margin-top:var(--space-2)">${external}</div>` : "") +
      (sourceLine ? `<p class="detail__source">${sourceLine}</p>` : "");

    $("detailBack").addEventListener("click", () => {
      if (state.cameFromList && history.length > 1) history.back();
      else location.hash = "";
    });
    $("detailStar").addEventListener("click", () => {
      toggleSaved(ev.id);
      renderDetail(ev);
    });
    $("detailIcs").addEventListener("click", () => downloadIcs(ev));
    $("detailShare").addEventListener("click", () => shareEvent(ev));
    el.main.focus({ preventScroll: true });
  }

  function fact(icon, html) {
    return `<li>${svg(icon)}<div>${html}</div></li>`;
  }

  // Der Backend-Fallback-Link zeigt auf diese Seite selbst (baseUrl/#id) -
  // als "Zur Veranstaltung"-Button wäre das ein Link im Kreis.
  function isSelfLink(url) {
    const base = (state.site.baseUrl || "").replace(/\/+$/, "");
    return (base && url.startsWith(base + "/#")) || url.startsWith("#");
  }

  function eventLink(ev) {
    return location.href.split("#")[0] + "#" + encodeURIComponent(ev.id);
  }

  function shareEvent(ev) {
    const url = eventLink(ev);
    const text = `${ev.title} · ${dayHeading(ev.date)}${ev.startTime ? ", " + ev.startTime : ""}`;
    if (navigator.share) {
      navigator.share({ title: ev.title, text, url }).catch(() => {});
      return;
    }
    if (navigator.clipboard) {
      navigator.clipboard.writeText(url).then(() => toast(STR.linkCopied), () => toast(url));
    } else {
      toast(url);
    }
  }

  // ---- iCalendar (.ics) ----
  function icsEscape(s) {
    return String(s || "").replace(/\\/g, "\\\\").replace(/\n/g, "\\n").replace(/[,;]/g, (c) => "\\" + c);
  }
  function icsDate(date, time) {
    return date.replace(/-/g, "") + (time ? "T" + time.replace(":", "") + "00" : "");
  }
  function downloadIcs(ev) {
    const tz = state.site.timezone || "Europe/Berlin";
    const host = (state.site.baseUrl || location.host || "openlitfass").replace(/^https?:\/\//, "").replace(/\/.*$/, "");
    const lines = [
      "BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//openLitfass//DE", "CALSCALE:GREGORIAN", "BEGIN:VEVENT",
      `UID:${ev.id}@${host}`,
      `DTSTAMP:${new Date().toISOString().replace(/[-:]/g, "").replace(/\.\d+Z$/, "Z")}`
    ];
    if (ev.startTime) {
      lines.push(`DTSTART;TZID=${tz}:${icsDate(ev.date, ev.startTime)}`);
      if (ev.endTime && ev.endTime > ev.startTime) lines.push(`DTEND;TZID=${tz}:${icsDate(ev.date, ev.endTime)}`);
    } else {
      lines.push(`DTSTART;VALUE=DATE:${icsDate(ev.date)}`);
      lines.push(`DTEND;VALUE=DATE:${icsDate(iso(addDays(parseDate(ev.date), 1)))}`);
    }
    lines.push(`SUMMARY:${icsEscape(ev.title)}`);
    if (ev.description) lines.push(`DESCRIPTION:${icsEscape(ev.description)}`);
    const loc = ev.location ? [ev.location.name, ev.location.address].filter(Boolean).join(", ") : "";
    if (loc) lines.push(`LOCATION:${icsEscape(loc)}`);
    lines.push(`URL:${ev.url && !isSelfLink(ev.url) ? ev.url : eventLink(ev)}`);
    lines.push("END:VEVENT", "END:VCALENDAR");

    const blob = new Blob([lines.join("\r\n") + "\r\n"], { type: "text/calendar;charset=utf-8" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `${ev.id}.ics`;
    document.body.appendChild(a);
    a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 1000);
  }

  // ---- Über ----
  function renderAbout() {
    const s = state.site;
    const paras = String(s.about || s.description || "").split(/\n\s*\n/).filter(Boolean)
      .map((p) => `<p>${escapeHtml(p)}</p>`).join("");

    const sources = [...state.sources.values()].map((src) => {
      const name = src.name === "inbox" ? STR.inbox : src.label;
      const label = src.homepage
        ? `<a href="${escapeHtml(safeUrl(src.homepage))}" target="_blank" rel="noopener">${escapeHtml(name)}</a>`
        : escapeHtml(name);
      const when = src.lastSuccess ? `<small>${STR.updated} ${relativeTime(new Date(src.lastSuccess))}</small>` : "";
      return `<li><span>${label}</span>${when}</li>`;
    }).join("");


    el.viewAbout.innerHTML =
      `<h1>${escapeHtml(s.name || document.title)}</h1>` + paras +
      (s.submitUrl ? `<h2>${STR.aboutSubmit}</h2><p><a class="btn" href="${escapeHtml(safeUrl(s.submitUrl))}">${STR.aboutSubmitBtn}</a></p>` : "") +
      (sources ? `<h2>${STR.aboutSources}</h2><ul class="source-list">${sources}</ul>` : "") +
      `<p style="margin-top:var(--space-5)"><small>${STR.poweredBy}</small></p>`;
  }

  // ---- Hilfen ----
  let toastTimer = null;
  function toast(text) {
    el.toast.textContent = text;
    el.toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.toast.hidden = true; }, 2500);
  }

  // Nur http(s), mailto und relative Links durchlassen: URLs kommen auch aus
  // fremden Feeds, ein "javascript:"-Link wäre sonst ausführbarer Code.
  function safeUrl(url) {
    const u = String(url || "").trim();
    if (/^(https?:|mailto:)/i.test(u)) return u;
    if (/^[a-z][a-z0-9+.-]*:/i.test(u) || u.startsWith("//")) return "#";
    return u;
  }

  function escapeHtml(str) {
    return String(str).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  }

  // ---- PWA ----
  if ("serviceWorker" in navigator && (location.protocol === "https:" || location.hostname === "localhost")) {
    window.addEventListener("load", () => {
      navigator.serviceWorker.register("sw.js").catch((err) => console.warn("Service Worker:", err));
    });
  }
})();
