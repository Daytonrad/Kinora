#!/bin/sh
# .deb/.rpm pre-remove: stop the service. Data in /var/lib/kinora is kept (delete it by hand).
if command -v systemctl >/dev/null; then
    systemctl disable --now kinora >/dev/null 2>&1 || true
fi
