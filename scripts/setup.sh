#!/usr/bin/env bash
# scripts/setup.sh — interactive secret + variable setup for dead-man-switch.
#
# Walks through the required secrets + variables and fires `gh secret set` /
# `gh variable set` per item. Assumes `gh` is installed and authenticated for
# the repo. Skip any prompt with empty input to leave the existing value
# untouched.
#
# Usage: ./scripts/setup.sh [owner/repo]
#   defaults to fzerorubigd/dead-man-switch if no repo given.

set -euo pipefail

REPO="${1:-fzerorubigd/dead-man-switch}"

if ! command -v gh >/dev/null 2>&1; then
  echo "error: gh CLI not found in PATH" >&2
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "error: gh not authenticated. run 'gh auth login' first." >&2
  exit 1
fi

echo "dead-man-switch setup → $REPO"
echo "skip any prompt with empty input to leave the existing value untouched."
echo

ask_secret() {
  local name="$1" prompt="$2"
  local value
  read -r -s -p "$prompt: " value
  echo
  if [ -z "$value" ]; then
    echo "  (skipped $name)"
    return 0
  fi
  printf '%s' "$value" | gh secret set "$name" --repo "$REPO" >/dev/null
  echo "  ✓ secret $name set"
}

ask_var() {
  local name="$1" prompt="$2" default="${3:-}"
  local value display_default=""
  if [ -n "$default" ]; then display_default=" [$default]"; fi
  read -r -p "$prompt${display_default}: " value
  if [ -z "$value" ]; then value="$default"; fi
  if [ -z "$value" ]; then
    echo "  (skipped $name)"
    return 0
  fi
  gh variable set "$name" --repo "$REPO" --body "$value" >/dev/null
  echo "  ✓ variable $name=$value"
}

echo "=== required secrets ==="
ask_secret SMTP_HOST       "SMTP host (e.g. smtp.gmail.com)"
ask_secret SMTP_USERNAME   "SMTP username (gmail address)"
ask_secret SMTP_PASSWORD   "SMTP password (gmail app password, hidden)"
ask_secret SMTP_FROM       "SMTP From: address (often same as username)"

echo
echo "=== required variables ==="
ask_var OPERATOR_NAME      "Operator name (used in email template + default subject)"

echo
echo "=== optional variables (press enter to accept default) ==="
ask_var SMTP_PORT                "SMTP port"                       "587"
ask_var EMAIL_SUBJECT            "Email subject"                   ""
ask_var EMAIL_SEND_DELAY_SECONDS "Send delay between emails (sec)" "7"
ask_var THRESHOLD_FAILS_DAYS     "Threshold fails days"            "30"
ask_var WAITING_PERIOD_DAYS      "Waiting period days"             "30"
ask_var STATUS_PAGE_DOMAIN       "Status page custom domain"       ""
ask_var STATUS_PAGE_NOTE         "Status page operator note"       ""

echo
echo "done. GPG_PRIVATE_KEY is assumed already set; if not:"
echo "  gh secret set GPG_PRIVATE_KEY --repo $REPO < your-private-key.asc"
