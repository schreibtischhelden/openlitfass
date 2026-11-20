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
    periodDay: "Tag",
    periodWeekend: "Wochenende",
    periodWeek: "Woche",
    period: "Zeitraum",
    earlier: "Früher",
    later: "Später",
    pickDate: "Datum wählen",
    allDates: "Alle Termine",
    backToToday: "zurück zu heute",
    nextEvent: "Nächster Termin:",
    filterBy: "Filter:",
    emptyRange: ["Keine Termine in diesem Zeitraum.", ""],
    allDay: "ganztägig",
    free: "Kostenlos",
    ticket: "Ticket nötig",
    registration: "Anmeldung nötig",
    family: "Familie & Kinder",
    resultOne: "1 Termin",
    resultMany: (n) => `${n} Termine`,
    resultCancelled: (n) => ` · davon ${n} abgesagt`,
    cancelled: "Abgesagt",
    cancelledNotice: "Laut Veranstalter findet dieser Termin nicht statt.",
    savedHeading: "Gemerkt",
    emptyNoData: ["Gerade stehen keine Termine an.", "Schau bald wieder vorbei."],
    emptyFiltered: ["Keine Termine für diese Auswahl.", ""],
    emptySaved: ["Noch nichts gemerkt.", "Tipp auf den Stern in einem Termin, um ihn hier zu sammeln."],
    resetFilters: "Filter zurücksetzen",
    clearCategories: "Kategorien zurücksetzen",
    clearQuick: "Merkmale zurücksetzen",
    clearSearch: "Suche leeren",
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
    sources: "Quellen",
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
    back: ["M15 6l-6 6l6 6"],
    forward: ["M9 6l6 6l-6 6"],
    clear: ["M18 6l-12 12", "M6 6l12 12"]
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
  // Zeiträume der Datumsleiste: die Pfeile blättern um genau diese Einheit.
  // Eigene Symbole aus einer Familie (Tabler-Stil): derselbe Kalender, die
  // Markierung zeigt den Zeitraum - ein Punkt = Tag, zwei Punkte rechts =
  // die letzten beiden Wochentage, ein Balken = die ganze Woche.
  const CAL = ["M4 7a2 2 0 0 1 2 -2h12a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-12a2 2 0 0 1 -2 -2v-12", "M16 3v4", "M8 3v4", "M4 11h16"];
  const PERIODS = [
    { key: "tag", label: STR.periodDay, icon: CAL.concat(["M10.5 14.5h3v2.5h-3z"]) },
    { key: "wochenende", label: STR.periodWeekend, icon: CAL.concat(["M12 14.5h2v2.5h-2z", "M15.5 14.5h2v2.5h-2z"]) },
    { key: "woche", label: STR.periodWeek, icon: CAL.concat(["M6.5 14.5h11v2.5h-11z"]) }
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
    period: "tag",   // tag | wochenende | woche
    anchor: "",      // erster Tag des gewählten Zeitraums (YYYY-MM-DD); "" = heute
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
    dateNav: $("dateNav"),
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
  // ---- Zeitraum (Datumsleiste) ----
  // Normalisiert ein Datum auf den Anfang seines Zeitraums: Tag = der Tag,
  // Wochenende = dessen Samstag (am Sonntag: der Samstag davor), Woche = Montag.
  function periodStart(period, dateStr) {
    const d = parseDate(dateStr);
    const dow = d.getDay(); // 0 = So
    if (period === "wochenende") return iso(dow === 0 ? addDays(d, -1) : addDays(d, 6 - dow));
    if (period === "woche") return iso(addDays(d, -((dow + 6) % 7)));
    return dateStr;
  }
  function periodLength(period) {
    return period === "woche" ? 7 : period === "wochenende" ? 2 : 1;
  }
  function periodStep(period) {
    return period === "tag" ? 1 : 7;
  }
  // [erster, letzter] Tag des aktuellen Zeitraums; nie vor heute beginnend
  // (der Kalender hat keine vergangenen Termine).
  function currentRange() {
    const today = todayISO();
    const start = periodStart(state.period, state.anchor || today);
    const end = iso(addDays(parseDate(start), periodLength(state.period) - 1));
    return [start < today ? today : start, end];
  }
  function shortDate(dateStr, withWeekday) {
    const d = parseDate(dateStr);
    return `${withWeekday ? WEEKDAYS_SHORT[d.getDay()] + " " : ""}${d.getDate()}.${d.getMonth() + 1}.`;
  }
  function rangeLabel() {
    if (state.search) return STR.allDates;
    const today = todayISO();
    const start = periodStart(state.period, state.anchor || today);
    const end = iso(addDays(parseDate(start), periodLength(state.period) - 1));
    if (state.period === "tag") {
      if (start === today) return `${STR.today}, ${shortDate(start)}`;
      if (start === iso(addDays(new Date(), 1))) return `${STR.tomorrow}, ${shortDate(start)}`;
      const d = parseDate(start);
      return `${WEEKDAYS_SHORT[d.getDay()]}, ${shortDate(start)}`;
    }
    // Beschriftet wird, was sichtbar ist: die laufende Woche beginnt heute,
    // nicht am vergangenen Montag.
    const visibleStart = currentRange()[0];
    if (visibleStart === end) return shortDate(end);
    const s = parseDate(visibleStart), e = parseDate(end);
    // Kompakt, damit die Leiste auf 320-px-Handys einzeilig bleibt; dass es
    // ein Wochenende ist, zeigt der aktive Zeitraum-Knopf.
    const left = s.getMonth() === e.getMonth() ? `${s.getDate()}.` : shortDate(visibleStart);
    return `${left}–${shortDate(end)}`;
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
    buildDateNav();
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
  // Jede Filterzeile (Suche, Kategorien, Schnellfilter) hat einen eigenen
  // Löschen-Button, der nur sichtbar ist, solange in genau dieser Zeile
  // etwas gesetzt ist - so lässt sich eine Zeile zurücksetzen, ohne die
  // anderen zu verlieren.
  function clearButton(row, label, cls) {
    return `<button type="button" class="${cls}" data-clear="${row}" hidden ` +
      `aria-label="${escapeHtml(label)}" title="${escapeHtml(label)}">${svg(ICON.clear)}</button>`;
  }

  function buildCategoryMenu() {
    el.categoryMenu.innerHTML = state.categories.map((c) =>
      `<button type="button" class="cm-item" data-category="${escapeHtml(c.name)}" aria-pressed="false" ` +
      `aria-label="${escapeHtml(c.name)}" title="${escapeHtml(c.name)}">${svg(c.icon, "cm-icon")}</button>`
    ).join("") + clearButton("categories", STR.clearCategories, "cm-item cm-clear");
    el.categoryMenu.hidden = state.categories.length === 0;
  }

  // Datumsleiste: [‹] [Datum + Kalender-Symbol] [›] [Tag|Wochenende|Woche].
  // Das Datum öffnet die native Datumsauswahl des Browsers (auf dem Handy
  // das gewohnte Datumsrad).
  function buildDateNav() {
    const periods = PERIODS.map((p) =>
      `<button type="button" class="period-btn" data-period="${p.key}" aria-pressed="false" ` +
      `aria-label="${escapeHtml(p.label)}" title="${escapeHtml(p.label)}">${svg(p.icon)}</button>`
    ).join("");
    el.dateNav.innerHTML =
      `<button type="button" class="date-step" data-step="-1" aria-label="${STR.earlier}" title="${STR.earlier}">${svg(ICON.back)}</button>` +
      `<label class="date-label" title="${STR.pickDate}"><span class="date-label__text" id="dateText"></span>${svg(ICON.calendar)}` +
      `<input type="date" class="date-input" id="dateInput" aria-label="${STR.pickDate}"></label>` +
      `<button type="button" class="date-step" data-step="1" aria-label="${STR.later}" title="${STR.later}">${svg(ICON.forward)}</button>` +
      `<span class="period" role="group" aria-label="${STR.period}">${periods}</span>`;
    el.dateText = $("dateText");
    el.dateInput = $("dateInput");
  }

  function syncDateNav() {
    if (!el.dateText) return;
    const today = todayISO();
    const [start] = currentRange();
    el.dateText.textContent = rangeLabel();
    el.dateInput.min = today;
    el.dateInput.value = start;
    el.dateNav.classList.toggle("is-searching", Boolean(state.search));
    const prev = el.dateNav.querySelector('[data-step="-1"]');
    prev.disabled = Boolean(state.search) || start <= today;
    el.dateNav.querySelector('[data-step="1"]').disabled = Boolean(state.search);
    el.dateNav.querySelectorAll("[data-period]").forEach((b) => setPressed(b, b.dataset.period === state.period));
  }

  // Setzt Zeitraum/Datum und schreibt es in die Adresse (teilbar), ohne
  // für jeden Pfeil-Klick einen Verlaufseintrag anzulegen.
  function setRange(period, anchor) {
    const today = todayISO();
    state.period = period;
    let a = periodStart(period, anchor || today);
    if (a < periodStart(period, today)) a = periodStart(period, today);
    state.anchor = a;
    const isDefault = period === "tag" && a === today;
    history.replaceState(null, "", isDefault ? location.pathname + location.search : `#${period}/${a}`);
    syncDateNav();
    renderList();
    window.scrollTo({ top: 0 });
  }

  function buildQuickFilters() {
    const attrs = ATTRS.map((a) =>
      `<button type="button" class="chip chip--icon" data-attr="${a.key}" aria-pressed="false" ` +
      `aria-label="${escapeHtml(a.label)}" title="${escapeHtml(a.label)}">${svg(a.icon)}</button>`
    ).join("");
    // Löschen-Button vorne: die Zeile scrollt horizontal, hinten wäre er
    // auf schmalen Bildschirmen oft nicht zu sehen.
    el.quickFilters.innerHTML = clearButton("quick", STR.clearQuick, "chip chip--icon chip--clear") +
      `<span class="quick-filters__label">${STR.filterBy}</span>` + attrs;
    el.searchBar.insertAdjacentHTML("beforeend", clearButton("search", STR.clearSearch, "icon-btn search-clear"));
    el.searchClear = el.searchBar.querySelector("[data-clear]");
    el.searchClear.addEventListener("click", () => {
      clearRow("search");
      el.searchInput.focus();
    });
    syncFilterButtons();
  }

  function syncFilterButtons() {
    el.categoryMenu.querySelectorAll(".cm-item[data-category]").forEach((b) => setPressed(b, state.categoriesOn.has(b.dataset.category)));
    el.quickFilters.querySelectorAll("[data-attr]").forEach((b) => setPressed(b, state.attrsOn.has(b.dataset.attr)));
    const catClear = el.categoryMenu.querySelector("[data-clear]");
    if (catClear) catClear.hidden = state.categoriesOn.size === 0;
    const quickClear = el.quickFilters.querySelector("[data-clear]");
    if (quickClear) quickClear.hidden = state.attrsOn.size === 0;
    if (el.searchClear) el.searchClear.hidden = !state.search;
    syncDateNav();
  }

  function clearRow(row) {
    if (row === "categories") state.categoriesOn.clear();
    if (row === "quick") state.attrsOn.clear();
    if (row === "search") {
      state.search = "";
      el.searchInput.value = "";
    }
    afterFilterChange();
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
      if (el.searchClear) el.searchClear.hidden = !state.search;
      syncDateNav();
      renderList();
    });

    el.dateNav.addEventListener("click", (e) => {
      const step = e.target.closest("[data-step]");
      if (step && !step.disabled) {
        const [start] = currentRange();
        const base = periodStart(state.period, state.anchor || todayISO());
        const from = step.dataset.step === "1" ? base : start;
        setRange(state.period, iso(addDays(parseDate(from), Number(step.dataset.step) * periodStep(state.period))));
        return;
      }
      const p = e.target.closest("[data-period]");
      if (p) setRange(p.dataset.period, currentRange()[0]);
    });
    // Klick aufs Datum öffnet den Picker (showPicker, wo vorhanden; sonst
    // reicht der Klick auf das darüberliegende, unsichtbare Datumsfeld).
    el.dateNav.querySelector(".date-label").addEventListener("click", (e) => {
      if (state.search) { e.preventDefault(); return; }
      if (el.dateInput.showPicker) {
        e.preventDefault();
        try { el.dateInput.showPicker(); } catch (_) { el.dateInput.focus(); }
      }
    });
    el.dateInput.addEventListener("change", () => {
      if (el.dateInput.value) setRange(state.period, el.dateInput.value);
    });

    el.categoryMenu.addEventListener("click", (e) => {
      const b = e.target.closest(".cm-item");
      if (!b) return;
      if (b.dataset.clear) { clearRow("categories"); return; }
      toggleIn(state.categoriesOn, b.dataset.category);
      afterFilterChange();
    });
    el.quickFilters.addEventListener("click", (e) => {
      const b = e.target.closest(".chip");
      if (!b) return;
      if (b.dataset.clear) { clearRow("quick"); return; }
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
    afterFilterChange();
  }

  // ---- Routing ----
  function route() {
    const hash = decodeURIComponent(location.hash.replace(/^#/, ""));
    let view = "list";
    // Zeitraum in der Adresse: #tag/2026-11-22, #wochenende/…, #woche/…
    const rangeMatch = hash.match(/^(tag|wochenende|woche)\/(\d{4}-\d{2}-\d{2})$/);
    if (rangeMatch) {
      state.period = rangeMatch[1];
      state.anchor = periodStart(rangeMatch[1], rangeMatch[2]);
    }
    // Ohne Hash (Startseite, Zurück aus der Detailansicht) bleibt der
    // zuletzt gewählte Zeitraum.
    if (rangeMatch || hash === "") view = "list";
    else if (hash === "gemerkt") view = "saved";
    else if (hash === "ueber") view = "about";
    else if (hash && state.byId.has(hash)) view = "detail";

    const fromView = state.view;
    state.view = view;
    el.viewList.hidden = !(view === "list" || view === "saved");
    el.viewDetail.hidden = view !== "detail";
    el.viewAbout.hidden = view !== "about";
    const filtersVisible = view === "list";
    el.categoryMenu.hidden = !filtersVisible || state.categories.length === 0;
    el.dateNav.hidden = !filtersVisible;
    el.quickFilters.hidden = !filtersVisible;
    syncDateNav();
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
    root.setProperty("--dn", `${el.dateNav.hidden ? 0 : el.dateNav.offsetHeight}px`);
  }

  // ---- Liste ----
  // ignoreRange: für die Suche nach dem nächsten Termin außerhalb des
  // gewählten Zeitraums (Leerzustand).
  function filteredEvents(ignoreRange) {
    const today = todayISO();
    const savedOnly = state.view === "saved";
    // Die Suche gilt über alle Daten hinweg, nicht nur im Zeitraum.
    const [from, to] = ignoreRange || state.search ? [today, "9999-12-31"] : currentRange();

    return state.events.filter((ev) => {
      if (ev.date < today) return false;
      if (savedOnly) return saved.has(ev.id);
      if (ev.date < from || ev.date > to) return false;
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
    return state.search || state.categoriesOn.size || state.attrsOn.size;
  }

  function renderList() {
    if (state.view !== "list" && state.view !== "saved") return;
    const results = filteredEvents();
    const savedView = state.view === "saved";

    const cancelledCount = results.filter((ev) => ev.cancelled).length;
    el.resultCount.textContent = (savedView ? `${STR.savedHeading} · ` : "") +
      (results.length === 1 ? STR.resultOne : STR.resultMany(results.length)) +
      (cancelledCount ? STR.resultCancelled(cancelledCount) : "");
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

    // "Zurück zu heute", wenn ein anderer Zeitraum gewählt ist.
    if (!savedView && !state.search && currentRange()[0] > todayISO()) {
      el.resultCount.hidden = false;
      el.resultCount.insertAdjacentHTML("beforeend",
        ` · <button type="button" class="link-btn" id="backToToday">${STR.backToToday}</button>`);
      $("backToToday").addEventListener("click", () => setRange(state.period, todayISO()));
    }

    el.empty.hidden = results.length > 0;
    if (results.length === 0) {
      let msg = STR.emptyNoData;
      let action = "";
      if (savedView) {
        msg = STR.emptySaved;
      } else if (!state.search) {
        // Leerer Zeitraum: zum nächsten Tag mit passenden Terminen springen.
        const [, to] = currentRange();
        const next = filteredEvents(true).find((ev) => ev.date > to);
        if (next) {
          msg = STR.emptyRange;
          action = `<br><button type="button" class="btn" id="emptyNext">${STR.nextEvent} ${escapeHtml(dayHeading(next.date))}</button>`;
        } else if (hasActiveFilters()) {
          msg = STR.emptyFiltered;
        }
      } else {
        msg = STR.emptyFiltered;
      }
      if (!action && !savedView && hasActiveFilters()) {
        action = `<br><button type="button" class="btn" id="emptyReset">${STR.resetFilters}</button>`;
      }
      el.empty.innerHTML = `<strong>${escapeHtml(msg[0])}</strong>${msg[1] ? `<span>${escapeHtml(msg[1])}</span>` : ""}` + action;
      const reset = $("emptyReset");
      if (reset) reset.addEventListener("click", resetFilters);
      const nextBtn = $("emptyNext");
      if (nextBtn) {
        const [, to] = currentRange();
        const next = filteredEvents(true).find((ev) => ev.date > to);
        nextBtn.addEventListener("click", () => setRange(state.period, next.date));
      }
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

    // Abgesagte Termine bleiben in der Liste (wer hingehen wollte, soll es
    // erfahren), sind aber deutlich gekennzeichnet: Etikett + durchgestrichener
    // Titel + gedämpfte Karte. Das Etikett ist Text, nicht nur Farbe.
    const cancelled = ev.cancelled
      ? `<span class="cancelled-badge">${STR.cancelled}</span>`
      : "";
    return `<li><a class="event-card${ev.cancelled ? " event-card--cancelled" : ""}" href="#${encodeURIComponent(ev.id)}">` +
      `<div class="event-card__time">${time}</div>` +
      `<div class="event-card__body">${cancelled}${categoryLabelHtml(ev.category)}` +
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
      // Adresse bevorzugen: der Name ist manchmal nur ein Raum ("Bewegungsraum").
      const q = ev.location.address || ev.location.name;
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

    // Derselbe Termin kann aus mehreren Quellen stammen (vom Backend
    // zusammengeführt, siehe writer.MergeAcrossSources): alle nennen,
    // die angezeigte Version zuerst.
    const sourceLabel = (name) => {
      if (name === "inbox") return STR.inbox;
      const src = state.sources.get(name);
      return src ? src.label : name;
    };
    const srcNames = (ev.sources && ev.sources.length ? ev.sources : [ev.source]).filter(Boolean);
    const srcLabel = [...new Set(srcNames.map(sourceLabel))].join(", ");
    const sourceLine = [srcLabel ? `${srcNames.length > 1 ? STR.sources : STR.source}: ${escapeHtml(srcLabel)}` : "",
      state.generatedAt ? `${STR.updated} ${relativeTime(state.generatedAt)}` : ""].filter(Boolean).join(" · ");

    const isSaved = saved.has(ev.id);
    el.viewDetail.innerHTML =
      `<div class="detail__bar">` +
      `<button type="button" class="detail__back" id="detailBack">${svg(ICON.back)} ${STR.back}</button>` +
      `<button type="button" class="icon-btn star-btn${isSaved ? " is-active" : ""}" id="detailStar" aria-pressed="${isSaved}" ` +
      `aria-label="${isSaved ? STR.unsave : STR.save}" title="${isSaved ? STR.unsave : STR.save}">${svg(ICON.star)}</button>` +
      `</div>` +
      (ev.cancelled
        ? `<div class="cancelled-notice" role="status"><strong>${STR.cancelled}</strong> ${STR.cancelledNotice}</div>`
        : "") +
      categoryLabelHtml(ev.category) +
      `<h1 class="detail__title${ev.cancelled ? " is-cancelled" : ""}">${escapeHtml(ev.title)}</h1>` +
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
    const text = `${ev.cancelled ? STR.cancelled + ": " : ""}${ev.title} · ${dayHeading(ev.date)}${ev.startTime ? ", " + ev.startTime : ""}`;
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
    lines.push(`SUMMARY:${icsEscape((ev.cancelled ? STR.cancelled + ": " : "") + ev.title)}`);
    if (ev.cancelled) lines.push("STATUS:CANCELLED");
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
