#!/usr/bin/env bash
# sso-lab.sh — drive the chapter 7.7/7.9 labs end to end with curl, as a browser
# would: SSO login through Keycloak, TOTP enrollment using snip's own TOTP code,
# a password + code login, and a replayed code being refused.
#
# Needs: Keycloak on :8180 with deploy/keycloak/snip-realm.json imported, and
# the relying party (examples/oidcrp/cmd) on :8765. Run from snip/.
set -euo pipefail
RP="${RP:-http://localhost:8765}"
log()  { echo "[sso-lab] $*"; }
fail() { echo "[sso-lab] FAIL: $*" >&2; exit 1; }
text() { sed 's/<[^>]*>/ /g' | tr -s ' \n' ' '; }
form_action() { grep -o "<form[^>]*id=\"$1\"[^>]*>" | grep -o 'action="[^"]*"' | sed 's/action="//; s/"$//; s/&amp;/\&/g' || true; }
totp_code() { go run ./examples/totp/cmd code "$1" | cut -d' ' -f1; }

# Start a login at the relying party and submit username and password to Keycloak.
# Leaves the cookie jar in $JAR and the resulting page in $PAGE.
password_step() {
  JAR=$(mktemp)
  PAGE=$(curl -sS -L -c "$JAR" -b "$JAR" "$RP/login")
  local action; action=$(printf '%s' "$PAGE" | form_action kc-form-login)
  [ -n "$action" ] || fail "no Keycloak login form"
  PAGE=$(curl -sS -L -c "$JAR" -b "$JAR" --data-urlencode "username=$1" --data-urlencode "password=$2" "$action")
}

# 1. Single sign-on with a password.
password_step alice alice-lab-password
printf '%s' "$PAGE" | text | grep -q "Signed in as alice" || fail "alice did not end up signed in"
log "alice signed in through Keycloak; the relying party verified her ID token"

password_step alice wrong-password
printf '%s' "$PAGE" | text | grep -q "Signed in as" && fail "a wrong password must not sign anyone in"
log "a wrong password stays at Keycloak's login form"

# 2. Bob must enroll an authenticator. The form carries the secret; an
#    authenticator app would read it from the QR code. We compute the code with
#    snip's own implementation of RFC 6238.
password_step bob bob-lab-password
raw=$(printf '%s' "$PAGE" | grep -o 'name="totpSecret" value="[^"]*"' | sed 's/.*value="//; s/"$//' || true)
[ -n "$raw" ] || fail "bob was not asked to set up an authenticator"
secret=$(python3 -c "import base64,sys; print(base64.b32encode(sys.argv[1].encode()).decode().rstrip('='))" "$raw")
action=$(printf '%s' "$PAGE" | form_action kc-totp-settings-form)
PAGE=$(curl -sS -L -c "$JAR" -b "$JAR" --data-urlencode "totp=$(totp_code "$secret")" \
  --data-urlencode "totpSecret=$raw" --data-urlencode "userLabel=lab" "$action")
printf '%s' "$PAGE" | text | grep -q "Signed in as bob" || fail "TOTP enrollment failed"
log "bob enrolled an authenticator; Keycloak accepted a code from snip's TOTP code"

# 3. Next login: password, then a code from the next 30-second step
#    (the one used for enrollment can't be used again).
sleep $(( 31 - $(date +%s) % 30 ))
code=$(totp_code "$secret")
password_step bob bob-lab-password
action=$(printf '%s' "$PAGE" | form_action kc-otp-login-form)
[ -n "$action" ] || fail "bob was not asked for a code"
PAGE=$(curl -sS -L -c "$JAR" -b "$JAR" --data-urlencode "otp=$code" "$action")
printf '%s' "$PAGE" | text | grep -q "Signed in as bob" || fail "password + code login failed"
log "bob signed in with password + code"

# 4. The same code again, within its 30 seconds: a replay, which must fail.
password_step bob bob-lab-password
action=$(printf '%s' "$PAGE" | form_action kc-otp-login-form)
PAGE=$(curl -sS -L -c "$JAR" -b "$JAR" --data-urlencode "otp=$code" "$action")
printf '%s' "$PAGE" | text | grep -q "Signed in as bob" && fail "a replayed code was accepted"
log "the same code, replayed, was refused"
log "all checks passed"
