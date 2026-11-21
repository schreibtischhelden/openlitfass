# Finding sources (discovery playbook)

This guide is deliberately **location-independent** and applies to every
openLitfass instance. The location-specific inventory (what has already been
found and checked) belongs in the instance itself, for example
[`docs/sources-checklist.md`](https://github.com/weissensee/weissens.ee/blob/main/docs/sources-checklist.md)
in weissens.ee.

It is based on research into comparable open-source projects, in particular
[judell/community-calendar](https://github.com/judell/community-calendar)
(see its `docs/procedures.md` for full details), condensed here to what
matters in our context (Go and the source types described in
[backend.md](backend.md)).

## Principle: comprehensive rather than curated

Better one source too many than one too few. Small, inconspicuous sources are
often more productive than they look: associations, parishes, schools, sports
clubs, neighbourhood initiatives, repair cafés and local newspapers. When in
doubt, add the source and narrow it down later with the `categories` or
`daysAhead` filters (see [Common filters](backend.md#common-filters-all-source-types))
instead of discarding it up front.

## 1. Search

Search engines often index the raw content of ICS files, which makes them
directly discoverable:

```
"BEGIN:VCALENDAR" "<place/neighbourhood>"
```

Further productive search patterns (replace `<place>`). The German terms are
intentional, as they are what local organisers use:

| Search | Finds |
|---|---|
| `<place> site:meetup.com` | Meetup groups with ICS export |
| `<place> "Veranstaltungskalender"` | local aggregators, district office pages |
| `<place> Kiezzeitung OR Nachbarschaftsinitiative` | small, often overlooked sources |
| `<place> Gemeinde OR Kirchengemeinde Veranstaltungen` | parishes |
| `<place> Sportverein Termine` | sports clubs |
| `<place> "Add to Calendar" OR "iCal"` | pages with an ICS/iCal export |

Prefer DuckDuckGo if Google blocks automated queries.

## 2. Known platform patterns

If a source runs on one of these platforms, there is usually a direct feed and
no scraping is needed:

| Platform | Feed URL pattern |
|---|---|
| Meetup | `https://www.meetup.com/{group}/events/ical/` |
| Tockify | `https://tockify.com/api/feeds/ics/{calendar-name}` |
| WordPress: The Events Calendar (Tribe) | `https://example.org/events/?ical=1` |
| WordPress: Modern Events Calendar | `https://example.org/events/?mec-ical-feed=1` |
| WordPress: Weekly Class Schedule (wcs) | no public feed; inspect the browser's Network tab while the calendar loads (see below) |
| Squarespace | `https://example.com/events?format=json` |
| Embedded Google Calendar | extract the calendar ID from the embed code, then `https://calendar.google.com/calendar/ical/{ID}/public/basic.ics` |
| GrowthZone (business and trade associations) | `https://business.{association}.com/api/events` |

## 3. No official feed: the browser's Network tab

This is how the `wcs` source for Familienzentrum Weißensee was found (see
[wcs sources](backend.md#wcs-sources-wordpress-weekly-class-schedule-plugin)
for the full story). Open the calendar page in the browser, open DevTools →
Network, filter on XHR/Fetch, reload the page or switch the week view, and
look for the request that actually loads the events (often `admin-ajax.php`
on WordPress, or an `/api/` path). Derive the response format and query
parameters from it.

**If an ICS or JSON export is blocked by a firewall or WAF:** sometimes the
underlying REST API is less strictly protected than the dedicated export
endpoint. With WordPress and The Events Calendar, for example,
`wp-json/tribe/events/v1/events/` is worth trying as a fallback when
`?ical=1` is blocked.

## 4. Test before connecting a source

```sh
curl -sL "<FEED-URL>" -A "Mozilla/5.0" | grep -c "BEGIN:VEVENT"   # ICS: number of events
curl -sL "<JSON-URL>" | python3 -m json.tool | head -50            # JSON: inspect the structure
```

Check: does the response contain real, current events? Is the time zone
correct (see the warning in
[wcs sources](backend.md#wcs-sources-wordpress-weekly-class-schedule-plugin))?
How often does the content change (relevant for `daysAhead` and the cron
schedule of the workflow)?

## 5. Document

Record found, checked and rejected sources in the instance's source checklist
(or the local equivalent in your own fork); see that file for its format.

## 6. Connect

- Does the source fit `type: "rss"`, `"json"`, `"ical"` or `"html"`? Add a new
  file to `<instance>/sources/` based on the `.example.json` templates; no Go
  code is needed (see [How field mapping works](backend.md#how-field-mapping-works)).
- WordPress with the "Weekly Class Schedule" plugin? Use `type: "wcs"` (see
  [wcs sources](backend.md#wcs-sources-wordpress-weekly-class-schedule-plugin)).
- None of these fit (custom scraping, pagination, authentication)? Write your
  own `Source` implementation based on `internal/sources/example.go` and
  register it via `harden(withCommonFilters(...))` (see
  [New source with custom Go code](backend.md#new-source-with-custom-go-code-exceptional-cases)).
