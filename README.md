# openLitfass

**Die digitale Litfaßsäule für deinen Kiez.**

Ein schlanker, statischer Event-Kalender für einen Kiez, ein Dorf oder einen
Stadtteil: Events kommen automatisch aus RSS-Feeds, JSON-APIs und
WordPress-Kalendern (plus Einzel-Events von Hand) und werden als statische
Seite ausgeliefert, ohne Datenbank und ohne API-Server.

Dieses Repo ist der **Kern**. Konkrete Kalender sind **Instanzen** in
eigenen Repos, z.B. `weissens.ee` für Berlin-Weißensee. Eine Instanz enthält
nur Konfiguration, Quellen und Gestaltung.

```
openlitfass (dieses Repo)                       Instanz-Repo (z.B. weissens.ee)
  backend/  Go: fetch, build-site               instance.json
  web/      Frontend (HTML/CSS/JS)              sources/  inbox/  categories.json
  Containerfile ──► ghcr.io-Image ────────────► update-events.yml (Cron)
  example-instance/  Vorlage                    site/  (Theme, Logo)
                                                data/  (generiert)
```

## Dokumentation

- [docs/instance.md](docs/instance.md): Instanz aufsetzen, `instance.json`
- [docs/backend.md](docs/backend.md): Quelltypen, Feld-Mapping, Filter,
  Fehlertoleranz, Event-Schema, lokal ausführen, Workflows
- [docs/inbox.md](docs/inbox.md): Einzel-Events von Hand
- [docs/source-discovery.md](docs/source-discovery.md): neue Quellen finden

## Schnellstart (Docker)

```sh
docker build -t openlitfass -f Containerfile .
docker run --rm -v "$PWD/example-instance:/instance" -w /instance openlitfass build-site .
docker run --rm -p 8080:80 -v "$PWD/example-instance/public:/usr/share/nginx/html:ro" nginx:alpine
```

Dann <http://localhost:8080> öffnen.

Fertige Images: `ghcr.io/<owner>/openlitfass` (amd64/arm64), gebaut von
`.github/workflows/image.yml`.

## Lizenz

[MIT](LICENSE). Die Kategorie- und Filter-Piktogramme stammen aus
[Tabler Icons](https://tabler.io/icons) (ebenfalls MIT).
