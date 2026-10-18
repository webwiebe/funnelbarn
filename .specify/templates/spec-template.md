# Feature Specification: [FEATURE NAME]

**Spec**: `specs/[NNN-feature-name]/`
**Status**: Draft | Shipped | Partial | Deferred | Superseded
**Last verified against the tree**: [DATE] at `[short sha]`
**Issues**: #[epic], #[issue]
**Principles touched**: [constitution principle numerals, e.g. I, III]

<!--
A refined GitHub issue maps onto this file:
  issue "What" and "Why"        -> Purpose
  issue "Acceptance Criteria"   -> Acceptance Scenarios and Functional Requirements
  issue "Decision for you"      -> Open Questions (mark needs-you)
The issue's "Execution Plan" and "Implementation Details" go to plan.md, and its
checklist goes to tasks.md.

Rules:
  - The spec describes the tree. Where a spec, another document or an issue
    disagrees with the code, the code is the fact. Record it under Drift.
  - A retired area keeps its folder with Status: Deferred or Superseded and a pointer.
  - Numbers are never reused or renumbered.
  - Results, load-test output and design sketches belong in docs/, not here.
-->

## Purpose

[What the feature does and the question it answers for the user. Two to five sentences.]

## Users

- **[Role]**: [what they need from this feature]

## User Stories

### US1 - [Title] (Priority: P1)

[The user journey in plain language, as it ships today.]

**Acceptance Scenarios**:

1. **Given** [state], **When** [action], **Then** [outcome]

## Functional Requirements

Each requirement names the issue that asked for it and the test that proves it.
A requirement with neither is marked `[inferred]`. A requirement the code does not
meet yet is marked `[gap]` and names an issue.

- **FR-001**: The system MUST [behaviour]. Issues: [#123](https://github.com/webwiebe/funnelbarn/issues/123). Tests: `[path::test_name]`.

## Non-Functional Requirements

- **NFR-001**: [performance, cost, privacy, or tenancy constraint]. Source: [issue, test, or gate].

## Key Entities

- **[Entity]**: [description, owning package]

## Non-Goals

- [What this feature deliberately does not do, with its source.]

## Open Questions

- #[n] [open issue or needs-you decision, one line]

## Drift

Where a document or issue disagrees with the tree. The tree is the fact. An empty
table is a valid answer; write `not established` when the tree cannot answer.

| Claim | Source | Tree |
|---|---|---|
| [what the document says] | [document or issue] | [what the code does, with the path] |

## Proof contract

Each item states one deliverable as an outcome someone who did not write the code can observe.

- [ ] [Observable outcome, with the evidence that shows it: a screenshot, an e2e run, or `path::test_name`.]

## Sources

- [docs, ADRs, issues, test files, code paths read to write this spec]
