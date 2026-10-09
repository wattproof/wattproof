# Contributing

Thank you for considering a contribution. Wattproof is Apache-2.0
([ADR-0008](docs/adr/0008-open-source-boundary.md)); every contribution is made under that licence.

## Sign off every commit (DCO)

Every commit must carry a `Signed-off-by` line with your real name and email:

```
Signed-off-by: Jane Doe <jane@example.org>
```

`git commit -s` adds it for you. By signing off you certify the Developer Certificate of Origin
below: that you wrote the change, or otherwise have the right to submit it under the project's
licence. It is the same rule the Linux kernel, Kubernetes and most CNCF projects use. There is no
separate agreement to sign.

CI checks every commit in a pull request. A commit without a matching sign-off fails the check;
`git commit --amend -s` (or `git rebase --signoff main` for several commits) fixes it.

Why: the sign-off is a dated record that each contribution may be distributed under Apache-2.0.
That keeps the code's provenance clear for everyone who uses or redistributes it.

## Before you open a pull request

- `gofmt`, `go vet ./...` and `go test -race ./...` pass.
- A change to behaviour comes with a test that fails without it.
- A change to a decision comes with an ADR in `docs/adr/`, or an edit to the one it amends.
- Public documents follow the [publishing rules](docs/publishing.md).

## Developer Certificate of Origin

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

Source: [developercertificate.org](https://developercertificate.org/).
