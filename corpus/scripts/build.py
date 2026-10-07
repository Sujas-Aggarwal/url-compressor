#!/usr/bin/env python3

import argparse
import gzip
import json
import math
import os
import threading
import time
import urllib.request
from concurrent.futures import FIRST_COMPLETED, ThreadPoolExecutor, wait
from pathlib import Path

import duckdb


BASE_URL = "https://data.commoncrawl.org"

ROOT = Path(__file__).resolve().parents[1]
RAW_DIR = ROOT / "raw"
SHARD_DIR = RAW_DIR / "tranco_shards"

STATE_FILE = RAW_DIR / "tranco_state.json"
FINAL_FILE = RAW_DIR / "commoncrawl-tranco.txt"
TRANCO_DB = RAW_DIR / "tranco.duckdb"

DEFAULT_CRAWL = "CC-MAIN-2026-39"
DEFAULT_TARGET = 10_000_000
DEFAULT_WORKERS = 8
DEFAULT_CANDIDATES_PER_PARTITION = 100_000
DEFAULT_DOMAIN_QUOTA = 50
DEFAULT_OVERSAMPLE = 3.0

print_lock = threading.Lock()


# ============================================================================
# Helpers
# ============================================================================

def log(message: str) -> None:
    with print_lock:
        print(message, flush=True)


def sql_string(value: str) -> str:
    """Safely turn a Python string into a SQL string literal."""
    return "'" + value.replace("'", "''") + "'"


def save_state(state: dict) -> None:
    STATE_FILE.parent.mkdir(parents=True, exist_ok=True)

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
            f"but requested crawl is {crawl}. "
            f"Use --reset to start again."
        )

    state.setdefault("partitions", [])
    state.setdefault("completed", {})

    return state


def completed_candidate_count(state: dict) -> int:
    """
    Count candidates from completed shards that actually still exist.

    This deliberately does not blindly trust the JSON state file.
    """
    total = 0

    for result in state.get("completed", {}).values():
        shard_file = result.get("file")
        rows = result.get("rows", 0)

        if not shard_file:
            continue

        shard_path = ROOT / shard_file

        if shard_path.exists():
            total += int(rows)

    return total


def cleanup_stale_temp_files() -> None:
    SHARD_DIR.mkdir(parents=True, exist_ok=True)

    for path in SHARD_DIR.glob("*.tmp.parquet"):
        try:
            path.unlink()
        except FileNotFoundError:
            pass


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
        raise RuntimeError("Common Crawl manifest is empty")

    urls = [
        f"{BASE_URL}/{path}"
        for path in paths
    ]

    log(f"Found {len(urls)} parquet partitions")

    return urls


# ============================================================================
# Tranco
# ============================================================================

def prepare_tranco_db(tranco_csv: Path) -> None:
    if TRANCO_DB.exists():
        log(f"Using existing Tranco database: {TRANCO_DB}")
        return

    log("")
    log("Creating local Tranco database...")
    log(f"Source: {tranco_csv}")

    conn = duckdb.connect(str(TRANCO_DB))

    try:
        csv_sql = sql_string(str(tranco_csv))

        conn.execute(
            f"""
            CREATE TABLE tranco AS
            SELECT
                CAST(column0 AS INTEGER) AS rank,
                lower(
                    regexp_replace(
                        trim(column1),
                        '\\\\.$',
                        ''
                    )
                ) AS domain
            FROM read_csv(
                {csv_sql},
                header = false,
                columns = {{
                    'column0': 'VARCHAR',
                    'column1': 'VARCHAR'
                }}
            )
            WHERE column0 ~ '^[0-9]+$'
              AND column1 IS NOT NULL
              AND length(trim(column1)) > 0
            """
        )

        conn.execute(
            """
            CREATE INDEX tranco_domain_idx
            ON tranco(domain)
            """
        )

        count = conn.execute(
            "SELECT COUNT(*) FROM tranco"
        ).fetchone()[0]

        if count == 0:
            raise RuntimeError(
                "Tranco CSV produced zero valid domains"
            )

        log(f"Loaded {count:,} Tranco domains")

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

    if shard_path.exists():
        raise RuntimeError(
            f"Refusing to overwrite existing shard: "
            f"{shard_path}"
        )

    conn = duckdb.connect(
        str(TRANCO_DB),
        read_only=True,
    )

    try:
        parquet_sql = sql_string(parquet_url)
        output_sql = sql_string(str(temp_path))
        limit = int(candidates_per_partition)

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

        if not temp_path.exists():
            raise RuntimeError(
                "Common Crawl query completed but shard "
                f"was not created: {temp_path}"
            )

        # Atomic completion.
        os.replace(
            temp_path,
            shard_path,
        )

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
            "rows": int(count),
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
        SHARD_DIR.glob("shard-*.parquet")
    )

    if not shard_files:
        raise RuntimeError(
            "No completed shard files found."
        )

    log(
        f"Candidate shards: {len(shard_files):,}"
    )

    conn = duckdb.connect(
        str(TRANCO_DB)
    )

    try:
        shard_sql = ", ".join(
            sql_string(str(path))
            for path in shard_files
        )

        # --------------------------------------------------------------------
        # All candidate URLs
        # --------------------------------------------------------------------

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
        # Exact URL deduplication
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
        # Domain quota + structural diversity
        #
        # Within each domain we prefer:
        #
        #   - URLs with query strings
        #   - longer paths
        #   - deeper paths
        #   - slightly longer URLs
        #
        # hash(url) provides a deterministic final tie-breaker.
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
                        ) DESC,

                        hash(url)

                ) AS domain_rank

            FROM deduplicated
            """
        )

        eligible_count = conn.execute(
            f"""
            SELECT COUNT(*)
            FROM ranked
            WHERE domain_rank <= {quota}
            """
        ).fetchone()[0]

        log(
            f"Eligible after quota: "
            f"{eligible_count:,}"
        )

        # Not enough candidates after applying the domain quota.
        #
        # We deliberately DO NOT replace the final file in this case.
        # The caller will collect more partitions and retry.
        if eligible_count < target:
            log(
                f"Final corpus is short by "
                f"{target - eligible_count:,} URLs."
            )

            return int(eligible_count)

        # --------------------------------------------------------------------
        # Export exactly `target` URLs.
        #
        # A tab delimiter with one column gives us one URL per line without
        # adding CSV quotes around URLs containing commas.
        # --------------------------------------------------------------------

        tmp_output = FINAL_FILE.with_suffix(
            ".tmp"
        )

        if tmp_output.exists():
            tmp_output.unlink()

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
                DELIMITER '\\t',
                QUOTE ''
            )
            """
        )

        if not tmp_output.exists():
            raise RuntimeError(
                "Final corpus export did not create "
                f"{tmp_output}"
            )

        # Atomic final corpus replacement.
        os.replace(
            tmp_output,
            FINAL_FILE,
        )

        # --------------------------------------------------------------------
        # Validate final output.
        # --------------------------------------------------------------------

        final_count = conn.execute(
            f"""
            SELECT COUNT(*)

            FROM read_csv(
                {sql_string(str(FINAL_FILE))},

                columns = {{
                    'url': 'VARCHAR'
                }},

                header = false,
                delimiter = '\\t',
                quote = ''
            )
            """
        ).fetchone()[0]

        if final_count != target:
            raise RuntimeError(
                f"Final corpus count mismatch: "
                f"expected {target:,}, "
                f"got {final_count:,}"
            )

        # Count domains represented by exactly the same final selection.
        domain_count = conn.execute(
            f"""
            SELECT COUNT(DISTINCT domain)

            FROM (
                SELECT
                    domain

                FROM ranked

                WHERE domain_rank <= {quota}

                ORDER BY hash(url)

                LIMIT {int(target)}
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

        return int(final_count)

    finally:
        conn.close()


# ============================================================================
# Target-aware collection scheduler
# ============================================================================

def collect_until_target(
    state: dict,
    target_candidates: int,
    workers: int,
    candidates_per_partition: int,
) -> bool:
    """
    Collect partitions until the candidate target is reached.

    IMPORTANT:
    We never submit the entire pending partition list.

    At most `workers` partitions are in flight, and the number of in-flight
    partitions is also bounded by the number needed to reach the target.

    This is what fixes the old "target=1000 but all 900 partitions run"
    behavior.
    """

    completed = state.setdefault(
        "completed",
        {},
    )

    pending = [
        partition

        for partition in state["partitions"]

        if str(partition["id"]) not in completed
    ]

    current_candidates = completed_candidate_count(
        state
    )

    log("")
    log(
        f"Existing completed candidates: "
        f"{current_candidates:,}"
    )

    log(
        f"Candidate target: "
        f"{target_candidates:,}"
    )

    log(
        f"Pending partitions: "
        f"{len(pending):,}"
    )

    if current_candidates >= target_candidates:
        log(
            "Candidate target already reached."
        )
        return True

    if not pending:
        log(
            "No pending partitions remain."
        )
        return False

    executor = ThreadPoolExecutor(
        max_workers=workers
    )

    futures = {}

    pending_index = 0

    def desired_inflight() -> int:
        """
        Number of partitions worth having in flight.

        Example:
            target remaining = 1,000
            candidates/partition = 100,000

        => only ONE partition is submitted.

        For a large target, the pool expands to `workers`.
        """

        remaining = max(
            0,
            target_candidates - current_candidates,
        )

        partitions_needed = max(
            1,
            math.ceil(
                remaining /
                candidates_per_partition
            ),
        )

        return min(
            workers,
            partitions_needed,
        )

    def submit_more() -> None:
        nonlocal pending_index

        desired = desired_inflight()

        while (
            len(futures) < desired
            and pending_index < len(pending)
        ):
            partition = pending[pending_index]
            pending_index += 1

            future = executor.submit(
                collect_partition,

                int(partition["id"]),

                partition["url"],

                candidates_per_partition,
            )

            futures[future] = partition

            log(
                f"[scheduler] submitted partition "
                f"{int(partition['id']):04d} "
                f"({len(futures)}/{desired} in flight)"
            )

    try:
        submit_more()

        while futures:
            done, _ = wait(
                futures,
                return_when=FIRST_COMPLETED,
            )

            for future in done:
                partition = futures.pop(
                    future
                )

                partition_id = int(
                    partition["id"]
                )

                try:
                    result = future.result()

                    completed[
                        str(partition_id)
                    ] = result

                    current_candidates += int(
                        result["rows"]
                    )

                    # Checkpoint immediately after every shard.
                    save_state(state)

                    log(
                        f"[checkpoint] "
                        f"partition "
                        f"{partition_id:04d} saved | "
                        f"candidates="
                        f"{current_candidates:,}/"
                        f"{target_candidates:,}"
                    )

                except Exception as exc:
                    # Failed partitions are intentionally NOT added to
                    # `completed`. They remain retryable on the next run.
                    log(
                        f"[partition "
                        f"{partition_id:04d}] "
                        f"FAILED: {exc}"
                    )

            if current_candidates >= target_candidates:
                log(
                    "Candidate target reached; "
                    "no new partitions will be submitted."
                )
                break

            submit_more()

    except KeyboardInterrupt:
        log("")
        log(
            "Interrupted. Saving completed shard state..."
        )

        save_state(state)

        raise

    finally:
        # Let already-running partition jobs finish cleanly.
        #
        # This means Ctrl+C does not leave half-written Parquet files behind.
        executor.shutdown(
            wait=True
        )

    return (
        current_candidates >= target_candidates
    )


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
            "Common Crawl crawl "
            f"(default: {DEFAULT_CRAWL})"
        ),
    )

    parser.add_argument(
        "--target",
        type=int,
        default=DEFAULT_TARGET,
        help=(
            "Final URL target "
            f"(default: {DEFAULT_TARGET:,})"
        ),
    )

    parser.add_argument(
        "--workers",
        type=int,
        default=DEFAULT_WORKERS,
        help=(
            "Parallel workers "
            f"(default: {DEFAULT_WORKERS})"
        ),
    )

    parser.add_argument(
        "--candidates-per-partition",
        type=int,
        default=DEFAULT_CANDIDATES_PER_PARTITION,
        help=(
            "Maximum candidates per partition "
            f"(default: "
            f"{DEFAULT_CANDIDATES_PER_PARTITION:,})"
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
        "--oversample",
        type=float,
        default=DEFAULT_OVERSAMPLE,
        help=(
            "Candidate oversampling multiplier "
            "before final selection "
            f"(default: "
            f"{DEFAULT_OVERSAMPLE:g})"
        ),
    )

    parser.add_argument(
        "--reset",
        action="store_true",
        help=(
            "Delete previous state, Tranco DB, "
            "final corpus, and shards"
        ),
    )

    args = parser.parse_args()

    # ------------------------------------------------------------------------
    # Validate arguments.
    # ------------------------------------------------------------------------

    if args.target <= 0:
        parser.error(
            "--target must be greater than 0"
        )

    if args.workers <= 0:
        parser.error(
            "--workers must be greater than 0"
        )

    if args.candidates_per_partition <= 0:
        parser.error(
            "--candidates-per-partition "
            "must be greater than 0"
        )

    if args.domain_quota <= 0:
        parser.error(
            "--domain-quota must be greater than 0"
        )

    if args.oversample < 1.0:
        parser.error(
            "--oversample must be at least 1.0"
        )

    # ------------------------------------------------------------------------
    # Directories.
    # ------------------------------------------------------------------------

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
            f"Tranco CSV not found: "
            f"{tranco_csv}"
        )

    # ------------------------------------------------------------------------
    # Reset.
    # ------------------------------------------------------------------------

    if args.reset:
        log(
            "RESETTING CORPUS"
        )

        if STATE_FILE.exists():
            STATE_FILE.unlink()

        if TRANCO_DB.exists():
            TRANCO_DB.unlink()

        if FINAL_FILE.exists():
            FINAL_FILE.unlink()

        for path in SHARD_DIR.glob("*"):
            if path.is_file():
                path.unlink()

    cleanup_stale_temp_files()

    # ------------------------------------------------------------------------
    # Tranco.
    # ------------------------------------------------------------------------

    prepare_tranco_db(
        tranco_csv
    )

    # ------------------------------------------------------------------------
    # State.
    # ------------------------------------------------------------------------

    state = load_state(
        args.crawl
    )

    # ------------------------------------------------------------------------
    # Common Crawl manifest.
    # ------------------------------------------------------------------------

    parquet_urls = download_manifest(
        args.crawl
    )

    manifest_partitions = [
        {
            "id": index,
            "url": url,
        }

        for index, url
        in enumerate(parquet_urls)
    ]

    # First run: persist the exact manifest.
    if not state["partitions"]:
        state["partitions"] = (
            manifest_partitions
        )

        state["completed"] = {}

        save_state(state)

    # Resume: refuse to silently mix manifests.
    elif state["partitions"] != manifest_partitions:
        raise RuntimeError(
            "Common Crawl manifest differs from "
            "the manifest stored in "
            "tranco_state.json. Refusing to mix "
            "partition sets. Use --reset if you "
            "intentionally want a fresh corpus."
        )

    completed = state.setdefault(
        "completed",
        {},
    )

    # ------------------------------------------------------------------------
    # Remove completion records whose Parquet files no longer exist.
    # ------------------------------------------------------------------------

    stale_ids = []

    for partition_id, result in completed.items():
        shard_file = result.get(
            "file"
        )

        if (
            not shard_file
            or not (
                ROOT / shard_file
            ).exists()
        ):
            stale_ids.append(
                partition_id
            )

    for partition_id in stale_ids:
        del completed[
            partition_id
        ]

    if stale_ids:
        save_state(state)

        log(
            f"Removed {len(stale_ids)} "
            "stale completion entries "
            "whose shard files were missing."
        )

    # ------------------------------------------------------------------------
    # Candidate target.
    # ------------------------------------------------------------------------

    candidate_target = math.ceil(
        args.target *
        args.oversample
    )

    # ------------------------------------------------------------------------
    # Configuration summary.
    # ------------------------------------------------------------------------

    log("")
    log("=" * 70)
    log("COMMON CRAWL COLLECTION")
    log("=" * 70)

    log(
        f"Crawl:                  "
        f"{args.crawl}"
    )

    log(
        f"Tranco:                 "
        f"{tranco_csv}"
    )

    log(
        f"Final target:           "
        f"{args.target:,}"
    )

    log(
        f"Candidate target:       "
        f"{candidate_target:,}"
    )

    log(
        f"Oversample:             "
        f"{args.oversample:g}x"
    )

    log(
        f"Workers:                "
        f"{args.workers}"
    )

    log(
        f"Partitions:             "
        f"{len(state['partitions']):,}"
    )

    log(
        f"Completed:              "
        f"{len(completed):,}"
    )

    log(
        f"Candidates/partition:   "
        f"{args.candidates_per_partition:,}"
    )

    log(
        f"Domain quota:           "
        f"{args.domain_quota}"
    )

    log("=" * 70)

    # ------------------------------------------------------------------------
    # Collect candidates and build the final corpus.
    #
    # We may need multiple collection rounds:
    #
    #   1. Collect 3x target.
    #   2. Apply dedup + domain quota.
    #   3. If fewer than target remain, collect another 3x target.
    #   4. Repeat until target is reached or partitions are exhausted.
    # ------------------------------------------------------------------------

    while True:

        reached = collect_until_target(
            state=state,
            target_candidates=candidate_target,
            workers=args.workers,
            candidates_per_partition=(
                args.candidates_per_partition
            ),
        )

        if not reached:
            log("")
            log(
                "All available partitions were "
                "exhausted before reaching the "
                "candidate target."
            )

        final_count = build_final_corpus(
            target=args.target,
            domain_quota=args.domain_quota,
        )

        # Success.
        if final_count >= args.target:
            return

        # Check whether anything remains to collect.
        remaining_partitions = [
            partition

            for partition
            in state["partitions"]

            if str(partition["id"])
            not in state["completed"]
        ]

        if not remaining_partitions:
            raise RuntimeError(
                f"Unable to build the requested "
                f"{args.target:,}-URL corpus. "
                f"Only {final_count:,} URLs are "
                f"eligible after the "
                f"{args.domain_quota}-URL/domain "
                f"quota, and all Common Crawl "
                f"partitions have been exhausted."
            )

        # Add another full oversampled batch.
        old_target = candidate_target

        candidate_target += math.ceil(
            args.target *
            args.oversample
        )

        log("")
        log(
            f"Final selection produced only "
            f"{final_count:,} URLs. "
            f"Expanding candidate target from "
            f"{old_target:,} to "
            f"{candidate_target:,}."
        )


if __name__ == "__main__":
    main()