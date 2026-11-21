# Event-Inbox

Manueller Gegenpart zu den automatisierten Quellen in `<instance>/sources/`: für
einzelne Events, die durch keinen Feed kommen — ein Flyer, ein Anruf, ein
Tipp aus dem Kiez. Eine JSON-Datei pro Event hier ablegen und committen,
fertig. Kein Eintrag in `<instance>/sources/` nötig, keine URL, kein Mapping.

## Format

Siehe `example-instance/inbox/beispiel-event.example.json` als Vorlage (Endung `.example.json` wird
nicht eingelesen, zum Aktivieren kopieren und `.example` aus dem Namen
entfernen). Die Felder entsprechen direkt dem Event-Schema aus
`docs/backend.md` → "Event-JSON-Schema":

- `title` (Pflicht), `date` (Pflicht, `YYYY-MM-DD`)
- `description`, `category`, `startTime`/`endTime` (`HH:MM`)
- `location: { name, address }`
- `price: { free, amount }` — **weggelassen = kostenlos**; `{ "free": false }` = Preis unbekannt (nicht `null`
  angeben, das würde "null €" im Frontend erzeugen)
- `organizer`, `url`
- `ticketRequired`, `registrationRequired`, `familyFriendly`, `cancelled` (alle optional,
  Standard `false`)

Datum/Uhrzeit direkt als Europe/Berlin-Wanduhrzeit eintragen — keine
Zeitzonen-Umrechnung wie bei den automatisierten Quellen, weil hier ein
Mensch die Zeit direkt einträgt.

Der Dateiname (ohne `.json`) ist die stabile ID des Events über mehrere
Läufe hinweg — beim Bearbeiten einer bestehenden Datei also den Namen nicht
ändern, sonst entsteht beim nächsten Lauf ein zweiter, neuer Eintrag statt
den bestehenden zu aktualisieren.

## Wiederkehrende Termine (`repeat`)

Für Öffnungszeiten, Wochenkurse und Ähnliches statt `date` ein
`repeat`-Objekt angeben; die übrigen Felder gelten für jeden Termin:

```json
{
  "title": "Kinderbauernhof geöffnet",
  "startTime": "09:00", "endTime": "16:30",
  "location": { "name": "Kinderbauernhof", "address": "Musterweg 2, 12345 Berlin" },
  "repeat": {
    "weekdays": ["mo", "di", "mi", "do", "fr"],
    "from": "2026-10-01",
    "until": "2027-03-31",
    "dates": ["2026-12-26"],
    "except": ["2026-12-25"],
    "daysAhead": 60
  }
}
```

- `weekdays`: `mo di mi do fr sa so` (auch englisch `mon`/`tue`/…), braucht
  `from` und `until`.
- `dates`: zusätzliche Einzeltermine, z. B. Feiertage mit
  Wochenend-Öffnungszeiten (geht auch ohne `weekdays`).
- `except`: Termine, die ausfallen.
- `daysAhead`: wie weit voraus Termine erzeugt werden (Standard 60 Tage),
  damit die Liste nicht mit Monaten gleicher Einträge vollläuft.

Jeder Termin bekommt eine eigene stabile ID (`<Dateiname>@<Datum>`). Die
Datei wird gelöscht, sobald `until` bzw. das letzte Datum aus `dates`
vorbei ist. Unterschiedliche Zeiten (Sommer/Winter, Werktag/Wochenende)
als eigene Dateien anlegen.

**Preis unbekannt:** `"price": { "free": false }` – dann zeigt die App
keinen Preis an (weggelassen hieße "kostenlos").

## Ablauf

1. Datei hier ablegen, committen, pushen.
2. Der `fetch-events`-Workflow läuft (Push-Trigger auf `inbox/**`) und
   nimmt das Event ab sofort in `data/events.json` mit auf.
3. Die Datei bleibt liegen und wird bei jedem Lauf erneut eingelesen — das
   Event bleibt so lange sichtbar. Erst sobald das Datum in der
   Vergangenheit liegt, löscht `InboxSource` (`internal/sources/inbox.go`)
   die Datei automatisch und committet das mit (siehe
   `.github/workflows/update-events.yml` der Instanz) — genau der Zeitpunkt, an dem das
   Event ohnehin aus dem Kalender fallen würde.

Warum nicht sofort nach der ersten Verarbeitung löschen: `cmd/fetch`
berechnet `data/events.json` bei jedem Lauf komplett neu aus den aktuellen
Fetch-Ergebnissen aller Quellen, es liest die vorherige Datei nicht ein.
Würde die Datei sofort gelöscht, verschwände das Event schon beim nächsten
Lauf wieder, weil dann keine Quelle mehr existiert, die es liefert.

## Fehlerhafte Dateien

Eine Datei mit ungültigem JSON, fehlendem `title` oder unparsebarem `date`
wird übersprungen und **nicht** gelöscht (nichts geht dabei verloren) —
stattdessen loggt der Lauf eine klare Fehlermeldung mit Dateiname. Datei
reparieren und erneut committen.
