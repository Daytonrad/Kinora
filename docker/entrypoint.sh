#!/bin/sh
# Kinora container entrypoint: applies PUID/PGID, grants GPU device access, drops root.
set -e

if [ "$(id -u)" != "0" ]; then
    # Started with --user: nothing to remap, just run.
    exec "$@"
fi

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if [ "$(id -g kinora)" != "$PGID" ]; then groupmod -o -g "$PGID" kinora; fi
if [ "$(id -u kinora)" != "$PUID" ]; then usermod -o -u "$PUID" kinora; fi

# Hardware acceleration: join whatever group owns the passed-through /dev/dri nodes
# (render/video GIDs differ between Unraid, TrueNAS, Synology, plain Debian...).
for dev in /dev/dri/* /dev/nvidia0; do
    [ -e "$dev" ] || continue
    gid="$(stat -c %g "$dev")"
    [ "$gid" = "0" ] && continue
    grp="$(getent group "$gid" | cut -d: -f1 || true)"
    if [ -z "$grp" ]; then
        grp="hw$gid"
        groupadd -o -g "$gid" "$grp"
    fi
    usermod -aG "$grp" kinora
done

data="${KINORA_DATA:-/config}"
cache="${KINORA_CACHE:-/cache}"
mkdir -p "$data" "$cache"
for d in "$data" "$cache"; do
    # Recursive chown only when ownership is wrong, so big caches don't slow every start.
    if [ "$(stat -c %u:%g "$d")" != "$PUID:$PGID" ]; then
        echo "kinora: fixing ownership of $d -> $PUID:$PGID"
        chown -R "$PUID:$PGID" "$d"
    fi
done

exec gosu kinora "$@"
