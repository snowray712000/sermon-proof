# ai-log — Rules (read this before writing or reading logs)

`ai-log/` is a development journal **written by AI, for future AI**. It is not for humans (a human who wants Chinese will ask an AI to translate). It follows the same convention as the maintainer's FHLBible project (`github.com/snowray712000/FHLBible`, `ai-log/README.md`), adapted for this project (sermon-proof).

## Purpose

1. A future AI can recover the **situation and reasoning** of past development: why something exists, what was tried, what was decided.
2. Strong goal: **even if the source code were lost, the logs should be enough to roughly reproduce the program**, possibly in another language. So logs must capture behavior, data formats, API contracts, algorithms, and constraints, not just "files changed".

## Language

English. Keep identifiers, file names, API names, and UI strings in their original form (UI strings in Traditional Chinese stay Chinese, quoted).

## Location and naming

```
ai-log/
  README.md          this file
  index.md           lookup table (see below), always kept up to date
  <YYYY>/s<Q>/<MMDD><seq>_<title>.md
```

- `<Q>` is the calendar quarter: `s1` = Jan–Mar, `s2` = Apr–Jun, `s3` = Jul–Sep, `s4` = Oct–Dec.
- `<MMDD>` is the local date the work was done. Take it from the system's current date, never guess.
- `<seq>` is a lowercase letter sequence per day, in creation order: `a, b, c, … z, aa, ab, … az, ba, …`. To pick the next one, list the quarter folder for that day and continue from the last one. Order is by length, then alphabetically (`z` < `aa`).
- `<title>`: lowercase English `snake_case`, about 2–6 words. No spaces, no Chinese.
- Example: 2026-10-26 → `ai-log/2026/s4/1026a_add_review_checkboxes.md`.
- Create the year / quarter folders if missing.

## When to write a log

Write **one log per meaningful unit of work** at the end of the task (not one per tool call): a feature, a behavior change, a bug fix with a non-obvious cause, a refactor, a data-format discovery (e.g. a new variant of the transcript markdown), a build/config change, an important decision or rejected approach, an investigation that ended without code changes.

Skip pure typo / formatting changes and trivial edits whose diff explains itself.

Also update `index.md` in the same task. Include both in the same commit when the user asks for a commit. Never put secrets in a log.

## When to read logs

At the start of any non-trivial task: read `ai-log/index.md` first, then open only the logs relevant to the area you are about to touch. Logs record the context at that time; if code now disagrees, trust the code but find out why (a later log may explain).

## Append-only

Do not rewrite history. Old logs are not edited except to fix typos, add a `Superseded by:` line at the top, or append a dated `## Addendum` section at the end. If a past decision is reversed, write a new log with `Supersedes: <path>` and explain why.

## Log template

```markdown
# <Human-readable title>

- Date: YYYY-MM-DD
- Project: sermon-proof
- Type: feature | fix | refactor | investigation | config | baseline
- Related logs: <paths, or "none">
- Cross-project: <e.g. ../語音辨識/ file, or "none">
- Commit: <short hash if known, or "uncommitted">
- Supersedes / Superseded by: <path or omit>

## Goal / why
## Starting state
## What changed (behavior level)
## Design and decisions
## Key files / types / functions
## Contracts and algorithms
## Gotchas
## Verification
## Open issues / next steps
```

Sections with nothing to say may be shortened to one line or `n/a`, but keep the headings.

## index.md format

- Short header: what the project is (2–3 lines) and a pointer to this README.
- One section per quarter, **newest quarter first, newest entry first**, as a table:

  `| Log | Type | Summary | Areas / keywords | Files touched |`

  - `Log`: relative link, e.g. `[1026a_add_review_checkboxes](2026/s4/1026a_add_review_checkboxes.md)`
  - `Summary`: one sentence, enough to decide whether to open the log.
  - `Areas / keywords`: terms an AI would grep for.
- Keep rows to one line each. The index is a map, not a duplicate of the logs.

## Cross-project work

This tool consumes transcripts produced by the sibling project `../語音辨識/` (pipeline rules in its `AGENTS.md`). If a task changes the transcript format on either side, note it in the log's `Cross-project:` field.
