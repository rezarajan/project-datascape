"""Apply declarative OpenMetadata reference metadata and lineage."""

import base64
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


ENTITY_ENDPOINTS = {
    "databaseService": "/services/databaseServices",
    "pipelineService": "/services/pipelineServices",
    "database": "/databases",
    "databaseSchema": "/databaseSchemas",
    "table": "/tables",
    "pipeline": "/pipelines",
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

    def get_by_name(self, kind: str, fqn: str) -> dict:
        encoded = urllib.parse.quote(fqn, safe="")
        return self.request("GET", f"{ENTITY_ENDPOINTS[kind]}/name/{encoded}")

    def add_lineage(self, from_ref: dict, to_ref: dict) -> None:
        payload = {
            "edge": {
                "fromEntity": {"id": from_ref["id"], "type": from_ref["type"]},
                "toEntity": {"id": to_ref["id"], "type": to_ref["type"]},
            }
        }
        try:
            self.request("PUT", "/lineage", payload)
            print(f"lineage {from_ref['name']} -> {to_ref['name']}")
        except RuntimeError as error:
            if "409" not in str(error):
                raise
            print(f"lineage exists {from_ref['name']} -> {to_ref['name']}")


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


def entity_ref(client: OpenMetadata, value: str) -> dict:
    kind, fqn = value.split(":", 1)
    entity = client.get_by_name(kind, fqn)
    return {"id": entity["id"], "type": kind, "name": fqn}


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
    for pipeline in config.get("pipelines", []):
        client.create("pipeline", pipeline)
    for edge in config.get("lineage", []):
        client.add_lineage(entity_ref(client, edge["from"]), entity_ref(client, edge["to"]))


if __name__ == "__main__":
    main()
