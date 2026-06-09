# dead-man-switch

A self-hosted **dead-man's switch** built on GitHub Actions.

Scheduled jobs probe a set of *life signals* (public activity sources). Each
signal reports pass/fail on its own cron. An aggregate evaluator reads the
workflow run history and decides whether the operator still appears to be
alive; if the signals go quiet past a configured threshold, downstream
trigger and handler layers fire the configured death actions.

## Why

This started years ago. My brother died, and years later I found his stash of messages: the things he would have sent if he had known. I built a first version on a VPS I was using for VPN access. It got blocked because I was logging in from Iran. Sanctions. How I forgot about it. A year ago my mother died and I started again. Three months ago my father went into surgery. The protests had started a few days before, the internet was shut down, and I never got to say goodbye. He went to the ICU and never came back. He lived another 100 days through the worst of the war and the worst of the shutdown, and I could not reach him. After he passed, I realized this is not a project I can keep abandoning: a dead-man switch built on infrastructure I do not control, that survives sanctions, survives shutdowns, delivers the message even when I am the one who is gone.

## Architecture

The system spans **two repos**: a public code repo (this one) and a private sibling that holds only the encrypted payloads. The public side runs the signal probes, the aggregate evaluator, and the trigger workflow. The trigger fetches the encrypted blobs from the private sibling at runtime via a read-only SSH deploy key, decrypts in memory with the GPG secret, and dispatches the death actions. Nothing decrypts on disk; the ciphertext never sits in public history.

```
<owner>/dead-man-switch             (public, this repo: code + workflows + pubkey)
  signal/                        one package per life-signal source (idlerpg, mostodon, bggplays, ...)
  aggregate/                     reads workflow run history; threshold + waiting-period evaluator
  trigger/                       consumes trigger_ready state, decrypts payloads, dispatches handlers
  handler/                       death-action handlers (email, ...); each registers itself by name
    email/                       the first concrete handler: SMTP send with per-recipient template + isolation + rate-limit
  statuspage/                    renders the alive/waiting/triggered HTML page (gh-pages)
  digest/                        weekly keep-alive digest mailer
  internal/
    crypt/                       OpenPGP encrypt/decrypt
    dmstate/                     shared state machine type + state-branch load/save
    cli/, fetch/                 small shared helpers
  cmd/lt-crypt/                  operator CLI to encrypt/decrypt payload blobs locally
  pubkey/operator.asc            the operator's GPG public key (used to encrypt blobs)
  scripts/                       operator helpers (setup.sh)
  .github/workflows/
    *-check.yaml                 one per life signal (cron)
    aggregate.yaml               evaluates signals, writes state.json
    trigger.yaml                 daily poll, fires handlers when state==trigger_ready
    statuspage.yaml              weekly, force-pushes public/ to gh-pages
    crypt-sanity.yaml            end-to-end encrypt→decrypt round-trip self-check
    email-test.yaml              dispatch-only: send a test email to operator-self
    keepalive-digest.yaml        weekly digest to operator-self (SMTP keep-warm + credential-rot)
    tests.yaml                   go build / vet / test -race on every PR + push

<owner>/<sibling>                (PRIVATE, fetched at runtime by trigger / email-test / keepalive-digest)
  operator-self.gpg              encrypted {email, name} of the operator
  recipients/*.gpg               encrypted recipient lists + message templates
  actions.json                   { handler, payload_file } wiring; paths resolve relative to this file
```

## Flow

1. **Signals** run on cron; each exits 0 on a fresh sign of life, non-zero otherwise.
2. **Aggregate** scans `*-check.yaml` runs. Trip rule: no successful run across any signal within `THRESHOLD_FAILS_DAYS` (default 30). On trip, state goes `alive → waiting`. Any signal recovery resets to alive (even mid-wait). After `WAITING_PERIOD_DAYS` (default 30) of waiting, state latches to `trigger_ready`.
3. **Trigger** polls daily, gates on `trigger_ready`, decrypts each entry in `actions.json` via the GPG secret, and dispatches to the named handler. Payload paths in `actions.json` resolve relative to its own directory. Per-handler failures (including panics) are isolated.
4. **Status page** renders `state.json` as a self-contained HTML page weekly, force-pushed to `gh-pages`. The page is the public alive/dead indicator.
5. **Keep-alive digest** mails the operator weekly (Mon 12:00 UTC) with status + last-signal + per-signal pass/fail. Doubles as SMTP keep-warm + credential-rot check.

## Setup

The system needs two repos before anything else. Fork or clone this repo as your public side, and create a separate **private** repo of any name for the encrypted blobs. Empty is fine; step 4 below seeds it. Wire-up between the two is via a deploy key configured in step 3, so the names don't have to match.

### 1. Generate the project keypair (local, one-time)

The "death key" is a project-specific OpenPGP keypair, NOT your personal key. Generate locally so the private half never leaves your machine:

```sh
gpg --batch --generate-key <<EOF
%no-protection
Key-Type: EDDSA
Key-Curve: ed25519
Subkey-Type: ECDH
Subkey-Curve: cv25519
Name-Real: dead-man-switch
Name-Email: dms+nokey@local
Expire-Date: 0
%commit
EOF

KEYID=$(gpg --list-keys --with-colons dms+nokey@local | awk -F: '/^pub:/ {print $5; exit}')
gpg --armor --export $KEYID > pubkey/operator.asc           # commit this
gpg --armor --export-secret-keys $KEYID > /tmp/death-key.asc  # for the GH secret
```

Commit `pubkey/operator.asc`. The secret install is the next step.

### 2. Install secrets + variables

Use the interactive helper:

```sh
./scripts/setup.sh                          # defaults to fzerorubigd/dead-man-switch
./scripts/setup.sh <owner/repo>             # for a fork
```

The script asks for each value and uses `gh secret set` / `gh variable set` per item. Press enter on any prompt to skip (keep existing) or accept the default. Then install the GPG secret manually:

```sh
gh secret set GPG_PRIVATE_KEY --repo fzerorubigd/dead-man-switch < /tmp/death-key.asc
shred -u /tmp/death-key.asc
```

Keep an offline backup of the private key (paperless / password manager) separate from GitHub, in case the GH secret is lost.

### 3. Wire the private blobs repo to the public one

The private sibling holds the encrypted payloads + `actions.json` so ciphertext never sits in public history (where a future quantum adversary could harvest it). The public repo fetches from it at runtime via a read-only SSH deploy key; the sibling's identity is also kept as a secret so the public workflow YAML doesn't name it.

```sh
PUBLIC_REPO="<owner>/dead-man-switch"
PRIVATE_REPO="<owner>/<your-private-blobs-repo>"   # created at the very start

# generate a deploy keypair locally
ssh-keygen -t ed25519 -f /tmp/lt-blobs -N "" -C "dead-man-switch blobs deploy key"

# public half → read-only deploy key on the PRIVATE repo
gh repo deploy-key add /tmp/lt-blobs.pub --repo "$PRIVATE_REPO" --title "dead-man-switch reader"

# private half → secret on the PUBLIC repo
gh secret set BLOBS_DEPLOY_KEY --repo "$PUBLIC_REPO" < /tmp/lt-blobs
shred -u /tmp/lt-blobs

# tell the workflows which repo to fetch blobs from
gh secret set BLOBS_REPO --repo "$PUBLIC_REPO" --body "$PRIVATE_REPO"
```

### 4. Encrypt + commit payload blobs (to the private repo)

The `lt-crypt encrypt` calls below read `pubkey/operator.asc` from the public repo and write ciphertext into the private repo's working tree. Nothing decryptable is ever written to the public side.

```sh
git clone "git@github.com:$PRIVATE_REPO" /tmp/lt-private
cd /tmp/lt-private

# operator-self (used by test-mode To-override + keep-alive digest)
echo '{"email":"you@example.com","name":"Your Name"}' \
  | go run github.com/fzerorubigd/dead-man-switch/cmd/lt-crypt encrypt --in /dev/stdin --out operator-self.gpg

# recipient list for the email death-action — one file per recipient (or grouped)
cat <<'EOF' | go run github.com/fzerorubigd/dead-man-switch/cmd/lt-crypt encrypt --in /dev/stdin --out recipients/1.gpg
[
  {
    "email": "friend1@example.com",
    "name": "Friend 1",
    "message_template": "Hi {{.Name}}, ..."
  }
]
EOF

# wire the death actions — payload paths resolve relative to actions.json
cat > actions.json <<'EOF'
[
  { "handler": "email", "payload_file": "recipients/1.gpg" }
]
EOF

git add operator-self.gpg recipients actions.json
git commit -m "ops: encrypted blobs + actions wiring"
git push
```

Templates render `{{.Name}}`, `{{.Email}}`, `{{.Date}}`, `{{.Operator}}`.

### 5. Enable GitHub Pages (after making the repo public)

Settings → Pages → Source = "Deploy from a branch" → Branch = `gh-pages` / `(root)`. Optionally set `STATUS_PAGE_DOMAIN` + the CNAME for a custom domain.

### 6. Verify

- `crypt-sanity.yaml` → run on push or workflow_dispatch; greens when pubkey + GPG_PRIVATE_KEY round-trip.
- `email-test.yaml` → workflow_dispatch; sends every recipient's rendered email to operator-self with intended-recipient footer.
- `keepalive-digest.yaml` → workflow_dispatch (or wait for Monday) to confirm the weekly digest lands in operator's inbox.

## Secrets + variables reference

| name | kind | description |
|------|------|-------------|
| `GPG_PRIVATE_KEY` | secret | ASCII-armored private half of the project keypair (the "death key"). |
| `BLOBS_DEPLOY_KEY` | secret | SSH private key for read-only access to the private blobs repo. |
| `BLOBS_REPO` | secret | `<owner>/<repo>` of the private blobs sibling (kept as a secret so its identity is not in the public workflow YAML). |
| `SMTP_HOST` | secret | e.g. `smtp.gmail.com`. |
| `SMTP_USERNAME` | secret | SMTP login (gmail address). |
| `SMTP_PASSWORD` | secret | Gmail **app password**, not your account password. |
| `SMTP_FROM` | secret | `From:` shown to recipients (usually = SMTP_USERNAME). |
| `OPERATOR_NAME` | variable | Used in email template + default subject. |
| `SMTP_PORT` | variable | Default `587`. |
| `EMAIL_SUBJECT` | variable | Default `A message from <OPERATOR_NAME>`. |
| `EMAIL_SEND_DELAY_SECONDS` | variable | Default `7`. Jittered ±50%. |
| `THRESHOLD_FAILS_DAYS` | variable | Default `30`. |
| `WAITING_PERIOD_DAYS` | variable | Default `30`. |
| `STATUS_PAGE_DOMAIN` | variable | Optional custom domain shown on the status page. |
| `STATUS_PAGE_NOTE` | variable | Optional one-line operator note rendered on the status page. |

## Operator escape hatch

If you're alive but the system thinks otherwise: run `aggregate.yaml` via workflow_dispatch with `reset=true`. State resets to alive immediately, even mid-waiting-period.
