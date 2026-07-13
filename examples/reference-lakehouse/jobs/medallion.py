"""Deterministic staged medallion jobs for synthetic attendance data."""

import json
import os
import sqlite3
import sys
import urllib.request
import uuid
from datetime import datetime, timezone

from pyspark.sql import SparkSession, functions as F, types as T


WAREHOUSE_NAMESPACE = "s3://datascape-lakehouse/warehouse"
PRODUCER = "https://github.com/rezarajan/project-datascape"
COLUMN_LINEAGE_SCHEMA = "https://openlineage.io/spec/facets/1-2-0/ColumnLineageDatasetFacet.json"
OPENLINEAGE_TOPIC = "openlineage-events"


def spark_session(stage: str) -> SparkSession:
    return (
        SparkSession.builder.appName(f"datascape-reference-{stage}")
        .config("spark.sql.catalog.nessie", "org.apache.iceberg.spark.SparkCatalog")
        .config("spark.sql.catalog.nessie.type", "rest")
        .config("spark.sql.catalog.nessie.uri", "http://iceberg-catalog:19120/iceberg")
        .config("spark.sql.catalog.nessie.warehouse", "datascape")
        .config("spark.sql.catalog.nessie.ref", "main")
        .config("spark.sql.catalog.nessie.s3.endpoint", "http://lakehouse-store:9000")
        .config("spark.sql.catalog.nessie.s3.path-style-access", "true")
        .config("spark.sql.catalog.nessie.client.region", "us-east-1")
        .config("spark.sql.defaultCatalog", "nessie")
        .config("spark.hadoop.fs.s3a.endpoint", "http://lakehouse-store:9000")
        .config("spark.hadoop.fs.s3a.path.style.access", "true")
        .config("spark.hadoop.fs.s3a.endpoint.region", "us-east-1")
        .config("spark.hadoop.fs.s3a.access.key", os.environ["EDUCATION_OBJECT_STORE_CREDENTIALS_ACCESSKEY"])
        .config("spark.hadoop.fs.s3a.secret.key", os.environ["EDUCATION_OBJECT_STORE_CREDENTIALS_SECRETKEY"])
        .config("spark.hadoop.fs.s3a.impl", "org.apache.hadoop.fs.s3a.S3AFileSystem")
        .config("spark.sql.extensions", "org.apache.iceberg.spark.extensions.IcebergSparkSessionExtensions")
        .getOrCreate()
    )


def iceberg_dataset(name: str) -> dict:
    return {"namespace": WAREHOUSE_NAMESPACE, "name": f"datascape.education.{name}"}


def input_field(namespace: str, name: str, field: str, subtype: str, description: str) -> dict:
    return {
        "namespace": namespace,
        "name": name,
        "field": field,
        "transformations": [
            {
                "type": "DIRECT",
                "subtype": subtype,
                "description": description,
                "masking": False,
            }
        ],
    }


def column_lineage(fields: dict[str, list[dict]]) -> dict:
    return {
        "_producer": PRODUCER,
        "_schemaURL": COLUMN_LINEAGE_SCHEMA,
        "fields": {
            column: {"inputFields": sorted(inputs, key=lambda field: (field["namespace"], field["name"], field["field"]))}
            for column, inputs in sorted(fields.items())
        },
    }


def with_column_lineage(dataset: dict, fields: dict[str, list[dict]]) -> dict:
    return {**dataset, "facets": {"columnLineage": column_lineage(fields)}}


def direct_columns(namespace: str, name: str, columns: list[str], description: str) -> dict[str, list[dict]]:
    return {
        column: [input_field(namespace, name, column, "IDENTITY", description)]
        for column in columns
    }


def emit_lineage(
    spark: SparkSession,
    event_type: str,
    job_name: str,
    run_id: str,
    inputs: list[dict],
    outputs: list[dict],
) -> None:
    event = {
        "eventType": event_type,
        "eventTime": datetime.now(timezone.utc).isoformat(),
        "run": {"runId": run_id},
        "job": {"namespace": "datascape-reference", "name": job_name},
        "producer": PRODUCER,
        "schemaURL": "https://openlineage.io/spec/1-0-5/OpenLineage.json",
        "inputs": inputs,
        "outputs": outputs,
    }
    payload = json.dumps(event, sort_keys=True)
    request = urllib.request.Request(
        "http://lineage-backend:5000/api/v1/lineage",
        data=payload.encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        if response.status >= 300:
            raise RuntimeError(f"lineage backend returned {response.status}")
    spark.createDataFrame([(payload,)], ["value"]).write.format("kafka").option(
        "kafka.bootstrap.servers", "attendance-changes:9092"
    ).option("topic", OPENLINEAGE_TOPIC).save()


def run_stage(stage: str, inputs: list[dict], outputs: list[dict], fn) -> None:
    run_id = str(uuid.uuid4())
    spark = spark_session(stage)
    try:
        emit_lineage(spark, "START", f"attendance-{stage}", run_id, inputs, outputs)
        spark.sql("CREATE NAMESPACE IF NOT EXISTS education")
        fn(spark)
        emit_lineage(spark, "COMPLETE", f"attendance-{stage}", run_id, inputs, outputs)
    finally:
        spark.stop()


def run_bronze(spark: SparkSession) -> None:
    after_schema = T.StructType(
        [
            T.StructField("attendance_id", T.LongType()),
            T.StructField("school_id", T.StringType()),
            T.StructField("student_id", T.StringType()),
            T.StructField("record_date", T.IntegerType()),
            T.StructField("status_code", T.StringType()),
            T.StructField("submitted_at", T.LongType()),
        ]
    )
    envelope_schema = T.StructType(
        [T.StructField("payload", T.StructType([T.StructField("after", after_schema)]))]
    )
    attendance = (
        spark.read.format("kafka")
        .option("kafka.bootstrap.servers", "attendance-changes:9092")
        .option("subscribe", "attendance-changes")
        .option("startingOffsets", "earliest")
        .option("endingOffsets", "latest")
        .load()
        .select(F.from_json(F.col("value").cast("string"), envelope_schema).alias("event"))
        .select("event.payload.after.*")
        .filter(F.col("attendance_id").isNotNull())
    )
    attendance.writeTo("education.attendance_bronze").using("iceberg").createOrReplace()

    with sqlite3.connect("/data/sqlite/supplementary.db") as database:
        rows = database.execute(
            "SELECT school_id, district, school_type FROM school_context ORDER BY school_id"
        ).fetchall()
    context = spark.createDataFrame(rows, ["school_id", "district", "school_type"])
    context.writeTo("education.school_context_bronze").using("iceberg").createOrReplace()


def run_silver(spark: SparkSession) -> None:
    attendance = spark.table("education.attendance_bronze")
    valid = attendance.filter(F.col("status_code").isin("P", "A", "L"))
    valid.writeTo("education.attendance_silver").using("iceberg").createOrReplace()
    invalid = attendance.filter(~F.col("status_code").isin("P", "A", "L"))
    invalid.writeTo("education.attendance_quarantine").using("iceberg").createOrReplace()


def run_gold(spark: SparkSession) -> None:
    valid = spark.table("education.attendance_silver")
    context = spark.table("education.school_context_bronze")
    gold = (
        valid.groupBy("school_id", "record_date")
        .agg(
            F.count("*").alias("submitted_count"),
            F.sum(F.when(F.col("status_code") == "P", 1).otherwise(0)).alias("present_count"),
            F.sum(F.when(F.col("status_code") == "A", 1).otherwise(0)).alias("absent_count"),
        )
        .join(context, "school_id", "left")
        .withColumn("attendance_rate", F.round(F.col("present_count") / F.col("submitted_count"), 4))
        .select(
            "school_id",
            "district",
            "school_type",
            "record_date",
            "submitted_count",
            "present_count",
            "absent_count",
            "attendance_rate",
        )
    )
    gold.writeTo("education.school_daily_attendance_summary").using("iceberg").createOrReplace()


STAGES = {
    "bronze": (
        [
            {"namespace": "postgresql://postgres-source:5432", "name": "attendance.public.student_attendance"},
            {"namespace": "file:///data/sqlite", "name": "supplementary.main.school_context"},
        ],
        [
            with_column_lineage(
                iceberg_dataset("attendance_bronze"),
                direct_columns(
                    "postgresql://postgres-source:5432",
                    "attendance.public.student_attendance",
                    [
                        "attendance_id",
                        "school_id",
                        "student_id",
                        "record_date",
                        "status_code",
                        "submitted_at",
                    ],
                    "Bronze table preserves the CDC source column without transformation.",
                ),
            ),
            with_column_lineage(
                iceberg_dataset("school_context_bronze"),
                direct_columns(
                    "file:///data/sqlite",
                    "supplementary.main.school_context",
                    ["school_id", "district", "school_type"],
                    "Bronze table preserves the SQLite source column without transformation.",
                ),
            ),
        ],
        run_bronze,
    ),
    "silver": (
        [iceberg_dataset("attendance_bronze")],
        [
            with_column_lineage(
                iceberg_dataset("attendance_silver"),
                direct_columns(
                    WAREHOUSE_NAMESPACE,
                    "datascape.education.attendance_bronze",
                    [
                        "attendance_id",
                        "school_id",
                        "student_id",
                        "record_date",
                        "status_code",
                        "submitted_at",
                    ],
                    "Silver table preserves validated bronze attendance columns.",
                ),
            ),
            with_column_lineage(
                iceberg_dataset("attendance_quarantine"),
                direct_columns(
                    WAREHOUSE_NAMESPACE,
                    "datascape.education.attendance_bronze",
                    [
                        "attendance_id",
                        "school_id",
                        "student_id",
                        "record_date",
                        "status_code",
                        "submitted_at",
                    ],
                    "Quarantine table preserves rejected bronze attendance columns for review.",
                ),
            ),
        ],
        run_silver,
    ),
    "gold": (
        [iceberg_dataset("attendance_silver"), iceberg_dataset("school_context_bronze")],
        [
            with_column_lineage(
                iceberg_dataset("school_daily_attendance_summary"),
                {
                    "school_id": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.attendance_silver",
                            "school_id",
                            "IDENTITY",
                            "Gold table groups attendance by school.",
                        )
                    ],
                    "district": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.school_context_bronze",
                            "district",
                            "IDENTITY",
                            "Gold table enriches attendance with district context.",
                        )
                    ],
                    "school_type": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.school_context_bronze",
                            "school_type",
                            "IDENTITY",
                            "Gold table enriches attendance with school type context.",
                        )
                    ],
                    "record_date": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.attendance_silver",
                            "record_date",
                            "IDENTITY",
                            "Gold table groups attendance by record date.",
                        )
                    ],
                    "submitted_count": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.attendance_silver",
                            "attendance_id",
                            "AGGREGATION",
                            "Submitted count aggregates validated attendance rows.",
                        )
                    ],
                    "present_count": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.attendance_silver",
                            "status_code",
                            "AGGREGATION",
                            "Present count aggregates rows whose status code is present.",
                        )
                    ],
                    "absent_count": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.attendance_silver",
                            "status_code",
                            "AGGREGATION",
                            "Absent count aggregates rows whose status code is absent.",
                        )
                    ],
                    "attendance_rate": [
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.attendance_silver",
                            "attendance_id",
                            "TRANSFORMATION",
                            "Attendance rate divides present attendance by submitted attendance.",
                        ),
                        input_field(
                            WAREHOUSE_NAMESPACE,
                            "datascape.education.attendance_silver",
                            "status_code",
                            "TRANSFORMATION",
                            "Attendance rate depends on present status-code counts.",
                        ),
                    ],
                },
            )
        ],
        run_gold,
    ),
}


def main() -> None:
    stage = sys.argv[1] if len(sys.argv) > 1 else "all"
    if stage == "all":
        for stage_name in ("bronze", "silver", "gold"):
            inputs, outputs, fn = STAGES[stage_name]
            run_stage(stage_name, inputs, outputs, fn)
        return
    if stage not in STAGES:
        raise SystemExit(f"usage: medallion.py [all|bronze|silver|gold], got {stage!r}")
    inputs, outputs, fn = STAGES[stage]
    run_stage(stage, inputs, outputs, fn)


if __name__ == "__main__":
    main()
