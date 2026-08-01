#!/usr/bin/env python3
"""Contract test for the forward-only SQLite migration set."""

from __future__ import annotations

import json
import sqlite3
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
MIGRATIONS = ROOT / "db" / "migrations"
INVENTORY = ROOT / "test" / "fixtures" / "db" / "schema_inventory.json"


def migration_files() -> list[Path]:
    files = sorted(MIGRATIONS.glob("[0-9][0-9][0-9][0-9]_*.sql"))
    versions = [int(path.name[:4]) for path in files]
    if not files or versions != list(range(1, len(files) + 1)):
        raise AssertionError(f"migration versions must be contiguous from 0001: {versions}")
    return files


def prepare(connection: sqlite3.Connection) -> None:
    connection.execute("PRAGMA foreign_keys = ON")
    connection.execute(
        "CREATE TABLE IF NOT EXISTS schema_migrations ("
        "version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at_ms INTEGER NOT NULL)"
    )


def apply(connection: sqlite3.Connection, files: list[Path]) -> int:
    prepare(connection)
    current = connection.execute(
        "SELECT COALESCE(MAX(version), 0) FROM schema_migrations"
    ).fetchone()[0]
    latest = len(files)
    if current > latest:
        raise RuntimeError(f"database schema version {current} is newer than supported {latest}")
    for path in files[current:]:
        version = int(path.name[:4])
        script = (
            "BEGIN IMMEDIATE;\n"
            + path.read_text(encoding="utf-8")
            + "\nINSERT INTO schema_migrations(version, name, applied_at_ms) "
            + f"VALUES ({version}, {sql_string(path.name)}, 0);\nCOMMIT;"
        )
        try:
            connection.executescript(script)
        except Exception:
            if connection.in_transaction:
                connection.rollback()
            raise
    return connection.execute(
        "SELECT COALESCE(MAX(version), 0) FROM schema_migrations"
    ).fetchone()[0]


def sql_string(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def expect_integrity_error(connection: sqlite3.Connection, sql: str, values: tuple) -> None:
    try:
        connection.execute(sql, values)
    except sqlite3.IntegrityError:
        return
    raise AssertionError("expected SQLite integrity constraint failure")


def seed(connection: sqlite3.Connection) -> None:
    connection.execute(
        "INSERT INTO profiles(id, updated_at_ms) VALUES ('local-profile', 0)"
    )
    connection.execute(
        "INSERT INTO sources(id,name,url,kind,enabled,content_permission,feed_format,"
        "scraper_policy_status) VALUES (?,?,?,?,?,?,?,?)",
        ("source-1", "Example", "https://example.test/feed", "feed", 1,
         "metadata_only", "auto", "not_applicable"),
    )
    connection.execute(
        "INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,"
        "fetched_at_ms,content_permission) VALUES (?,?,?,?,?,?,?)",
        ("article-1", "fp-1", "source-1", "https://example.test/a", "A", 0,
         "metadata_only"),
    )


def assert_schema(connection: sqlite3.Connection) -> None:
    expected = json.loads(INVENTORY.read_text(encoding="utf-8"))
    tables = sorted(row[0] for row in connection.execute(
        "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'"
    ))
    indexes = sorted(row[0] for row in connection.execute(
        "SELECT name FROM sqlite_schema WHERE type='index' AND name NOT LIKE 'sqlite_%'"
    ))
    assert tables == sorted(expected["tables"]), (tables, expected["tables"])
    assert indexes == sorted(expected["indexes"]), (indexes, expected["indexes"])
    schema = "\n".join(row[0] or "" for row in connection.execute(
        "SELECT sql FROM sqlite_schema"
    )).lower()
    for forbidden in expected["forbidden_schema_terms"]:
        assert forbidden.lower() not in schema, f"forbidden schema term: {forbidden}"


def assert_constraints(connection: sqlite3.Connection) -> None:
    seed(connection)
    expect_integrity_error(
        connection,
        "INSERT INTO library_states(article_id) VALUES (?)",
        ("missing-article",),
    )
    expect_integrity_error(
        connection,
        "INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,"
        "fetched_at_ms,content_permission) VALUES (?,?,?,?,?,?,?)",
        ("article-2", "fp-2", "source-1", "https://example.test/a", "Duplicate", 0,
         "metadata_only"),
    )
    expect_integrity_error(
        connection,
        "INSERT INTO articles(id,fingerprint,primary_source_id,canonical_url,title,"
        "fetched_at_ms,full_content,content_permission) VALUES (?,?,?,?,?,?,?,?)",
        ("article-3", "fp-3", "source-1", "https://example.test/c", "Forbidden", 0,
         "unpermitted body", "metadata_only"),
    )


def assert_interrupted_migration(files: list[Path]) -> None:
    connection = sqlite3.connect(":memory:")
    prepare(connection)
    try:
        connection.executescript(
            "BEGIN IMMEDIATE; CREATE TABLE interrupted(value TEXT); "
            "INSERT INTO missing_table VALUES (1); COMMIT;"
        )
    except sqlite3.OperationalError:
        if connection.in_transaction:
            connection.rollback()
    assert connection.execute(
        "SELECT count(*) FROM sqlite_schema WHERE name='interrupted'"
    ).fetchone()[0] == 0
    assert apply(connection, files) == len(files)


def assert_newer_version_rejected(files: list[Path]) -> None:
    connection = sqlite3.connect(":memory:")
    prepare(connection)
    connection.execute(
        "INSERT INTO schema_migrations(version,name,applied_at_ms) VALUES (?,?,0)",
        (len(files) + 1, "future.sql"),
    )
    try:
        apply(connection, files)
    except RuntimeError:
        return
    raise AssertionError("unsupported newer schema version was accepted")


def main() -> int:
    files = migration_files()
    with tempfile.TemporaryDirectory(prefix="news-aggregator-migrations-") as directory:
        path = Path(directory) / "contract.db"
        connection = sqlite3.connect(path)
        assert apply(connection, files) == len(files)
        first_changes = connection.total_changes
        assert apply(connection, files) == len(files)
        assert connection.total_changes == first_changes
        assert_schema(connection)
        assert_constraints(connection)
        connection.close()
    assert_interrupted_migration(files)
    assert_newer_version_rejected(files)
    print(
        f"RESULT OK migrations={len(files)} idempotent=true constraints=true "
        "interrupted_rollback=true newer_rejected=true secrets_absent=true"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
