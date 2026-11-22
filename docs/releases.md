# Roadmap and releases

The roadmap is a GitHub Project; every planned version is a milestone; a
version is released by pushing a git tag of the same name. The tag, the
milestone, the GitHub release and the container image all carry the same
version.

```
GitHub Project "openLitfass roadmap"   (board + roadmap view, grouped by milestone)
  └─ issues ──► milestone v0.2.0 ──► git tag v0.2.0 ──► release.yml: GitHub release, milestone closed
                                                   └──► image.yml:   ghcr.io/…/openlitfass:0.2.0, :0.2
```

## Versions

openLitfass uses [Semantic Versioning](https://semver.org/) with a `v` prefix
on tags: `v0.1.0`, `v0.2.0`, `v1.0.0`.

- **Major** (`v2.0.0`): an instance has to change something, for example a
  renamed `instance.json` field or a changed event schema.
- **Minor** (`v0.3.0`): new features that existing instances can ignore.
- **Patch** (`v0.3.1`): fixes only.

Before `v1.0.0`, a minor version may also contain breaking changes; they are
listed under *Breaking changes* in the release notes.

Pre-releases use a suffix (`v0.3.0-rc.1`). They get a GitHub release marked as
pre-release, do not close the milestone and do not move the `:0.3` image tag
(only `:0.3.0-rc.1` is published).

## Roadmap (GitHub Project)

The project
[openLitfass roadmap](https://github.com/orgs/schreibtischhelden/projects/5)
belongs to the `schreibtischhelden` organisation and contains the issues of
this repository.

| Field | Values | Purpose |
|---|---|---|
| Status | `Ideas`, `Planned`, `In Progress`, `Done` | board columns |
| Milestone | `v0.1.0`, `v0.2.0`, … (built-in) | which release an item goes into |
| Labels | see below | release-note categories |

Views:

- **Board**: columns by *Status*, for day-to-day work. `Ideas` is the
  backlog without a milestone.
- **Roadmap** (roadmap layout): grouped by *Milestone* with milestone markers,
  showing what comes when. The timeline only shows milestones that have a due
  date, so give a milestone a date once it is roughly scheduled.

The project's built-in workflows (*Workflows* in the project menu) add every
new issue of this repository with *Status* `Ideas` and set *Status* to `Done`
when an issue is closed.

An idea becomes a plan by giving it a milestone and moving it to `Planned`.
Issues of instance repositories (for example `weissensee/weissens.ee`) cannot
be added, because a project only accepts issues of repositories of the same
owner; instance-specific work is tracked in the instance repository.

## Labels

| Label | Release-note section |
|---|---|
| `breaking` | Breaking changes |
| `enhancement` | New features |
| `bug` | Fixes |
| `documentation` | Documentation |
| `skip-changelog` | left out |

## Release notes

The *Release* workflow assembles the notes from four parts:

1. **Highlights**: the message of the annotated tag, without its first line.
   A few hand-written bullet points on what matters to users and instance
   operators.
2. **Roadmap items**: the closed issues of the milestone, with their labels.
3. **Pull requests**: GitHub's generated notes, sorted into the label
   sections above (`.github/release.yml`), with a full-changelog link.
4. **Commits**: every commit since the previous tag, collapsed.

## Releasing a version

1. Check the milestone: closed issues belong to it (select them in the issue
   list → *Milestone*); move open ones to the next milestone.
2. Make sure `main` is green (*CI* workflow).
3. Write the highlights into a file, first line the title, then a blank line
   and the bullet points (no lines starting with `#`, git drops them):

   ```text
   openLitfass v0.2.0

   - Places show their opening hours.
   - Fixed: …
   ```

4. Tag and push:

   ```sh
   git tag -a v0.2.0 -F notes.txt
   git push origin v0.2.0
   ```

5. The *Release* workflow (`.github/workflows/release.yml`) creates the GitHub
   release and closes the milestone; the *Image* workflow publishes
   `ghcr.io/schreibtischhelden/openlitfass:0.2.0` and `:0.2`. The version is
   also visible in the metric `openlitfass_info`.
6. Create the next milestone if it does not exist yet.

If a tag has no matching milestone, the release is still created, with a
warning and without roadmap items. A lightweight tag (`git tag v0.2.0`
without `-a`) gives a release without highlights.

## Instances and versions

An instance chooses how closely it follows the core via the image tag in its
`update-events.yml`:

| Tag | Gets |
|---|---|
| `:latest` | every commit on `main` |
| `:0.2` | patch releases of 0.2 (recommended) |
| `:0.2.0` | exactly this release |
