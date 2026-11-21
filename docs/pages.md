# GitHub Pages with a custom domain

This applies to the project website (`openlitfass.org`, workflow
`.github/workflows/pages.yml`) and equally to instances that publish via GitHub
Pages (for example weissens.ee, workflow `update-events.yml`). Both workflows
write the finished site to the `pages` branch, including a `CNAME` file
containing the domain.

## 1. DNS at the domain registrar

For the apex domain (for example `openlitfass.org`):

| Type | Name | Value |
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

`<organisation>` is the GitHub owner of the repository:
`schreibtischhelden.github.io` for openlitfass and `weissensee.github.io` for
weissens.ee. The repository name does **not** belong in the CNAME. GitHub
automatically redirects `www` to the apex domain.

Delete any existing A or AAAA records set by the registrar (parking page) for
`@` and `www` beforehand. Do not create wildcard records (`*`).

## 2. Verify the domain with GitHub (recommended)

Organisation → Settings → Pages → "Add a domain": GitHub provides a TXT record
of the form

| Type | Name | Value |
|---|---|---|
| TXT | `_github-pages-challenge-<organisation>` | (value provided by GitHub) |

Once verified, nobody else can use the domain for their own Pages site.

## 3. Enable Pages in the repository

Repository → Settings → Pages:

1. Source: "Deploy from a branch", branch `pages`, folder `/ (root)`.
   (The branch is created by the workflow's first run.)
2. Custom domain: already set by the `CNAME` file.
3. Tick "Enforce HTTPS" once GitHub has issued the certificate (after the DNS
   check; usually minutes, occasionally up to 24 hours).

## Checking

```sh
dig +short openlitfass.org A
dig +short www.openlitfass.org CNAME
```

Expected: the four `185.199.10x.153` addresses and
`schreibtischhelden.github.io.` respectively.
