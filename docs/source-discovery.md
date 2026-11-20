# Quellen finden (Discovery-Playbook)

Diese Anleitung ist bewusst **ortsunabhängig** geschrieben und gilt für jede
Instanz von openLitfass. Die ortsspezifische Bestandsaufnahme
(was schon gefunden/geprüft ist) gehört in die Instanz selbst, z.B.
`docs/quellen-checkliste.md` in weissens.ee.

Basiert auf Recherche zu vergleichbaren Open-Source-Projekten, vor allem
[judell/community-calendar](https://github.com/judell/community-calendar)
(`docs/procedures.md`) — dort im Detail nachlesbar, hier auf das Nötigste für
unseren Kontext (Go, `rss`/`json`/`wcs`-Quelltypen, siehe `docs/backend.md`)
verdichtet.

## Grundhaltung: vollständig statt kuratiert

Lieber eine Quelle zu viel als eine zu wenig. Kleine, unscheinbare Quellen
sind oft ergiebiger als sie aussehen — Vereine, Kirchengemeinden, Schulen,
Sportvereine, Nachbarschaftsinitiativen, Repair-Cafés, Kiezzeitungen. Im
Zweifel aufnehmen und später per `categories`/`daysAhead`-Filter (siehe
`docs/backend.md` → "Gemeinsame Filter") wieder einschränken, statt vorher
schon auszusortieren.

## 1. Suchen

Suchmaschinen indexieren oft den rohen Inhalt von ICS-Dateien — das macht sie
direkt auffindbar:

```
"BEGIN:VCALENDAR" "<Ort/Kiez>"
```

Weitere ergiebige Suchmuster (`<Ort>` ersetzen):

| Suche | Findet |
|---|---|
| `<Ort> site:meetup.com` | Meetup-Gruppen mit ICS-Export |
| `<Ort> "Veranstaltungskalender"` | Lokale Aggregatoren, Bezirksamt-Seiten |
| `<Ort> Kiezzeitung OR Nachbarschaftsinitiative` | Kleine, oft übersehene Quellen |
| `<Ort> Gemeinde OR Kirchengemeinde Veranstaltungen` | Kirchengemeinden |
| `<Ort> Sportverein Termine` | Sportvereine |
| `<Ort> "Add to Calendar" OR "iCal"` | Seiten mit ICS/iCal-Export |

DuckDuckGo bevorzugen, falls Google automatisierte Anfragen blockiert.

## 2. Bekannte Plattform-Muster

Wenn eine Quelle über eine dieser Plattformen läuft, gibt es meist einen
direkten Feed, ohne dass gescraped werden muss:

| Plattform | Feed-URL-Muster |
|---|---|
| Meetup | `https://www.meetup.com/{gruppe}/events/ical/` |
| Tockify | `https://tockify.com/api/feeds/ics/{kalendername}` |
| WordPress: Tribe Events Calendar | `https://example.org/events/?ical=1` |
| WordPress: Modern Events Calendar | `https://example.org/events/?mec-ical-feed=1` |
| WordPress: Weekly Class Schedule (wcs) | kein öffentlicher Feed — Browser-Network-Tab beim Laden des Kalenders prüfen, siehe unten |
| Squarespace | `https://example.com/events?format=json` |
| Google-Kalender-Einbettung | Kalender-ID aus dem Embed-Code extrahieren, dann `https://calendar.google.com/calendar/ical/{ID}/public/basic.ics` |
| GrowthZone (Gewerbevereine/IHK-artig) | `https://business.{verband}.com/api/events` |

## 3. Wenn kein offizieller Feed existiert: Browser-Network-Tab

So wurde die `wcs`-Quelle für das Familienzentrum Weißensee gefunden (siehe
`docs/backend.md` → "wcs-Quellen" für die volle Geschichte): die
Kalenderseite im Browser öffnen, DevTools → Network-Tab, Filter auf
XHR/Fetch, die Seite neu laden bzw. die Wochenansicht wechseln, und nach der
Anfrage suchen, die die Termine tatsächlich lädt (oft `admin-ajax.php` bei
WordPress, oder ein `/api/`-Pfad). Response-Format und Query-Parameter davon
ableiten.

**Falls ein ICS-/JSON-Export durch eine Firewall/WAF blockiert ist:**
manchmal ist die zugrundeliegende REST-API weniger streng geschützt als der
dedizierte Export-Endpunkt. Bei WordPress mit Tribe Events z.B. lohnt sich
`wp-json/tribe/events/v1/events/` als Fallback, wenn `?ical=1` blockiert ist.

## 4. Testen, bevor eine Quelle angebunden wird

```sh
curl -sL "<FEED-URL>" -A "Mozilla/5.0" | grep -c "BEGIN:VEVENT"   # ICS: Anzahl Termine
curl -sL "<JSON-URL>" | python3 -m json.tool | head -50            # JSON: Struktur ansehen
```

Prüfen: enthält die Antwort echte, aktuelle Termine? Passt die Zeitzone
(siehe `docs/backend.md` → "Warnung, nicht blind auf andere Quellen
übertragen")? Wie oft ändert sich der Inhalt (relevant für `daysAhead` und
die Cron-Frequenz im Workflow)?

## 5. Dokumentieren

Gefundene, geprüfte und verworfene Quellen in
die Quellen-Checkliste der Instanz (bzw. der lokalen Entsprechung im
eigenen Fork) festhalten — Format dort siehe Datei selbst.

## 6. Anbinden

- Passt die Quelle zu `type: "rss"` oder `type: "json"`? → neue Datei in
  `<instance>/sources/` nach den `.example.json`-Vorlagen, kein Go-Code nötig
  (siehe `docs/backend.md` → "Wie das Feld-Mapping funktioniert").
- WordPress mit "Weekly Class Schedule"-Plugin? → `type: "wcs"`, siehe
  `docs/backend.md` → "wcs-Quellen".
- Passt nichts davon (eigenes Scraping, Pagination, Auth)? → eigene
  `Source`-Implementierung nach Vorlage `internal/sources/example.go`, dabei
  über `harden(withCommonFilters(...))` registrieren (siehe
  `docs/backend.md` → "Neue Quelle mit eigenem Go-Code").
