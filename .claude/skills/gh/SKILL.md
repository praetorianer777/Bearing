---
name: gh
description: GitHub workflow for this repo — read before any code change. Creating issues, issue branches, running tests, opening PRs. Use this whenever you want to change code, commit, push or open a pull request.
---

# GitHub workflow

Repo: `praetorianer777/Bearing` · default branch: `main`

## Rules

- Every change needs an issue and a branch named `<type>/<issue>-<slug>`.
  Types: `feat fix chore docs refactor test perf ci build revert`.
- Never push to `main`. Never merge yourself — that is the user's decision.
- Agree on issues and PRs with the user before creating them.
- Always call `gh` non-interactively: `--title` / `--body-file`, never open the editor.
- Issues carry a milestone (`M0 Foundation` … `Backlog`), an `area/*` label, a `type/*` label and
  `mvp` or `backlog`.

## Language

Everything in the repository is written in English: issues, pull requests, commit messages, code,
comments and documentation.

## Siblings

Changes Bearing needs in Armature, Exciter, Stator, Rotor or Dynamo go through issues and pull
requests in those repositories, agreed with the user first, and are linked from the Bearing issue.

## Recipe

```bash
gh auth status
gh issue list --limit 20
gh issue create --title "..." --body-file /tmp/body.md --label area/backend --label type/feature --label mvp --milestone "M1 Organisation model"
gh issue develop 42 --name feat/42-reporting-lines --base main --checkout
# ... work ...
./run-tests.sh
git push -u origin HEAD
gh pr create --title "..." --body-file /tmp/pr.md   # body contains "Closes #42"
gh pr checks --watch
```

## Pitfalls

- Branch slugs are lowercase ASCII.
- Commit messages are Conventional Commits, lowercase, imperative, subject ≤ 72 characters, and
  describe the effect rather than the files touched.
