# Bearing

Organisation modeling beside Armature, Exciter, Stator and Rotor. Read
`docs/plan.md` and `docs/decisions.md` before changing anything, and
`.claude/skills/gh/SKILL.md` before any change to the repository.

- Everything in the repository is in English.
- The stack follows the siblings: Go (chi, pgx, goose), PostgreSQL with
  CloudNativePG and the read/write split, Valkey, React with Tailwind and
  React Flow. Look at how a sibling solved something before solving it anew.
- Bearing runs standalone; partner features are gated by the partner's
  `/api/v1/meta` capabilities.
