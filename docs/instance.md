# Eine Instanz aufsetzen

Eine **Instanz** ist ein eigenes Repository für einen konkreten Kalender
(z.B. `weissens.ee` für Berlin-Weißensee). Sie enthält nur Inhalte und
Gestaltung, keinen Code. Den Code liefert das Container-Image dieses
Kern-Repos.

## Aufbau

```
<instance>/
  instance.json                    Pflicht: Name, Titel, Zeitzone, ...
  categories.json                  Kategorien + Piktogramme (Reihenfolge = Menü)
  sources/*.json                   eine Datei pro Quelle (*.example.json = Vorlage)
  inbox/*.json                     Einzel-Events von Hand (siehe docs/inbox.md)
  site/                            wird über web/ gelegt
    css/theme.css                  Farb-Tokens überschreiben
    logo.svg                       Logo (inline eingefügt, currentColor möglich)
    …                              beliebige weitere Dateien (Fonts, Favicon, …)
  data/                            von fetch erzeugt (events, categories, sources), wird committet
  public/                          von build-site erzeugt, nicht committen
  .github/workflows/update-events.yml
```

## instance.json

```jsonc
{
  "name": "weissens.ee",                       // Pflicht, Schriftzug im Header
  "title": "weissens.ee – Events in Weißensee", // <title>, Standard: name
  "description": "Der Event-Kalender für …",   // <meta name="description">
  "baseUrl": "https://weissens.ee",            // öffentliche Adresse: Sitemap, Canonical
  "timezone": "Europe/Berlin",                 // IANA-Zone, Standard Europe/Berlin
  "language": "de",                            // <html lang>, Standard de
  "themeColor": "#17383D",                     // theme-color + PWA-Farben
  "shortName": "weissens.ee",                  // Name unter dem App-Icon, Standard: name
  "about": "Absatz 1 …\n\nAbsatz 2 …",         // Text der Über-Seite
  "imprintUrl": "impressum.html",              // Impressum (Pflicht in DE!)
  "privacyUrl": "datenschutz.html",            // Datenschutzerklärung
  "submitUrl": "mailto:termine@weissens.ee"    // "Termin melden" auf der Über-Seite
}
```

`imprintUrl`/`privacyUrl` dürfen absolut sein oder relativ auf eine Seite
in `site/` zeigen (Vorlagen: `example-instance/site/impressum.html`,
`datenschutz.html`). Ohne Impressum darf eine öffentliche Seite in
Deutschland nicht betrieben werden.

## Quellen-Anzeige

Jede Quelldatei kann `"label"` (Anzeigename) und `"homepage"` (Website des
Veranstalters) setzen. Beides erscheint im Frontend als "Quelle: …" in der
Detailansicht und in der Quellenliste der Über-Seite (`data/sources.json`,
von `fetch` erzeugt).

## Frontend

Die Standard-Oberfläche (`web/`) bringt mit:

- Eine Werkzeugzeile unter dem Kopf als Zusammenfassung:
  `[‹] [Datum] [›] … [gesetzte Filter] [Filter]`. Die Pfeile blättern um
  den gewählten Zeitraum, das Datum öffnet die native Datumsauswahl;
  startet auf "Heute", nie vor heute. Das Datum zeigt den Zeitraum
  (immer Wochentag + Datum, z. B. "Sa, 21.11." oder "21.–27.11.", damit
  die Breite beim Blättern gleich bleibt). Gesetzte Filter erscheinen als kleine
  Symbole (Antippen entfernt den Filter), der Filter-Knopf zeigt ihre
  Anzahl und klappt das Filter-Panel auf. Passt die Zeile nicht, weichen
  zuerst die Symbole.
- Filter-Panel mit Gruppen: Zeitraum (Tag / Wochenende / 7 Tage ab dem gewählten Datum),
  Kategorien aus `categories.json`, Merkmale (Kostenlos, Familie, Ticket,
  Anmeldung), als Chips mit Symbol und Text, je Filtergruppe
  "zurücksetzen"; Escape schließt es. Der Zeitraum steht in der Adresse
  (`#tag/2026-11-22`, `#wochenende/…`, `#woche/…`); ist er leer, springt
  "Nächster Termin" zum nächsten passenden Tag. Weitere Filtergruppen
  kommen als weitere `.fp-group` dazu, ohne die Zeile zu verbreitern.
- Zeitstatus für heute: laufende Termine sind grün getönt mit Etikett
  "Läuft", beendete stärker abgestuft (entsättigt, gedämpft, gestrichelter
  Rand - auch im Dunkelmodus erkennbar). Ohne Endzeit gilt ein Termin
  2 Stunden als laufend, ganztägige den ganzen Tag. Die Liste scrollt
  beim Öffnen zum ersten noch nicht beendeten Termin und frischt den
  Status jede Minute auf.
- Liste nach Tagen gruppiert (Tagesüberschriften bleiben unter der
  Werkzeugzeile stehen), Suche über das Lupen-Symbol (gilt über alle
  Daten, nicht nur den Zeitraum)
- Detailansicht je Termin mit teilbarer Adresse (`/#<event-id>`), Kartenlink
  (OpenStreetMap), "In Kalender" (.ics) und "Teilen". Adressen in der
  Beschreibung werden zu Links. Externe Links (Veranstaltung, Karte,
  Quellen, Adressen in Beschreibungen) öffnen in einem neuen Tab und tragen
  dasselbe Symbol wie "Zur Veranstaltung".
- "Gemerkt": jede Karte hat einen Stern (grau = nicht gemerkt, gefüllt =
  gemerkt), der direkt aus der Liste merkt, ohne die Detailansicht zu
  öffnen. Gespeichert wird nur lokal im Browser, kein Konto.
- Über-Seite mit `about`, Quellenliste, "Termin melden", Impressum/Datenschutz
- PWA: `build-site` erzeugt `manifest.webmanifest`, `sw.js` hält die Seite
  und die zuletzt geladenen Termine offline vor. Für installierbare Icons
  `site/icon-192.png` und `site/icon-512.png` mitliefern.
- Hell/Dunkel: ohne Wahl nach Systemeinstellung; der Schalter im Kopf
  (Mond/Sonne) legt das Schema fest und merkt es lokal. Entspricht die Wahl
  wieder dem System, wird sie gelöscht und die Seite folgt wieder dem
  System. Gesetzt wird `data-theme="dark"|"light"` auf `<html>`. **Eigene
  Dark-Regeln in `site/css/theme.css`** deshalb so schreiben:
  `@media (prefers-color-scheme: dark) { :root:not([data-theme="light"]) { … } }`
  plus `[data-theme="dark"] { … }` - sonst gewinnt bei "Hell" das System.
- "Teilen" öffnet auf dem Handy das System-Teilen-Menü (Web Share API,
  nur über HTTPS bzw. localhost); ohne die API wird der Link kopiert.
  Geteilt wird die statische Terminseite (siehe unten), damit Messenger
  eine Vorschau mit Titel und Datum zeigen.

### Statische Seiten und SEO

`build-site` erzeugt neben der App reines HTML, für Suchmaschinen,
Link-Vorschauen und Besucher ohne JavaScript:

- `termine/index.html`: alle kommenden Termine nach Tagen
- `termine/<id>.html`: je Termin eine Seite mit Titel, Beschreibung,
  Open-Graph-Tags und schema.org-`Event` (JSON-LD, inkl. abgesagt-Status)
- `sitemap.xml` und `robots.txt` (nur mit `baseUrl`, da Sitemaps absolute
  Adressen brauchen; ein eigenes `site/robots.txt` hat Vorrang)
- `sitemap.xml` ist reines sitemaps.org-XML ohne Stylesheet, damit jeder
  Browser seine XML-Baumansicht zeigt (Firefox zeigt mit Stylesheet-Verweis
  bei blockiertem XSLT nur Text, und XSLT verschwindet aus den Browsern).
  Die lesbare Liste für Menschen ist `termine/`. Eine XSL-Fassung liegt in
  der Git-Historie (Commit ab98aff) - denkbar später nur mit JavaScript
  über https://github.com/zcorpan/xslt_extension.
- `index.html` (also `/`) enthält die heutigen Termine schon im HTML (gibt
  es heute keine: die des nächsten Tages mit Terminen) plus Link auf
  `termine/`, dazu Canonical- und Open-Graph-Tags. Die App ersetzt die
  Liste, sobald sie geladen hat. "Heute" ist der Stand des letzten Builds:
  `serve` baut nach jedem Abruf neu, über Mitternacht hinweg also erst
  mit dem nächsten Abruf.

Die Vorlagen liegen in `web/templates/` (Go `html/template`) und werden
nicht mit ausgeliefert. Vergangene Termine bekommen keine Seite mehr.

## Neue Instanz in 5 Schritten

1. `example-instance/` aus diesem Repo in ein neues, leeres Repo kopieren.
2. `instance.json` ausfüllen, Impressum und Datenschutz in `site/` anpassen.
3. Quellen anlegen: Vorlage in `sources/` kopieren, `.example` aus dem Namen
   entfernen, URL + Mapping anpassen (Syntax: `docs/backend.md`). Wie man
   Quellen findet: `docs/source-discovery.md`.
4. Gestaltung: `site/css/theme.css` (Tokens aus `web/css/tokens.css`
   überschreiben) und optional `site/logo.svg`.
5. In `.github/workflows/update-events.yml` den Image-Pfad (`IMAGE`)
   anpassen, pushen.

## Betrieb: eine Instanz je Container

Der einfachste Betrieb – ohne CI, ohne separaten Webserver: das
Instanz-Verzeichnis in den Container hängen, fertig.

```sh
docker run -d --name weissens --restart unless-stopped \
  -v /pfad/zu/weissens.ee:/instance \
  -p 8080:8080 \
  ghcr.io/<owner>/openlitfass
```

Der Standardbefehl des Images ist `serve /instance`:

- liefert die Seite auf Port 8080 aus – aus den vorhandenen Daten schon
  Sekunden nach dem Start,
- ruft sofort und dann alle `FETCH_INTERVAL` (Standard `6h`) alle Quellen ab
  und baut die Seite neu; jeder Build landet in einem neuen Verzeichnis unter
  `.openlitfass-serve/` und wird erst live geschaltet, wenn er vollständig ist
  (ein fehlgeschlagener Build lässt die alte Version stehen),
- schreibt dabei wie `fetch` in das Instanz-Verzeichnis: `data/`, das
  `status`-Feld der Quellen, abgelaufene Inbox-Dateien werden gelöscht,
- `/healthz` liefert den Zustand als JSON (letzter Abruf, Anzahl Termine,
  ausgefallene Quellen, nächster Abruf) und dient dem `HEALTHCHECK` des
  Images; `/metrics` für Prometheus, siehe unten.

| Variable / Flag | Standard | Bedeutung |
|---|---|---|
| `FETCH_INTERVAL` / `-interval` | `6h` | Abruf-Intervall; `0` = nur ausliefern |
| `ADDR` / `-addr` | `:8080` | Listen-Adresse |
| `PUBLIC_DIR` / `-public` | `<instance>/.openlitfass-serve` | Build-Verzeichnis |
| `METRICS_ADDR` / `-metrics-addr` | leer | eigener Port für `/metrics`, z.B. `:9090` |

### Metriken (Prometheus)

`/metrics` liefert Kennzahlen im Prometheus-Textformat (geprüft mit
`promtool check metrics`). Mit `METRICS_ADDR=:9090` laufen sie auf einem
eigenen Port und sind **nicht** neben der öffentlichen Seite erreichbar –
empfohlen; ohne `METRICS_ADDR` liegt `/metrics` auf dem Haupt-Port.

```sh
docker run -d … -p 8080:8080 -p 127.0.0.1:9090:9090 -e METRICS_ADDR=:9090 ghcr.io/<owner>/openlitfass
```

| Metrik | Typ | Bedeutung |
|---|---|---|
| `openlitfass_info{version,instance}` | gauge | immer 1, trägt Version und Instanz |
| `openlitfass_up` | gauge | 1 = Seite wird ausgeliefert |
| `openlitfass_fetch_interval_seconds` | gauge | eingestelltes Abruf-Intervall |
| `openlitfass_last_fetch_timestamp_seconds` / `…_next_fetch_…` / `…_last_build_…` | gauge | Zeitpunkte (Unix) |
| `openlitfass_fetch_runs_total{result}` | counter | Abrufläufe: `ok`, `partial` (einzelne Quellen ausgefallen), `failed`, `nothing`, `error` |
| `openlitfass_builds_total{result}` | counter | Seiten-Builds: `ok`, `failed` |
| `openlitfass_http_requests_total{code}` | counter | Anfragen nach Statusklasse (`2xx` …) |
| `openlitfass_fetch_duration_seconds` | gauge | Dauer des letzten Abruflaufs |
| `openlitfass_source_up{source}` | gauge | 1 = letzter Abruf der Quelle erfolgreich (aus dem `status`-Feld, stimmt also auch, wenn CI abruft) |
| `openlitfass_source_last_success_timestamp_seconds{source}` | gauge | letzter Erfolg der Quelle |
| `openlitfass_source_fetch_duration_seconds{source}` | gauge | Dauer des letzten Abrufs der Quelle |
| `openlitfass_source_fetched_events{source}` | gauge | Termine der Quelle im letzten Lauf (vor dem Zusammenführen) |
| `openlitfass_source_carried_events{source}` | gauge | wegen Ausfall aus dem Vorlauf übernommene Termine |
| `openlitfass_events` / `…_cancelled` / `…_merged` | gauge | veröffentlichte Termine: gesamt, abgesagt, aus mehreren Quellen |
| `openlitfass_events_by_source{source}` / `…_by_category{category}` | gauge | veröffentlichte Termine je Quelle / Kategorie |

Scrape-Konfiguration:

```yaml
scrape_configs:
  - job_name: openlitfass
    static_configs:
      - targets: ["weissens-host:9090"]
```

Beispiel-Alarme:

```yaml
groups:
  - name: openlitfass
    rules:
      - alert: OpenLitfassSourceDown
        expr: time() - openlitfass_source_last_success_timestamp_seconds > 86400
        labels: { severity: warning }
        annotations: { summary: "Quelle {{ $labels.source }} seit über einem Tag ohne Erfolg" }
      - alert: OpenLitfassNoFetch
        expr: time() - openlitfass_last_fetch_timestamp_seconds > 2 * openlitfass_fetch_interval_seconds and openlitfass_fetch_interval_seconds > 0
        labels: { severity: warning }
        annotations: { summary: "{{ $labels.instance }}: kein Abruf seit zwei Intervallen" }
      - alert: OpenLitfassDown
        expr: openlitfass_up == 0
        for: 5m
        labels: { severity: critical }
```

Die Version im Image setzt `image.yml` (Build-Argument `VERSION`); lokal
gebaute Images melden `dev`.

HTTPS, eigene Domain: einen Reverse-Proxy davor setzen (Caddy, Traefik,
nginx). `serve` setzt passende Cache-Header (Daten, Startseite und Service
Worker werden nie gecacht).

Wer die Daten zusätzlich im Git-Repo der Instanz versionieren möchte, nimmt
stattdessen (oder zusätzlich) den CI-Workflow `update-events.yml`; mit
`FETCH_INTERVAL=0` liefert `serve` dann nur aus.

## Lokal testen

Siehe `docs/backend.md` → "Lokal ausführen", mit dem Pfad der eigenen
Instanz statt `example-instance`.

## Kern aktualisieren

Instanzen ziehen das Image `…/openlitfass:latest`. Für
reproduzierbare Läufe stattdessen einen Commit-Tag pinnen
(`…/openlitfass:<sha>`) und bewusst hochziehen.

## Grenzen der Anpassung

Texte der Oberfläche (Filter-Labels, Leerzustand, …) sind derzeit auf
Deutsch; sie stehen gesammelt in `STR` am Anfang von `web/js/app.js`. Mehrsprachigkeit
bzw. instanzspezifische Texte sind noch nicht umgesetzt.
