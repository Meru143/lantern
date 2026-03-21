"""Generate README screenshots and demo GIFs from a running lantern instance."""

from __future__ import annotations

import argparse
import json
import sqlite3
import time
from pathlib import Path

from PIL import Image
from playwright.sync_api import sync_playwright


def seed_demo_data(db_path: Path) -> None:
    now = time.time_ns()
    base = now - 180_000_000_000

    trace_primary = "6b2c3d4e5f60718293a4b5c6d7e8f901"
    trace_search = "9a8b7c6d5e4f30123456789abcdef012"
    trace_inventory = "1234567890abcdef1234567890abcdef"

    spans = [
        (
            trace_primary,
            "1111111111111111",
            "",
            "gateway",
            "POST /checkout",
            2,
            base,
            base + 220_000_000,
            2,
            "payment downstream failed",
            json.dumps({"http.method": "POST", "http.route": "/checkout"}),
            "[]",
        ),
        (
            trace_primary,
            "2222222222222222",
            "1111111111111111",
            "checkout",
            "place-order",
            1,
            base + 12_000_000,
            base + 205_000_000,
            0,
            "",
            json.dumps({"messaging.system": "kafka", "user.id": "42"}),
            "[]",
        ),
        (
            trace_primary,
            "3333333333333333",
            "2222222222222222",
            "inventory",
            "reserve-stock",
            2,
            base + 25_000_000,
            base + 118_000_000,
            0,
            "",
            json.dumps({"inventory.warehouse": "blr-1"}),
            "[]",
        ),
        (
            trace_primary,
            "4444444444444444",
            "2222222222222222",
            "payments",
            "charge-card",
            2,
            base + 42_000_000,
            base + 168_000_000,
            2,
            "stripe timeout",
            json.dumps(
                {
                    "peer.service": "stripe",
                    "exception.type": "TimeoutError",
                    "exception.message": "Stripe timeout for order 48291",
                }
            ),
            json.dumps(
                [
                    {
                        "name": "exception",
                        "timestamp": base + 150_000_000,
                        "attributes": {
                            "exception.type": "TimeoutError",
                            "exception.message": "Stripe timeout for order 48291",
                            "exception.stacktrace": (
                                "TimeoutError: Stripe timeout for order 48291\n"
                                "  at payments/charge.go:88\n"
                                "  at checkout/submit.go:144"
                            ),
                        },
                    }
                ]
            ),
        ),
        (
            trace_primary,
            "5555555555555555",
            "4444444444444444",
            "payments",
            "SELECT charges",
            3,
            base + 70_000_000,
            base + 136_000_000,
            0,
            "",
            json.dumps(
                {
                    "db.system": "postgresql",
                    "db.statement": "SELECT * FROM charges WHERE order_id = 48291 AND tenant_id = 17",
                }
            ),
            "[]",
        ),
        (
            trace_search,
            "6666666666666666",
            "",
            "gateway",
            "GET /search",
            2,
            base - 40_000_000_000,
            base - 39_420_000_000,
            0,
            "",
            json.dumps({"http.method": "GET", "http.route": "/search"}),
            "[]",
        ),
        (
            trace_search,
            "7777777777777777",
            "6666666666666666",
            "search",
            "search-products",
            2,
            base - 39_980_000_000,
            base - 39_500_000_000,
            0,
            "",
            json.dumps({"search.query": "lamp"}),
            "[]",
        ),
        (
            trace_inventory,
            "8888888888888888",
            "",
            "gateway",
            "POST /inventory/rebuild",
            2,
            base - 90_000_000_000,
            base - 89_100_000_000,
            0,
            "",
            json.dumps({"http.method": "POST", "http.route": "/inventory/rebuild"}),
            "[]",
        ),
        (
            trace_inventory,
            "9999999999999999",
            "8888888888888888",
            "inventory",
            "reindex-catalog",
            1,
            base - 89_800_000_000,
            base - 89_200_000_000,
            0,
            "",
            json.dumps({"job.name": "reindex-catalog"}),
            "[]",
        ),
    ]

    logs = [
        (
            "log-1",
            trace_primary,
            "1111111111111111",
            "gateway",
            base + 3_000_000,
            9,
            "INFO",
            "checkout request accepted",
            json.dumps({"http.route": "/checkout"}),
        ),
        (
            "log-2",
            trace_primary,
            "2222222222222222",
            "checkout",
            base + 48_000_000,
            13,
            "WARN",
            "retrying payment provider after 504",
            json.dumps({"retry": 1}),
        ),
        (
            "log-3",
            trace_primary,
            "4444444444444444",
            "payments",
            base + 154_000_000,
            17,
            "ERROR",
            "stripe timeout for order 48291",
            json.dumps({"provider": "stripe"}),
        ),
        (
            "log-4",
            trace_search,
            "7777777777777777",
            "search",
            base - 39_700_000_000,
            9,
            "INFO",
            "search cache warm hit",
            json.dumps({"cache": "products"}),
        ),
    ]

    errors = [
        (
            "9ff4c65ca3b0d538e9c4a1a0c99c3a2f4b2d8b8df6d84ab4cb95edb7de2f0f3b",
            "TimeoutError",
            "Stripe timeout for order ?",
            3,
            base + 145_000_000,
            base + 168_000_000,
            json.dumps(["payments"]),
            "4444444444444444",
        )
    ]

    connection = sqlite3.connect(db_path)
    try:
        connection.execute("PRAGMA journal_mode=WAL;")
        connection.executemany(
            """
            INSERT OR REPLACE INTO spans (
                trace_id, span_id, parent_span_id, service_name, name, kind,
                start_time, end_time, status_code, status_message, attributes, events
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            spans,
        )
        connection.executemany(
            """
            INSERT OR REPLACE INTO log_records (
                id, trace_id, span_id, service_name, timestamp, severity_num,
                severity_text, body, attributes
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            logs,
        )
        connection.executemany(
            """
            INSERT OR REPLACE INTO error_groups (
                fingerprint, exc_type, message_norm, count, first_seen, last_seen, services, sample_span_id
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            errors,
        )
        connection.commit()
    finally:
        connection.close()


def build_gif(frames: list[Path], output: Path) -> None:
    images = [Image.open(frame).convert("P", palette=Image.Palette.ADAPTIVE) for frame in frames]
    images[0].save(
        output,
        save_all=True,
        append_images=images[1:],
        duration=[1200, 900, 1400],
        loop=0,
        optimize=False,
    )


def capture_assets(base_url: str, db_path: Path, out_dir: Path) -> None:
    out_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1600, "height": 1200})
        page.goto(base_url)
        page.wait_for_load_state("networkidle")

        frame0 = out_dir / "demo-frame-0.png"
        frame1 = out_dir / "demo-frame-1.png"
        frame2 = out_dir / "demo-frame-2.png"

        page.screenshot(path=str(frame0))
        seed_demo_data(db_path)
        page.reload(wait_until="networkidle")
        page.wait_for_selector("#traces-table tr[data-trace-id]")
        page.screenshot(path=str(frame1))

        page.locator("#traces-table tr[data-trace-id]").first.click()
        page.wait_for_selector("#waterfall .span-bar")
        page.locator("#waterfall .span-bar").nth(1).click()
        page.wait_for_timeout(300)
        page.screenshot(path=str(frame2))
        page.screenshot(path=str(out_dir / "trace-waterfall.png"))

        page.get_by_role("button", name="Logs").click()
        page.wait_for_selector("#logs-table tr")
        page.wait_for_timeout(200)
        page.screenshot(path=str(out_dir / "log-stream.png"))

        page.get_by_role("button", name="Errors").click()
        page.wait_for_selector("#errors-table tr[data-error-id]")
        page.locator("#errors-table tr[data-error-id]").first.click()
        page.wait_for_timeout(300)
        page.screenshot(path=str(out_dir / "error-groups.png"))

        build_gif([frame0, frame1, frame2], out_dir / "lantern-demo.gif")
        browser.close()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", required=True, type=Path)
    parser.add_argument("--url", required=True)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    capture_assets(args.url, args.db, args.out)


if __name__ == "__main__":
    main()
