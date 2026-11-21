# openLitfass

**Your own digital advertising pillar.** · <https://openlitfass.org>

openLitfass is a lightweight, static events calendar that you build for
yourself and can share publicly: for a neighbourhood (*Kiez*), a village, a
club, or event days with many talks and workshops. Events are collected automatically from
RSS feeds, JSON APIs, iCalendar exports, HTML pages and WordPress calendars,
complemented by individual events added by hand. The result is served as a
static website: no database and no API server.

This repository contains the **core**. Each calendar is an **instance** in a
repository of its own, for example
[`weissens.ee`](https://github.com/weissensee/weissens.ee) for
Berlin-Weißensee. An instance contains only configuration, sources and
styling.

```
openlitfass (this repository)                   instance repository (e.g. weissens.ee)
  backend/  Go: fetch, build-site, serve        instance.json
  web/      front end (HTML/CSS/JS)             sources/  inbox/  categories.json
  Containerfile ──► ghcr.io image ────────────► update-events.yml (cron)
  example-instance/  template                   site/  (theme, logo)
                                                data/  (generated)
```

## Documentation

- [Setting up an instance](docs/instance.md): directory layout,
  `instance.json`, front end, operation, metrics
- [Backend](docs/backend.md): source types, field mapping, filters, fault
  tolerance, event schema, running locally, workflows
- [Event inbox](docs/inbox.md): adding individual events by hand
- [Finding sources](docs/source-discovery.md): a playbook for discovering new
  event sources
- [GitHub Pages](docs/pages.md): publishing via GitHub Pages and DNS for a
  custom domain
- [Roadmap and releases](docs/releases.md): GitHub Project, milestones,
  versions and how to release

## Quick start (Docker)

```sh
docker build -t openlitfass -f Containerfile .
docker run --rm -v "$PWD/example-instance:/instance" -p 8080:8080 -e OL_FETCH_INTERVAL=0 openlitfass
```

Then open <http://localhost:8080>.

`OL_FETCH_INTERVAL=0` disables fetching, because the example sources are
templates only. Omit it for a real instance; sources are then fetched every
six hours (see [Operation](docs/instance.md#operation-one-instance-per-container)).

Prebuilt images for amd64 and arm64 are published as
`ghcr.io/schreibtischhelden/openlitfass` by
[`.github/workflows/image.yml`](.github/workflows/image.yml).

## Licence

Released under the [MIT Licence](LICENSE). The category and filter icons are
taken from [Tabler Icons](https://tabler.io/icons), also under the MIT
Licence.
