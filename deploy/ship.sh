#!/bin/sh
set -eu

env=$1 version=$2 url=$3

install -d -m 700 ~/.ssh
printf '%s\n' "$DEPLOY_KNOWN_HOSTS" > ~/.ssh/known_hosts
printf '%s\n' "$DEPLOY_SSH_KEY" > ~/.ssh/deploy
chmod 600 ~/.ssh/deploy

ssh -i ~/.ssh/deploy "deploy@$DEPLOY_HOST" "$env" "$version" < deploy/compose.yaml

for _ in $(seq 1 30); do
  if curl -fsS "$url/" | grep -q "\"version\":\"$version\""; then
    echo "$env is running $version"
    exit 0
  fi
  sleep 2
done
echo "$env did not come up with $version" >&2
exit 1
