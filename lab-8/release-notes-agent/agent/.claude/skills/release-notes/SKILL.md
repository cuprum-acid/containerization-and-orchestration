---
name: release-notes
description: Decide the semver bump and write release notes for the Tickerline public API from the changes since the last tag. Use when asked to prepare a release.
---

# Release notes for the Tickerline API

The readers are integrators at our clients: developers who call the API from
their own products. They read the notes to answer one question - "do I have
to change my code, and how?". Write for them, not for our team.

The public contract is `lab-8/release-notes-agent/api/openapi.yaml`. Commit
messages and PR titles are a hint, not the truth: a commit called
"refactor serializer" can rename a field. When they disagree with the
contract, the contract wins.

Only `lab-8/release-notes-agent/api/` belongs to the API. Changes elsewhere in
the repository are not part of the release - ignore them.

## Order of work

1. Read the PRs. For every commit in the request that references a PR
   (`(#12)` or `Merge pull request #12`), read it with `pull_request_read`
   (`get`, then `get_comments` if the description is empty). Note what the
   author says changed.
2. See what changed. Run `git_diff` against the last tag and look only at
   files under the API directory.
3. Compare the contract. Read the old `openapi.yaml` with
   `git_show <last-tag>:<path>` and the current one with `read_text_file`.
   List every path, method, parameter, request field and response field that
   was added, removed, renamed, changed type, became required, or got
   `deprecated: true`.
4. Check the code. If the contract did not change, or a new endpoint has no
   description, read `app/main.py` with `read_text_file`: behaviour can
   change without the spec (validation, defaults, what a field means).
5. Decide the bump by the rules below. The highest rule that matches wins.
6. Write the title and the notes.

## Bump rules

### major - an existing integration can break
- A path or method removed.
- A field removed or renamed in a response or a request body.
- A field changed type or format (`string` -> `number`, a new date format).
- A request field or query parameter became required, or a new required one
  was added: old clients do not send it and now get 422.
- An enum value removed from a request or a response.
- Behaviour changed in a way an old client notices, even with the same spec:
  stricter validation, a different default, a field that now means something
  else.

### minor - something new, nothing breaks
- A new path or method.
- A new optional request field or query parameter.
- A new field in a response: clients ignore fields they do not know.
- Something marked `deprecated: true`. It still works, so this is not major -
  but it must be in the notes.

### patch - nothing in the contract changed
- Bug fixes, performance, internal refactoring, docs - and the code check in
  step 4 found no visible behaviour change.

If you are unsure between two levels, take the higher one and say why in the
notes. A false major costs a client five minutes of reading; a false patch
costs a night incident.

## Writing the notes

Sections, in this order, only those that have entries:

- **Breaking changes** - what changed, who is affected, and what to change.
  Always give a before/after `curl` with the real path and fields.
- **Added** - every new endpoint or field with one sentence on what it does.
  If the spec has no description, explain it from the code. Never leave a
  bare path.
- **Deprecated** - what is deprecated, what to use instead, and the removal
  date. If the code or the PR gives no date, write "No removal date has been
  set yet." Never invent one.
- **Fixed** - bug fixes visible from outside, in the client's words
  ("`GET /v2/orders/{id}` returned 500 for cancelled orders").

Rules:
- Use `https://api.tickerline.example` as the host in examples.
- No internal details: no class names, commit hashes, author names, ticket
  numbers. Link the PR at the end of an entry: `(#12)`.
- Do not describe changes you did not see in the diff, and do not pad: one
  real line is better than three vague ones.
- The title is one line, under 70 characters, naming the most important
  change: "Orders: price renamed to limit_price", not "Release 2.0.0".

Example of a breaking entry:

````markdown
### Breaking changes

`POST /v2/orders`: the field `price` is renamed to `limit_price`. Requests
with `price` are rejected with 422, and responses no longer contain it.

Before:
```bash
curl -X POST https://api.tickerline.example/v2/orders \
  -d '{"ticker": "SBER", "side": "buy", "quantity": 1, "price": 312.4}'
```
After:
```bash
curl -X POST https://api.tickerline.example/v2/orders \
  -d '{"ticker": "SBER", "side": "buy", "quantity": 1, "limit_price": 312.4}'
```
(#12)
````

## Output

Return exactly the JSON object the request asks for, with no text around it.
The pipeline that called you validates it and creates the release itself -
you only propose.
