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

- `weekdays`: `mo di mi do fr sa so` (German abbreviations; the English
  `mon`/`tue`/… are accepted as well). Requires `from` and `until`.
- `dates`: additional single dates, for example public holidays with weekend
  opening hours (also works without `weekdays`).
- `except`: dates on which the event does not take place.
- `daysAhead`: how far ahead occurrences are generated (default 60 days), so
  that the list does not fill up with months of identical entries.

Each occurrence gets its own stable ID (`<file name>@<date>`). The file is
deleted once `until`, or the last date in `dates`, has passed. Create separate
files for different times (summer/winter, weekdays/weekends).

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

A file with invalid JSON, a missing `title` or an unparsable `date` is skipped
and **not** deleted, so nothing is lost. Instead, the run logs a clear error
message including the file name. Fix the file and commit it again.
