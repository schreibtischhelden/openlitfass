# Backend: fetch, build-site and serve

The backend consists of three small Go programs that run against an
**instance directory** (layout: [instance.md](instance.md)):

- **`fetch`** fetches all of the instance's sources, normalises the events and
  writes `<instance>/data/events.json`, `categories.json` and `sources.json`.
  The front end (`web/js/app.js`) loads these files directly with `fetch()`;
  no API server is needed.
- **`build-site`** assembles the finished static site from the core front end
  (`web/`), the instance overrides (`<instance>/site/`) and the data, rendering
  `web/index.html` with the values from `instance.json`. It also generates
  `data/site.json` (`instance.json` for the front end) and
  `manifest.webmanifest` (PWA), and stamps `sw.js` with a build ID.
- **`serve`** (the image's default command) runs both at a regular interval
  and serves the site itself: one instance per container, see
  [Operation](instance.md#operation-one-instance-per-container).

All three are part of the core's container image (`Containerfile`), which the
instance's workflow (`update-events.yml`) runs every six hours. Apart from
`golang.org/x/net/html` for HTML parsing, only the Go standard library is
used.

## Structure

```
backend/
  cmd/fetch/            entry point: fetch sources, write JSON
  cmd/build-site/       entry point: assemble the static site
  cmd/serve/            entry point: fetch, build and serve in a loop
  internal/app/         shared logic of the commands (environment, serve, metrics, static pages)
  internal/instance/    loads <instance>/instance.json, knows the paths
  internal/model/       event schema, instance time zone (model.Local)
  internal/config/      loads <instance>/sources/*.json
  internal/mapping/     dot-path resolver and field-mapping engine (see below)
  internal/htmlsel/     small CSS selector engine for html sources
  internal/sources/     Source interface, generic RSS/JSON/iCal/HTML/wcs fetchers,
                        inbox, resilience wrappers (timeout, panic recovery, filters)
  internal/categories/  category -> icon (from <instance>/categories.json)
  internal/writer/      validation, merging, sorting, stable IDs, JSON output
```

## What is instance-specific?

Everything that distinguishes one instance from another lives in
`instance.json` and the files next to it. The core contains no
location-specific content:

| What | Where |
|---|---|
| Name, title, description, language, theme colour, About text | `instance.json` |
| Legal notice, privacy policy, "submit an event", issue tracker | `instance.json` → `imprintUrl`, `privacyUrl`, `submitUrl`, `issuesUrl` |
| Time zone of all event times | `instance.json` → `timezone` (default `Europe/Berlin`) |
| Fallback link for events without their own URL | `instance.json` → `baseUrl` (+ `#<event-id>`) |
| Sources | `sources/*.json` |
| Individual events | `inbox/*.json` |
| Categories and icons | `categories.json` |
| Named day lists for recurring events (holidays) | `holidays.json` (optional, see [inbox.md](inbox.md#day-lists-holidaysjson)) |
| Colours, logo, fonts | `site/css/theme.css`, `site/logo.svg`, `site/…` |

## How does fetch know what to fetch?

Through `<instance>/sources/`: **one JSON file per source**, not a shared
list. On start-up, `sources.All()` (in `internal/sources/source.go`) reads
every `*.json` file in this directory (in alphabetical order, for
deterministic runs) and builds an `RSSSource`, `JSONSource`, `ICalSource`,
`HTMLSource` or `WCSSource` from it. **RSS feeds, JSON APIs, iCalendar
exports, HTML pages and WordPress "Weekly Class Schedule" calendars require no
Go code**, only a new file.

Unless the file sets its own `"name"` field, the file name (without `.json`)
becomes the source name. Adding, changing or removing a source therefore
always affects exactly one file: a clean Git diff and no merge conflicts in a
shared array.

Files ending in `.example.json` are **not** loaded (they are templates to
copy). Templates for every type are in `example-instance/sources/`:

- `beispiel-rss.example.json`: `type: "rss"`
- `beispiel-json.example.json`: `type: "json"` (JSON API)
- `beispiel-ical.example.json`: `type: "ical"` (see [ical sources](#ical-sources-icalendar--ics))
- `beispiel-html.example.json`: `type: "html"` (see [html sources](#html-sources-pages-without-a-feed))
- `beispiel-wcs.example.json`: `type: "wcs"` (see [wcs sources](#wcs-sources-wordpress-weekly-class-schedule-plugin))

A source can be switched off with `"disabled": true` without deleting the
file: no fetch attempt and no status update take place until the field is
removed or set to `false`. The `"status"` field is maintained by the pipeline
itself (see [Health status](#health-status)); do not edit it by hand.

Only formats that fit none of these types (logins, POST requests,
authenticated APIs, …) require a custom `Source` implementation; see
`internal/sources/example.go` as a template.

## How field mapping works

Each `rss`, `json` and `html` source file has a `mapping` object. Every entry
in it is a **FieldRule**:

```jsonc
{
  "path": "location.title",   // dot path into the item, e.g. "a.b.0" for array index 0
  "regex": "Beginn (?P<value>\\d{2}:\\d{2})",  // optional: extract the value from the string with a regex
  "format": "RFC1123Z",        // optional, start/end only: time layout for parsing
  "default": "Marktplatz",     // optional: fallback if path yields nothing
  "template": "{venue.address}, {venue.zip} {venue.city}"  // instead of path: combine several values
}
```

`template` replaces every `{dot.path}` with its value and tidies up separators
next to empty values (if the street is missing, the result is "13086 Berlin",
not ", 13086 Berlin"). `{a|b}` takes the first non-empty value (for example
`{beginn|einlass}`).

`regex` is applied to the value from `path` or `template` and yields: the group
named `value`, if present; otherwise **all non-empty groups joined with
spaces**; without groups, the whole match; without a match, nothing (in which
case `default` applies). This makes alternatives possible
(`(\d+)\s*€|Eintritt (frei)` → "27" or "frei"), and parts from different
fields can be combined, for example the date from the title and the time from
the text:

```jsonc
"start": {
  "template": "{title} | {encoded}",   // "21.11.26 jazz … | <h4>20 Uhr, Aula</h4>…"
  "regex": "(?s)(\\d{1,2}\\.\\d{1,2}\\.\\d{2,4}).*?[|](?:.*?(\\d{1,2}(?::\\d{2})?)\\s*Uhr)?",
  "format": "2.1.06 15:04|2.1.06 15|2.1.06"   // "21.11.26 20"; without a time: all-day
}
```

(`[|]` instead of `\|` saves one level of escaping in JSON.) The same rule
applies to `regex` in the `extract` block of html sources.

**Text is always cleaned:** all text fields from `rss` and `json` sources pass
through `mapping.CleanText`. HTML tags are removed, entities decoded
(`&#8211;` → `–`, `&#038;` → `&`), soft hyphens and zero-width characters
removed and whitespace collapsed. `description` keeps its paragraphs and is
truncated with "…" at 1,200 characters.

**For JSON sources**, `path` is a dot path into the decoded JSON response.
`itemsPath` first points to the array of individual items (for example
`"data.items"`); if empty, the response itself is the array.

**Paginated JSON APIs:** `nextPath` points to the URL of the next page in the
response (for example `"next_rest_url"` with the WordPress plugin "The Events
Calendar"). Pages are followed until there is no next page, `maxPages`
(default 10) is reached, or an entire page lies beyond the `daysAhead` window.
If a later page fails, the events collected so far are kept.

**For RSS sources**, `path` is the lower-case tag name, without namespace,
within `<item>`. The standard fields are `title`, `link`, `description`,
`pubdate`, `guid` and `category`; additional or custom tags of a feed work in
the same way, as every child element of `<item>` is made available.

**Available target fields** in `mapping` (all optional, although a meaningful
result requires at least `title`):

- `sourceId`: stable ID within the source (for example the RSS `guid`).
- `title`, `description`, `category`, `organizer`, `url`
- `locationName`, `locationAddress`
- **Times**, in one of two ways:
  - `start` (and optionally `end`) with `format` (recommended): parses a
    timestamp and automatically derives `date`, `startTime` and `endTime`,
    correctly converted to the instance's time zone (`instance.json` →
    `timezone`), using `model.LocalParts` and `time.ParseInLocation`. This
    works both with timestamps that carry an explicit time zone and with
    "naive" local strings.
  - `date`, `startTime` and `endTime` directly, if the source already provides
    these parts separately in exactly this format (`YYYY-MM-DD` / `HH:MM`).
    They are taken over as they are (optionally extracted with `regex`), not
    reformatted.
- `price`: an object of its own with `amountPath` (amount in euros, also as a
  string; 0 or missing = free) and/or `freePath` (explicit Boolean, takes
  precedence). With `"unknownIfMissing": true`, a missing amount means "price
  unknown" (nothing is shown) instead of "free", for sources that omit the
  price when they do not know it. `amountRegex` extracts the amount from free
  text at `amountPath` (same rules as `regex`), for example
  `"(?i)eintritt\\W{0,3}(frei)|(\\d+(?:,\\d+)?)\\s*€"`; "frei" (free) counts as
  free, no match as missing.
- `ticketRequired`, `registrationRequired`, `familyFriendly`, `cancelled`:
  attribute rules, see [Cancelled events](#cancelled-events).

`format` accepts either a Go reference layout string
(`2006-01-02T15:04:05Z07:00`) or one of the aliases `RFC3339`, `RFC1123Z`
(the standard RSS `pubDate` format) and `RFC1123`. Several layouts can be
separated with `|`.

## Common filters (all source types)

The following fields apply to every source, regardless of its `type`. They are
enforced by `withCommonFilters` in `internal/sources/filter.go`, which
`All()` in `source.go` wraps around every source:

```jsonc
{
  "daysAhead": 30,                       // optional: window starting today
  "categoryMap": { "Kino": "Kultur" },   // optional: source category -> instance category
  "categories": ["Kultur", "Markt"],     // optional: category allowlist
  "include": [{ "field": "location", "regex": "(?i)weißensee|1308[68]" }],  // optional
  "exclude": [{ "field": "title", "regex": "(?i)geschlossene gesellschaft" }]  // optional
}
```

- `daysAhead`: drops events more than N days in the future (instance time
  zone). Without this field there is no window; everything the source provides
  is kept. For `wcs` sources it also controls the time window of the request
  itself, and for paginated `json` sources the end of pagination.
- `categoryMap`: translates the source's category names into the instance's
  vocabulary (`categories.json`); keys are matched case-insensitively. Values
  without a mapping are kept as they are (and are then shown with the fallback
  icon, see below).
- `categories`: an allowlist applied to the resolved `category` field (after
  mapping and `categoryMap`). Without this field there is no restriction.
- `include` / `exclude`: lists of `{ "field": …, "regex": … }`. With
  `include`, only events for which at least one rule matches are kept; with
  `exclude`, events for which any rule matches are dropped. Fields: `title`,
  `location` (name and address), `organizer`, `category`, `description`.
  The example above keeps, from a university calendar, only what takes place
  in the neighbourhood. Invalid rules are logged and skipped.
- **Past events are always discarded**, regardless of these fields. This is
  not a feature flag but fixed behaviour for an events calendar.

All of these fields are optional and take effect only after the actual fetch
and mapping, so they apply equally to all source types, including future
hand-written sources, provided they are registered in `All()` with
`harden(withCommonFilters(...))`.

### Example: RSS source (`example-instance/sources/beispiel-rss.example.json`)

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

No `"name"` is needed: the file name `beispiel-rss` becomes the source name.

### Example: JSON API (`example-instance/sources/beispiel-json.example.json`)

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

To connect a new source, copy one of the `.example.json` files from
`example-instance/sources/` to `<instance>/sources/`, remove `.example` from
the file name, and adjust the URL and mapping. No Go code needs to be
redeployed, only a new file added.

## wcs sources (WordPress "Weekly Class Schedule" plugin)

Some neighbourhood calendars (for example Familienzentrum Weißensee in the
weissens.ee instance) run on WordPress with the "Weekly Class Schedule" plugin
by CurlyThemes. The weekly table on the page is an empty `<div>` populated on
the client side by Vue.js: there is no static HTML table and no documented
public REST endpoint. Instead, the data is loaded through the standard
WordPress mechanism `admin-ajax.php` with `action=wcs_get_events_json` (found
via the browser's Network tab while the calendar page loads the week view).
This does not fit the generic mapping, so there is a dedicated source type
`"type": "wcs"` with its own Go parser (`internal/sources/wcs.go`), which is
still configured through a source file:

```jsonc
{
  "type": "wcs",
  "url": "https://www.familienzentrum-weissensee.de",  // base URL, not the Ajax endpoint itself
  "daysAhead": 30,               // rolling window from today; default 30
  "organizer": "Familienzentrum Weißensee",   // the plugin provides no organiser
  "locationDefault": "Familienzentrum Weißensee, Mahlerstraße 4, 13088 Berlin",  // name = part before the first comma; address = all of it, room inserted after the venue
  "wcsTypeIds": [27, 28, 29, 30],  // see below; IMPORTANT, do not omit
  "familyFriendly": true,          // applies to EVERY event of this source, see below
  "categoryMap": {                 // wcs_type slug -> instance category (for icons)
    "austausch": "Unkategorisiert",
    "beratung": "Unkategorisiert",
    "bewegung": "Sport",
    "kreatives": "Kultur"
  }
}
```

`familyFriendly` is set at source level here, not per event: the plugin API
provides no per-event signal for it, but the fact that Familienzentrum
Weißensee as a whole is a family centre is a verified fact about the source
itself, not an assumption. For a `wcs` source whose programme is not
consistently aimed at families, simply omit the field (default `false`). This
source never provides `ticketRequired` or `registrationRequired` (there is no
signal in the plugin API), so they are always `false` for `wcs` events; see
[Event JSON schema](#event-json-schema-instancedataeventsjson).

**`wcsTypeIds` is required, not fine-tuning:** without this filter the
endpoint also returns the demo data that ships with the plugin template (for
example "Advanced Aqua Zumba", "Chemistry Lab" and fictitious instructors),
which apparently still sits unused in the WordPress installation and has
nothing to do with the real programme. To find the IDs, open the target site's
calendar page in the browser, look for the
`admin-ajax.php?action=wcs_get_events_json` request in the Network tab and
read the `content[wcs_type][]` values from it.

**Time quirk:** the `start`/`end` strings returned by the plugin carry a
`+00:00`/`Z` suffix but are actually already in the organiser's local time
(no real UTC conversion). This was confirmed against the times published on
the website: "Queer Café 16:30–18:00" appears exactly like that in the raw
data, whereas a genuine UTC-to-local conversion would have shifted the digits
by two hours. `parseWCSLocal` in `wcs.go` therefore deliberately takes only
the first 19 characters and interprets them directly as the instance's local
time (`model.Local`) instead of trusting the offset. For any new wcs source,
always check against the real opening hours on the website whether the same
applies; plugins of this kind are known to be careless with time zones.

**Warning: do not apply this blindly to other sources.** This "local digits"
approach is a verified exception for this one plugin, not a general rule.
Research into comparable aggregator projects (judell/community-calendar, see
its `docs/timezone.md`) confirms why this matters: a single city instance
there receives feeds with genuinely different time zone information, because
individual platforms (Meetup, Eventbrite) write platform-specific defaults
instead of the local time zone into their timestamps; in one of their cities
this affected more than 10,000 events. iCalendar also has two real
conventions, `X-WR-TIMEZONE` (calendar-wide default, a Google convention) and
`VTIMEZONE` + `TZID` (per event, RFC 5545), and both occur in the wild. In
short: every new source needs its own, individually verified time handling.
The approach in `parseWCSLocal` must not be copied without comment simply
because it happened to be correct here.

Cancelled occurrences (`"visible": false` in the response, corresponding to
`meta._wcs_canceled` on the source's side) are marked as cancelled; see
[Cancelled events](#cancelled-events).

## ical sources (iCalendar / .ics)

For calendar exports: Google Calendar, Nextcloud, Outlook and most WordPress
event plugins (Events Manager `/events.ics`, The Events Calendar `?ical=1`, …).
No `mapping` block is needed, as the fields are defined by the standard:

```jsonc
{
  "type": "ical",
  "url": "https://example.org/events.ics",
  "daysAhead": 120,                 // window; if omitted, 180 days for recurrences
  "organizer": "Beispiel-Bühne",    // used if ORGANIZER;CN is missing
  "locationDefault": "Beispiel-Bühne, Musterstraße 5",  // used if LOCATION is missing
  "categoryMap": { "Konzert": "Kultur" },
  "familyFriendly": false
}
```

| iCal | Event |
|---|---|
| `SUMMARY` | `title` |
| `DESCRIPTION` | `description` (cleaned, truncated) |
| `URL` | `url` |
| first value of `CATEGORIES` | `category` (translate with `categoryMap`) |
| `ORGANIZER;CN=…` | `organizer`, otherwise `organizer` from the configuration |
| `X-APPLE-STRUCTURED-LOCATION` (`X-TITLE`/`X-ADDRESS`) | `location.name`/`address` |
| otherwise `LOCATION` | `location.name`, otherwise `locationDefault` |
| `UID` (+ date for recurrences) | stable ID |
| — | price: unknown (iCalendar has no price) |

Times: UTC (`…Z`), `TZID=…` and "floating" times without a zone (then the
calendar's `X-WR-TIMEZONE`, otherwise the instance time zone); all-day events
(`VALUE=DATE`); `DURATION` instead of `DTEND`. If the end equals the start
(Events Manager does this when no end time is set), the end time is left
empty. Unknown zone names (for example Windows names) fall back to the
instance time zone.

Recurrences (`RRULE`): `DAILY`, `WEEKLY` (also with `BYDAY=TU,TH`), `MONTHLY`
(same day of the month) and `YEARLY`, each with `INTERVAL`, `COUNT` and
`UNTIL`; `EXDATE` and modified single occurrences (`RECURRENCE-ID`) are
supported. Recurrences are expanded only within the window (at most 500
occurrences). Unsupported rules (for example "every first Monday",
`BYDAY=1MO` with `MONTHLY`) yield only the first occurrence and a log line.
Events in a series that share a UID without an `RRULE` each get their own ID.
Cancelled events (`STATUS:CANCELLED`) are marked as cancelled.

## html sources (pages without a feed)

For event pages without RSS, iCal or an API, events are read from the HTML
with CSS selectors. There are three parts:

1. `itemSelector`: finds each event container on the page.
2. `extract`: extracts named values from a container. Either in short form,
   `"title": "h2"` (text of the first match), or as an object:

   | Field | Meaning |
   |---|---|
   | `selector` | CSS selector relative to the container (empty = the container itself) |
   | `attr` | an attribute instead of the text, e.g. `"href"` (relative URLs are resolved) |
   | `html` | inner HTML instead of text, for descriptions, so that paragraphs are kept |
   | `all` / `skip` | join all matches (instead of only the first), skipping the first `skip` |
   | `regex` | extract part of the value (group `value`, otherwise all non-empty groups) |

3. `mapping`: as for `json` sources; `path` refers to the names from
   `extract`.

```jsonc
{
  "type": "html",
  "url": "https://example.org/veranstaltungen.html",
  "itemSelector": "#events .event",
  "extract": {
    "day": ".date .day", "month": ".date .month", "year": ".date .year",
    "title": "h2",
    "start": { "selector": "p.time", "regex": "(\\d{1,2}:\\d{2})" },
    "end":   { "selector": "p.time", "regex": "-\\s*(\\d{1,2}:\\d{2})" },
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

Complete template: `example-instance/sources/beispiel-html.example.json`.

**Headings without a container (`sectionSelector`):** some pages group events
under a heading that does not enclose the entries but precedes them, such as a
cinema programme:

```html
<h3 class="program_date1">Samstag, 21.11.2026</h3>
<div class="program_entry"><a href="/film/x/1">12:45 Film X</a></div>
<div class="program_entry"><a href="/film/y/2">20:15 Film Y</a></div>
<h3 class="program_date1">Sonntag, 22.11.2026</h3>
…
```

With `"sectionSelector": "h3.program_date1"`, each entry receives the text of
the last heading before it (in document order) as the value `section`, which
can be used in the mapping like an `extract` name:

```jsonc
"start": {
  "template": "{section} | {time}",
  "regex": "(\\d{2}\\.\\d{2}\\.\\d{4})[^|]*[|]\\s*(\\d{1,2}:\\d{2})?",
  "format": "02.01.2006 15:04|02.01.2006"
}
```

Supported selectors: `tag`, `*`, `.class`, `#id`, `[attr]`, `[attr=value]`,
`[attr~=word]`, `[attr*=part]`, `:first-of-type`, `:nth-of-type(n)`,
descendant (`a b`) and child (`a > b`). Selector lists (`,`) and sibling
combinators are not supported.

**Dates from separate parts:** `format` may specify several layouts separated
by `|` (`"2 Jan 2006 15:04|2 Jan 2006"`); if only a layout without a time
matches, the event is all-day. German month names and abbreviations ("Nov.",
"März", "Dezember") are understood in layouts with month names. If the layout
has no year (`"2.1. 15:04"` for "FR | 20.11."), the nearest year is added: the
current year, unless the date is more than a month in the past, in which case
the next year.

**Robustness:** if `itemSelector` finds nothing, this counts as an error of the
source (recorded in `status`). Usually the page has changed its layout, and
this should be noticed rather than silently returning zero events. HTML
sources are inherently more fragile than feeds: if a site offers a feed or an
API, always use that instead.

## Fault tolerance

Every source, regardless of type, is built in `sources.All()` via `harden(...)`
(`internal/sources/source.go`, `safety.go`). This prevents a single
malfunctioning source from bringing down the whole run or the results of the
other sources:

- **Per-source timeout** (`withTimeout`, default 20 s): a hanging source does
  not use up the shared time budget of the others. The outer timeout in
  `cmd/fetch` (5 minutes) is only a coarse backstop.
- **Panic recovery** (`withPanicRecovery`): a panic in a source (for example
  caused by unexpected data from a third-party API) becomes a normal, logged
  error for that one source, instead of crashing the whole process and losing
  the results already fetched from other sources.
- **Common filters** (`withCommonFilters`): see above.
- **Status tracking** (`withStatusTracking`): writes success or failure back to
  the source file after every fetch attempt; see
  [Health status](#health-status) below.

In addition, independently of individual sources:

- **`internal/config.Load`** skips a single broken
  `<instance>/sources/*.json` file (invalid JSON, unreadable) instead of
  aborting the whole loading process, so a typo in one file does not take all
  other, valid sources down with it.
- **`internal/writer.validate`** discards individual events with an empty
  title or an implausible date instead of passing them on to the front end,
  and logs a warning stating how many events were affected.
- **`internal/writer.Write`** refuses to write an empty event list (see
  [Safeguard: never commit empty data](#safeguard-never-commit-empty-data));
  this also applies if nothing valid remains after validation.

In short: individual broken sources, configuration files or events are skipped
and logged, never silently ignored and never fatal for the whole run, unless
**all** sources deliver nothing usable, in which case
`<instance>/data/events.json` is deliberately left untouched.

## Health status

Whether a source still works is recorded directly in its own configuration
file, not in a separate report and not only in the CI log, which is gone after
the next run. Every fetch attempt writes back a `"status"` field:

```json
"status": {
  "lastAttempt": "2026-07-13T08:00:12Z",
  "lastSuccess": "2026-07-06T08:00:09Z",
  "lastError": "fetch https://example.org/feed.xml: unexpected status 404 Not Found"
}
```

- `lastAttempt`: time of the last attempt, successful or not.
- `lastSuccess`: time of the last *successful* fetch. It is not updated by a
  failed attempt, so it shows how long a source has been broken, not just
  that the last attempt failed.
- `lastError`: the plain-text error message of the last attempt (timeout, 404,
  parse error, `panic: ...`). Empty or absent on success.

This makes a source that has broken silently (a website redesign breaks the
mapping, a feed disappears, an endpoint changes) visible without searching
through CI logs: simply look at `git diff` or the current content of the
source file. The status is committed together with
`<instance>/data/events.json` (see [Workflows](#workflows-github-actions)).

The implementation is in `internal/config.RecordAttempt` (reads the source
file fresh from disk, updates only `status`, writes it back) and
`internal/sources/status.go` (`withStatusTracking`, the outermost wrapper in
`sources.All()`, which calls `RecordAttempt` after every `Fetch()`, on success
or failure). Because `RecordAttempt` re-serialises the whole file via
`SourceConfig`, the field order of the Go struct determines the order in the
JSON (stable across runs, but not necessarily as a person originally
formatted the file), and JSON keys unknown to `SourceConfig` are removed when
the file is written back.

## Event JSON schema (`<instance>/data/events.json`)

The output schema matches exactly what the front end (`web/js/app.js`)
expects:

```jsonc
{
  "generatedAt": "2026-07-05T15:40:00Z",  // metadata; ignored by the front end
  "events": [
    {
      "id": "flohmarkt-am-antonplatz-3f9a1b2c",  // stable, from date + time + title; see "The same event from several sources"
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
      "source": "beispiel-rss",                   // source of the displayed version; label in data/sources.json
      "sources": ["beispiel-rss"],                // all sources of this event
      "ticketRequired": false,
      "registrationRequired": false,
      "familyFriendly": false,
      "cancelled": false                          // see "Cancelled events"
    }
  ]
}
```

Events are sorted chronologically (date, then start time). Repeated runs with
unchanged data produce a byte-identical file (a clean Git diff in the
workflow).

`category` is a free-form string. Which icon it gets in the front end is not
defined here but in `<instance>/data/categories.json` (see
[Category icons](#category-icons-instancedatacategoriesjson)).

`ticketRequired`, `registrationRequired` and `familyFriendly` are
deliberately real Booleans that are always set (no `omitempty`, no pointer).
They are cross-cutting attributes, not categories: an event can combine them
freely, regardless of its `category` (see
[Categories versus attributes](#categories-versus-attributes)). They feed the
attribute filters in the front end (`ATTRS` in `web/js/app.js`), not the
category menu. On the source side they are set with attribute rules in the
mapping (`{"path": "..."}`, default `false` if `path` is missing or does not
resolve), or, for `wcs` sources, with `familyFriendly` at source level. The
wcs plugin API provides no per-event ticket or registration signal, so
`ticketRequired` and `registrationRequired` are currently always `false` for
`wcs` sources (a known limitation, not a made-up value).

## Cancelled events

Cancelled events are **not** discarded. They are published with
`"cancelled": true` and clearly marked in the front end (the label "Abgesagt",
a struck-through title, a notice in the details and `STATUS:CANCELLED` in the
.ics file). Anyone who planned to attend should find out, rather than the event
silently disappearing. A cancellation is detected via:

| Source | Signal |
|---|---|
| `ical` | `STATUS:CANCELLED`, also for a single occurrence of a series (`RECURRENCE-ID`) |
| `wcs` | hidden occurrence (`visible: false`, corresponds to `_wcs_canceled`) |
| `rss`/`json`/`html` | mapping rule `cancelled`, e.g. `{"path": "status", "equals": "cancelled\|canceled"}` |
| inbox | `"cancelled": true` in the file |
| all | marker in the title: `ABGESAGT: …`, `Abgesagt! …`, `+++ Entfällt +++ …`, `[abgesagt] …`, `… (abgesagt)`, `… – ABGESAGT`, `… - fällt aus!`, `… [VERSCHOBEN!]` |

**The title is never altered:** it is taken over as the organiser wrote it, and
only the flag is derived from it. A postponed event counts as cancelled on its
original date (the new date usually appears separately in the source).
Detection requires a separator or brackets (or capital letters at the start):
"Abgesagt – ein Theaterstück" remains an ordinary title. Other notes in the
title are deliberately not evaluated: "ausverkauft" (sold out; the calendar
shows events, it does not sell tickets), "Zusatztermin" (additional date) and
"Achtung: Terminverschiebung" (unclear whether the old or the new date is
meant).

Attribute rules (`ticketRequired`, `registrationRequired`, `familyFriendly`,
`cancelled`) read the value as a Boolean; with `"equals": "a|b"` they are true
instead if the value matches one of the given texts (case-insensitively).

## The same event from several sources

If an event comes from several sources (the venue itself and an aggregator
calendar), it is merged into **one** event (`internal/writer/merge.go`):

- **Matching** uses date, start time and normalised title (lower case,
  umlauts unified, and notes in square brackets such as `[AUSVERKAUFT!]` and
  punctuation ignored; this applies only to the comparison, the displayed
  title stays unchanged). The location is deliberately not part of the key,
  because every source names the venue differently; however, if the two events
  give different postcodes, they are not merged.
- **Which source wins:** the one with the highest `priority` (a field in the
  source file, default 0; for example `10` for the venue itself, `0` or
  negative for an aggregator). Empty fields (description, link, address, price,
  end time, category "Unkategorisiert") are filled in from the other sources;
  if any source reports a cancellation, the event counts as cancelled.
- **`sources`** in the event lists all contributing sources (winner first);
  the front end shows "Quellen: A, B".
- Events are never merged within **one** source (two rooms, the same class at
  the same time remain two events).

**Event IDs** are formed from the same source-independent key: a readable
beginning of the title plus a hash, for example `tanztee-fuer-alle-3f9a1b2c`.
An event therefore keeps its ID if it later also comes from a second source
(shared links, saved stars). Identical keys within one source get a fixed
`-2`, `-3`. If the organiser changes the title, date or start time, a new ID is
created.

**Limitation:** matching is exact. Rephrased titles ("Kiezchor:
Weihnachtskonzert" versus "Weihnachtskonzert des Kiezchors") are not
recognised. A fuzzy matching stage will follow once an aggregator calendar
provides real data to tune it against.

## Category icons (`<instance>/data/categories.json`)

The mapping from categories to icons is a deliberately one-way interface: the
backend is the only writer, and the front end only reads it for display. This
keeps the categories of the sources and the icons of the front end in sync,
and a new category from a new source appears in the front end automatically.

- **`<instance>/categories.json`** is the canonical registry (loaded by
  `internal/categories`): name → SVG path data (taken unchanged from the
  official Tabler Icons). The order of the entries is the display order;
  `fallback` names the fallback category, for example `"Unkategorisiert"`.
- **`internal/writer.WriteCategories`** uses it, together with the events
  actually present in the current run, to write
  `<instance>/data/categories.json`, always from the same final event set
  returned by `writer.Write` as `<instance>/data/events.json`, so that the two
  files never diverge.
- **`web/js/app.js`** loads `<instance>/data/categories.json` with `fetch()`
  and builds both the category filter and the icons on the event cards from
  it. It contains no category icon list of its own, only a single local
  emergency fallback in case the file cannot be loaded.

```jsonc
{
  "generatedAt": "2026-07-15T08:00:00Z",
  "categories": [
    { "name": "Kultur", "icon": ["M12 21a9 9 0 0 1 0 -18…", "…"] },
    { "name": "Sport", "icon": ["…"] }
    // … the remaining registry entries, then any newly discovered categories
  ]
}
```

### Categories versus attributes

The category list in `<instance>/categories.json` deliberately uses a single
axis: the subject area, not the format or the target audience. In the
template: `Kultur, Sport, Kulinarik, Natur, Markt, Unkategorisiert`. Formats
such as "Konzert" (concert) or "Workshop" always belong to a subject (a
concert is a cultural format) and are expressed in the title or description,
not as a category. Target audiences such as families are attributes
(`familyFriendly`, see [Event JSON schema](#event-json-schema-instancedataeventsjson)),
shown among the attribute filters rather than in the category menu.

### Categories grow dynamically

`Build()` (`internal/categories/categories.go`) always writes all registry
entries first, whether or not an event of that category currently exists, so
the menu stays stable instead of shrinking or growing with every run. If the
events additionally contain a `category` the registry does not know (a new
source, a forgotten `categoryMap` entry, a typo), it is appended in
alphabetical order, temporarily gets the fallback icon and produces a logged
warning. The run does not fail because of it, but the category is visible
instead of silently hiding under "Unkategorisiert". To give it an icon of its
own, add an entry to `<instance>/categories.json`; no front-end change is
needed.

## Safeguard: never commit empty data

`internal/writer.Write` refuses to write an empty event list, and `cmd/fetch`
skips writing altogether while `sources.All()` is empty or all sources fail.
This way, neither an instance without active sources (only `.example.json`
files) nor a failure of all sources overwrites the existing data in
`<instance>/data/events.json` with an empty file.

## Finding new sources

For how to discover new sources in the first place (search patterns, known
platform URL patterns, what to do when no official feed exists), see
[source-discovery.md](source-discovery.md). It is deliberately
location-independent, for anyone setting up an instance for another
neighbourhood. The current state of each instance (found, checked, rejected)
belongs in that instance's source checklist.

## New source with custom Go code (exceptional cases)

For formats beyond the built-in types: copy `internal/sources/example.go` as a
template, implement the `Source` interface (`Name()`,
`Fetch(ctx) ([]model.Event, error)`), and register it at the end of `All()`
in `internal/sources/source.go` with
`append(all, harden(withCommonFilters(...)))`, so that the new source gets the
same fault tolerance and filters as the built-in types. Use
`model.LocalParts(t time.Time)` for time conversion and always set `Price`
explicitly (the zero value would be `{free: false, amount: null}`, shown as
"null €" in the front end).

## Manual events: the inbox (`<instance>/inbox/`)

The counterpart to the automated sources, for individual events without a
feed (a flyer, a tip from the neighbourhood): place a JSON file in
`<instance>/inbox/` and commit it; no entry in `<instance>/sources/` is
needed. `InboxSource` (`internal/sources/inbox.go`) reads every `*.json` file
there (except `*.example.json`) and turns it into one or more events. Format
and workflow are described in [inbox.md](inbox.md).

Note that `InboxSource` deletes the file itself, but only once the event date
is in the past, not immediately after the first run. The reason:
`cmd/fetch` rebuilds `<instance>/data/events.json` from scratch on every run
from the current `Fetch()` results of all sources, without reading the
previous file. An inbox file deleted straight away would make the event
disappear again on the next run, because no source would provide it any more.
The file therefore stays in place (and is read again each time, so the event
remains visible) until its date has passed, which is exactly the point at
which `withCommonFilters` would discard the event anyway. The instance's
`update-events.yml` workflow commits the deletion like any other change made
by the run.

## Running locally

With Go:

```sh
cd backend
go run ./cmd/fetch ../example-instance                       # or $OL_INSTANCE_DIR
go run ./cmd/build-site -web ../web ../example-instance      # -> example-instance/public/
```

Without Go, using only Docker or Podman (the build context is the repository
root):

```sh
docker build -t openlitfass -f Containerfile .
docker run --rm -v "$PWD/example-instance:/instance" -w /instance openlitfass fetch .
docker run --rm -v "$PWD/example-instance:/instance" -w /instance openlitfass build-site .
docker run --rm -p 8080:80 -v "$PWD/example-instance/public:/usr/share/nginx/html:ro" nginx:alpine
```

`fetch` modifies the instance directory (status fields in `sources/`, and
expired `inbox/` files are deleted), which is why it requires an
`instance.json` in the target directory. As long as `sources/` contains no
active source and `inbox/` no event, it deliberately writes nothing.

`build-site` empties its output directory before each run, but only if it
created the directory itself (marker file `.openlitfass-build`).

## Checks before pushing

`scripts/check.sh` runs every back-end check in Docker (no local Go
needed) and is exactly what CI runs, so a green local run means a green CI
run. It works from any directory, in Git Bash on Windows as well.

```sh
scripts/check.sh              # everything, in this order:
scripts/check.sh fmt          # gofmt
scripts/check.sh vet          # go vet
scripts/check.sh lint         # golangci-lint, configured in .golangci.yml
scripts/check.sh test         # go test -race
scripts/check.sh smoke        # build-site against example-instance/
scripts/check.sh vuln         # govulncheck

scripts/check.sh workflows    # actionlint and zizmor on all workflows
scripts/check.sh image        # build the image, Trivy scan of it and of the Containerfile
```

`workflows` and `image` are not part of the default run; they belong to
`security.yml` and `image.yml`. `image` fails on fixable `CRITICAL` or `HIGH`
vulnerabilities and on serious Containerfile misconfigurations. zizmor runs
its online audits too when `GH_TOKEN` is set.

The Go image follows the `go` line in `backend/go.mod`; all other tool
versions are pinned at the top of the script and are updated by hand
(Dependabot does not see them). Named Docker volumes (`ol-gocache`,
`ol-gomod`, `ol-lintcache`, `ol-trivycache`) keep builds, modules and the
vulnerability database between runs.

`.golangci.yml` enables, besides the standard linters, `gosec`, `bodyclose`,
`noctx`, `errorlint`, `revive`, `unconvert`, `gocritic` (diagnostic and
performance checks) and the performance linters `perfsprint` and `prealloc`.
A finding that is intentional is silenced on its line with
`//nolint:<linter> // <reason>`; the reason is required.

To run the fast checks automatically before every push:

```sh
printf '#!/bin/sh\nexec scripts/check.sh fmt vet lint test\n' > .git/hooks/pre-push
chmod +x .git/hooks/pre-push
```

## Workflows (GitHub Actions)

**Core: `.github/workflows/ci.yml`** runs on every push and pull request:
each step calls `scripts/check.sh` (gofmt, go vet, golangci-lint,
`go test -race`, a complete `build-site` run against `example-instance/`,
govulncheck); see [Checks before pushing](#checks-before-pushing).

**Core: `.github/workflows/security.yml`** runs on pull requests that touch
code, the image or workflows, every Monday and manually:
`scripts/check.sh workflows` and `scripts/check.sh image`, plus govulncheck
on the weekly run, because new vulnerabilities appear without any change in
the repository. On the weekly and manual runs the zizmor and Trivy results
are uploaded to the repository's Security tab (code scanning).

**Core: `.github/workflows/codeql.yml`** runs CodeQL (`security-extended`
queries) on pushes to `main`, on pull requests that touch code or
workflows, and every Monday, for Go, JavaScript (`web/`) and the workflows
themselves. Unlike gosec it follows data across functions, for example from
a fetched page into a file path or into the DOM. Findings appear in the
Security tab and as annotations on pull requests. It is not part of
`scripts/check.sh`.

**Core: `.github/workflows/scorecard.yml`** runs the
[OpenSSF Scorecard](https://scorecard.dev) every Monday and when branch
protection changes. It rates the repository's security practices; the
results go to the Security tab and to the public Scorecard API, which
serves the badge in the README. Publishing signs the results with the
workflow's OIDC identity; no secret or app is involved.

Every workflow grants the `GITHUB_TOKEN` only `contents: read`; write
permissions are set on the single job that needs them.

**Core: `.github/workflows/image.yml`** runs on pushes to `main` (only if
`backend/`, `web/` or the `Containerfile` change), on `v*` tags, or manually.
It first runs the Trivy gate (`scripts/check.sh image`); only if that
passes does it build the image for `linux/amd64` and `linux/arm64` and push
it to `ghcr.io/<owner>/<repo>`:

| Tag | When |
|---|---|
| `latest` | every build of `main` |
| `sha-<short>` | every build, for pinning |
| `1.2.3`, `1.2` | Git tag `v1.2.3` |

The image is pushed with the built-in `GITHUB_TOKEN`
(job permission `packages: write`); no separate token is needed. So that
instances can pull the image without logging in, set the package to
**public** once (GitHub → Packages → openlitfass → Package settings → Change
visibility).

In the `Containerfile`, the Go stage always runs on the architecture of the
build machine and cross-compiles for the target (`--platform=$BUILDPLATFORM`,
`GOARCH=$TARGETARCH`), which is considerably faster than running under QEMU.
The runtime image runs as the unprivileged user `65532:65532`
([instance.md](instance.md#file-permissions)).

**Pinned actions and Dependabot.** Every action is pinned to a commit SHA
with its version as a comment (`@<sha> # v7.0.1`), because a tag can be
moved to other code. `.github/dependabot.yml` proposes updates once a week
as pull requests, for Go modules, the base images in the `Containerfile`
and the actions (SHA and comment together, as one grouped pull request).
New releases are proposed only after a 7-day cooldown. Security updates
(repository setting *Dependabot security updates*, Settings → Advanced
Security) are not delayed by it. Dependabot is built into GitHub: no token
and no third-party app. Updated by hand: the Go release (`go` line in `backend/go.mod`) and
the tool versions in `scripts/check.sh`.

**Core: `.github/workflows/pages.yml`** publishes the project website to the
`pages` branch; see [pages.md](pages.md).

**Instance: `.github/workflows/update-events.yml`** (template in
`example-instance/`) runs every six hours via cron, manually, and on pushes
to `sources/`, `inbox/`, `site/`, `instance.json`, `categories.json` and
`holidays.json`. It starts the image with `docker run` (not as a job
container, because GitHub's JavaScript actions such as `checkout` do not run
in Alpine containers), runs `fetch` and `build-site`, and commits `data/`,
`sources/` (status) and `inbox/` (deleted files) back. It works the same way
on Forgejo or Gitea runners with Docker. The template does not publish `public/`; weissens.ee
shows how to publish to GitHub Pages (see [pages.md](pages.md)).

**Before an instance's first run:** set `IMAGE` in `update-events.yml` to
`ghcr.io/schreibtischhelden/openlitfass:<tag>`.

## Test status

- `scripts/check.sh` passes locally and in CI: gofmt, go vet, golangci-lint
  (0 issues), `go test -race`, `build-site` smoke test and govulncheck (no
  vulnerabilities, Go 1.27).
- `fetch` against the real wcs source Familienzentrum Weißensee: 95 events,
  status field and `sources.json` correct; an inbox event is included.
- `build-site` against `example-instance` and weissens.ee: complete site
  including manifest, `site.json` and stamped service worker.
- `json` against a real source: Brotfabrik (WordPress "The Events Calendar",
  4 pages, 192 events in 30 days) with pagination, template address,
  `categoryMap` and HTML cleaning.
- Go unit tests for text cleaning, templates, prices, pagination,
  filters/`categoryMap` and writing back source files.
- `ical` against a real Events Manager export (Freilichtbühne Weißensee) as a
  test fixture, plus tests for time zones, all-day events, RRULE, EXDATE and
  RECURRENCE-ID.
- `html` with a hand-written fixture modelled on a Joomla JEvents list:
  selector engine, German month names, all-day events and cancellation
  detection in titles.
- `rss` against a real source: KuBiZ Raoul Wallenberg in weissens.ee.
