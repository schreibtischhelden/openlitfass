# GitHub Pages mit eigener Domain

Gilt für die Projektseite (`openlitfass.org`, Workflow
`.github/workflows/pages.yml`) und genauso für Instanzen, die über GitHub
Pages veröffentlichen (z. B. weissens.ee, Workflow `update-events.yml`).
Beide Workflows schreiben die fertige Seite in den Branch `pages`, samt
`CNAME`-Datei mit der Domain.

## 1. DNS beim Domain-Anbieter

Für die Hauptdomain (Apex, z. B. `openlitfass.org`):

| Typ | Name | Wert |
|---|---|---|
| A | `@` | `185.199.108.153` |
| A | `@` | `185.199.109.153` |
| A | `@` | `185.199.110.153` |
| A | `@` | `185.199.111.153` |
| AAAA | `@` | `2606:50c0:8000::153` |
| AAAA | `@` | `2606:50c0:8001::153` |
| AAAA | `@` | `2606:50c0:8002::153` |
| AAAA | `@` | `2606:50c0:8003::153` |
| CNAME | `www` | `<organisation>.github.io.` |

`<organisation>` ist der GitHub-Besitzer des Repos: für openlitfass
`schreibtischhelden.github.io`, für weissens.ee `weissensee.github.io`. Der
Repo-Name gehört **nicht** in den CNAME. GitHub leitet `www` automatisch auf
die Hauptdomain um.

Vorhandene A-/AAAA-Records des Anbieters (Parkseite) für `@` und `www`
vorher löschen. Keine Wildcard-Records (`*`) anlegen.

## 2. Domain bei GitHub verifizieren (empfohlen)

Organisation → Settings → Pages → "Add a domain": GitHub nennt einen
TXT-Record der Form

| Typ | Name | Wert |
|---|---|---|
| TXT | `_github-pages-challenge-<organisation>` | (Wert von GitHub) |

Danach kann niemand sonst die Domain für eine eigene Pages-Seite verwenden.

## 3. Pages im Repo einschalten

Repo → Settings → Pages:

1. Source: "Deploy from a branch", Branch `pages`, Ordner `/ (root)`.
   (Der Branch entsteht beim ersten Lauf des Workflows.)
2. Custom domain: ist durch die `CNAME`-Datei schon eingetragen.
3. "Enforce HTTPS" anhaken, sobald GitHub das Zertifikat ausgestellt hat
   (nach der DNS-Prüfung, meist Minuten, selten bis 24 h).

## Prüfen

```sh
dig +short openlitfass.org A
dig +short www.openlitfass.org CNAME
```

Erwartet: die vier `185.199.10x.153` bzw. `schreibtischhelden.github.io.`
