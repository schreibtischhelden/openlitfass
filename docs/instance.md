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
  .forgejo/workflows/update-events.yml
```

## instance.json

```jsonc
{
  "name": "weissens.ee",                       // Pflicht, Schriftzug im Header
  "title": "weissens.ee – Events in Weißensee", // <title>, Standard: name
  "description": "Der Event-Kalender für …",   // <meta name="description">
  "baseUrl": "https://weissens.ee",            // Fallback-Link für Events ohne URL
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

- Liste nach Tagen gruppiert, Kategorie-Menü, Schnellfilter (Heute,
  Wochenende, Kostenlos, Familie, Ticket, Anmeldung), Suche
- Detailansicht je Termin mit teilbarer Adresse (`/#<event-id>`), Kartenlink
  (OpenStreetMap), "In Kalender" (.ics) und "Teilen"
- "Gemerkt": Sterne werden nur lokal im Browser gespeichert, kein Konto
- Über-Seite mit `about`, Quellenliste, "Termin melden", Impressum/Datenschutz
- PWA: `build-site` erzeugt `manifest.webmanifest`, `sw.js` hält die Seite
  und die zuletzt geladenen Termine offline vor. Für installierbare Icons
  `site/icon-192.png` und `site/icon-512.png` mitliefern.
- Hell/Dunkel automatisch nach Systemeinstellung

## Neue Instanz in 5 Schritten

1. `example-instance/` aus diesem Repo in ein neues, leeres Repo kopieren.
2. `instance.json` ausfüllen, Impressum und Datenschutz in `site/` anpassen.
3. Quellen anlegen: Vorlage in `sources/` kopieren, `.example` aus dem Namen
   entfernen, URL + Mapping anpassen (Syntax: `docs/backend.md`). Wie man
   Quellen findet: `docs/source-discovery.md`.
4. Gestaltung: `site/css/theme.css` (Tokens aus `web/css/tokens.css`
   überschreiben) und optional `site/logo.svg`.
5. In `.forgejo/workflows/update-events.yml` Runner-Label und Image-Pfad
   anpassen, pushen.

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
