# FunnelBarn

Working rules for this repository. See also `CLAUDE.md` for the CI/CD pipeline and environments.

## Specs And Constitution

Work here is spec-driven. Before changing behaviour, read:

- `.specify/memory/constitution.md`: the principles CI and the tests enforce, each with its source.
- `specs/README.md`: the index of feature specs (one per product area) and the workflow.
- The `specs/NNN-<area>/spec.md` for the area you are changing.

A new feature starts as a spec or a spec change referenced from its issue. A PR
that changes behaviour updates the spec it implements in the same PR, including
its Drift section. When a spec, the docs and the code disagree, the code is the
fact; fix the document that is wrong.
