#!/usr/bin/env bash
# Idempotent Velero + MinIO setup for Kind dev clusters.
# Backup data lives on the host at .data/velero-backups/ (via Kind extraMounts),
# so it survives cluster deletion.
set -euo pipefail

VELERO="${VELERO:-bin/velero}"
VELERO_AWS_PLUGIN="${VELERO_AWS_PLUGIN:-velero/velero-plugin-for-aws:v1.10.1}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# 1. Deploy MinIO (idempotent — kubectl apply)
echo "Deploying MinIO..."
kubectl apply -f "$SCRIPT_DIR/minio.yaml"
echo "Waiting for MinIO to be ready..."
kubectl wait --for=condition=available deployment/minio -n velero --timeout=120s

# 2. Create the "velero" bucket if it doesn't exist
echo "Ensuring 'velero' bucket exists..."
kubectl -n velero run minio-bucket-init --rm -i --restart=Never \
    --image=minio/mc:latest --command -- \
    sh -c 'until mc alias set minio http://minio:9000 minioadmin minioadmin 2>/dev/null; do sleep 1; done; mc mb --ignore-existing minio/velero'

# 3. Install Velero if not already present
if kubectl get deployment velero -n velero &>/dev/null; then
    echo "Velero already installed."
else
    echo "Installing Velero..."
    "$VELERO" install \
        --provider aws \
        --bucket velero \
        --plugins "$VELERO_AWS_PLUGIN" \
        --backup-location-config region=minio,s3ForcePathStyle=true,s3Url=http://minio.velero.svc:9000 \
        --secret-file "$SCRIPT_DIR/credentials-velero" \
        --use-volume-snapshots=false \
        --wait
fi

# 4. Wait for the backup storage location to sync (discovers existing backups on host)
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

# 5. Ensure hourly backup schedule exists
if "$VELERO" schedule get dev-backup &>/dev/null; then
    echo "Backup schedule 'dev-backup' already exists."
else
    echo "Creating hourly backup schedule (7-day retention)..."
    "$VELERO" schedule create dev-backup --schedule="0 * * * *" --ttl 168h0m0s
fi

echo "Velero is ready."
