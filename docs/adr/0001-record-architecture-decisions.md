# 0001. Record architecture decisions

- Status: Accepted
- Date: 2026-09-30
- Phase: 0 (Platform)

## Context

MyCloud is built over ten roadmap phases, largely in sessions with Claude Code. Decisions made in one
session (why Garage, why tus, why 404 instead of 403) are easy to lose, and a later session, human or
AI, may undo them without knowing why they were made. The architecture doc
(`docs/MyCloudArchitecture&BuildPlan.pdf`) is the starting design. Decisions made after it need their
own record.

## Decision

We record every architecturally significant decision as a lightweight Architecture Decision Record
(Michael Nygard format) in `docs/adr/`:

- File name `NNNN-kebab-case-title.md`, numbered sequentially from 0001, numbers never reused.
- Sections: Context, Decision, Alternatives considered, Consequences. Header lists status, date and
  roadmap phase.
- Accepted ADRs are not rewritten. A changed decision gets a new ADR, and the old one is marked
  `Superseded by NNNN`.
- The ADR is committed in the same PR as the change it justifies.
- The `adr` project skill (`.claude/skills/adr/SKILL.md`) holds the template, and `CLAUDE.md` requires
  an ADR for every architectural decision.

## Alternatives considered

- **Only the architecture doc:** a single document drifts and loses the history of why it changed.
- **Decisions in PR descriptions or issues:** hard to find later and not versioned with the code.
- **No written record:** fastest now, but each new Claude Code session would lose the reasoning.

## Consequences

- Slight overhead per decision. ADRs are kept short to limit it.
- New sessions can read `docs/adr/` to recover the reasoning behind the current design.
- Reviewers can reject architectural changes that arrive without an ADR.
