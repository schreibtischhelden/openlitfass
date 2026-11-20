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
- `price: { free, amount }` — **weggelassen = kostenlos** (nicht `false`/`null`
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
