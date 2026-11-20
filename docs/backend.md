# Backend: fetch & build-site

Zwei kleine Go-Programme (reine stdlib, keine externen Dependencies), die
gegen ein **Instanz-Verzeichnis** laufen (Aufbau: `docs/instance.md`):

- **`fetch`** ruft alle Quellen der Instanz ab, normalisiert die Events und
  schreibt `<instance>/data/events.json`, `categories.json` und `sources.json`.
  Das Frontend (`web/js/app.js`) lädt diese Dateien direkt per `fetch()` —
  kein API-Server nötig.
- **`build-site`** setzt daraus die fertige statische Seite zusammen:
  Kern-Frontend (`web/`) + Instanz-Overrides (`<instance>/site/`) + Daten,
  `web/index.html` gerendert mit den Werten aus `instance.json`.
  Dazu erzeugt es `data/site.json` (instance.json für das Frontend),
  `manifest.webmanifest` (PWA) und stempelt `sw.js` mit einer Build-ID.

Beide laufen im Container-Image des Kerns (`Containerfile`), das der
Workflow der Instanz (`update-events.yml`) alle 6 Stunden ausführt.

## Struktur

```
backend/
  cmd/fetch/            Einstiegspunkt: Quellen abrufen, JSON schreiben
  cmd/build-site/       Einstiegspunkt: statische Seite zusammensetzen
  internal/instance/    Lädt <instance>/instance.json, kennt die Pfade
  internal/model/       Event-Schema, Instanz-Zeitzone (model.Local)
  internal/config/      Lädt <instance>/sources/*.json
  internal/mapping/     Dot-Path-Resolver + Feld-Mapping-Engine (siehe unten)
  internal/sources/     Source-Interface, generische RSS/JSON/wcs-Fetcher,
                        Inbox, Resilienz-Wrapper (Timeout/Panic-Recovery/Filter)
  internal/categories/  Kategorie -> Piktogramm (aus <instance>/categories.json)
  internal/writer/      Validierung, Dedupe, Sortierung, stabile IDs, JSON-Ausgabe
```

## Was ist instanzspezifisch?

Alles, was eine Instanz von einer anderen unterscheidet, steht in
`instance.json` und den Dateien daneben — der Kern enthält keine
ortsbezogenen Inhalte:

| Was | Wo |
|---|---|
| Name, Titel, Beschreibung, Sprache, Theme-Farbe, Über-Text | `instance.json` |
| Impressum, Datenschutz, "Termin melden" | `instance.json` → `imprintUrl`, `privacyUrl`, `submitUrl` |
| Zeitzone aller Event-Zeiten | `instance.json` → `timezone` (Standard `Europe/Berlin`) |
| Fallback-Link für Events ohne eigene URL | `instance.json` → `baseUrl` (+ `#<event-id>`) |
| Quellen | `sources/*.json` |
| Einzel-Events | `inbox/*.json` |
| Kategorien + Piktogramme | `categories.json` |
| Farben, Logo, Schriften | `site/css/theme.css`, `site/logo.svg`, `site/…` |

## Woher weiß das Script, was es abrufen soll?

Über `<instance>/sources/` — **eine JSON-Datei pro Quelle**, keine
gemeinsame Liste. `sources.All()` (in `internal/sources/source.go`) liest
beim Start jede `*.json`-Datei in diesem Verzeichnis (alphabetisch, für
deterministische Runs) und baut daraus automatisch einen `RSSSource`,
`JSONSource`, `ICalSource`, `HTMLSource` oder `WCSSource` — **für RSS-Feeds, JSON-APIs, iCal-Kalender, HTML-Seiten und WordPress
"Weekly Class Schedule"-Kalender muss kein Go-Code geschrieben werden**, nur
eine neue Datei angelegt werden.

Der Dateiname (ohne `.json`) wird automatisch der Quellen-Name, sofern die
Datei kein eigenes `"name"`-Feld setzt. Damit betrifft das Hinzufügen,
Ändern oder Entfernen einer Quelle immer genau eine Datei — sauberer git
diff, keine Merge-Konflikte in einem gemeinsamen Array.

Dateien, die auf `.example.json` enden, werden **nicht** geladen (Vorlagen
zum Kopieren). Vorlagen für alle drei Typen liegen in
`example-instance/sources/`:

- `beispiel-rss.example.json` — RSS
- `beispiel-json.example.json` — JSON-API
- `beispiel-ical.example.json` — `type: "ical"` (siehe Abschnitt "ical-Quellen")
- `beispiel-html.example.json` — `type: "html"` (siehe Abschnitt "html-Quellen")
- `beispiel-wcs.example.json` — `type: "wcs"` (siehe Abschnitt "wcs-Quellen")

Eine Quelle lässt sich mit `"disabled": true` abschalten, ohne die Datei zu
löschen. Das `"status"`-Feld verwaltet die Pipeline selbst (siehe
"Health-Status"), nicht von Hand pflegen.

Nur für Formate, die auch da nicht reinpassen (Logins, POST-Anfragen,
authentifizierte APIs, …), muss man selbst eine `Source`
implementieren — siehe `internal/sources/example.go` als Vorlage.

## Wie das Feld-Mapping funktioniert

Jede Quellen-Datei hat ein `mapping`-Objekt (bei `rss`/`json`-Quellen). Jeder
Eintrag darin ist eine **FieldRule**:

```jsonc
{
  "path": "location.title",   // Dot-Pfad in das Item, z.B. "a.b.0" für Array-Index 0
  "regex": "Beginn (?P<value>\\d{2}:\\d{2})",  // optional: Wert per Regex aus dem String extrahieren
  "format": "RFC1123Z",        // optional: nur für start/end – Zeit-Layout zum Parsen
  "default": "Marktplatz",     // optional: Fallback, wenn path nichts liefert
  "template": "{venue.address}, {venue.zip} {venue.city}"  // statt path: mehrere Werte kombinieren
}
```

`template` setzt jeden `{dot.pfad}` durch seinen Wert und räumt Trennzeichen
neben leeren Werten auf (fehlt die Straße, wird daraus "13086 Berlin", nicht
", 13086 Berlin").

**Text wird immer bereinigt:** Alle Textfelder aus `rss`/`json`-Quellen
laufen durch `mapping.CleanText` — HTML-Tags raus, Entitäten dekodiert
(`&#8211;` → `–`, `&#038;` → `&`), weiche Trennstriche und Nullbreite-Zeichen
entfernt, Leerraum zusammengefasst. `description` behält Absätze und wird bei
1200 Zeichen mit "…" gekürzt.

**Für JSON-Quellen** ist `path` ein Dot-Pfad in die dekodierte JSON-Antwort
(`itemsPath` zeigt vorher auf das Array der Einzel-Items, z.B. `"data.items"`;
leer = die Antwort selbst ist bereits das Array).

**Paginierte JSON-APIs:** `nextPath` zeigt auf die URL der nächsten Seite in
der Antwort (z.B. `"next_rest_url"` bei WordPress "The Events Calendar").
Gefolgt wird, bis keine nächste Seite mehr kommt, `maxPages` (Standard 10)
erreicht ist oder eine ganze Seite hinter dem `daysAhead`-Fenster liegt.
Schlägt eine spätere Seite fehl, bleiben die bisherigen Termine erhalten.

**Für RSS-Quellen** ist `path` der kleingeschriebene, Namespace-freie
Tag-Name innerhalb von `<item>` – Standardfelder sind `title`, `link`,
`description`, `pubdate`, `guid`, `category`; zusätzliche/eigene Tags eines
Feeds funktionieren genauso, da einfach jedes Kind-Element von `<item>`
verfügbar gemacht wird.

**Verfügbare Ziel-Felder** in `mapping` (alle optional außer für ein
sinnvolles Ergebnis braucht's mindestens `title`):

- `sourceId` – stabile ID innerhalb der Quelle (z.B. RSS-`guid`), Basis für
  die abgeleitete `Event.ID`
- `title`, `description`, `category`, `organizer`, `url`
- `locationName`, `locationAddress`
- **Zeiten**, zwei Wege:
  - `start` (+ optional `end`) mit `format` – empfohlen: parst einen
    Zeitstempel und leitet daraus automatisch korrekt in die Instanz-Zeitzone (`instance.json` → `timezone`)
    umgerechnete `date`/`startTime`/`endTime` ab (`model.LocalParts`,
    per `time.ParseInLocation` – funktioniert sowohl mit Zeitstempeln, die
    eine explizite Zeitzone tragen, als auch mit "naiven" lokalen Strings)
  - `date`, `startTime`, `endTime` direkt – wenn die Quelle diese Teile
    schon einzeln in exakt diesem Format liefert (`YYYY-MM-DD` / `HH:MM`);
    werden 1:1 übernommen (ggf. per `regex` extrahiert), nicht umformatiert
- `price` – eigenes Objekt: `amountPath` (Zahl in Euro, auch als String;
  0/fehlend = kostenlos) und/oder `freePath` (expliziter Boolean, hat
  Vorrang). Mit `"unknownIfMissing": true` bedeutet ein fehlender Betrag
  "Preis unbekannt" (keine Anzeige) statt "kostenlos" — für Quellen, die den
  Preis weglassen, wenn sie ihn nicht kennen.

`format` akzeptiert entweder einen Go-Referenz-Layout-String
(`2006-01-02T15:04:05Z07:00`) oder die Aliase `RFC3339`, `RFC1123Z`
(Standard-RSS-`pubDate`-Format), `RFC1123`.

## Gemeinsame Filter (alle Quelltypen)

Drei Felder gelten unabhängig vom `type` (`rss`, `json`, `wcs`, ...) für jede
Quelle gleichermaßen — durchgesetzt von `withCommonFilters` in
`internal/sources/filter.go`, das sich in `source.go` → `All()` um jede
Quelle wickelt:

```jsonc
{
  "daysAhead": 30,                       // optional: Fenster ab heute
  "categoryMap": { "Kino": "Kultur" },   // optional: Quell-Kategorie -> Instanz-Kategorie
  "categories": ["Kultur", "Markt"]      // optional: Kategorie-Allowlist
}
```

- `daysAhead` – lässt Events raus, die mehr als N Tage in der Zukunft liegen
  (Instanz-Zeitzone). Ohne dieses Feld: kein Fenster, es bleibt, was die Quelle
  liefert. Bei `wcs`-Quellen steuert es zusätzlich das Zeitfenster der
  Anfrage selbst, bei paginierten `json`-Quellen das Ende der Paginierung.
- `categoryMap` – übersetzt die Kategorie-Namen der Quelle in das Vokabular
  der Instanz (`categories.json`), Schlüssel ohne Rücksicht auf
  Groß-/Kleinschreibung. Nicht zugeordnete Werte bleiben, wie sie sind (und
  erscheinen dann mit Ersatz-Piktogramm, siehe unten).
- `categories` – Allowlist auf das aufgelöste `category`-Feld (nach Mapping
  und `categoryMap`). Ohne dieses Feld: keine Einschränkung.
- **Bereits vergangene Events werden immer verworfen**, unabhängig von
  diesen beiden Feldern — das ist kein Feature-Flag, sondern fixes Verhalten
  für einen Event-Kalender.

Beide Felder sind rein optional und wirken erst nach dem eigentlichen
Abruf/Mapping, treffen also für `rss`, `json` und `wcs` gleichermaßen zu
(und auch für künftige, handgeschriebene Sources, sofern sie über `All()`
mit `harden(withCommonFilters(...))` registriert werden).

### Beispiel: RSS-Quelle — `example-instance/sources/beispiel-rss.example.json`

```jsonc
{
  "type": "rss",
  "url": "https://example.org/events/feed",
  "mapping": {
    "sourceId": { "path": "guid" },
    "title": { "path": "title" },
    "description": { "path": "description" },
    "url": { "path": "link" },
    "category": { "path": "category", "default": "Kultur" },
    "organizer": { "default": "Beispiel-Kulturzentrum" },
    "locationName": { "default": "Beispiel-Kulturzentrum" },
    "start": { "path": "pubdate", "format": "RFC1123Z" }
  }
}
```

(Kein `"name"` nötig — der Dateiname `beispiel-rss` wird zum
Quellen-Namen.)

### Beispiel: JSON-API — `example-instance/sources/beispiel-json.example.json`

```jsonc
{
  "type": "json",
  "url": "https://example.org/api/events",
  "itemsPath": "data.items",
  "mapping": {
    "sourceId": { "path": "id" },
    "title": { "path": "name" },
    "description": { "path": "text" },
    "url": { "path": "detailUrl" },
    "category": { "path": "tags.0", "default": "Unkategorisiert" },
    "organizer": { "path": "organizer.name" },
    "locationName": { "path": "location.title" },
    "locationAddress": { "path": "location.address" },
    "start": { "path": "startDateTime", "format": "2006-01-02T15:04:05Z07:00" },
    "end": { "path": "endDateTime", "format": "2006-01-02T15:04:05Z07:00" },
    "price": { "amountPath": "price.amount" }
  }
}
```

Eine neue Quelle anbinden: eine der `.example.json`-Dateien in
`example-instance/sources/` nach `<instance>/sources/` kopieren, `.example` aus dem Dateinamen entfernen, URL +
Mapping anpassen, fertig. Kein Redeploy von Go-Code nötig, nur eine neue
Datei.

## wcs-Quellen (WordPress "Weekly Class Schedule"-Plugin)

Manche Kiez-Kalender (z.B. das Familienzentrum Weißensee in der Instanz weissens.ee) laufen auf WordPress
mit dem Plugin "Weekly Class Schedule" von CurlyThemes. Die Wochentabelle auf
der Seite ist ein leeres `<div>`, das per Vue.js clientseitig befüllt wird —
es gibt keine statische HTML-Tabelle und keinen dokumentierten öffentlichen
REST-Endpunkt. Die Daten kommen stattdessen über den WordPress-Standard-
Mechanismus `admin-ajax.php` mit `action=wcs_get_events_json` (gefunden über
den Network-Tab des Browsers, während die Kalenderseite die Wochenansicht
lädt). Das passt nicht ins generische RSS/JSON-Mapping, deshalb gibt's einen
dritten Quelltyp `"type": "wcs"` mit eigenem Go-Parser
(`internal/sources/wcs.go`), aber weiterhin per Config-Datei nutzbar:

```jsonc
{
  "type": "wcs",
  "url": "https://www.familienzentrum-weissensee.de",  // Basis-URL, nicht der Ajax-Endpunkt selbst
  "daysAhead": 30,               // rollierendes Zeitfenster ab heute, Default 30
  "organizer": "Familienzentrum Weißensee",   // Plugin liefert keinen Organizer
  "locationDefault": "Familienzentrum Weißensee, Mahlerstraße 4",
  "wcsTypeIds": [27, 28, 29, 30],  // s.u. – WICHTIG, nicht weglassen
  "familyFriendly": true,          // gilt für JEDES Event dieser Quelle, siehe unten
  "categoryMap": {                 // wcs_type-Slug -> unsere Kategorie (für Icons)
    "austausch": "Unkategorisiert",
    "beratung": "Unkategorisiert",
    "bewegung": "Sport",
    "kreatives": "Kultur"
  }
}
```

`familyFriendly` ist hier Config-Ebene, nicht pro Event: die Plugin-API gibt
kein Signal dafür pro Termin her, aber dass das Familienzentrum Weißensee
als Ganzes ein Familien-/Elternzentrum ist, ist ein verifizierter Fakt über
die Quelle selbst, keine Vermutung. Bei einer `wcs`-Quelle, deren Angebot
nicht durchgängig familienorientiert ist, das Feld einfach weglassen
(Standard `false`). `ticketRequired`/`registrationRequired` liefert diese
Quelle dagegen grundsätzlich nicht (kein Signal in der Plugin-API) — bei
`wcs`-Events immer `false`, siehe "Event-JSON-Schema".

**`wcsTypeIds` ist notwendig, kein Feintuning:** Ohne diesen Filter liefert
der Endpoint auch die Demo-/Testdaten, mit denen das Plugin-Template
ausgeliefert wird (z.B. "Advanced Aqua Zumba", "Chemistry Lab", fiktive
Dozent:innen) — die liegen offenbar noch unbenutzt in der WordPress-
Installation und haben nichts mit dem echten Programm zu tun. Die IDs
findet man, indem man die Kalenderseite der Zielseite im Browser öffnet, im
Network-Tab die `admin-ajax.php?action=wcs_get_events_json`-Anfrage sucht
und die `content[wcs_type][]`-Werte daraus abliest.

**Zeit-Besonderheit:** Die vom Plugin gelieferten `start`/`end`-Strings
tragen ein `+00:00`/`Z`-Suffix, sind aber tatsächlich schon
Ortszeit des Veranstalters (keine echte UTC-Umrechnung) — bestätigt gegen die
auf der Seite veröffentlichten Zeiten (z.B. "Queer Café 16:30–18:00" steht
exakt so in den Rohdaten, eine echte UTC→Ortszeit-Umrechnung hätte die Ziffern
um 2h verschoben). `parseWCSLocal` in `wcs.go` nimmt deshalb bewusst nur die
ersten 19 Zeichen und interpretiert sie direkt als Ortszeit der Instanz (`model.Local`), statt dem
Offset zu vertrauen. Bei einer neuen wcs-Quelle unbedingt gegen die echten
Öffnungszeiten der Seite gegenchecken, ob das dort genauso ist — Plugins
dieser Art sind dafür bekannt, mit Zeitzonen zu schludern.

**Warnung, nicht blind auf andere Quellen übertragen:** Dieser Local-Digits-
Trick ist eine verifizierte Ausnahme für genau dieses eine Plugin, keine
allgemeine Regel. Recherche zu vergleichbaren Aggregator-Projekten
(judell/community-calendar, siehe deren `docs/timezone.md`) bestätigt, warum
das wichtig ist: ein und dieselbe Stadt-Instanz bekommt dort Feeds mit
handfest unterschiedlichen Zeitzonen-Angaben, weil einzelne Plattformen
(Meetup, Eventbrite) plattform-eigene Defaults statt der Orts-Zeitzone in
ihre Zeitstempel schreiben — bei einer ihrer Städte betraf das über 10.000
Events. iCalendar/ICS kennt zudem zwei echte Konventionen: `X-WR-TIMEZONE`
(Kalender-weiter Default, Google-Konvention) und `VTIMEZONE`+`TZID` (pro
Termin, RFC-5545-Standard) — beide kommen in freier Wildbahn vor. Kurz: jede
neue Quelle (insbesondere ein künftiger `ical`/ICS-Quelltyp) braucht ihre
eigene, einzeln verifizierte Zeit-Behandlung. Der Local-Digits-Trick aus
`parseWCSLocal` darf nicht kommentarlos kopiert werden, nur weil er hier
zufällig richtig war.

Abgesagte Termine (`"visible": false` in der Antwort, korrespondiert mit
`meta._wcs_canceled` auf Seiten der Quelle) werden herausgefiltert statt
angezeigt.

## ical-Quellen (iCalendar / .ics)

Für Kalender-Exporte — Google Kalender, Nextcloud, Outlook und die meisten
WordPress-Event-Plugins (Events Manager `/events.ics`, The Events Calendar
`?ical=1`, …). Kein `mapping`-Block nötig, die Felder sind durch den
Standard festgelegt:

```jsonc
{
  "type": "ical",
  "url": "https://example.org/events.ics",
  "daysAhead": 120,                 // Fenster; ohne Angabe 180 Tage für Wiederholungen
  "organizer": "Beispiel-Bühne",    // falls ORGANIZER;CN fehlt
  "locationDefault": "Beispiel-Bühne, Musterstraße 5",  // falls LOCATION fehlt
  "categoryMap": { "Konzert": "Kultur" },
  "familyFriendly": false
}
```

| iCal | Event |
|---|---|
| `SUMMARY` | `title` |
| `DESCRIPTION` | `description` (bereinigt, gekürzt) |
| `URL` | `url` |
| erster Wert von `CATEGORIES` | `category` (über `categoryMap` übersetzen) |
| `ORGANIZER;CN=…` | `organizer`, sonst `organizer` aus der Konfiguration |
| `X-APPLE-STRUCTURED-LOCATION` (`X-TITLE`/`X-ADDRESS`) | `location.name`/`address` |
| sonst `LOCATION` | `location.name`, sonst `locationDefault` |
| `UID` (+ Datum bei Wiederholungen) | stabile ID |
| — | Preis: unbekannt (iCal kennt keinen Preis) |

Zeiten: UTC (`…Z`), `TZID=…` und "schwebende" Zeiten ohne Zone (dann
`X-WR-TIMEZONE` des Kalenders, sonst die Instanz-Zeitzone); ganztägige
Termine (`VALUE=DATE`); `DURATION` statt `DTEND`. Ist das Ende gleich dem
Start (Events Manager macht das, wenn keine Endzeit gepflegt ist), bleibt
die Endzeit leer. Unbekannte Zonennamen (z.B. Windows-Namen) fallen auf die
Instanz-Zeitzone zurück.

Wiederholungen (`RRULE`): `DAILY`, `WEEKLY` (auch mit `BYDAY=TU,TH`),
`MONTHLY` (gleicher Tag im Monat), `YEARLY`, jeweils mit `INTERVAL`,
`COUNT`, `UNTIL`; `EXDATE` und geänderte Einzeltermine (`RECURRENCE-ID`)
werden berücksichtigt. Aufgefaltet wird nur innerhalb des Fensters
(höchstens 500 Termine). Nicht unterstützte Regeln (z.B. "jeder erste
Montag", `BYDAY=1MO` bei `MONTHLY`) liefern nur den ersten Termin und eine
Log-Zeile. Abgesagte Termine (`STATUS:CANCELLED`) werden übersprungen.

## html-Quellen (Seiten ohne Feed)

Für Veranstaltungsseiten ohne RSS/iCal/API: Termine werden per
CSS-Selektor aus dem HTML gelesen. Drei Teile:

1. `itemSelector` – findet jeden Termin-Container auf der Seite.
2. `extract` – zieht benannte Werte aus einem Container. Kurzform
   `"title": "h2"` (Text des ersten Treffers) oder als Objekt:

   | Feld | Bedeutung |
   |---|---|
   | `selector` | CSS-Selektor relativ zum Container (leer = Container selbst) |
   | `attr` | Attribut statt Text, z.B. `"href"` (relative URLs werden aufgelöst) |
   | `html` | inneres HTML statt Text – für Beschreibungen, damit Absätze erhalten bleiben |
   | `all` / `skip` | alle Treffer verbinden (statt nur des ersten), die ersten `skip` überspringen |
   | `regex` | Teil des Werts herausziehen (Gruppe `value`, sonst erste Gruppe) |

3. `mapping` – wie bei `json`-Quellen; `path` verweist auf die Namen aus
   `extract`.

```jsonc
{
  "type": "html",
  "url": "https://example.org/veranstaltungen.html",
  "itemSelector": "#events .event",
  "extract": {
    "day": ".date .day", "month": ".date .month", "year": ".date .year",
    "title": "h2",
    "start": { "selector": "p.time", "regex": "(\d{1,2}:\d{2})" },
    "end":   { "selector": "p.time", "regex": "-\s*(\d{1,2}:\d{2})" },
    "description": { "selector": "p", "all": true, "skip": 2, "html": true }
  },
  "mapping": {
    "title": { "path": "title" },
    "description": { "path": "description" },
    "start": { "template": "{day} {month} {year} {start}", "format": "2 Jan 2006 15:04|2 Jan 2006" },
    "end":   { "template": "{day} {month} {year} {end}", "format": "2 Jan 2006 15:04" }
  }
}
```

Vollständige Vorlage: `example-instance/sources/beispiel-html.example.json`.

Unterstützte Selektoren: `tag`, `*`, `.klasse`, `#id`, `[attr]`,
`[attr=wert]`, `[attr~=wort]`, `[attr*=teil]`, `:first-of-type`,
`:nth-of-type(n)`, Nachfahre (`a b`) und Kind (`a > b`). Keine
Selektor-Listen (`,`) und keine Geschwister-Kombinatoren.

**Datum aus Einzelteilen:** `format` darf mehrere Layouts mit `|` angeben
(`"2 Jan 2006 15:04|2 Jan 2006"`); passt nur ein Layout ohne Uhrzeit, ist der
Termin ganztägig. Deutsche Monatsnamen und -kürzel ("Nov.", "März",
"Dezember") werden bei Layouts mit Monatsnamen verstanden.

**Robustheit:** Findet `itemSelector` nichts, ist das ein Fehler der Quelle
(steht im `status`) – meist hat die Seite ihr Layout geändert, und das soll
auffallen statt still null Termine zu liefern. HTML-Quellen sind von Natur
aus empfindlicher als Feeds: bei einer Seite mit Feed oder API immer diese
nehmen.

## Fehlertoleranz

Jede Quelle, unabhängig vom Typ, wird in `sources.All()` über `harden(...)`
gebaut (`internal/sources/source.go`, `safety.go`). Das soll verhindern,
dass eine einzelne schlecht funktionierende Quelle den ganzen Lauf oder die
Ergebnisse der anderen Quellen mitreißt:

- **Pro-Quelle-Timeout** (`withTimeout`, Standard 20s): eine hängende
  Quelle blockiert nicht das gemeinsame Zeitbudget der anderen. Das äußere
  Timeout in `cmd/fetch/main.go` (5 Minuten) ist nur noch ein grobes
  Backstop, keine primäre Begrenzung mehr.
- **Panic-Recovery** (`withPanicRecovery`): ein Panic in einer Quelle (z.B.
  durch unerwartete Daten in einer Drittanbieter-API) wird zu einem
  normalen, geloggten Fehler für diese eine Quelle statt den gesamten
  Prozess abstürzen zu lassen und die bereits erfolgreich abgerufenen
  Ergebnisse anderer Quellen zu verlieren.
- **Gemeinsame Filter** (`withCommonFilters`): siehe oben.
- **Status-Tracking** (`withStatusTracking`): schreibt nach jedem
  Fetch-Versuch Erfolg/Fehler in die Quelldatei zurück — siehe
  "Health-Status" unten.

Zusätzlich, unabhängig von einzelnen Quellen:

- **`internal/config.Load`** überspringt eine einzelne kaputte
  `<instance>/sources/*.json`-Datei (ungültiges JSON, nicht lesbar) statt den
  gesamten Ladevorgang abzubrechen — ein Tippfehler in einer Datei reißt
  nicht alle anderen, gültigen Quellen mit runter.
- **`internal/writer.validate`** verwirft einzelne Events mit leerem Titel
  oder unplausiblem Datum, statt sie kaputt ins Frontend durchzureichen —
  mit einer geloggten Warnung, wie viele Events das betraf.
- **`internal/writer.Write`** weigert sich weiterhin, eine leere
  Event-Liste zu schreiben (siehe "Wichtige Sicherung" unten) — greift jetzt
  auch, falls nach der Validierung nichts Gültiges übrig bleibt.

Kurz: einzelne kaputte Quellen, kaputte Config-Dateien oder kaputte
Einzel-Events werden übersprungen und geloggt, nie stillschweigend
ignoriert und nie fatal für den ganzen Lauf — außer wirklich **alle**
Quellen liefern nichts Verwertbares, dann wird `<instance>/data/events.json` bewusst
nicht angefasst (siehe unten).

## Health-Status

Ob eine Quelle noch funktioniert, steht direkt in ihrer eigenen Config-Datei
— nicht in einem separaten Report und nicht nur im CI-Log, das nach dem
nächsten Lauf wieder verschwunden ist. Jeder Fetch-Versuch schreibt ein
`"status"`-Feld zurück:

```json
"status": {
  "lastAttempt": "2026-07-13T08:00:12Z",
  "lastSuccess": "2026-07-06T08:00:09Z",
  "lastError": "fetch https://example.org/feed.xml: unexpected status 404 Not Found"
}
```

- `lastAttempt` — Zeitpunkt des letzten Versuchs, egal ob erfolgreich.
- `lastSuccess` — Zeitpunkt des letzten *erfolgreichen* Abrufs. Bleibt bei
  einem fehlgeschlagenen Versuch stehen, sodass sichtbar ist, seit wann eine
  Quelle schon kaputt ist, nicht nur dass der letzte Versuch fehlschlug.
- `lastError` — die Klartext-Fehlermeldung des letzten Versuchs (Timeout,
  404, Parse-Fehler, `panic: ...`). Leer/fehlt bei Erfolg.

Damit wird eine still kaputtgegangene Quelle (Website-Redesign bricht das
Mapping, Feed verschwindet, Endpoint ändert sich) sichtbar, ohne CI-Logs
durchsuchen zu müssen — einfach `git diff` auf die Quelldatei oder ihren
aktuellen Inhalt anschauen. Der Status wird ganz normal mit
`<instance>/data/events.json` zusammen committet (siehe Workflow unten).

Eine Quelle lässt sich mit `"disabled": true` abschalten, ohne die Datei zu
löschen oder umzubenennen (siehe `config/sources/README.md`) — kein
Fetch-Versuch, kein Status-Update, bis das Feld wieder entfernt/auf `false`
gesetzt wird.

Umgesetzt in `internal/config.RecordAttempt` (liest die Quelldatei frisch
von der Platte, aktualisiert nur `status`, schreibt sie zurück) und
`internal/sources/status.go` (`withStatusTracking`, äußerster Wrapper in
`sources.All()`, ruft `RecordAttempt` nach jedem `Fetch()` auf — Erfolg oder
Fehler). Da `RecordAttempt` die ganze Datei über `SourceConfig` neu
serialisiert, bestimmt die Feldreihenfolge im Go-Struct die Reihenfolge im
JSON (stabil über mehrere Läufe, aber nicht zwingend so, wie eine Person die
Datei ursprünglich formatiert hat), und JSON-Schlüssel, die `SourceConfig`
nicht kennt, werden beim Zurückschreiben entfernt.

## Event-JSON-Schema (`<instance>/data/events.json`)

Das Ausgabe-Schema folgt exakt dem, was das Frontend erwartet
(`web/js/app.js`, Demo-Daten in `<instance>/data/events.json`):

```jsonc
{
  "generatedAt": "2026-07-05T15:40:00Z",  // Metadaten, Frontend ignoriert das Feld
  "events": [
    {
      "id": "beispiel-rss-3f9a1b2c4d",  // stabil, aus source.name+sourceId
      "title": "Flohmarkt am Antonplatz",
      "description": "…",
      "category": "Markt",
      "date": "2026-07-07",
      "startTime": "10:00",
      "endTime": "16:00",
      "location": { "name": "Antonplatz" },
      "price": { "free": true, "amount": null },
      "organizer": "Kiezinitiative",
      "url": "https://example.events/#kiezinitiative-3f9a1b2c4d",
      "source": "beispiel-rss",                   // Quellen-Name, Label steht in data/sources.json
      "ticketRequired": false,
      "registrationRequired": false,
      "familyFriendly": false,
      "cancelled": false                          // abgesagt, siehe "Abgesagte Termine"
    }
  ]
}
```

## Abgesagte Termine

Abgesagte Termine werden **nicht** verworfen, sondern mit `"cancelled": true`
ausgeliefert und im Frontend deutlich gekennzeichnet (Etikett "Abgesagt",
durchgestrichener Titel, Hinweis in der Detailansicht, `STATUS:CANCELLED` im
.ics). Wer hingehen wollte, soll es erfahren, statt dass der Termin still
verschwindet. Erkannt wird eine Absage über:

| Quelle | Signal |
|---|---|
| `ical` | `STATUS:CANCELLED`, auch für ein einzelnes Vorkommen einer Serie (`RECURRENCE-ID`) |
| `wcs` | ausgeblendetes Vorkommen (`visible: false`, entspricht `_wcs_canceled`) |
| `rss`/`json` | Mapping-Regel `cancelled`, z.B. `{"path": "status", "equals": "cancelled|canceled"}` |
| Inbox | `"cancelled": true` in der Datei |
| alle | Titel-Markierung, die dabei aus dem Titel entfernt wird: `ABGESAGT: …`, `Abgesagt! …`, `+++ Entfällt +++ …`, `[abgesagt] …`, `… (abgesagt)`, `… – ABGESAGT`, `… - fällt aus!` |

Die Titel-Heuristik verlangt ein Trennzeichen oder Klammern (bzw. Großschreibung
am Anfang): "Abgesagt – ein Theaterstück" bleibt ein normaler Titel.

Merkmal-Regeln (`ticketRequired`, `registrationRequired`, `familyFriendly`,
`cancelled`) lesen den Wert als Boolean; mit `"equals": "a|b"` werden sie
stattdessen wahr, wenn der Wert einem der Texte entspricht (ohne Rücksicht
auf Groß-/Kleinschreibung).

Events sind chronologisch sortiert (Datum, dann Startzeit). Wiederholte Runs
mit unveränderten Daten erzeugen eine byte-identische Datei (sauberer git
diff im Workflow).

`category` ist ein freier String — welches Piktogramm er im Frontend
bekommt, steht nicht hier, sondern in `<instance>/data/categories.json` (siehe
nächster Abschnitt).

`ticketRequired`/`registrationRequired`/`familyFriendly` sind bewusst echte,
immer gesetzte Booleans (kein `omitempty`, kein Pointer) - querschnittliche
Attribute, keine Kategorien: ein Event kann frei kombinieren, unabhängig
davon, welches `category` es hat (siehe "Kategorien vs. Attribute" unten).
Sie speisen die "Filtern nach"-Zeile im Frontend (`web/js/app.js`
`FILTER_OPTIONS`), nicht das Kategorie-Menü. Quell-seitig gesetzt über
`mapping.BoolRule` (`ticketRequired`/`registrationRequired`/`familyFriendly`
in einer `rss`/`json`-Quelle: `{"path": "..."}`, Standard `false` wenn
`path` fehlt oder nicht auflöst) bzw. für `wcs`-Quellen über
`familyFriendly` auf Config-Ebene (siehe `wcs-Quellen` unten) - die
Plugin-API liefert kein Ticket-/Anmeldungssignal pro Event, daher sind
`ticketRequired`/`registrationRequired` bei `wcs`-Quellen aktuell immer
`false` (bekannte Einschränkung, kein Fake-Wert).

## Kategorie-Piktogramm-Schnittstelle (`<instance>/data/categories.json`)

Früher stand die Kategorie-Icon-Zuordnung doppelt und unabhängig
voneinander im Code: einmal als `categoryMap`-Werte in Quell-Configs
(`<instance>/sources/`), einmal hartkodiert als `CATEGORY_ICON_PATHS` direkt in
`web/js/app.js` — nichts hielt beide synchron, und eine neue Kategorie aus einer
neuen Quelle tauchte im Frontend nie automatisch mit einem eigenen Icon auf.

Jetzt ist das eine bewusst einseitige Schnittstelle: das Backend ist alleiniger
Schreiber, das Frontend liest nur zur Darstellung.

- **`<instance>/categories.json`** ist die kanonische Registry (geladen von
  `internal/categories`): Name →
  SVG-Pfaddaten (1:1 aus den offiziellen Tabler Icons), die Reihenfolge der
  Einträge ist die Anzeige-Reihenfolge, `fallback` ist z.B. `"Unkategorisiert"`.
- **`internal/writer.WriteCategories`** schreibt daraus, zusammen mit den
  tatsächlich im aktuellen Lauf vorkommenden Events,
  `<instance>/data/categories.json` — immer mit demselben, von `writer.Write`
  zurückgegebenen finalen Event-Set wie `<instance>/data/events.json`, damit beide
  Dateien nie auseinanderlaufen.
- **`web/js/app.js`** lädt `<instance>/data/categories.json` per `fetch()` und baut daraus
  sowohl das Kategorie-Filtermenü als auch die Icons auf den Event-Karten —
  es enthält selbst keine Kategorie-Icon-Liste mehr, nur noch einen einzigen
  lokalen Notfall-Fallback für den Fall, dass die Datei mal nicht geladen
  werden kann.

```jsonc
{
  "generatedAt": "2026-07-15T08:00:00Z",
  "categories": [
    { "name": "Kultur", "icon": ["M12 21a9 9 0 0 1 0 -18…", "…"] },
    { "name": "Sport", "icon": ["…"] }
    // … die restlichen Order-Einträge, dann ggf. neu entdeckte Kategorien
  ]
}
```

**Kategorien vs. Attribute:** Die Kategorie-Liste in `<instance>/categories.json`
ist bewusst auf eine einzige Achse vereinheitlicht - Themenbereich, nicht
Format oder Zielgruppe, in der Vorlage: `Kultur, Sport, Kulinarik, Natur,
Markt, Unkategorisiert`. Frühere Einträge wie `Konzert` oder `Workshop` waren
eigentlich Veranstaltungsformate (immer ein Thema darunter, z.B. ein
Konzert ist ein Kultur-Format), keine eigene Themenebene - die gibt es jetzt
nicht mehr als Kategorie, sondern stecken im Titel/der Beschreibung.
`Familie & Kinder` war eine Zielgruppe, keine Kategorie - dafür gibt es jetzt
das eigene Attribut `familyFriendly` (siehe oben und "Event-JSON-Schema"),
in der Frontend-"Filtern nach"-Zeile statt im Kategorie-Menü.

**"Kategorien wachsen dynamisch"**: `Build()` (`internal/categories/categories.go`)
schreibt immer zuerst alle `Order`-Einträge, unabhängig davon, ob gerade ein
Event dieser Kategorie existiert — die Menüleiste bleibt dadurch stabil,
statt bei jedem Lauf zu schrumpfen oder zu wachsen, je nachdem was gerade im
Kalender steht. Taucht in den Events zusätzlich eine `category` auf, die
`Registry` nicht kennt (neue Quelle, vergessener `categoryMap`-Eintrag bei
einer `wcs`-Quelle, Tippfehler), wird sie alphabetisch sortiert angehängt,
bekommt vorübergehend das `Fallback`-Icon und erzeugt eine geloggte Warnung
— der Lauf schlägt dafür nicht fehl, aber es ist sichtbar, statt sich
still unter "Unkategorisiert" zu verstecken. Um ihr ein eigenes Icon zu geben:
Eintrag in `<instance>/categories.json`
ergänzen, kein Frontend-Change nötig.

## Wichtige Sicherung: keine leeren Daten committen

`internal/writer.Write` weigert sich, eine leere Event-Liste zu schreiben,
und `cmd/fetch` überspringt das Schreiben komplett, solange `sources.All()`
leer ist oder alle Quellen fehlschlagen. Damit überschreibt weder der
aktuelle Platzhalter-Zustand (nur `.example.json`-Dateien, keine aktive
Quelle) noch ein Ausfall aller Quellen die bestehenden Demo-/Produktivdaten
in `<instance>/data/events.json` mit einer leeren Datei.

## Neue Quellen finden

Wie man überhaupt neue Quellen aufspürt (Suchmuster, bekannte Plattform-
URL-Muster, Vorgehen wenn kein offizieller Feed existiert): siehe
`docs/source-discovery.md` — bewusst ortsunabhängig geschrieben, falls
jemand eine Instanz für eine andere Nachbarschaft aufsetzt. Der aktuelle
Stand je Instanz (gefunden/geprüft/verworfen) gehört in
die Quellen-Checkliste der jeweiligen Instanz.

## Neue Quelle mit eigenem Go-Code (Ausnahmefälle)

Für Formate außerhalb RSS/JSON/wcs: `internal/sources/example.go` als
Vorlage kopieren, `Source`-Interface implementieren (`Name()`,
`Fetch(ctx) ([]model.Event, error)`), in `internal/sources/source.go` am
Ende von `All()` per `append(all, harden(withCommonFilters(...)))`
registrieren (damit die neue Quelle dieselbe Fehlertoleranz und dieselben
Filter bekommt wie die eingebauten Typen). `model.LocalParts(t time.Time)`
für die Zeit-Konvertierung nutzen, `Price` immer explizit setzen (Zero-Value
wäre `{free: false, amount: null}` → "null €" im Frontend).

## Manuelle Events: die Inbox (`<instance>/inbox/`)

Gegenstück zu den automatisierten Quellen, für Einzelevents ohne Feed (ein
Flyer, ein Tipp aus dem Kiez): JSON-Datei nach `<instance>/inbox/` legen und
committen — kein Eintrag in `<instance>/sources/` nötig. `InboxSource`
(`internal/sources/inbox.go`) liest jede `*.json`-Datei dort (außer
`*.example.json`) ein und wandelt sie in ein Event um; Format und Ablauf
stehen in `docs/inbox.md`.

Bemerkenswert: `InboxSource` löscht die Datei selbst, aber erst, sobald das
Event-Datum in der Vergangenheit liegt — nicht sofort nach der ersten
Verarbeitung. Grund: `cmd/fetch` berechnet `<instance>/data/events.json` bei jedem Lauf
komplett neu aus den aktuellen `Fetch()`-Ergebnissen aller Quellen, ohne die
vorherige Datei einzulesen. Eine sofort gelöschte Inbox-Datei würde das
Event schon beim nächsten Lauf wieder verschwinden lassen, weil dann keine
Quelle mehr existiert, die es liefert. Die Datei bleibt also liegen (und
wird jedes Mal neu eingelesen, das Event bleibt sichtbar), bis ihr eigenes
Datum abgelaufen ist — genau der Punkt, an dem `withCommonFilters` das Event
ohnehin verwerfen würde. Diese Löschung committet der `update-events.yml`-Workflow der Instanz mit,
wie jede andere Änderung des Laufs auch.

## Lokal ausführen

Mit Go:

```sh
cd backend
go run ./cmd/fetch ../example-instance                       # oder $INSTANCE_DIR
go run ./cmd/build-site -web ../web ../example-instance      # -> example-instance/public/
```

Ohne Go, nur mit Docker/Podman (Build-Kontext ist das Repo-Root):

```sh
docker build -t openlitfass -f Containerfile .
docker run --rm -v "$PWD/example-instance:/instance" -w /instance openlitfass fetch .
docker run --rm -v "$PWD/example-instance:/instance" -w /instance openlitfass build-site .
docker run --rm -p 8080:80 -v "$PWD/example-instance/public:/usr/share/nginx/html:ro" nginx:alpine
```

`fetch` verändert das Instanz-Verzeichnis (Status-Felder in `sources/`,
abgelaufene `inbox/`-Dateien werden gelöscht) — deshalb verlangt es eine
`instance.json` im Zielverzeichnis. Solange `sources/` keine aktive Quelle
und `inbox/` kein Event enthält, schreibt es absichtlich nichts.

`build-site` leert sein Ausgabeverzeichnis vor jedem Lauf, aber nur, wenn
es das Verzeichnis selbst angelegt hat (Marker-Datei
`.openlitfass-build`).

## Workflows (GitHub Actions)

**Kern: `.github/workflows/ci.yml`** — bei jedem Push und Pull Request:
`gofmt`, `go vet`, `go build`, `go test` und ein kompletter
`build-site`-Lauf gegen `example-instance/`.

**Kern: `.github/workflows/image.yml`** — bei Push auf `main` (nur wenn sich
`backend/`, `web/` oder das `Containerfile` ändern), bei Tags `v*` oder
manuell. Baut das Image für `linux/amd64` und `linux/arm64` und pusht es
nach `ghcr.io/<owner>/<repo>`:

| Tag | wann |
|---|---|
| `latest` | jeder Build von `main` |
| `sha-<kurz>` | jeder Build, zum Festpinnen |
| `1.2.3`, `1.2` | Git-Tag `v1.2.3` |

Gepusht wird mit dem eingebauten `GITHUB_TOKEN` (`permissions: packages:
write`), ein eigenes Token ist nicht nötig. Damit Instanzen das Image ohne
Login ziehen können, das Package einmalig auf **public** stellen (GitHub →
Packages → openlitfass → Package settings → Change visibility).

Im `Containerfile` läuft die Go-Stufe immer auf der Architektur des
Build-Rechners und cross-kompiliert für das Ziel (`--platform=$BUILDPLATFORM`,
`GOARCH=$TARGETARCH`) — deutlich schneller als unter QEMU.

**Instanz: `.github/workflows/update-events.yml`** (Vorlage in
`example-instance/`) — Cron alle 6 Stunden, manuell, und bei Push auf
`sources/`, `inbox/`, `site/`, `instance.json`, `categories.json`. Startet
das Image per `docker run` (nicht als Job-Container: GitHubs
JavaScript-Actions wie `checkout` laufen nicht in Alpine-Containern),
führt `fetch` und `build-site` aus und committet `data/`, `sources/`
(Status) und `inbox/` (gelöschte Dateien) zurück. Läuft genauso auf
Forgejo/Gitea-Runnern mit Docker. Das Veröffentlichen von `public/` kommt
dazu, sobald das Hosting feststeht.

**Vor dem ersten Lauf einer Instanz:** in `update-events.yml` `IMAGE` auf
`ghcr.io/<owner>/openlitfass:<tag>` setzen.

## Teststand

- Lokal mit Docker gebaut und geprüft: `go build`, `go vet`, `gofmt` sauber.
- `fetch` gegen die echte wcs-Quelle Familienzentrum Weißensee: 95 Termine,
  Status-Feld, `sources.json` korrekt; Inbox-Event wird übernommen.
- `build-site` gegen `example-instance` und weissens.ee: vollständige Seite
  inkl. Manifest, `site.json` und gestempeltem Service Worker.
- `json` gegen eine echte Quelle: Brotfabrik (WordPress "The Events Calendar",
  4 Seiten, 192 Termine in 30 Tagen) mit Paginierung, Template-Adresse,
  `categoryMap` und HTML-Bereinigung.
- Go-Unit-Tests für Text-Bereinigung, Template, Preis, Paginierung,
  Filter/`categoryMap` und das Zurückschreiben der Quelldateien.
- `ical` gegen einen echten Events-Manager-Export (Freilichtbühne Weißensee) als
  Test-Fixture plus Tests für Zeitzonen, ganztägig, RRULE, EXDATE, RECURRENCE-ID.
- `html` mit einer selbst geschriebenen Fixture im Aufbau einer Joomla-JEvents-Liste;
  Selektor-Engine, deutsche Monatsnamen, ganztägige Termine, Absage-Erkennung im Titel.
- `rss` ist noch nicht gegen eine echte Quelle gelaufen.
