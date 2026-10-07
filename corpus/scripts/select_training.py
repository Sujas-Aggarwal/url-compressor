#!/usr/bin/env python3

from pathlib import Path
import argparse
import json
import time

import duckdb


DEFAULT_INPUT = "corpus/raw/tranco_shards"
DEFAULT_OUTPUT = "corpus/training/urls.txt"

TARGET_URLS = 10_000_000
DOMAIN_QUOTA = 50


EXCLUDED_PATHS = {
    "/robots.txt",
    "/sitemap.xml",
    "/sitemap_index.xml",
    "/favicon.ico",
}


def build_query(shard_glob: str, target: int, domain_quota: int) -> str:
    excluded = ", ".join(
        "'" + path.replace("'", "''") + "'"
        for path in sorted(EXCLUDED_PATHS)
    )

    return f"""
        WITH candidates AS (
            SELECT DISTINCT
                url,
                domain,
                url_path
            FROM read_parquet('{shard_glob}')
            WHERE
                url_protocol IN ('http', 'https')
                AND url IS NOT NULL
                AND domain IS NOT NULL
                AND length(url) > 0
                AND length(url) <= 4096
                AND (
                    url_path IS NULL
                    OR url_path NOT IN ({excluded})
                )
        ),

        domain_limited AS (
            SELECT
                url,
                domain
            FROM candidates
            QUALIFY ROW_NUMBER() OVER (
                PARTITION BY domain
                ORDER BY hash(url), url
            ) <= {domain_quota}
        )

        SELECT url
        FROM domain_limited
        ORDER BY hash(url), url
        LIMIT {target}
    """


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Build the final URL-compression training corpus."
    )

    parser.add_argument(
        "--input",
        default=DEFAULT_INPUT,
        help="Directory containing Common Crawl Parquet shards.",
    )

    parser.add_argument(
        "--output",
        default=DEFAULT_OUTPUT,
        help="Output URL corpus.",
    )

    parser.add_argument(
        "--target",
        type=int,
        default=TARGET_URLS,
        help="Number of URLs to select.",
    )

    parser.add_argument(
        "--domain-quota",
        type=int,
        default=DOMAIN_QUOTA,
        help="Maximum URLs selected from each domain.",
    )

    args = parser.parse_args()

    input_dir = Path(args.input)
    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    shard_glob = str(
        input_dir / "shard-[0-9][0-9][0-9][0-9].parquet"
    )

    print("=" * 60)
    print("URL TRAINING CORPUS SELECTOR")
    print("=" * 60)
    print(f"Input:              {input_dir}")
    print(f"Target URLs:        {args.target:,}")
    print(f"Domain quota:       {args.domain_quota}")
    print(f"Output:             {output_path}")
    print()

    conn = duckdb.connect()

    # Give DuckDB reasonable settings for a multi-GB Parquet scan.
    conn.execute("SET preserve_insertion_order=false")
    conn.execute("SET threads=8")

    print("Scanning source shards...")
    start = time.time()

    stats = conn.execute(
        f"""
        SELECT
            COUNT(*) AS candidates,
            COUNT(DISTINCT url) AS unique_urls,
            COUNT(DISTINCT domain) AS unique_domains
        FROM read_parquet('{shard_glob}')
        WHERE
            url_protocol IN ('http', 'https')
            AND url IS NOT NULL
            AND domain IS NOT NULL
            AND length(url) > 0
            AND length(url) <= 4096
        """
    ).fetchone()

    candidates, unique_urls, unique_domains = stats

    print(f"Candidates:         {candidates:,}")
    print(f"Unique URLs:        {unique_urls:,}")
    print(f"Unique domains:     {unique_domains:,}")
    print(f"Scan time:          {time.time() - start:.1f}s")
    print()

    query = build_query(
        shard_glob,
        args.target,
        args.domain_quota,
    )

    # First determine how many URLs survive all filters and
    # the per-domain quota.
    print("Evaluating filtered/domain-balanced corpus...")

    start = time.time()

    available = conn.execute(
        f"""
        SELECT COUNT(*)
        FROM ({query})
        """
    ).fetchone()[0]

    print(f"Available for training: {available:,}")
    print(f"Availability check:      {time.time() - start:.1f}s")
    print()

    if available < args.target:
        print(
            f"WARNING: only {available:,} URLs are available, "
            f"but {args.target:,} were requested."
        )
        print("The output will contain all available URLs.")
        print()

    # DuckDB COPY writes directly to disk without materialising
    # the complete corpus in Python memory.
    print("Writing training corpus...")
    start = time.time()

    escaped_output = str(output_path).replace("'", "''")

    conn.execute(
        f"""
        COPY (
            {query}
        )
        TO '{escaped_output}'
        (
            FORMAT CSV,
            HEADER FALSE,
            QUOTE '',
            ESCAPE ''
        )
        """
    )

    elapsed = time.time() - start

    # Count the actual output.
    selected = conn.execute(
        f"""
        SELECT COUNT(*)
        FROM read_csv(
            '{escaped_output}',
            columns={{'url': 'VARCHAR'}},
            header=false,
            quote=''
        )
        """
    ).fetchone()[0]

    # Count domains in the final corpus.
    selected_domains = conn.execute(
        f"""
        SELECT COUNT(DISTINCT domain)
        FROM (
            SELECT
                regexp_extract(
                    url,
                    '^(?:https?://)?([^/:?#]+)',
                    1
                ) AS domain
            FROM read_csv(
                '{escaped_output}',
                columns={{'url': 'VARCHAR'}},
                header=false,
                quote=''
            )
        )
        """
    ).fetchone()[0]

    report = {
        "source": {
            "input_directory": str(input_dir),
            "candidate_rows": candidates,
            "unique_urls": unique_urls,
            "unique_domains": unique_domains,
        },
        "selection": {
            "target_urls": args.target,
            "selected_urls": selected,
            "domain_quota": args.domain_quota,
            "excluded_paths": sorted(EXCLUDED_PATHS),
            "max_url_length": 4096,
        },
        "output": {
            "path": str(output_path),
            "unique_domains": selected_domains,
        },
    }

    report_path = output_path.with_name("selection.json")

    with report_path.open("w", encoding="utf-8") as f:
        json.dump(report, f, indent=2)

    print()
    print("=" * 60)
    print("SELECTION COMPLETE")
    print("=" * 60)
    print(f"Selected URLs:      {selected:,}")
    print(f"Selected domains:   {selected_domains:,}")
    print(f"Output:             {output_path}")
    print(f"Report:             {report_path}")
    print(f"Write time:         {elapsed:.1f}s")
    print("=" * 60)

    conn.close()


if __name__ == "__main__":
    main()