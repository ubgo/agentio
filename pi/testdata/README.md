# pi fixtures

`homes/1.5.2/userhome/.pi/agent` is what pi 1.5.2 (`@earendil-works/pi-coding-agent`) wrote with a throwaway home, recorded live on 2026-11-14 without a login:

- `auth.json` and `models-store.json`: both `{}`, written on every start (even `pi --help`).
- `sessions/--home-fixture-proj--/<timestamp>_<id>.jsonl`: one session from `pi -p` runs against a local mock OpenAI-compatible provider on 127.0.0.1 (configured through a throwaway `models.json`, not kept), then `--name` and a resume by partial id. Paths are replaced with `/home/fixture` and `/opt/fixture-prefix`, the session id with a placeholder uuid, and long system-prompt text was truncated when recorded.

Not recorded live (no login was made): real `auth.json` entries, so tests write the OAuth and API-key shapes taken from pi's source (`pi-ai/dist/auth/oauth/*.js`, `dist/core/auth-storage.js`) with placeholder values; and a `settings.json`, which a first start does not write, so tests and the example write one in the shape of `schemas/settings.schema.json`.
