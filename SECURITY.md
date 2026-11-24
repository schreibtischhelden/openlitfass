# Security policy

## Supported versions

Security fixes go into `main` and the next release. The container image
`ghcr.io/schreibtischhelden/openlitfass:latest` is rebuilt from `main`, so
instances on `:latest` receive fixes automatically; instances pinned to a
version tag should move to the newest release.

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

Report it privately through GitHub instead:
**Security → Report a vulnerability** in this repository
(<https://github.com/schreibtischhelden/openlitfass/security/advisories/new>).

Helpful details:

- what is affected (back end, front end, container image, workflows);
- steps to reproduce, or a proof of concept;
- the impact you expect.

You will get an answer within a week. Once a fix is released, the advisory
is published and you are credited, unless you prefer otherwise.

## What is checked automatically

Every change runs through static analysis (golangci-lint with gosec,
CodeQL), the race detector, `govulncheck`, a Trivy scan of the container
image and an audit of the workflows (actionlint, zizmor). The scans repeat
weekly. Details: [Checks before pushing](https://docs.openlitfass.org/?page=development#checks-before-pushing).
