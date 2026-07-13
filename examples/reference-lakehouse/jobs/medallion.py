"""Deterministic reference medallion job for synthetic attendance data."""

import os
import json
import sqlite3
import uuid
import urllib.request
from datetime import datetime, timezone
from pyspark.sql import SparkSession, functions as F, types as T


def spark_session() -> SparkSession:
    return (
        SparkSession.builder.appName("datascape-reference-medallion")
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


def emit_lineage(event_type: str, run_id: str) -> None:
    event = {
        "eventType": event_type,
        "eventTime": datetime.now(timezone.utc).isoformat(),
        "run": {"runId": run_id},
        "job": {"namespace": "datascape-reference", "name": "medallion-attendance"},
        "producer": "https://github.com/rezarajan/project-datascape",
        "schemaURL": "https://openlineage.io/spec/1-0-5/OpenLineage.json",
        "inputs": [
            {"namespace": "postgresql://postgres-source:5432", "name": "attendance.public.student_attendance"},
            {"namespace": "file:///data/sqlite", "name": "supplementary.db.school_context"},
        ],
        "outputs": [
            {"namespace": "s3://datascape-lakehouse/warehouse", "name": "education.attendance_bronze"},
            {"namespace": "s3://datascape-lakehouse/warehouse", "name": "education.attendance_silver"},
            {"namespace": "s3://datascape-lakehouse/warehouse", "name": "education.school_daily_attendance_summary"},
        ],
    }
    request = urllib.request.Request(
        "http://lineage-backend:5000/api/v1/lineage",
        data=json.dumps(event).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        if response.status >= 300:
            raise RuntimeError(f"lineage backend returned {response.status}")


def main() -> None:
    run_id = str(uuid.uuid4())
    emit_lineage("START", run_id)
    spark = spark_session()
    spark.sql("CREATE NAMESPACE IF NOT EXISTS education")

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
        rows = database.execute("SELECT school_id, district, school_type FROM school_context ORDER BY school_id").fetchall()
    context = spark.createDataFrame(rows, ["school_id", "district", "school_type"])
    context.writeTo("education.school_context_bronze").using("iceberg").createOrReplace()

    valid = attendance.filter(F.col("status_code").isin("P", "A", "L"))
    valid.writeTo("education.attendance_silver").using("iceberg").createOrReplace()
    invalid = attendance.filter(~F.col("status_code").isin("P", "A", "L"))
    invalid.writeTo("education.attendance_quarantine").using("iceberg").createOrReplace()

    gold = (
        valid.groupBy("school_id", "record_date")
        .agg(
            F.count("*").alias("submitted_count"),
            F.sum(F.when(F.col("status_code") == "P", 1).otherwise(0)).alias("present_count"),
            F.sum(F.when(F.col("status_code") == "A", 1).otherwise(0)).alias("absent_count"),
        )
        .withColumn("attendance_rate", F.round(F.col("present_count") / F.col("submitted_count"), 4))
    )
    gold.writeTo("education.school_daily_attendance_summary").using("iceberg").createOrReplace()
    spark.stop()
    emit_lineage("COMPLETE", run_id)


if __name__ == "__main__":
    main()
