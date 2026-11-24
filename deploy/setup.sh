#!/bin/sh
# Run as root on a fresh Ubuntu 26.04 box: sudo sh setup.sh production "ssh-ed25519 AAAA... github-deploy"
set -eu

usage='usage: setup.sh <production|staging> "<public key for GitHub Actions>"'
env=${1:?$usage}
key=${2:?$usage}
case $env in
  production|staging) ;;
  *) echo "$usage" >&2; exit 2 ;;
esac

apt-get update
apt-get -y upgrade
apt-get -y install docker.io docker-compose-v2 ufw unattended-upgrades curl

id deploy >/dev/null 2>&1 || useradd --create-home --shell /bin/sh --groups docker deploy

install -d -o deploy -g deploy -m 700 /home/deploy/.ssh
echo "command=\"/usr/local/bin/tethr-deploy $env\",restrict $key" > /home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 600 /home/deploy/.ssh/authorized_keys

install -d -o deploy -g deploy -m 750 /srv/tethr
[ -f /srv/tethr/.env ] || install -o deploy -g deploy -m 600 /dev/null /srv/tethr/.env

cat > /usr/local/bin/tethr-deploy <<'SCRIPT'
#!/bin/sh
set -eu
allowed=$1
set -- ${SSH_ORIGINAL_COMMAND:-}
[ "$#" -eq 2 ] || { echo "usage: $allowed <version>" >&2; exit 2; }
[ "$1" = "$allowed" ] || { echo "this box only deploys $allowed" >&2; exit 2; }
case $1 in
  staging) service=tethr-staging var=STAGING_TAG ;;
  production) service=tethr var=PRODUCTION_TAG ;;
  *) echo "unknown environment: $1" >&2; exit 2 ;;
esac
case $2 in
  ''|*[!A-Za-z0-9._-]*) echo "bad version: $2" >&2; exit 2 ;;
esac

cd /srv/tethr
touch tags.env
cat > compose.yaml.new
{ grep -v "^$var=" tags.env || true; echo "$var=$2"; } > tags.env.new
compose="docker compose --env-file .env --env-file tags.env.new -f compose.yaml.new"
$compose config -q
mv compose.yaml.new compose.yaml
mv tags.env.new tags.env

compose="docker compose --env-file .env --env-file tags.env"
$compose pull "$service"
$compose up -d --wait postgres
$compose run --rm --no-deps "$service" migrate
$compose up -d --remove-orphans cloudflared "$service"
docker image prune -f >/dev/null
SCRIPT
chmod 755 /usr/local/bin/tethr-deploy

if [ "$env" = production ]; then
  walg=v3.0.9
  case $(dpkg --print-architecture) in
    amd64) asset=wal-g-pg-24.04-amd64 ;;
    arm64) asset=wal-g-pg-24.04-aarch64 ;;
  esac
  curl -fsSL -o /tmp/wal-g "https://github.com/wal-g/wal-g/releases/download/$walg/$asset"
  echo "$(curl -fsSL "https://github.com/wal-g/wal-g/releases/download/$walg/$asset.sha256" | cut -d' ' -f1)  /tmp/wal-g" | sha256sum -c -
  install -m 755 /tmp/wal-g /usr/local/bin/wal-g
  rm /tmp/wal-g

  cat > /usr/local/bin/tethr-backup <<'SCRIPT'
#!/bin/sh
set -eu
docker exec -u postgres tethr-postgres-1 sh -c 'wal-g backup-push "$PGDATA" && wal-g delete retain FULL 7 --confirm'
url=$(sed -n 's/^HEALTHCHECK_URL=//p' /srv/tethr/.env)
[ -z "$url" ] || curl -fsS -m 10 --retry 3 "$url" >/dev/null
SCRIPT
  chmod 755 /usr/local/bin/tethr-backup
  echo '30 3 * * * deploy /usr/local/bin/tethr-backup 2>&1 | logger -t tethr-backup' > /etc/cron.d/tethr-backup
else
  rm -f /etc/cron.d/tethr-backup /usr/local/bin/tethr-backup /usr/local/bin/wal-g
fi

ufw allow OpenSSH
ufw --force enable

admin=${SUDO_USER:-root}
home=$(getent passwd "$admin" | cut -d: -f6)
if [ -s "$home/.ssh/authorized_keys" ]; then
  printf 'PasswordAuthentication no\nPermitRootLogin prohibit-password\n' > /etc/ssh/sshd_config.d/10-tethr.conf
  systemctl try-reload-or-restart ssh
  echo "Password logins are off. Use your SSH key from now on."
else
  echo "No SSH key found for $admin, so password logins are still on. Add your key, then rerun this script." >&2
fi

case $env in
  production) example=deploy/env.example ;;
  staging) example=deploy/staging/env.example ;;
esac
echo "Done. Fill in /srv/tethr/.env before the next deploy; $example lists what it needs."
