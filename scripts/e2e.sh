#!/usr/bin/env bash
# End-to-end test for the local mail platform.
#
# Requires: running infrastructure (docker compose up -d), API and worker
# running, curl. Uses docker exec to seed a second tenant for isolation tests.
#
# Usage: bash scripts/e2e.sh
set -u

API="${API_URL:-http://localhost:8080}"
ADMIN_EMAIL="${BOOTSTRAP_ADMIN_EMAIL:-admin@company.test}"
ADMIN_PASSWORD="${BOOTSTRAP_ADMIN_PASSWORD:-admin-dev-password-1}"
PG_CONTAINER="${PG_CONTAINER:-mailplatform-postgres}"
PG_USER="${POSTGRES_USER:-mailplatform}"
PG_DB="${POSTGRES_DB:-mailplatform}"

R="$RANDOM$RANDOM"
DOMAIN="e2e$R.test"
A1="a1@$DOMAIN"
A2="a2@$DOMAIN"
PASS1="e2e-user-pass-111"
PASS2="e2e-user-pass-222"

PASSED=0
FAILED=0

say()  { printf '%s\n' "$*"; }
pass() { PASSED=$((PASSED+1)); say "  PASS: $*"; }
fail() { FAILED=$((FAILED+1)); say "  FAIL: $*"; }

jsonval() { # jsonval <key> — extracts "key":"value" from stdin (first match)
  sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -1
}

login() { # login <email> <password> → token (empty on failure)
  curl -s -X POST "$API/api/v1/auth/login" -H "Content-Type: application/json" \
    -d "{\"email\":\"$1\",\"password\":\"$2\"}" | jsonval token
}

code() { # code <method> <path> <token> [body] → http status
  if [ -n "${4:-}" ]; then
    curl -s -o /dev/null -w '%{http_code}' -X "$1" "$API$2" \
      -H "Authorization: Bearer $3" -H "Content-Type: application/json" -d "$4"
  else
    curl -s -o /dev/null -w '%{http_code}' -X "$1" "$API$2" -H "Authorization: Bearer $3"
  fi
}

say "== E2E: local mail platform (run $R) =="

# 1. Health
ready=$(curl -s -o /dev/null -w '%{http_code}' "$API/health/ready")
[ "$ready" = "200" ] && pass "API ready" || { fail "API not ready ($ready)"; exit 1; }

# 2. Admin login
ADM=$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")
[ -n "$ADM" ] && pass "admin login" || { fail "admin login"; exit 1; }

# 3. Domain + users
DOM_JSON=$(curl -s -X POST "$API/api/v1/domains" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$DOMAIN\",\"verification_mode\":\"development\"}")
DOM_ID=$(printf '%s' "$DOM_JSON" | jsonval id)
[ -n "$DOM_ID" ] && pass "development domain created" || { fail "domain create: $DOM_JSON"; exit 1; }

for u in "$A1|$PASS1|User A1" "$A2|$PASS2|User A2"; do
  IFS='|' read -r em pw dn <<EOF2
$u
EOF2
  st=$(code POST /api/v1/users "$ADM" \
    "{\"email\":\"$em\",\"display_name\":\"$dn\",\"password\":\"$pw\",\"mailbox_domain_id\":\"$DOM_ID\"}")
  [ "$st" = "201" ] && pass "user $em created with mailbox" || fail "user $em create ($st)"
done

# 4. a1 sends to a2
T1=$(login "$A1" "$PASS1")
[ -n "$T1" ] && pass "a1 login" || fail "a1 login"
SEND_JSON=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A2\"],\"subject\":\"E2E hello $R\",\"text\":\"pipeline test body $R\"}")
MSG_ID=$(printf '%s' "$SEND_JSON" | jsonval message_id)
[ -n "$MSG_ID" ] && pass "message accepted ($MSG_ID)" || fail "send: $SEND_JSON"

# 5. a2 receives (poll up to 15s)
T2=$(login "$A2" "$PASS2")
MM_ID=""
for _ in $(seq 1 15); do
  MM_ID=$(curl -s "$API/api/v1/mail/messages?folder=inbox" -H "Authorization: Bearer $T2" \
    | sed -n "s/.*\"id\":\"\([^\"]*\)\",\"message_id\":\"$MSG_ID\".*/\1/p" | head -1)
  [ -n "$MM_ID" ] && break
  sleep 1
done
[ -n "$MM_ID" ] && pass "a2 received message via pipeline" || fail "a2 did not receive message"

# 6. a2 opens (marks read) and replies
if [ -n "$MM_ID" ]; then
  st=$(code GET "/api/v1/mail/messages/$MM_ID" "$T2")
  [ "$st" = "200" ] && pass "a2 opened message" || fail "a2 open ($st)"
  REPLY_JSON=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T2" \
    -H "Content-Type: application/json" \
    -d "{\"to\":[\"$A1\"],\"subject\":\"Re: E2E hello $R\",\"text\":\"reply body\",\"in_reply_to\":\"$MSG_ID\"}")
  RMSG_ID=$(printf '%s' "$REPLY_JSON" | jsonval message_id)
  [ -n "$RMSG_ID" ] && pass "a2 reply accepted" || fail "reply: $REPLY_JSON"

  # 7. a1 receives reply
  GOT=""
  for _ in $(seq 1 15); do
    GOT=$(curl -s "$API/api/v1/mail/messages?folder=inbox" -H "Authorization: Bearer $T1" \
      | grep -c "$RMSG_ID")
    [ "$GOT" != "0" ] && break
    sleep 1
  done
  [ "$GOT" != "0" ] && pass "a1 received reply" || fail "a1 did not receive reply"
fi

# 8. Sent copies exist
sent=$(curl -s "$API/api/v1/mail/messages?folder=sent" -H "Authorization: Bearer $T1" | grep -c "$MSG_ID")
[ "$sent" != "0" ] && pass "a1 Sent contains message" || fail "a1 Sent copy missing"

# 9. Same-tenant mailbox isolation: a1 must not read a2's mailbox message
if [ -n "$MM_ID" ]; then
  st=$(code GET "/api/v1/mail/messages/$MM_ID" "$T1")
  [ "$st" = "404" ] && pass "a1 cannot read a2's mailbox copy (404)" || fail "mailbox isolation ($st)"
fi

# 10. Member RBAC: a1 cannot use admin endpoints
st=$(code GET /api/v1/users "$T1")
[ "$st" = "403" ] && pass "member blocked from admin API (403)" || fail "RBAC member ($st)"

# 11. Cross-tenant isolation: seed tenant B directly in SQL
BEMAIL="b1@e2eb$R.test"
BHASH='$argon2id$v=19$m=65536,t=3,p=4$5E18BDOM4wgtIj1jKivA8A$B8O044c2FH0zY6f7KiB62wtiznbv1AxzY2u9WWjgKxI'
SQL=$(cat <<EOSQL
WITH t AS (INSERT INTO tenants (name, slug) VALUES ('E2E Tenant B $R', 'e2e-b-$R') RETURNING id),
o AS (INSERT INTO organizations (tenant_id, name, slug) SELECT id, 'B Org', 'b-org' FROM t RETURNING id, tenant_id),
u AS (INSERT INTO users (tenant_id, organization_id, email, display_name)
      SELECT tenant_id, id, '$BEMAIL', 'B One' FROM o RETURNING id, organization_id),
c AS (INSERT INTO user_credentials (user_id, password_hash) SELECT id, '$BHASH' FROM u)
INSERT INTO user_roles (user_id, role_code, scope_type, scope_id)
SELECT id, 'org_admin', 'organization', organization_id FROM u;
EOSQL
)
if docker exec -i "$PG_CONTAINER" psql -q -U "$PG_USER" -d "$PG_DB" -c "$SQL" >/dev/null 2>&1; then
  TB=$(login "$BEMAIL" "e2e-tenantb-pass1")
  if [ -n "$TB" ]; then
    pass "tenant B admin login"
    st=$(code GET "/api/v1/mail/messages/$MM_ID" "$TB")
    [ "$st" = "404" ] && pass "tenant B cannot read tenant A message (404)" || fail "cross-tenant message read ($st)"
    UB=$(curl -s "$API/api/v1/users" -H "Authorization: Bearer $TB")
    if printf '%s' "$UB" | grep -q "$A1"; then
      fail "tenant B can list tenant A users"
    else
      pass "tenant B cannot list tenant A users"
    fi
    DB=$(curl -s "$API/api/v1/domains" -H "Authorization: Bearer $TB")
    if printf '%s' "$DB" | grep -q "$DOMAIN"; then
      fail "tenant B can see tenant A domains"
    else
      pass "tenant B cannot see tenant A domains"
    fi
  else
    fail "tenant B login (seed ok, login failed)"
  fi
else
  fail "could not seed tenant B via docker exec psql"
fi

# 12. Revoked session fails
curl -s -X POST "$API/api/v1/auth/logout" -H "Authorization: Bearer $T2" >/dev/null
st=$(code GET /api/v1/mail/summary "$T2")
[ "$st" = "401" ] && pass "revoked session rejected (401)" || fail "session revocation ($st)"

say ""
say "== Result: $PASSED passed, $FAILED failed =="
[ "$FAILED" = "0" ]
