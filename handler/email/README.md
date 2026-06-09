# email handler

The first concrete death-action handler. Registered under the name
`email`; the trigger dispatches a decrypted recipient list to it.

## Payload

The decrypted payload (the plaintext behind the action's `*.gpg` file) is
a JSON array of recipients:

```json
[
  {
    "email": "friend@example.com",
    "name": "Friend",
    "message_template": "Hi {{.Name}},\n\nIf you're reading this on {{.Date}}, the switch fired.\n— {{.Operator}}"
  }
]
```

`message_template` is a Go `text/template` rendered with `{{.Name}}`,
`{{.Email}}`, `{{.Date}}` (UTC `YYYY-MM-DD`) and `{{.Operator}}`
(`OPERATOR_NAME`).

Wire it into `actions.json`:

```json
[{ "handler": "email", "payload_file": "recipients.gpg" }]
```

and create the blob with `lt-crypt encrypt --in recipients.json --out recipients.gpg`.

## Configuration (secrets / variables)

| Env | Source | Notes |
|-----|--------|-------|
| `SMTP_HOST` / `SMTP_PORT` | secret / var | e.g. `smtp.gmail.com` / `587` |
| `SMTP_USERNAME` / `SMTP_PASSWORD` | secret | the sending account + an app password |
| `SMTP_FROM` | secret | defaults to `SMTP_USERNAME` |
| `OPERATOR_NAME` | var | template `{{.Operator}}` |
| `EMAIL_SUBJECT` | var | defaults to `A message from <OPERATOR_NAME>` |
| `EMAIL_SEND_DELAY_SECONDS` | var | inter-send delay, default 7s + 0–50% jitter, to stay under provider throttles |

## Failure semantics

Per-recipient failures (render or send) are logged and skipped; the
remaining recipients are still attempted. The handler returns an
aggregate error naming the failures, so the trigger marks the action
failed (non-zero run) while still having delivered to everyone reachable.
