#!/bin/sh
# Run once as root on a fresh Ubuntu 24.04 box: sudo sh setup.sh "ssh-ed25519 AAAA... github-deploy"
set -eu

key=${1:?usage: setup.sh "<public key for GitHub Actions>"}

apt-get update
apt-get -y upgrade
apt-get -y install docker.io docker-compose-v2 ufw unattended-upgrades curl

id deploy >/dev/null 2>&1 || useradd --create-home --shell /bin/sh --groups docker deploy

install -d -o deploy -g deploy -m 700 /home/deploy/.ssh
echo "command=\"/usr/local/bin/tethr-deploy\",restrict $key" > /home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 600 /home/deploy/.ssh/authorized_keys

install -d -o deploy -g deploy -m 750 /srv/tethr
[ -f /srv/tethr/.env ] || install -o deploy -g deploy -m 600 /dev/null /srv/tethr/.env

cat > /usr/local/bin/tethr-deploy <<'SCRIPT'
#!/bin/sh
set -eu
set -- ${SSH_ORIGINAL_COMMAND:-}
[ "$#" -eq 2 ] || { echo "usage: <staging|production> <version>" >&2; exit 2; }
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
$compose up -d cloudflared "$service"
docker image prune -f >/dev/null
SCRIPT
chmod 755 /usr/local/bin/tethr-deploy

ufw allow OpenSSH
ufw --force enable

admin=${SUDO_USER:-root}
home=$(getent passwd "$admin" | cut -d: -f6)
if [ -s "$home/.ssh/authorized_keys" ]; then
  printf 'PasswordAuthentication no\nPermitRootLogin prohibit-password\n' > /etc/ssh/sshd_config.d/10-tethr.conf
  systemctl reload ssh
  echo "Password logins are off. Use your SSH key from now on."
else
  echo "No SSH key found for $admin, so password logins are still on. Add your key, then rerun this script." >&2
fi

echo "Done. Put the Cloudflare tunnel token in /srv/tethr/.env as TUNNEL_TOKEN=..."
