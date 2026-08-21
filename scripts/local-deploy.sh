#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/home/admmail/.local/bin:$PATH"
export GOPATH="${GOPATH:-$ROOT_DIR/var/gopath}"
export GOCACHE="${GOCACHE:-$ROOT_DIR/var/go-build}"
export GOFLAGS="${GOFLAGS:--buildvcs=false}"
export PATH="/home/admmail/opt/postgres/bin:/home/admmail/opt/nats/bin:/home/admmail/opt/minio:$PATH"
export LD_LIBRARY_PATH="/home/admmail/opt/postgres/usr/lib/x86_64-linux-gnu:${LD_LIBRARY_PATH:-}"
WEB_PORT="${WEB_PORT:-3000}"
PG_RUNTIME_USER="${PG_RUNTIME_USER:-nobody}"
STALWART_BIN="${STALWART_BIN:-/home/admmail/opt/stalwart-0.13.4/stalwart}"
STALWART_CONFIG="${STALWART_CONFIG:-$ROOT_DIR/deploy/stalwart/config.toml}"
STALWART_DATA_DIR="${STALWART_DATA_DIR:-$ROOT_DIR/var/stalwart/data}"
if [[ "$(id -u)" -eq 0 ]]; then
  PG_DB_USER="${PG_DB_USER:-$PG_RUNTIME_USER}"
else
  PG_DB_USER="${PG_DB_USER:-$(id -un)}"
fi

cd "$ROOT_DIR"
set -a
source "$ROOT_DIR/.env"
set +a

mkdir -p "$ROOT_DIR/var"/{postgres,redis,nats,minio,logs}
mkdir -p "$STALWART_DATA_DIR"
mkdir -p "$ROOT_DIR/bin"

if [[ ! -x /home/admmail/opt/postgres/bin/initdb ]]; then
  echo "PostgreSQL binaries not found in /home/admmail/opt/postgres/bin" >&2
  exit 1
fi

POSTGRES_RUNNER=()
if [[ "$(id -u)" -eq 0 ]]; then
  chown -R "$PG_RUNTIME_USER":"$PG_RUNTIME_USER" "$ROOT_DIR/var/postgres" 2>/dev/null || true
  chmod 700 "$ROOT_DIR/var/postgres" 2>/dev/null || true
  POSTGRES_RUNNER=(runuser -u "$PG_RUNTIME_USER" --)
fi

if [[ -f "$ROOT_DIR/var/postgres/postmaster.pid" ]]; then
  pid="$(head -n 1 "$ROOT_DIR/var/postgres/postmaster.pid" 2>/dev/null || true)"
  if [[ -n "${pid:-}" ]] && ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$ROOT_DIR/var/postgres/postmaster.pid" "$ROOT_DIR/var/postgres/.s.PGSQL.5433" "$ROOT_DIR/var/postgres/.s.PGSQL.5433.lock"
  fi
fi

if [[ ! -f "$ROOT_DIR/var/postgres/PG_VERSION" ]]; then
  "${POSTGRES_RUNNER[@]}" /home/admmail/opt/postgres/bin/initdb -D "$ROOT_DIR/var/postgres" --auth=trust --no-locale >"$ROOT_DIR/var/logs/initdb.log"
fi

if [[ ! -f "$ROOT_DIR/var/postgres/postmaster.pid" ]]; then
  "${POSTGRES_RUNNER[@]}" /home/admmail/opt/postgres/bin/pg_ctl -D "$ROOT_DIR/var/postgres" -l "$ROOT_DIR/var/logs/postgres.log" -o "-p 5433 -h 127.0.0.1 -c unix_socket_directories=$ROOT_DIR/var/postgres" start
fi

if [[ ! -f "$ROOT_DIR/var/nats.pid" ]]; then
  /home/admmail/opt/nats/bin/nats-server -js -sd "$ROOT_DIR/var/nats" -m 8222 -p 4222 >"$ROOT_DIR/var/logs/nats.log" 2>&1 &
  echo $! > "$ROOT_DIR/var/nats.pid"
fi

MINIO_DATA_DIR="${MINIO_DATA_DIR:-/tmp/email-minio-data}"
if [[ ! -f "$ROOT_DIR/var/minio.pid" ]]; then
  mkdir -p "$MINIO_DATA_DIR"
  MINIO_ROOT_USER="${S3_ACCESS_KEY:-minioadmin}" \
  MINIO_ROOT_PASSWORD="${S3_SECRET_KEY:-minioadmin_dev}" \
    /home/admmail/opt/minio/minio server "$MINIO_DATA_DIR" --address 127.0.0.1:9000 --console-address 127.0.0.1:9001 >"$ROOT_DIR/var/logs/minio.log" 2>&1 &
  echo $! > "$ROOT_DIR/var/minio.pid"
fi

if [[ "${MAIL_CORE_PROVIDER:-none}" == "stalwart" && ! -f "$ROOT_DIR/var/stalwart.pid" ]]; then
  if [[ ! -x "$STALWART_BIN" ]]; then
    echo "Stalwart binary not found or not executable: $STALWART_BIN" >&2
    exit 1
  fi
  nohup env \
    STALWART_ADMIN_SECRET="${STALWART_ADMIN_PASSWORD}" \
    STALWART_MASTER_SECRET="${STALWART_MASTER_PASSWORD}" \
    "$STALWART_BIN" --config "$STALWART_CONFIG" >"$ROOT_DIR/var/logs/stalwart.log" 2>&1 &
  echo $! > "$ROOT_DIR/var/stalwart.pid"
fi

/home/admmail/opt/postgres/bin/createuser -U "$PG_DB_USER" -h 127.0.0.1 -p 5433 mailplatform >/dev/null 2>&1 || true
/home/admmail/opt/postgres/bin/createdb -U "$PG_DB_USER" -h 127.0.0.1 -p 5433 -O mailplatform mailplatform >/dev/null 2>&1 || true

go env -w GOTOOLCHAIN=local >/dev/null 2>&1 || true
go mod download
go build -o "$ROOT_DIR/bin/fakeredis" ./cmd/fakeredis
go build -o "$ROOT_DIR/bin/api" ./cmd/api
go build -o "$ROOT_DIR/bin/worker" ./cmd/worker
go build -o "$ROOT_DIR/bin/migrate" ./cmd/migrate

pushd "$ROOT_DIR/apps/web" >/dev/null
export PATH="/home/admmail/opt/node-v24.18.0-linux-x64/bin:$PATH"
if [[ ! -d node_modules ]]; then
  npm install
fi
npm run build
popd >/dev/null

"$ROOT_DIR/bin/migrate" up

if ! pgrep -f "$ROOT_DIR/bin/api" >/dev/null; then
  nohup "$ROOT_DIR/bin/api" >"$ROOT_DIR/var/logs/api.log" 2>&1 &
fi

if ! pgrep -f "$ROOT_DIR/bin/worker" >/dev/null; then
  nohup "$ROOT_DIR/bin/worker" >"$ROOT_DIR/var/logs/worker.log" 2>&1 &
fi

if ! pgrep -f "$ROOT_DIR/bin/fakeredis" >/dev/null; then
  nohup "$ROOT_DIR/bin/fakeredis" >"$ROOT_DIR/var/logs/redis.log" 2>&1 &
fi

if ! pgrep -f "next start" >/dev/null; then
  nohup env PATH="/home/admmail/opt/node-v24.18.0-linux-x64/bin:$PATH" PORT="$WEB_PORT" HOSTNAME=0.0.0.0 "$ROOT_DIR/apps/web/node_modules/.bin/next" start "$ROOT_DIR/apps/web" >"$ROOT_DIR/var/logs/web.log" 2>&1 &
fi

echo "Deployment started."
