#!/bin/sh
set -eu

template=${OPENLINEAGE_INGESTION_TEMPLATE:-/opt/datascape/openmetadata/openlineage-ingestion.yaml}
config=${OPENLINEAGE_INGESTION_CONFIG:-/tmp/openlineage-ingestion.yaml}
base_url=${OPENMETADATA_URL:-http://metadata-catalog:8585/api/v1}
username=${OPENMETADATA_USERNAME:-admin@open-metadata.org}
password=${OPENMETADATA_PASSWORD:-admin}

token=$(
  OPENMETADATA_URL="$base_url" OPENMETADATA_USERNAME="$username" OPENMETADATA_PASSWORD="$password" python - <<'PY'
import base64
import json
import os
import time
import urllib.request

base_url = os.environ["OPENMETADATA_URL"].rstrip("/")
payload = {
    "email": os.environ["OPENMETADATA_USERNAME"],
    "password": base64.b64encode(os.environ["OPENMETADATA_PASSWORD"].encode("utf-8")).decode("ascii"),
}
for _ in range(90):
    try:
        request = urllib.request.Request(
            f"{base_url}/users/login",
            data=json.dumps(payload).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(request, timeout=30) as response:
            print(json.loads(response.read().decode("utf-8"))["accessToken"])
            raise SystemExit(0)
    except Exception:
        time.sleep(2)
raise SystemExit("OpenMetadata did not become ready for OpenLineage ingestion")
PY
)

TEMPLATE="$template" CONFIG="$config" TOKEN="$token" python - <<'PY'
import os

with open(os.environ["TEMPLATE"], encoding="utf-8") as handle:
    rendered = handle.read().replace("__OPENMETADATA_JWT_TOKEN__", os.environ["TOKEN"])
with open(os.environ["CONFIG"], "w", encoding="utf-8") as handle:
    handle.write(rendered)
PY

metadata ingest -c "$config"
