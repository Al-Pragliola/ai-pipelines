#!/usr/bin/env bash
# Restore from the latest completed Velero backup.
# Safe to run when no backups exist — exits cleanly.
set -euo pipefail

VELERO="${VELERO:-bin/velero}"

# Wait for backup storage location to sync (so Velero discovers backups on disk)
echo "Waiting for backup storage location to sync..."
for _ in $(seq 1 60); do
    synced=$("$VELERO" backup-location get -o json 2>/dev/null \
        | jq -r '.status.lastSyncedTime // empty') || true
    if [ -n "$synced" ] && [ "$synced" != "null" ]; then
        echo "Backup storage location synced."
        break
    fi
    sleep 2
done

# Find the latest completed backup
latest=$("$VELERO" backup get -o json 2>/dev/null | jq -r '
    [.items[] | select(.status.phase == "Completed")]
    | sort_by(.status.completionTimestamp)
    | reverse | .[0].metadata.name // empty
') || true

if [ -z "$latest" ] || [ "$latest" = "null" ]; then
    echo "No completed backups found. Starting fresh."
    exit 0
fi

echo "Restoring from backup: $latest"
"$VELERO" restore create --from-backup "$latest" --wait
echo "Restore complete."
