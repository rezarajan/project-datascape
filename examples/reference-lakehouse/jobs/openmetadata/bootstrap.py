"""Apply declarative OpenMetadata reference catalog metadata."""

import base64
import json
import os
import sys
import time
import urllib.error
import urllib.request


ENTITY_ENDPOINTS = {
    "databaseService": "/services/databaseServices",
    "pipelineService": "/services/pipelineServices",
    "database": "/databases",
    "databaseSchema": "/databaseSchemas",
    "table": "/tables",
}


class OpenMetadata:
    def __init__(self, base_url: str, token: str):
        self.base_url = base_url.rstrip("/")
        self.token = token

    def request(self, method: str, path: str, payload: dict | None = None) -> dict:
        body = None
        headers = {"Authorization": f"Bearer {self.token}"}
        if payload is not None:
            body = json.dumps(payload, sort_keys=True).encode("utf-8")
            headers["Content-Type"] = "application/json"
        request = urllib.request.Request(
            f"{self.base_url}{path}", data=body, headers=headers, method=method
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                content = response.read()
                if not content:
                    return {}
                return json.loads(content.decode("utf-8"))
        except urllib.error.HTTPError as error:
            detail = error.read().decode("utf-8", "replace")
            raise RuntimeError(f"{method} {path} failed with {error.code}: {detail}") from error

    def create(self, kind: str, payload: dict) -> None:
        endpoint = ENTITY_ENDPOINTS[kind]
        try:
            self.request("POST", endpoint, payload)
            print(f"created {kind} {payload['name']}")
        except RuntimeError as error:
            if "409" not in str(error):
                raise
            print(f"exists {kind} {payload['name']}")


def login(base_url: str, email: str, password: str) -> str:
    payload = {
        "email": email,
        "password": base64.b64encode(password.encode("utf-8")).decode("ascii"),
    }
    request = urllib.request.Request(
        f"{base_url.rstrip('/')}/users/login",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.loads(response.read().decode("utf-8"))["accessToken"]


def wait_for_server(base_url: str) -> None:
    for _ in range(90):
        try:
            token = login(
                base_url,
                os.environ.get("OPENMETADATA_USERNAME", "admin@open-metadata.org"),
                os.environ.get("OPENMETADATA_PASSWORD", "admin"),
            )
            if token:
                return
        except Exception:
            time.sleep(2)
    raise RuntimeError("OpenMetadata did not become ready for bootstrap")


def load_config(path: str) -> dict:
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: bootstrap.py /path/to/bootstrap.json")
    config = load_config(sys.argv[1])
    base_url = os.environ.get("OPENMETADATA_URL", "http://metadata-catalog:8585/api/v1")
    wait_for_server(base_url)
    token = login(
        base_url,
        os.environ.get("OPENMETADATA_USERNAME", "admin@open-metadata.org"),
        os.environ.get("OPENMETADATA_PASSWORD", "admin"),
    )
    client = OpenMetadata(base_url, token)

    for service in config.get("databaseServices", []):
        client.create("databaseService", service)
    for service in config.get("pipelineServices", []):
        client.create("pipelineService", service)
    for database in config.get("databases", []):
        client.create("database", database)
    for schema in config.get("schemas", []):
        client.create("databaseSchema", schema)
    for table in config.get("tables", []):
        payload = {"tableType": "Regular", **table}
        client.create("table", payload)


if __name__ == "__main__":
    main()
