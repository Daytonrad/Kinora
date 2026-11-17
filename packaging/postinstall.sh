#!/bin/sh
# .deb/.rpm post-install: dedicated system user, ownership, start the service.
set -e
if ! id kinora >/dev/null 2>&1; then
    useradd --system --home-dir /var/lib/kinora --shell /usr/sbin/nologin kinora 2>/dev/null \
        || adduser --system --home /var/lib/kinora --no-create-home --group kinora
fi
for g in video render; do
    getent group "$g" >/dev/null && usermod -aG "$g" kinora || true   # hardware transcoding
done
chown -R kinora:kinora /var/lib/kinora /var/cache/kinora /opt/kinora/bin
if command -v systemctl >/dev/null; then
    systemctl daemon-reload || true
    systemctl enable kinora >/dev/null 2>&1 || true
    systemctl restart kinora || true
fi
echo "Kinora: http://$(hostname -I 2>/dev/null | awk '{print $1}'):8096  (beállítás: /etc/kinora/kinora.env)"
