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
# Mail store (optional): enables the JMAP parity checks.
STALWART_HTTP="${STALWART_HTTP:-}"
STALWART_MASTER_PASSWORD="${STALWART_MASTER_PASSWORD:-}"
STALWART_ADMIN_PASSWORD="${STALWART_ADMIN_PASSWORD:-}"

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
# Mailbox contents live in the mail store (ADR-003): listings are keyed by
# store ids and carry the RFC Message-ID, so subjects identify messages here.
poll_inbox "$T2" "E2E hello $R" && pass "a2 received via pipeline" || fail "a2 inbox"

MM_ID=$(curl -s "$API/api/v1/mail/messages?folder=inbox&q=E2E+hello+$R" -H "Authorization: Bearer $T2" | jsonval id)
st=$(code GET "/api/v1/mail/messages/$MM_ID" "$T2")
[ "$st" = "200" ] && pass "a2 opened message" || fail "open ($st)"

RMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T2" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A1\"],\"subject\":\"Re: E2E hello $R\",\"text\":\"reply\",\"in_reply_to\":\"$MM_ID\"}" | jsonval message_id)
poll_inbox "$T1" "Re: E2E hello $R" && pass "a1 received threaded reply" || fail "reply not delivered"

curl -s "$API/api/v1/mail/messages?folder=sent" -H "Authorization: Bearer $T1" | grep -q "E2E hello $R" \
  && pass "a1 Sent copy exists" || fail "Sent copy missing"

# --- 4. Alias + group --------------------------------------------------------
AL=$(curl -s -X POST "$API/api/v1/aliases" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"domain_id\":\"$DOM_ID\",\"local_part\":\"helpdesk\",\"target_mailbox_ids\":[\"$MB1\"]}" | jsonval id)
[ -n "$AL" ] && pass "alias helpdesk@ created" || fail "alias create"
ALMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T2" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"helpdesk@$DOMAIN\"],\"subject\":\"Alias test $R\",\"text\":\"via alias\"}" | jsonval message_id)
poll_inbox "$T1" "Alias test $R" && pass "alias delivered to a1" || fail "alias delivery"

GR=$(curl -s -X POST "$API/api/v1/groups" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"domain_id\":\"$DOM_ID\",\"local_part\":\"crew\",\"name\":\"Crew\",\"member_mailbox_ids\":[\"$MB1\",\"$MB2\"]}" | jsonval id)
[ -n "$GR" ] && pass "group crew@ created" || fail "group create"
GRMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"crew@$DOMAIN\"],\"subject\":\"Group test $R\",\"text\":\"fanout\"}" | jsonval message_id)
poll_inbox "$T1" "Group test $R" && poll_inbox "$T2" "Group test $R" \
  && pass "group fanout reached both members" || fail "group fanout"

# --- 5. Attachments ----------------------------------------------------------
printf 'attachment payload %s' "$R" > "$TMPDIR_E2E/file.txt"
ATT=$(curl -s -X POST "$API/api/v1/mail/attachments" -H "Authorization: Bearer $T1" \
  -F "file=@$TMPDIR_E2E/file.txt" | jsonval id)
[ -n "$ATT" ] && pass "attachment uploaded" || fail "attachment upload"
ATMSG=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A2\"],\"subject\":\"With file $R\",\"text\":\"see attached\",\"attachment_ids\":[\"$ATT\"]}" | jsonval message_id)
poll_inbox "$T2" "With file $R" && pass "attachment message delivered" || fail "attachment delivery"
# Attachments are MIME parts in the mail store: locate the copy, read its
# blob id, then stream it back through the backend.
AT_MM=$(curl -s "$API/api/v1/mail/messages?folder=inbox&q=With+file+$R" -H "Authorization: Bearer $T2" | jsonval id)
AT_BLOB=$(curl -s "$API/api/v1/mail/messages/$AT_MM" -H "Authorization: Bearer $T2" \
  | sed -n 's/.*"attachments":\[{"id":"\([^"]*\)".*/\1/p')
[ -n "$AT_BLOB" ] && pass "attachment visible as a MIME part" || fail "attachment part missing"
GOT=$(curl -s "$API/api/v1/mail/blob/$AT_BLOB?name=file.txt&type=text/plain" -H "Authorization: Bearer $T2")
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
poll_inbox "$T1" "API $R" && pass "API message delivered to inbox" || fail "API delivery"
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
poll_inbox "$T1" "SPAM-TEST] offer $R" spam && pass "spam-marked mail landed in Spam" || fail "spam routing"

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
if curl -s "$API/api/v1/mail/messages?folder=inbox" -H "Authorization: Bearer $T1" | grep -q "QUARANTINE-TEST] bad $R"; then
  fail "quarantined message leaked into inbox"
else
  pass "quarantined message absent from inbox"
fi
st=$(code POST "/api/v1/quarantine/$QID/release" "$ADM")
[ "$st" = "200" ] && poll_inbox "$T1" "QUARANTINE-TEST] bad $R" \
  && pass "release delivered held message to inbox" || fail "quarantine release"

# --- 8. Isolation & RBAC -----------------------------------------------------
# Mailbox ids are per-account in the mail store (ADR-003), so a foreign id is
# not globally unique: it either misses or resolves to the caller's OWN
# message. The guarantee under test is therefore content-based, using a
# message a1 has no copy of at all (admin → a2 only).
SECRET_BODY="isolation-secret-$R"
curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $ADM" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A2\"],\"subject\":\"Private $R\",\"text\":\"$SECRET_BODY\"}" > /dev/null
poll_inbox "$T2" "Private $R" || fail "isolation probe not delivered"
PRIV_ID=$(curl -s "$API/api/v1/mail/messages?folder=inbox&q=Private+$R" -H "Authorization: Bearer $T2" | jsonval id)
LEAK=$(curl -s "$API/api/v1/mail/messages/$PRIV_ID" -H "Authorization: Bearer $T1")
if printf '%s' "$LEAK" | grep -q "$SECRET_BODY"; then
  fail "mailbox isolation: a1 read a2's private message content"
else
  pass "a1 cannot read a2's mailbox content"
fi
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

# --- 10. Self-mail (A → A lands in both Sent and Inbox) ----------------------
SELF=$(curl -s -X POST "$API/api/v1/mail/send" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A1\"],\"subject\":\"selfmail $R\",\"text\":\"note to self\"}" | jsonval message_id)
[ -n "$SELF" ] && pass "self-mail accepted" || fail "self-mail send"
poll_inbox "$T1" "selfmail $R" inbox && pass "self-mail in Inbox" || fail "self-mail missing from Inbox"
poll_inbox "$T1" "selfmail $R" sent && pass "self-mail in Sent" || fail "self-mail missing from Sent"

# --- 10b. Unified mail store: webmail == JMAP, flags round-trip --------------
# A mailbox is one store; webmail ids are store ids. Prove the recipient's
# webmail copy is the very object JMAP serves, and that a flag set through
# webmail is visible over JMAP (and therefore to IMAP clients).
SELF_MM=$(curl -s "$API/api/v1/mail/messages?folder=inbox&q=selfmail+$R" -H "Authorization: Bearer $T1" | jsonval id)
SELF_RFC=$(curl -s "$API/api/v1/mail/messages?folder=inbox&q=selfmail+$R" -H "Authorization: Bearer $T1" | jsonval message_id)
[ -n "$SELF_MM" ] && pass "webmail exposes mail-store ids" || fail "no store id in listing"

curl -s -X PATCH "$API/api/v1/mail/messages/$SELF_MM" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" -d '{"is_starred":true}' > /dev/null

# JMAP account ids are the principal id in Stalwart's codec: big-endian
# base32 over 'a'-'z','0'-'5' (see ADR-003).
jmap_account_id() {
  awk -v n="$1" 'BEGIN{
    alpha="abcdefghijklmnopqrstuvwxyz012345";
    if (n==0) { print "a"; exit }
    s="";
    while (n>0) { s=substr(alpha,(n%32)+1,1) s; n=int(n/32) }
    print s
  }'
}
JMAP_ACCOUNT=""
if [ -n "$STALWART_HTTP" ] && [ -n "${STALWART_ADMIN_PASSWORD:-}" ]; then
  PRINCIPAL_ID=$(curl -s -u "admin:$STALWART_ADMIN_PASSWORD" "$STALWART_HTTP/api/principal/$A1" \
    | sed -n 's/.*"id":\([0-9]*\).*/\1/p' | head -1)
  [ -n "$PRINCIPAL_ID" ] && JMAP_ACCOUNT=$(jmap_account_id "$PRINCIPAL_ID")
fi

if [ -n "$STALWART_HTTP" ] && [ -n "$STALWART_MASTER_PASSWORD" ] && [ -n "$JMAP_ACCOUNT" ]; then
  JMAP_BODY="{\"using\":[\"urn:ietf:params:jmap:core\",\"urn:ietf:params:jmap:mail\"],\"methodCalls\":[[\"Email/get\",{\"accountId\":\"$JMAP_ACCOUNT\",\"ids\":[\"$SELF_MM\"],\"properties\":[\"id\",\"messageId\",\"keywords\"]},\"0\"]]}"
  JMAP_OUT=$(curl -s -u "$A1%master:$STALWART_MASTER_PASSWORD" -H "Content-Type: application/json" \
    -d "$JMAP_BODY" "$STALWART_HTTP/jmap/")
  printf '%s' "$JMAP_OUT" | grep -q "$SELF_MM" \
    && pass "JMAP returns the same message id as webmail" || fail "JMAP id mismatch"
  printf '%s' "$JMAP_OUT" | grep -q '\$flagged' \
    && pass "webmail star visible over JMAP (flag sync)" || fail "flag not synced to JMAP"
  printf '%s' "$JMAP_OUT" | grep -q "$(printf '%s' "$SELF_RFC" | sed 's/[]\/$*.^[]/\\&/g')" \
    && pass "same RFC Message-ID in webmail and JMAP" || fail "Message-ID mismatch"
else
  say "  SKIP: mail-store credentials unset, JMAP parity checks skipped"
fi

# --- 11. Message trace + delivery events -------------------------------------
tr_status=$(curl -s "$API/api/v1/admin/messages?q=$SELF" -H "Authorization: Bearer $ADM" | jsonval status)
[ "$tr_status" = "delivered" ] && pass "message trace search finds self-mail (delivered)" \
  || fail "message trace search ($tr_status)"
trace=$(curl -s "$API/api/v1/admin/messages/$SELF/trace" -H "Authorization: Bearer $ADM")
printf '%s' "$trace" | grep -q "email.accepted" && printf '%s' "$trace" | grep -q "email.delivered_local" \
  && pass "trace timeline has accepted + delivered events" || fail "trace timeline incomplete"
st=$(code GET "/api/v1/admin/messages/$SELF/trace" "$T1")
[ "$st" = "403" ] && pass "member blocked from message trace" || fail "trace RBAC ($st)"

# --- 12. Infrastructure health -----------------------------------------------
infra=$(curl -s "$API/api/v1/system/infrastructure" -H "Authorization: Bearer $ADM")
printf '%s' "$infra" | grep -q '"name":"worker"' \
  && pass "infrastructure report includes worker" || fail "infrastructure report"
st=$(code GET /api/v1/system/infrastructure "$T1")
[ "$st" = "403" ] && pass "member blocked from infrastructure" || fail "infrastructure RBAC ($st)"

# --- 13. Mail-core provisioning lifecycle ------------------------------------
prov=$(curl -s "$API/api/v1/domains" -H "Authorization: Bearer $ADM" \
  | grep -o "\"name\":\"$DOMAIN\"[^}]*" | jsonval provisioning_status)
case "$prov" in
  active|skipped) pass "e2e domain provisioning status is terminal ($prov)" ;;
  failed) fail "domain provisioning failed (mail core down during e2e?)" ;;
  *) fail "domain provisioning status ($prov)" ;;
esac

# --- 14. Unified mail store: webmail id is the mail-store id ------------------
# The message A1 sent to A2 must be reachable by the same identifier through
# the webmail API, and carry an RFC Message-ID (the cross-store identity).
first_id=$(curl -s "$API/api/v1/mail/messages?folder=inbox&limit=1" -H "Authorization: Bearer $T2" | jsonval id)
first_rfc=$(curl -s "$API/api/v1/mail/messages?folder=inbox&limit=1" -H "Authorization: Bearer $T2" | jsonval message_id)
[ -n "$first_id" ] && pass "inbox item has a mail-store id" || fail "inbox item id"
case "$first_rfc" in
  *@*) pass "inbox item carries an RFC Message-ID ($first_rfc)" ;;
  *) fail "inbox item has no RFC Message-ID" ;;
esac

# Flags are protocol flags: setting them through webmail must be readable back.
curl -s -X PATCH "$API/api/v1/mail/messages/$first_id" -H "Authorization: Bearer $T2" \
  -H "Content-Type: application/json" -d '{"is_starred":true}' > /dev/null
starred=$(curl -s "$API/api/v1/mail/messages/$first_id" -H "Authorization: Bearer $T2" | grep -o '"is_starred":true')
[ -n "$starred" ] && pass "flag set through webmail persists in the mail store" || fail "flag persistence"

# --- 15. Drafts live in the mail store ---------------------------------------
d1=$(curl -s -X POST "$API/api/v1/mail/drafts" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A2\"],\"subject\":\"draft $R\",\"text\":\"v1\"}" | jsonval id)
[ -n "$d1" ] && pass "draft created in the mail store" || fail "draft create"
d2=$(curl -s -X PUT "$API/api/v1/mail/drafts/$d1" -H "Authorization: Bearer $T1" \
  -H "Content-Type: application/json" \
  -d "{\"to\":[\"$A2\"],\"subject\":\"draft $R\",\"text\":\"v2\"}" | jsonval id)
[ -n "$d2" ] && pass "draft update returns the current id" || fail "draft update"
dcount=$(curl -s "$API/api/v1/mail/messages?folder=drafts" -H "Authorization: Bearer $T1" | grep -o "draft $R" | wc -l)
[ "$dcount" -eq 1 ] && pass "draft update leaves exactly one copy" || fail "draft duplicated ($dcount copies)"

# --- 16. Inbound SMTP from outside the platform ------------------------------
# Delivered straight to the mail core on the MTA port, unauthenticated, the
# way a foreign server would.
if command -v python3 > /dev/null 2>&1; then
  python3 - "$DOMAIN" "$A1" "$R" <<'PYEOF' > "$TMPDIR_E2E/inbound.out" 2>&1
import smtplib, ssl, sys, socket
domain, rcpt, run = sys.argv[1], sys.argv[2], sys.argv[3]
host = "mailplatform-stalwart" if socket.gethostbyname_ex("mailplatform-stalwart")[2:] else "localhost"
msg = (f"From: <outsider@external-sender.test>\r\nTo: <{rcpt}>\r\n"
       f"Subject: inbound {run}\r\nMessage-ID: <inbound-{run}@external-sender.test>\r\n\r\nhello\r\n")
s = smtplib.SMTP(host, 25, timeout=20)
s.helo("external-sender.test")
s.sendmail("outsider@external-sender.test", [rcpt], msg)
s.quit()
print("SENT")
PYEOF
  if grep -q SENT "$TMPDIR_E2E/inbound.out"; then
    pass "inbound SMTP accepted by the mail core"
    # Unauthenticated mail with no SPF/DKIM/DMARC is expected in Junk.
    found=""
    for _ in $(seq 1 15); do
      for folder in spam inbox; do
        if curl -s "$API/api/v1/mail/messages?folder=$folder" -H "Authorization: Bearer $T1" | grep -q "inbound $R"; then
          found="$folder"; break
        fi
      done
      [ -n "$found" ] && break
      sleep 1
    done
    [ -n "$found" ] && pass "inbound message delivered to the mailbox ($found)" || fail "inbound message not delivered"
  else
    fail "inbound SMTP rejected: $(head -3 "$TMPDIR_E2E/inbound.out")"
  fi
  # An unknown recipient must be refused at RCPT time, never accepted-then-dropped.
  python3 - "$DOMAIN" <<'PYEOF' > "$TMPDIR_E2E/unknown.out" 2>&1
import smtplib, sys, socket
domain = sys.argv[1]
host = "mailplatform-stalwart" if socket.gethostbyname_ex("mailplatform-stalwart")[2:] else "localhost"
s = smtplib.SMTP(host, 25, timeout=20)
s.helo("external-sender.test")
s.mail("outsider@external-sender.test")
code, _ = s.rcpt(f"definitely-no-such-user@{domain}")
print("RCPTCODE", code)
s.quit()
PYEOF
  code=$(sed -n 's/^RCPTCODE \([0-9]*\)/\1/p' "$TMPDIR_E2E/unknown.out")
  case "$code" in
    55*) pass "unknown recipient rejected at RCPT ($code)" ;;
    *) fail "unknown recipient not rejected (code=$code)" ;;
  esac
else
  say "  SKIP: python3 not found, inbound SMTP checks skipped"
fi

# --- 17. Sessions ------------------------------------------------------------
curl -s -X POST "$API/api/v1/auth/logout" -H "Authorization: Bearer $T2" > /dev/null
st=$(code GET /api/v1/mail/summary "$T2")
[ "$st" = "401" ] && pass "revoked session rejected" || fail "session revocation ($st)"

rm -rf "$TMPDIR_E2E"
say ""
say "== Result: $PASSED passed, $FAILED failed =="
[ "$FAILED" = "0" ]
