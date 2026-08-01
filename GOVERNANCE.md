# OpenCodeReview Governance

This document describes how OpenCodeReview is governed today and how technical
decisions are made in the project.

It is intended to reflect the project's current open development practice. As
OpenCodeReview grows, this document may evolve to support a more formal
governance model.

## Goals

OpenCodeReview governance is designed to keep the project:

- open to contributors and users
- pragmatic in day-to-day decision making
- transparent in technical direction
- safe for public API and security-sensitive changes
- sustainable across multiple components and maintainers

## Scope

This document applies to the OpenCodeReview repository and its major public
surfaces, including:

- the CLI tool and its subcommands
- LLM integration and provider interfaces
- diff parsing and code review engine
- configuration and plugin system
- SDKs, examples, and project documentation

## Project Values

OpenCodeReview maintainers and contributors are expected to act consistently
with these principles:

- **Open development**: design discussion, issue tracking, and code review
  happen in public whenever possible.
- **Compatibility awareness**: CLI behavior, configuration formats, and
  documented user workflows should not change casually.
- **Security first**: changes affecting credentials, API keys, or execution
  safety require extra scrutiny.
- **Component ownership with cross-project accountability**: subsystem
  maintainers own their areas, while cross-cutting changes require broader
  review.
- **Documentation and implementation alignment**: public contracts, code,
  examples, and docs should stay consistent.

## Roles

### Contributors

Contributors are anyone who participates in the project, including by:

- opening issues or discussions
- submitting pull requests
- reviewing code
- improving docs, tests, examples, or tooling

Contributors are expected to follow:

- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- [CONTRIBUTING.md](CONTRIBUTING.md)
- [SECURITY.md](SECURITY.md) for vulnerability reporting

### Maintainers

Maintainers are trusted contributors who have demonstrated sustained,
high-quality contributions and a solid understanding of the codebase. They have:

- write access to the repository
- authority to review and merge pull requests
- responsibility to uphold code quality and project standards

Maintainer responsibilities include:

- reviewing pull requests for owned areas
- helping preserve code quality, compatibility, and security
- requesting cross-component review when a change affects other surfaces
- keeping implementation, tests, docs, and examples aligned when needed
- helping contributors land changes successfully

New Maintainers are nominated by existing Maintainers and approved by the
Project Lead.

### Project Lead

The Project Lead provides overall direction for the project and has final
authority on decisions when consensus cannot be reached. The current Project Lead
is [@lizhengfeng101](https://github.com/lizhengfeng101).

## Decision Making

### Day-to-Day Changes

Most changes are made through the normal pull request workflow described in
[CONTRIBUTING.md](CONTRIBUTING.md):

1. discuss the change in an issue when appropriate
2. submit a pull request
3. pass automated checks
4. receive maintainer review for affected areas
5. address feedback
6. merge once approved

Normal changes are decided by maintainer review and rough consensus.

### Significant Changes

Changes with broader impact require wider review. This includes changes to:

- CLI behavior or command interfaces
- LLM provider integration interfaces
- configuration formats or defaults
- diff parsing or review engine behavior
- security-sensitive areas such as credential handling

For these changes, maintainers should seek explicit review from all materially
affected areas, not just the first area touched by the patch.

Significant changes (new features, architectural changes, breaking changes)
should be proposed via a GitHub Issue before implementation to allow community
discussion.

### Consensus and Voting

OpenCodeReview prefers **lazy consensus** for most technical decisions:

- if affected maintainers agree, the change may proceed
- if concerns are raised, they should be addressed in the PR or issue discussion

If consensus cannot be reached in a reasonable time, the Project Lead has final
decision-making authority.

## Reviews and Merge Expectations

The following expectations apply before merge:

- relevant CI checks should pass, or failures must be understood and accepted by
  maintainers
- at least one maintainer of the affected area should review the change
- cross-cutting changes should be reviewed by all materially affected areas when
  practical
- breaking changes should be clearly called out, with migration guidance where
  needed
- docs and tests should be updated when behavior changes

Maintainers may decline or defer a change if it:

- conflicts with approved design direction
- introduces unnecessary compatibility risk
- weakens security without a strong justification
- mixes unrelated work into a single change

## Communication Channels

The project's public collaboration channels are:

- GitHub Issues for bugs, feature requests, and implementation questions
- GitHub Discussions for broader design discussion and community help
- pull requests for concrete code and documentation review

Security issues should follow the private reporting guidance in
[SECURITY.md](SECURITY.md).

## Continuity

OpenCodeReview is hosted under the [Alibaba](https://github.com/alibaba) GitHub
organization. Multiple members of the organization have administrative access to
the repository, ensuring that the project can continue to operate even if any
single individual becomes unavailable.

Additionally:

- Multiple Maintainers are familiar with the full codebase, ensuring no single
  point of failure in terms of project knowledge.
- The bus factor of the project is greater than one — critical subsystems
  (agent, LLM integration, diff parsing, configuration) are understood by more
  than one person.
- Repository credentials, CI/CD pipelines, and release processes are accessible
  to multiple team members within the Alibaba organization.

## Becoming a Maintainer

New maintainers are selected based on sustained, high-quality contribution to
the project.

Signals that someone may be ready for maintainership include:

- repeated high-quality code or documentation contributions
- strong reviews and constructive technical feedback
- reliable follow-through on owned work
- good judgment on compatibility, security, and project direction
- collaborative behavior with contributors and maintainers

The typical process is:

1. nomination by an existing Maintainer
2. discussion among Maintainers and the Project Lead
3. no unresolved objections after a reasonable review period
4. granting of write access and update of any relevant maintainer records

## Maintainer Inactivity and Removal

Maintainers may step down at any time by notifying the project.

The Project Lead may also update maintainer status when someone has been inactive
for an extended period, for example several months without meaningful review or
maintenance activity.

Removal should be handled respectfully and pragmatically, with the goal of
keeping ownership accurate rather than punitive.

Maintainers may also be removed for serious violations of project expectations,
including repeated abuse of project privileges or violations of the Code of
Conduct.

## Changes to Governance

Changes to this governance document should be made through a public pull request.

Governance changes should receive review from Maintainers and the Project Lead,
and should not be merged without giving maintainers and contributors a
reasonable opportunity to comment.
