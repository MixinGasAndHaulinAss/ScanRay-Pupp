#!/bin/sh
# ScanRay Pupp container entrypoint.
#
# Templates live on the persistent data volume at $NUCLEI_TEMPLATES_DIR. On
# first container start the directory will be empty, so we seed it with a
# single synchronous -update-templates call before exec'ing the Pupp agent.
# The Pupp agent itself then handles refreshes on a 24h timer.
set -e

TDIR="${NUCLEI_TEMPLATES_DIR:-/opt/scanray/data/nuclei-templates}"
NUCLEI_BIN="${NUCLEI_BINARY:-/opt/scanray/bin/nuclei}"

mkdir -p "$TDIR"

if [ -z "$(ls -A "$TDIR" 2>/dev/null)" ]; then
    echo "[entrypoint] Seeding Nuclei templates into $TDIR ..."
    if ! "$NUCLEI_BIN" -update-templates -ud "$TDIR" 2>&1; then
        echo "[entrypoint] Initial template fetch failed; Pupp will retry on the 24h loop."
    fi
else
    echo "[entrypoint] Nuclei templates already present in $TDIR"
fi

exec /opt/scanray/bin/pupp
