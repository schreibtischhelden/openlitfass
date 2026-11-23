# Event inbox

The inbox is the manual counterpart to the automated sources in
`<instance>/sources/`. It is meant for individual events that do not arrive
through any feed: a flyer, a phone call, a tip from the neighbourhood. Place
one JSON file per event in `<instance>/inbox/` and commit it. No entry in
`<instance>/sources/`, no URL and no mapping are needed.

## Format

Use `example-instance/inbox/beispiel-event.example.json` as a template. Files
ending in `.example.json` are not read; to activate one, copy it and remove
`.example` from the file name. The fields correspond directly to the
[event JSON schema](backend.md#event-json-schema-instancedataeventsjson):

- `title` (required), `date` (required, `YYYY-MM-DD`)
- `description`, `category`, `startTime`/`endTime` (`HH:MM`)
- `location: { name, address }`
- `price: { free, amount }`: **if omitted, the event is free**;
  `{ "free": false }` means the price is unknown. Do not set it to `null`, as
  that would show "null €" in the front end.
- `organizer`, `url`
- `ticketRequired`, `registrationRequired`, `familyFriendly`, `cancelled`
  (all optional, default `false`)

Enter dates and times directly as Europe/Berlin wall-clock time. Unlike the
automated sources, no time zone conversion takes place, because a person
enters the time directly.

The file name (without `.json`) is the event's stable ID across runs. When
editing an existing file, do not rename it; otherwise the next run creates a
second, new entry instead of updating the existing one.

## Recurring events (`repeat`)

For opening hours, weekly classes and similar, specify a `repeat` object
instead of `date`; all other fields apply to every occurrence:

```json
{
  "title": "Kinderbauernhof geöffnet",
  "description": "Montag bis Freitag 9–16:30 Uhr, außer an Feiertagen.",
  "startTime": "09:00", "endTime": "16:30",
  "location": { "name": "Kinderbauernhof", "address": "Musterweg 2, 12345 Berlin" },
  "repeat": {
    "weekdays": ["mo", "di", "mi", "do", "fr"],
    "from": "10-01",
    "until": "03-31",
    "except": ["feiertage", "12-24", "12-31"],
    "daysAhead": 60
  }
}
```

- `weekdays`: `mo di mi do fr sa so` (German abbreviations; the English
  `mon`/`tue`/… are accepted as well). Requires `from`.
- `from`/`until`: the period in which `weekdays` apply.
  - Full dates (`"2026-10-01"`): a fixed period. `until` is optional; without
    it the series is open-ended.
  - Month and day only (`"10-01"` to `"03-31"`): a season that recurs every
    year, also across the turn of the year. Both need this form then.
- `dates`: additional days, for example public holidays with weekend opening
  hours (also works without `weekdays`).
- `except`: days on which the event does not take place.
- `daysAhead`: how far ahead occurrences are generated (default 60 days), so
  that the list does not fill up with months of identical entries.

### Days in `dates` and `except`

| Entry | Meaning |
|---|---|
| `"2027-05-06"` | once, on that date |
| `"12-25"` | every year on 25 December |
| `"*-01"` | every month on the 1st |
| `"easter"`, `"easter+1"`, `"easter-2"` | relative to (Western) Easter Sunday: Easter Monday, Good Friday; `easter+39` Ascension Day, `easter+49`/`+50` Whit Sunday/Monday, `easter+60` Corpus Christi |
| `"feiertage"` | the name of a day list from `holidays.json` (see below) |

Easter is the only day openLitfass computes. Everything else, including which
public holidays apply, is up to the instance.

### Day lists (`holidays.json`)

`<instance>/holidays.json` (optional) defines named day lists that every
inbox file can refer to, so that public holidays, school holidays or other
recurring days are maintained in one place. Each list contains days as in
the table above, or the names of other lists:

```json
{
  "ostern": ["easter-2", "easter", "easter+1"],
  "pfingsten": ["easter+39", "easter+49", "easter+50"],
  "feiertage": ["01-01", "03-08", "05-01", "10-03", "12-25", "12-26", "ostern", "pfingsten"],
  "kuchen-fuer-tobias": ["*-01"]
}
```

`example-instance/holidays.example.json` is a template (public holidays in
Berlin); copy it to `holidays.json` and adapt it. List names must not look
like a day (`easter`, `12-25`). Write the wording for visitors, such as
"außer an Feiertagen", into `description` yourself.

If `holidays.json` is invalid (invalid JSON, an unknown entry, a list that
includes itself), the whole inbox source fails for that run and keeps its
events from the previous run; the log names the problem. An inbox file that
refers to an unknown list is skipped like any other invalid file.

### IDs and clean-up

Each occurrence gets its own stable ID (`<file name>@<date>`). The file is
deleted once `until`, or the last date in `dates`, has passed. Files without
`until`, with a season, or with yearly, monthly or Easter-based days in
`dates` never expire; delete them yourself when they are no longer needed.
Create separate files for different times (summer/winter, weekdays/weekends).

## Workflow

1. Add the file, commit and push.
2. The instance's `update-events.yml` workflow runs (it is triggered by pushes
   to `inbox/**`) and includes the event in `data/events.json` from then on.
3. The file stays in place and is read again on every run, so the event
   remains visible. Only once its date is in the past does `InboxSource`
   (`internal/sources/inbox.go`) delete the file automatically; the workflow
   commits the deletion (see the instance's
   `.github/workflows/update-events.yml`). This is exactly the point at which
   the event would drop out of the calendar anyway.

Why the file is not deleted immediately after it has been processed:
`cmd/fetch` rebuilds `data/events.json` from scratch on every run, using the
current fetch results of all sources; it does not read the previous file. If
the inbox file were deleted straight away, the event would disappear again on
the next run, because no source would provide it any more.

## Invalid files

A file with invalid JSON, a missing `title`, an unparsable `date` or an
invalid `repeat` is skipped and **not** deleted, so nothing is lost. Instead,
the run logs a clear error message including the file name. Fix the file and commit it again.
