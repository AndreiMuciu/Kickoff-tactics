#!/usr/bin/env bash
# Runs on the server, in /opt/kickoff, started by the deploy workflow:
#   IMAGE_TAG=<commit sha> ./deploy.sh
# Every run replaces Redis and both Go servers with fresh containers. The
# servers go one at a time and Caddy retries on the other one, so the site
# stays up. Caddy itself is only reloaded: restarting it would drop every
# open connection.
set -euo pipefail
cd "$(dirname "$0")"

[ -f .env ] || { echo "Missing $(pwd)/.env (see deploy/.env.example)."; exit 1; }
export IMAGE_TAG="${IMAGE_TAG:-latest}"

# The Go image has no shell, so health is asked from the Caddy container.
healthy() {
  for _ in $(seq 1 30); do
    docker compose exec -T caddy wget -qO- "http://$1:8080/healthz" >/dev/null 2>&1 && return 0
    sleep 2
  done
  echo "$1 did not become healthy:"
  docker compose logs --tail 40 "$1"
  return 1
}

docker compose pull --quiet
docker compose up -d --no-deps caddy

docker compose up -d --no-deps --force-recreate --wait redis
for server in backend-a backend-b; do
  docker compose up -d --no-deps --force-recreate "$server"
  healthy "$server"
  echo "$server is up on $IMAGE_TAG"
done

# Pick up a changed Caddyfile, then drop containers of services that no longer exist.
docker compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile >/dev/null 2>&1
docker compose up -d --remove-orphans
docker image prune -f >/dev/null

docker compose ps --format 'table {{.Service}}\t{{.State}}\t{{.Image}}'
echo "servers listening on Redis: $(docker compose exec -T redis redis-cli pubsub numsub footy:wake | tail -1)"
