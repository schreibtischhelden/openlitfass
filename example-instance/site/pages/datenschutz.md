# Datenschutz

**Platzhalter der Beispiel-Instanz.** Jede Instanz muss hier ihre eigenen,
rechtlich korrekten Angaben hinterlegen (Datei `site/pages/datenschutz.md`
oder ein Link in `instance.json`).

Hinweis für Betreiber: Der Kartenausschnitt bei Orten und Terminen lädt
Kartenbilder von OpenStreetMap (tile.openstreetmap.org, OpenStreetMap
Foundation) erst, wenn Besucher auf „Karte anzeigen“ klicken. Dabei werden
IP-Adresse und Adresse dieser Website übertragen; Cookies werden nicht
gesetzt oder mitgesendet. Das gehört in die Datenschutzerklärung.

Weitere Punkte für die Datenschutzerklärung:

- **Einstellungen und Merkliste** liegen nur im Browser der Besucher
  (localStorage, Schlüssel `openlitfass:settings` und `openlitfass:saved:…`),
  zusätzlich eine Offline-Kopie der Seite (Service Worker). Es gibt keine
  Cookies; nichts davon wird an den Server geschickt. Unter „Einstellungen“
  lässt sich alles einsehen und löschen.
- **Bilder zu Terminen** (nur bei Quellen mit `"images": true`) werden nicht
  kopiert, sondern direkt vom Server der Quelle geladen – standardmäßig erst
  nach einem Klick, je nach Einstellung auch automatisch oder nie. Dabei sieht
  dieser Server die IP-Adresse, aber nicht, von welcher Seite der Abruf kommt.
- **Link-Vorschauen:** Terminseiten nennen dieses Bild in `og:image`. Einige
  Messenger (z. B. WhatsApp, iMessage) erzeugen die Vorschau auf dem Gerät
  der Person, die den Link teilt; dieses Gerät ruft das Bild dann beim Server
  der Quelle ab.
