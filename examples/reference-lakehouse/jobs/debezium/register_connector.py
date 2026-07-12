"""Register the reference PostgreSQL CDC connector idempotently."""

import json
import os
import time
import urllib.error
import urllib.request


CONNECT_URL = "http://cdc-connector:8083/connectors/attendance-source/config"


def connector_config() -> dict[str, str]:
    return {
        "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
        "database.hostname": "postgres-source",
        "database.port": "5432",
        "database.user": os.environ["EDUCATION_POSTGRES_CREDENTIALS_USERNAME"],
        "database.password": os.environ["EDUCATION_POSTGRES_CREDENTIALS_PASSWORD"],
        "database.dbname": "attendance",
        "topic.prefix": "attendance",
        "table.include.list": "public.student_attendance",
        "plugin.name": "pgoutput",
        "publication.autocreate.mode": "filtered",
        "snapshot.mode": "initial",
        "transforms": "route",
        "transforms.route.type": "io.debezium.transforms.ByLogicalTableRouter",
        "transforms.route.topic.regex": ".*",
        "transforms.route.topic.replacement": "attendance-changes",
        "key.converter": "org.apache.kafka.connect.json.JsonConverter",
        "key.converter.schemas.enable": "true",
        "value.converter": "org.apache.kafka.connect.json.JsonConverter",
        "value.converter.schemas.enable": "true",
    }


def main() -> None:
    body = json.dumps(connector_config()).encode("utf-8")
    for attempt in range(60):
        request = urllib.request.Request(
            CONNECT_URL,
            data=body,
            headers={"Content-Type": "application/json"},
            method="PUT",
        )
        try:
            with urllib.request.urlopen(request, timeout=5) as response:
                if response.status < 300:
                    return
        except (urllib.error.URLError, TimeoutError):
            if attempt == 59:
                raise
            time.sleep(2)
    raise RuntimeError("Kafka Connect did not accept the connector configuration")


if __name__ == "__main__":
    main()
