# Agent instructions

## Autonomy

Default to acting: decide, log the decision, keep going. Stop and ask only for:

- Changing a goal, non-goal or scope constraint in ARCHITECTURE.md.
- Adding a runtime dependency (Go module or npm package).
- A migration that drops or rewrites existing user data.
- Anything outward-facing: pushing, deploying, Dokku, DNS, contacting a new external site.
- Deleting or rewriting a plan document or ARCHITECTURE.md section you didn't write.

For anything else that would merit a question, pick the option you judge best. When options are close or you're unsure of my preference, prefer the one that's cheaper to undo. Log it under **Decisions** and add it to an **Open questions** list in the plan document. I review those asynchronously; don't block on them.

## Project invariants

- Personal use: lookups happen on demand only. No crawling, background scanning or bulk storage of third-party data.
- Code, comments, commits and docs in English; UI text in French.
- ARCHITECTURE.md is the living design. When code deliberately diverges from it, update it in the same diff.
- npm enforces a 7-day `min-release-age`. The only exception is the Solid 2 packages (`npm install --min-release-age=0 solid-js @solidjs/web @solidjs/vite-plugin`). Don't change the global config.

## Context hygiene across long sessions

- Keep planning documents in a `plans/` folder at the repo root, one document per task or feature, named `YYYY-MM-DD-short-slug.md` after the day the work starts. Living documents that outlive a single task (a standing backlog, a reusable prompt) keep a bare descriptive name instead.
- Work in the discrete increments defined by the plan document governing the current task; commit at each increment boundary, directly on main, with a very concise description. Never push.
- Maintain a short **Progress** section at the top of that plan document: one line per completed increment, plus any deviation from the plan. Update it when an increment lands.
- At each increment boundary, Progress, Decisions and the git log must be enough to resume the work from scratch.

## Done means verified

An increment is done when the checks pass (`make check` once it exists: go vet, go test, tsc, vite build) and, for user-visible changes, the app was run and the change exercised. Never commit red. If something can't be verified, say so in Progress.

## Decision reporting

Some choices you make while working are worth surfacing even though they didn't block you.

- Log a decision whenever you: pick between viable alternatives, resolve an ambiguity in the plan, deviate from the plan, are uncertain about a decision or its scope, introduce a dependency or pattern not already in the codebase, or defer/skip something the plan implied.
- Do not log routine mechanics, such as naming, formatting, obvious refactors, or anything the plan already specifies.
- Write each entry as: what was decided, the alternative(s) not taken, and the one-line reason.
- Append entries to a **Decisions** section in the plan document as they occur.

## Keep the codebase lean

Every diff should leave the repo simpler, or at least no more complex. Removing is as valuable as adding.

- Swap rule: when a change replaces X with Y, deleting X is part of the change. Old code paths, obsolete tests, stale docs and comments, finished TODOs, all in the same diff. Never keep the old thing for compatibility unless explicitly asked.
- Bug fixes should kill the cause. No special-case `if` shielding a symptom. Re-derive the design so the bug can't exist.
- Reuse before adding: scan for an existing helper or concept before introducing a new one.
- Adding can be subtraction. A new abstraction or generalization earns its place when it deletes more complexity than it adds: absorbs special cases, replaces several ad-hoc mechanisms with one principled one. Judge it by net effect on the system overall.
- If something in the code surprised you or was hard to follow, that's a bad abstraction. Fix it if it's in the area you're already changing, otherwise log it in the simplify backlog (below). Don't launch drive-by refactors outside the current task's scope.
- Old behavior is not sacred. When a simplification within the current task justifies breaking behavior, make the change and log what changed, who could notice, and why the simpler shape wins. Outside the current task, propose it instead. Only keep tests that guard what the project still promises.
- Docs are testimony, not ground truth. Respect high-level goals and character, but detailed claims in docs and comments may be wrong or stale. When doc and code disagree, investigate. Never preserve complexity because a comment claims it's needed.
- A comment earns its place only by stating a constraint the code doesn’t express. If a refactor makes a comment stale, remove it.
- No breadcrumbs. Comments must be coherent for someone with only the source in context. No "as discussed", "per the plan", links to plan files, or step numbers. Process belongs in the plan document.
- Before finishing any task, ask: _what did this change make obsolete, and did I delete it?_

## The system rots between tasks

Diff-level hygiene cannot prevent complexity to accumulate.

- Maintain `plans/simplify-backlog.md` as a living document. Every surprise, duplicated concept, bad abstraction, or "this fought me" moment should become one line there.
- Before starting a task, check the backlog for the area about to be touched. If clearing an entry first would make the task smaller or safer, do it first as its own commit and log it.
- When entries cluster around one area, propose a dedicated simplification task with its own plan document. The largest reductions are redesigns unreachable by in-task steps.
