#!/usr/bin/env bash
# End-to-end test suite for the local mail platform.
#
# Requires: running infrastructure (docker compose up -d), API and worker
# running, curl; docker exec is used to seed a second tenant; node (optional)
# powers the webhook receiver checks.
#
# Usage: bash scripts/e2e.sh
set -u

API="${API_URL:-http://localhost:8080}"
ADMIN_EMAIL="${BOOTSTRAP_ADMIN_EMAIL:-admin@company.test}"
ADMIN_PASSWORD="${BOOTSTRAP_ADMIN_PASSWORD:-admin-dev-password-1}"
PG_CONTAINER="${PG_CONTAINER:-mailplatform-postgres}"
PG_USER="${POSTGRES_USER:-mailplatform}"
PG_DB="${POSTGRES_DB:-mailplatform}"
WEBHOOK_PORT="${WEBHOOK_PORT:-39991}"

R="$RANDOM$RANDOM"
DOMAIN="e2e$R.test"
A1="a1@$DOMAIN"
A2="a2@$DOMAIN"
PASS1="e2e-user-pass-111"
PASS2="e2e-user-pass-222"

PASSED=0
FAILED=0
TMPDIR_E2E="${TMPDIR:-/tmp}/mail-e2e-$R"
mkdir -p "$TMPDIR_E2E"

say()  { printf '%s\n' "$*"; }
pass() { PASSED=$((PASSED+1)); say "  PASS: $*"; }
fail() { FAILED=$((FAILED+1)); say "  FAIL: $*"; }

jsonval() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -1; }

login() {
  curl -s -X POST "$API/api/v1/auth/login" -H "Content-Type: application/json" \
    -d "{\"email\":\"$1\",\"password\":\"$2\"}" | jsonval token
}

code() {
  if [ -n "${4:-}" ]; then
    curl -s -o /dev/null -w '%{http_code}' -X "$1" "$API$2" \
      -H "Authorization: Bearer $3" -H "Content-Type: application/json" -d "$4"
  else
    curl -s -o /dev/null -w '%{http_code}' -X "$1" "$API$2" -H "Authorization: Bearer $3"
  fi
}

# poll_inbox <token> <needle> [folder] → 0 when found within 15s
poll_inbox() {
  local folder="${3:-inbox}"
  for _ in $(seq 1 15); do
    if curl -s "$API/api/v1/mail/messages?folder=$folder" -H "Authorization: Bearer $1" | grep -q "$2"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

say "== E2E: mail platform (run $R) =="

# --- 1. Foundation -----------------------------------------------------------
ready=$(curl -s -o /dev/null -w '%{http_code}' "$API/health/ready")
[ "$ready" = "200" ] && pass "API ready" || { fail "API not ready ($ready)"; exit 1; }

ADM=$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")
[ -n "$ADM" ] && pass "admin login" || { fail "admin login"; exit 1; }

# --- 2. Provisioning ---------------------------------------------------------
DOM_ID=$(curl -s -X POST "$API/api/v1/domains" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$DOMAIN\",\"verification_mode\":\"development\"}" | jsonval id)
[ -n "$DOM_ID" ] && pass "development domain created" || { fail "domain create"; exit 1; }

for u in "$A1|$PASS1|User A1" "$A2|$PASS2|User A2"; do
  IFS='|' read -r em pw dn <<EOF2
$u
EOF2
  st=$(code POST /api/v1/users "$ADM" \
    "{\"email\":\"$em\",\"display_name\":\"$dn\",\"password\":\"$pw\",\"mailbox_domain_id\":\"$DOM_ID\"}")
  [ "$st" = "201" ] && pass "user $em created with mailbox" || fail "user $em ($st)"
done

T1=$(login "$A1" "$PASS1")
T2=$(login "$A2" "$PASS2")
[ -n "$T1" ] && [ -n "$T2" ] && pass "both users can log in" || fail "user logins"

MBOXES=$(curl -s "$API/api/v1/mailboxes" -H "Authorization: Bearer $ADM")
MB1=$(printf '%s' "$MBOXES" | sed -n "s/.*\"id\":\"\([^\"]*\)\"[^}]*\"address\":\"$A1\".*/\1/p" | head -1)
MB2=$(printf '%s' "$MBOXES" | sed -n "s/.*\"id\":\"\([^\"]*\)\"[^}]*\"address\":\"$A2\".*/\1/p" | head -1)

# --- 3. Core mail journey ----------------------------------------------------
MSG_ID=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A2\"],\"subject\":\"E2E hello $R\",\"text\":\"pipeline body $R\"}" | jsonval message_id)
[ -n "$MSG_ID" ] && pass "message accepted" || fail "send failed"
poll_inbox "$T2" "$MSG_ID" && pass "a2 received via pipeline" || fail "a2 inbox"

MM_ID=$(curl -s "$API/api/v1/mail/messages?folder=inbox" -H "Authorization: Bearer $T2" \
  | sed -n "s/.*\"id\":\"\([^\"]*\)\",\"message_id\":\"$MSG_ID\".*/\1/p" | head -1)
st=$(code GET "/api/v1/mail/messages/$MM_ID" "$T2")
[ "$st" = "200" ] && pass "a2 opened message" || fail "open ($st)"

RMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T2" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A1\"],\"subject\":\"Re: E2E hello $R\",\"text\":\"reply\",\"in_reply_to\":\"$MSG_ID\"}" | jsonval message_id)
poll_inbox "$T1" "$RMSG" && pass "a1 received threaded reply" || fail "reply not delivered"

curl -s "$API/api/v1/mail/messages?folder=sent" -H "Authorization: Bearer $T1" | grep -q "$MSG_ID" \
  && pass "a1 Sent copy exists" || fail "Sent copy missing"

# --- 4. Alias + group --------------------------------------------------------
AL=$(curl -s -X POST "$API/api/v1/aliases" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"domain_id\":\"$DOM_ID\",\"local_part\":\"helpdesk\",\"target_mailbox_ids\":[\"$MB1\"]}" | jsonval id)
[ -n "$AL" ] && pass "alias helpdesk@ created" || fail "alias create"
ALMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T2" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"helpdesk@$DOMAIN\"],\"subject\":\"Alias test $R\",\"text\":\"via alias\"}" | jsonval message_id)
poll_inbox "$T1" "$ALMSG" && pass "alias delivered to a1" || fail "alias delivery"

GR=$(curl -s -X POST "$API/api/v1/groups" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"domain_id\":\"$DOM_ID\",\"local_part\":\"crew\",\"name\":\"Crew\",\"member_mailbox_ids\":[\"$MB1\",\"$MB2\"]}" | jsonval id)
[ -n "$GR" ] && pass "group crew@ created" || fail "group create"
GRMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"crew@$DOMAIN\"],\"subject\":\"Group test $R\",\"text\":\"fanout\"}" | jsonval message_id)
poll_inbox "$T1" "$GRMSG" && poll_inbox "$T2" "$GRMSG" \
  && pass "group fanout reached both members" || fail "group fanout"

# --- 5. Attachments ----------------------------------------------------------
printf 'attachment payload %s' "$R" > "$TMPDIR_E2E/file.txt"
ATT=$(curl -s -X POST "$API/api/v1/mail/attachments" -H "Authorization: Bearer $T1" \
  -F "file=@$TMPDIR_E2E/file.txt" | jsonval id)
[ -n "$ATT" ] && pass "attachment uploaded" || fail "attachment upload"
ATMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A2\"],\"subject\":\"With file $R\",\"text\":\"see attached\",\"attachment_ids\":[\"$ATT\"]}" | jsonval message_id)
poll_inbox "$T2" "$ATMSG" && pass "attachment message delivered" || fail "attachment delivery"
GOT=$(curl -s "$API/api/v1/mail/attachments/$ATT" -H "Authorization: Bearer $T2")
[ "$GOT" = "attachment payload $R" ] && pass "recipient downloaded exact content" || fail "attachment download"

# --- 6. Email API + idempotency ---------------------------------------------
KEY=$(curl -s -X POST "$API/api/v1/api-keys" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d '{"name":"e2e key","scopes":["emails.send","emails.read"]}')
SECRET=$(printf '%s' "$KEY" | jsonval secret)
KEYID=$(printf '%s' "$KEY" | jsonval id)
[ -n "$SECRET" ] && pass "api key issued" || fail "api key create"

BODY="{\"from\":\"$ADMIN_EMAIL\",\"to\":[\"$A1\"],\"subject\":\"API $R\",\"text\":\"api body\"}"
APIMSG=$(curl -s -X POST "$API/api/v1/emails" -H "Authorization: Bearer $SECRET" \
  -H "Content-Type: application/json" -H "Idempotency-Key: e2e-$R" -d "$BODY" | jsonval message_id)
[ -n "$APIMSG" ] && pass "email API send accepted" || fail "email API send"
REPLAY=$(curl -s -X POST "$API/api/v1/emails" -H "Authorization: Bearer $SECRET" \
  -H "Content-Type: application/json" -H "Idempotency-Key: e2e-$R" -d "$BODY")
printf '%s' "$REPLAY" | grep -q "\"replayed\":true" && printf '%s' "$REPLAY" | grep -q "$APIMSG" \
  && pass "idempotent replay returned original id" || fail "idempotency replay"
st=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/v1/emails" \
  -H "Authorization: Bearer $SECRET" -H "Content-Type: application/json" \
  -H "Idempotency-Key: e2e-$R" -d "{\"from\":\"$ADMIN_EMAIL\",\"to\":[\"$A1\"],\"subject\":\"DIFF\",\"text\":\"x\"}")
[ "$st" = "409" ] && pass "idempotency payload conflict → 409" || fail "idempotency conflict ($st)"
poll_inbox "$T1" "$APIMSG" && pass "API message delivered to inbox" || fail "API delivery"
curl -s "$API/api/v1/emails/$APIMSG/events" -H "Authorization: Bearer $SECRET" \
  | grep -q "email.delivered_local" && pass "delivery events readable via API" || fail "events endpoint"
curl -s -X DELETE "$API/api/v1/api-keys/$KEYID" -H "Authorization: Bearer $ADM" > /dev/null
st=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/api/v1/emails" \
  -H "Authorization: Bearer $SECRET" -H "Content-Type: application/json" -d "$BODY")
[ "$st" = "401" ] && pass "revoked API key rejected" || fail "revoked key ($st)"

# --- 7. Security: spam + quarantine -----------------------------------------
SPMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T2" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A1\"],\"subject\":\"[SPAM-TEST] offer $R\",\"text\":\"spam\"}" | jsonval message_id)
poll_inbox "$T1" "$SPMSG" spam && pass "spam-marked mail landed in Spam" || fail "spam routing"

QMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T2" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A1\"],\"subject\":\"[QUARANTINE-TEST] bad $R\",\"text\":\"held\"}" | jsonval message_id)
QID=""
for _ in $(seq 1 15); do
  QID=$(curl -s "$API/api/v1/quarantine" -H "Authorization: Bearer $ADM" \
    | sed -n "s/.*\"id\":\"\([^\"]*\)\",\"message_id\":\"$QMSG\".*/\1/p" | head -1)
  [ -n "$QID" ] && break
  sleep 1
done
[ -n "$QID" ] && pass "quarantine holds the marked message" || fail "quarantine intake"
if curl -s "$API/api/v1/mail/messages?folder=inbox" -H "Authorization: Bearer $T1" | grep -q "$QMSG"; then
  fail "quarantined message leaked into inbox"
else
  pass "quarantined message absent from inbox"
fi
st=$(code POST "/api/v1/quarantine/$QID/release" "$ADM")
[ "$st" = "200" ] && poll_inbox "$T1" "$QMSG" \
  && pass "release delivered held message to inbox" || fail "quarantine release"

# --- 8. Isolation & RBAC -----------------------------------------------------
st=$(code GET "/api/v1/mail/messages/$MM_ID" "$T1")
[ "$st" = "404" ] && pass "a1 cannot read a2's mailbox copy" || fail "mailbox isolation ($st)"
st=$(code GET /api/v1/users "$T1")
[ "$st" = "403" ] && pass "member blocked from admin API" || fail "member RBAC ($st)"

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
    st=$(code GET "/api/v1/mail/messages/$MM_ID" "$TB")
    [ "$st" = "404" ] && pass "tenant B cannot read tenant A message" || fail "cross-tenant message ($st)"
    st=$(code GET "/api/v1/mail/attachments/$ATT" "$TB")
    [ "$st" = "404" ] && pass "tenant B cannot download tenant A attachment" || fail "cross-tenant attachment ($st)"
    curl -s "$API/api/v1/users" -H "Authorization: Bearer $TB" | grep -q "$A1" \
      && fail "tenant B sees tenant A users" || pass "tenant B cannot list tenant A users"
  else
    fail "tenant B login"
  fi
else
  fail "tenant B seed via psql"
fi

# --- 9. Webhooks (requires node) --------------------------------------------
if command -v node >/dev/null 2>&1; then
  RCV="$TMPDIR_E2E/webhook.json"
  # On Git Bash, node resolves POSIX-style /tmp differently; hand node a
  # native path for the same file.
  RCV_NODE=$(cygpath -m "$RCV" 2>/dev/null || echo "$RCV")
  node -e "
const http=require('http');const fs=require('fs');const out=[];
http.createServer((req,res)=>{let b='';req.on('data',c=>b+=c);req.on('end',()=>{
out.push({headers:req.headers,body:b});fs.writeFileSync('$RCV_NODE',JSON.stringify(out));
res.writeHead(200);res.end('ok');});}).listen($WEBHOOK_PORT);
setTimeout(()=>process.exit(0),30000);" >/dev/null 2>&1 &
  sleep 1
  WH=$(curl -s -X POST "$API/api/v1/webhooks" -H "Authorization: Bearer $ADM" \
    -H "Content-Type: application/json" \
    -d "{\"url\":\"http://localhost:$WEBHOOK_PORT/h\",\"events\":[\"email.accepted\"]}")
  WHSECRET=$(printf '%s' "$WH" | jsonval secret)
  WHID=$(printf '%s' "$WH" | jsonval id)
  curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T1" \
    -H "Content-Type: application/json" \
    -d "{\"to\":[\"$A2\"],\"subject\":\"hook $R\",\"text\":\"x\"}" > /dev/null
  OK=""
  for _ in $(seq 1 20); do
    [ -f "$RCV" ] && grep -q "email.accepted" "$RCV" && OK=1 && break
    sleep 1
  done
  [ -n "$OK" ] && pass "webhook delivered to receiver" || fail "webhook delivery"
  if [ -n "$OK" ]; then
    node -e "
const fs=require('fs');const crypto=require('crypto');
const rec=JSON.parse(fs.readFileSync('$RCV_NODE','utf8'))[0];
const exp='v1='+crypto.createHmac('sha256','$WHSECRET').update(rec.headers['x-mailplatform-timestamp']+'.'+rec.body).digest('hex');
process.exit(exp===rec.headers['x-mailplatform-signature']?0:1);" \
      && pass "webhook signature valid" || fail "webhook signature"
  fi
  curl -s -X DELETE "$API/api/v1/webhooks/$WHID" -H "Authorization: Bearer $ADM" > /dev/null
else
  say "  SKIP: node not found, webhook checks skipped"
fi

# --- 10. Sessions ------------------------------------------------------------
curl -s -X POST "$API/api/v1/auth/logout" -H "Authorization: Bearer $T2" > /dev/null
st=$(code GET /api/v1/mail/summary "$T2")
[ "$st" = "401" ] && pass "revoked session rejected" || fail "session revocation ($st)"

rm -rf "$TMPDIR_E2E"
say ""
say "== Result: $PASSED passed, $FAILED failed =="
[ "$FAILED" = "0" ]
