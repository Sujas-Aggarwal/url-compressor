#!/usr/bin/env python3

import argparse
import gzip
import json
import os
import threading
import time
import urllib.request
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor, as_completed

import duckdb


BASE_URL = "https://data.commoncrawl.org"

ROOT = Path(__file__).resolve().parents[1]
RAW_DIR = ROOT / "raw"
SHARD_DIR = RAW_DIR / "tranco_shards"

STATE_FILE = RAW_DIR / "tranco_state.json"
FINAL_FILE = RAW_DIR / "commoncrawl-tranco.txt"
TRANCO_DB = RAW_DIR / "tranco.duckdb"

DEFAULT_CRAWL = "CC-MAIN-2026-39"

DEFAULT_TARGET = 1_000_000
DEFAULT_WORKERS = 8
DEFAULT_CANDIDATES_PER_PARTITION = 100_000
DEFAULT_DOMAIN_QUOTA = 50

print_lock = threading.Lock()


# ============================================================================
# Helpers
# ============================================================================

def log(message: str) -> None:
    with print_lock:
        print(message, flush=True)


def sql_string(value: str) -> str:
    """
    Safely turn a Python string into a SQL string literal.
    """
    return "'" + value.replace("'", "''") + "'"


def save_state(state: dict) -> None:
    tmp = STATE_FILE.with_suffix(".tmp")

    with tmp.open("w", encoding="utf-8") as f:
        json.dump(state, f, indent=2, sort_keys=True)
        f.flush()
        os.fsync(f.fileno())

    os.replace(tmp, STATE_FILE)


def load_state(crawl: str) -> dict:
    if not STATE_FILE.exists():
        return {
            "crawl": crawl,
            "partitions": [],
            "completed": {},
        }

    with STATE_FILE.open("r", encoding="utf-8") as f:
        state = json.load(f)

    if state.get("crawl") != crawl:
        raise RuntimeError(
            f"Existing state belongs to {state.get('crawl')}, "
            f"but requested crawl is {crawl}.\n"
            f"Use --reset to start again."
        )

    return state


# ============================================================================
# Common Crawl manifest
# ============================================================================

def download_manifest(crawl: str) -> list[str]:
    url = (
        f"{BASE_URL}/crawl-data/"
        f"{crawl}/cc-index-table.paths.gz"
    )

    log("")
    log("Downloading Common Crawl manifest:")
    log(f"  {url}")

    with urllib.request.urlopen(url, timeout=60) as response:
        compressed = response.read()

    text = gzip.decompress(compressed).decode("utf-8")

    paths = [
        line.strip()
        for line in text.splitlines()
        if line.strip()
    ]

    if not paths:
        raise RuntimeError(
            "Common Crawl manifest is empty"
        )

    urls = [
        f"{BASE_URL}/{path}"
        for path in paths
    ]

    log(
        f"Found {len(urls)} parquet partitions"
    )

    return urls


# ============================================================================
# Tranco
# ============================================================================

def prepare_tranco_db(tranco_csv: Path) -> None:

    if TRANCO_DB.exists():
        return

    log("")
    log("Creating local Tranco database...")
    log(f"Source: {tranco_csv}")

    conn = duckdb.connect(
        str(TRANCO_DB)
    )

    try:

        conn.execute(
            """
            CREATE TABLE tranco AS
            SELECT
                CAST(column0 AS INTEGER) AS rank,
                lower(
                    regexp_replace(
                        trim(column1),
                        '\\.$',
                        ''
                    )
                ) AS domain
            FROM read_csv(
                ?,
                header = false,
                columns = {
                    'column0': 'VARCHAR',
                    'column1': 'VARCHAR'
                }
            )
            WHERE column0 ~ '^[0-9]+$'
              AND column1 IS NOT NULL
              AND length(trim(column1)) > 0
            """,
            [str(tranco_csv)],
        )

        conn.execute(
            """
            CREATE INDEX tranco_domain_idx
            ON tranco(domain)
            """
        )

        count = conn.execute(
            """
            SELECT COUNT(*)
            FROM tranco
            """
        ).fetchone()[0]

        log(
            f"Loaded {count:,} Tranco domains"
        )

    finally:
        conn.close()


# ============================================================================
# Common Crawl partition worker
# ============================================================================

def collect_partition(
    partition_id: int,
    parquet_url: str,
    candidates_per_partition: int,
) -> dict:

    shard_path = SHARD_DIR / (
        f"shard-{partition_id:04d}.parquet"
    )

    temp_path = SHARD_DIR / (
        f"shard-{partition_id:04d}.tmp.parquet"
    )

    start = time.time()

    log(
        f"[partition {partition_id:04d}] START"
    )

    if temp_path.exists():
        temp_path.unlink()

    conn = duckdb.connect(
        str(TRANCO_DB),
        read_only=True,
    )

    try:

        # --------------------------------------------------------------------
        # IMPORTANT:
        #
        # Do NOT use ? parameters inside this COPY statement.
        #
        # DuckDB can bind COPY parameters in surprising ways, which previously
        # caused:
        #
        #   read_parquet(INTEGER)
        #
        # and:
        #
        #   No files found that match "100000"
        #
        # All values below originate from our own manifest/config, so we
        # construct safe SQL literals explicitly.
        # --------------------------------------------------------------------

        parquet_sql = sql_string(
            parquet_url
        )

        output_sql = sql_string(
            str(temp_path)
        )

        limit = int(
            candidates_per_partition
        )

        query = f"""
            COPY (
                SELECT
                    url,
                    lower(url_host_registered_domain) AS domain,
                    url_protocol,
                    url_path,
                    url_query,
                    url_port

                FROM read_parquet(
                    {parquet_sql}
                )

                WHERE url IS NOT NULL

                  AND url_host_registered_domain IS NOT NULL

                  AND url_protocol IN (
                      'http',
                      'https'
                  )

                  AND length(url) <= 4096

                  AND EXISTS (
                      SELECT 1
                      FROM tranco t
                      WHERE t.domain =
                            lower(
                                url_host_registered_domain
                            )
                  )

                ORDER BY hash(url)

                LIMIT {limit}
            )

            TO {output_sql}

            (
                FORMAT PARQUET,
                COMPRESSION ZSTD
            )
        """

        conn.execute(query)

        # Atomic completion.
        os.replace(
            temp_path,
            shard_path,
        )

        # Count rows in the completed shard.
        shard_sql = sql_string(
            str(shard_path)
        )

        count = conn.execute(
            f"""
            SELECT COUNT(*)
            FROM read_parquet(
                {shard_sql}
            )
            """
        ).fetchone()[0]

        elapsed = time.time() - start

        log(
            f"[partition {partition_id:04d}] "
            f"DONE {count:,} candidates "
            f"in {elapsed:.1f}s"
        )

        return {
            "status": "completed",
            "file": str(
                shard_path.relative_to(ROOT)
            ),
            "rows": count,
            "elapsed": elapsed,
            "parquet_url": parquet_url,
        }

    except Exception as exc:

        if temp_path.exists():
            temp_path.unlink()

        log(
            f"[partition {partition_id:04d}] "
            f"FAILED: {exc}"
        )

        raise

    finally:
        conn.close()


# ============================================================================
# Final corpus construction
# ============================================================================

def build_final_corpus(
    target: int,
    domain_quota: int,
) -> int:

    log("")
    log("=" * 70)
    log("BUILDING FINAL CORPUS")
    log("=" * 70)

    shard_files = sorted(
        SHARD_DIR.glob(
            "shard-*.parquet"
        )
    )

    if not shard_files:
        raise RuntimeError(
            "No completed shard files found."
        )

    log(
        f"Candidate shards: {len(shard_files)}"
    )

    conn = duckdb.connect(
        str(TRANCO_DB)
    )

    try:

        # --------------------------------------------------------------------
        # Build a SQL array containing every shard.
        # --------------------------------------------------------------------

        shard_sql = ", ".join(
            sql_string(str(path))
            for path in shard_files
        )

        conn.execute(
            f"""
            CREATE OR REPLACE TEMP VIEW candidates AS

            SELECT
                url,
                domain,
                url_protocol,
                url_path,
                url_query,
                url_port

            FROM read_parquet(
                [{shard_sql}],
                union_by_name = true
            )
            """
        )

        candidate_count = conn.execute(
            """
            SELECT COUNT(*)
            FROM candidates
            """
        ).fetchone()[0]

        log(
            f"Total candidates: {candidate_count:,}"
        )

        # --------------------------------------------------------------------
        # Exact URL deduplication.
        # --------------------------------------------------------------------

        conn.execute(
            """
            CREATE OR REPLACE TEMP VIEW deduplicated AS

            SELECT
                url,
                domain,
                url_protocol,
                url_path,
                url_query,
                url_port

            FROM (
                SELECT
                    *,
                    ROW_NUMBER() OVER (
                        PARTITION BY url
                        ORDER BY hash(url)
                    ) AS duplicate_rank

                FROM candidates
            )

            WHERE duplicate_rank = 1
            """
        )

        unique_count = conn.execute(
            """
            SELECT COUNT(*)
            FROM deduplicated
            """
        ).fetchone()[0]

        log(
            f"Unique URLs:      {unique_count:,}"
        )

        # --------------------------------------------------------------------
        # Domain quota + structural diversity.
        #
        # Prefer URLs containing:
        #
        #   - queries
        #   - longer paths
        #   - deeper paths
        #
        # while still using hash(url) as a deterministic tie-breaker.
        # --------------------------------------------------------------------

        quota = int(domain_quota)

        conn.execute(
            f"""
            CREATE OR REPLACE TEMP VIEW ranked AS

            SELECT
                *,

                ROW_NUMBER() OVER (
                    PARTITION BY domain

                    ORDER BY
                        (
                            CASE
                                WHEN url_query IS NOT NULL
                                     AND url_query != ''
                                THEN 100
                                ELSE 0
                            END

                            +

                            CASE
                                WHEN url_path IS NOT NULL
                                THEN length(url_path)
                                ELSE 0
                            END

                            +

                            CASE
                                WHEN url_path IS NOT NULL
                                THEN
                                    length(
                                        regexp_replace(
                                            url_path,
                                            '[^/]',
                                            '',
                                            'g'
                                        )
                                    )
                                ELSE 0
                            END

                            +

                            length(url) / 1000.0

                            +

                            hash(url)
                                / 9223372036854775808.0
                        ) DESC
                ) AS domain_rank

            FROM deduplicated
            """
        )

        # --------------------------------------------------------------------
        # Final output.
        # --------------------------------------------------------------------

        tmp_output = FINAL_FILE.with_suffix(
            ".tmp"
        )

        output_sql = sql_string(
            str(tmp_output)
        )

        conn.execute(
            f"""
            COPY (
                SELECT url

                FROM ranked

                WHERE domain_rank <= {quota}

                ORDER BY hash(url)

                LIMIT {int(target)}
            )

            TO {output_sql}

            (
                FORMAT CSV,
                HEADER false,
                QUOTE ''
            )
            """
        )

        os.replace(
            tmp_output,
            FINAL_FILE,
        )

        # --------------------------------------------------------------------
        # Final statistics.
        # --------------------------------------------------------------------

        final_sql = sql_string(
            str(FINAL_FILE)
        )

        final_count = conn.execute(
            f"""
            SELECT COUNT(*)
            FROM read_csv(
                {final_sql},
                columns = {
                    'url': 'VARCHAR'
                },
                header = false,
                quote = ''
            )
            """
        ).fetchone()[0]

        domain_count = conn.execute(
            f"""
            SELECT COUNT(DISTINCT domain)

            FROM ranked

            WHERE domain_rank <= {quota}

              AND url IN (
                  SELECT url
                  FROM read_csv(
                      {final_sql},
                      columns = {
                          'url': 'VARCHAR'
                      },
                      header = false,
                      quote = ''
                  )
              )
            """
        ).fetchone()[0]

        log("")
        log("=" * 70)
        log("CORPUS COMPLETE")
        log("=" * 70)
        log(
            f"URLs:     {final_count:,}"
        )
        log(
            f"Domains:  {domain_count:,}"
        )
        log(
            f"Output:   {FINAL_FILE}"
        )
        log("=" * 70)

        return final_count

    finally:
        conn.close()


# ============================================================================
# Main
# ============================================================================

def main() -> None:

    parser = argparse.ArgumentParser(
        description=(
            "Build a large, Tranco-driven, "
            "resumable Common Crawl URL corpus."
        )
    )

    parser.add_argument(
        "--tranco",
        required=True,
        help="Path to Tranco CSV",
    )

    parser.add_argument(
        "--crawl",
        default=DEFAULT_CRAWL,
        help=(
            f"Common Crawl crawl "
            f"(default: {DEFAULT_CRAWL})"
        ),
    )

    parser.add_argument(
        "--target",
        type=int,
        default=DEFAULT_TARGET,
        help=(
            f"Final URL target "
            f"(default: {DEFAULT_TARGET:,})"
        ),
    )

    parser.add_argument(
        "--workers",
        type=int,
        default=DEFAULT_WORKERS,
        help=(
            f"Parallel workers "
            f"(default: {DEFAULT_WORKERS})"
        ),
    )

    parser.add_argument(
        "--candidates-per-partition",
        type=int,
        default=DEFAULT_CANDIDATES_PER_PARTITION,
        help=(
            "Maximum candidates per partition "
            f"(default: {DEFAULT_CANDIDATES_PER_PARTITION:,})"
        ),
    )

    parser.add_argument(
        "--domain-quota",
        type=int,
        default=DEFAULT_DOMAIN_QUOTA,
        help=(
            "Maximum final URLs per domain "
            f"(default: {DEFAULT_DOMAIN_QUOTA})"
        ),
    )

    parser.add_argument(
        "--reset",
        action="store_true",
        help="Delete previous state and shards",
    )

    args = parser.parse_args()

    RAW_DIR.mkdir(
        parents=True,
        exist_ok=True,
    )

    SHARD_DIR.mkdir(
        parents=True,
        exist_ok=True,
    )

    tranco_csv = Path(
        args.tranco
    ).resolve()

    if not tranco_csv.exists():
        raise FileNotFoundError(
            f"Tranco CSV not found: {tranco_csv}"
        )

    # ------------------------------------------------------------------------
    # Reset
    # ------------------------------------------------------------------------

    if args.reset:

        log("RESETTING CORPUS")

        if STATE_FILE.exists():
            STATE_FILE.unlink()

        if TRANCO_DB.exists():
            TRANCO_DB.unlink()

        if FINAL_FILE.exists():
            FINAL_FILE.unlink()

        for path in SHARD_DIR.glob("*"):
            path.unlink()

    # ------------------------------------------------------------------------
    # Tranco
    # ------------------------------------------------------------------------

    prepare_tranco_db(
        tranco_csv
    )

    # ------------------------------------------------------------------------
    # State
    # ------------------------------------------------------------------------

    state = load_state(
        args.crawl
    )

    # ------------------------------------------------------------------------
    # Common Crawl manifest
    # ------------------------------------------------------------------------

    parquet_urls = download_manifest(
        args.crawl
    )

    # ------------------------------------------------------------------------
    # Save exact partition list.
    # ------------------------------------------------------------------------

    if not state["partitions"]:

        state["partitions"] = [
            {
                "id": index,
                "url": url,
            }

            for index, url
            in enumerate(parquet_urls)
        ]

        state["completed"] = {}

        save_state(state)

    completed = state.setdefault(
        "completed",
        {},
    )

    pending = [
        partition

        for partition
        in state["partitions"]

        if str(partition["id"])
        not in completed
    ]

    log("")
    log("=" * 70)
    log("COMMON CRAWL COLLECTION")
    log("=" * 70)
    log(
        f"Crawl:                 {args.crawl}"
    )
    log(
        f"Tranco:                {tranco_csv}"
    )
    log(
        f"Final target:          {args.target:,}"
    )
    log(
        f"Workers:               {args.workers}"
    )
    log(
        f"Partitions:            "
        f"{len(state['partitions'])}"
    )
    log(
        f"Completed:             "
        f"{len(completed)}"
    )
    log(
        f"Pending:               "
        f"{len(pending)}"
    )
    log(
        f"Candidates/partition:  "
        f"{args.candidates_per_partition:,}"
    )
    log(
        f"Domain quota:          "
        f"{args.domain_quota}"
    )
    log("=" * 70)

    # ------------------------------------------------------------------------
    # Parallel collection
    # ------------------------------------------------------------------------

    if pending:

        with ThreadPoolExecutor(
            max_workers=args.workers
        ) as executor:

            futures = {
                executor.submit(
                    collect_partition,
                    partition["id"],
                    partition["url"],
                    args.candidates_per_partition,
                ): partition

                for partition
                in pending
            }

            for future in as_completed(
                futures
            ):

                partition = futures[
                    future
                ]

                partition_id = partition[
                    "id"
                ]

                try:

                    result = future.result()

                    completed[
                        str(partition_id)
                    ] = result

                    # Checkpoint immediately after
                    # every successfully completed shard.
                    save_state(state)

                    log(
                        f"[checkpoint] "
                        f"partition "
                        f"{partition_id:04d} "
                        f"saved"
                    )

                except Exception as exc:

                    log(
                        f"[partition "
                        f"{partition_id:04d}] "
                        f"FAILED: {exc}"
                    )

    # ------------------------------------------------------------------------
    # Remaining partitions
    # ------------------------------------------------------------------------

    remaining = [
        partition

        for partition
        in state["partitions"]

        if str(partition["id"])
        not in completed
    ]

    if remaining:

        log("")
        log(
            f"{len(remaining)} partitions "
            f"remain incomplete."
        )
        log(
            "Run the same command again "
            "to resume."
        )

        return

    # ------------------------------------------------------------------------
    # Final corpus
    # ------------------------------------------------------------------------

    build_final_corpus(
        target=args.target,
        domain_quota=args.domain_quota,
    )


if __name__ == "__main__":
    main()