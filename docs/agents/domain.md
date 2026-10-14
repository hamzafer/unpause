# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

Layout: **single-context**. This repo's glossary is **`CONTEXT.md`** at the root (it plays the role the skills call `GLOSSARY.md`); decisions are in **`docs/adr/`**.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root: the shared vocabulary (Session, Transcript, Account, Opener, Deep search, ...).
- **`docs/adr/`**: read ADRs that touch the area you're about to work in.

New terms go into `CONTEXT.md` in its existing format (term, definition, _Avoid_ line). Don't create a separate `GLOSSARY.md`.

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms it explicitly avoids.

If the concept you need isn't there yet, either you're inventing language the project doesn't use (reconsider) or there's a real gap (add it).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0004 (parse transcripts directly), but worth reopening because…_
