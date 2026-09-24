#!/usr/bin/env python3

import argparse
import datetime as dt
import json
import math
import os
import sys

import tushare as ts


def parse_args():
    parser = argparse.ArgumentParser()
    parser.add_argument("--symbol", required=True)
    parser.add_argument("--as-of", required=True)
    parser.add_argument("--lookback-days", type=int, default=7)
    return parser.parse_args()


def clean(value):
    if value is None:
        return None

    try:
        if math.isnan(value):
            return None
    except (TypeError, ValueError):
        pass

    if hasattr(value, "isoformat"):
        return value.isoformat(sep=" ")

    text = str(value).strip()
    if text.lower() in {"nan", "nat", "none"}:
        return None

    return text


def main():
    args = parse_args()

    try:
        as_of = dt.datetime.strptime(args.as_of, "%Y%m%d").date()
    except ValueError:
        raise RuntimeError("as_of must use YYYYMMDD format")

    lookback_days = args.lookback_days
    if lookback_days <= 0:
        raise RuntimeError("lookback_days must be positive")

    token = os.environ.get("TUSHARE_TOKEN", "").strip()
    if not token:
        raise RuntimeError("TUSHARE_TOKEN is required")

    window_start = as_of - dt.timedelta(days=lookback_days - 1)

    pro = ts.pro_api(token)
    df = pro.anns_d(
        ts_code=args.symbol,
        start_date=window_start.strftime("%Y%m%d"),
        end_date=as_of.strftime("%Y%m%d"),
    )

    announcements = []

    if df is not None and not df.empty:
        sort_columns = []
        if "rec_time" in df.columns:
            sort_columns.append("rec_time")
        elif "ann_date" in df.columns:
            sort_columns.append("ann_date")

        if sort_columns:
            df = df.sort_values(sort_columns, ascending=False)

        for _, row in df.iterrows():
            ts_code = clean(row.get("ts_code"))
            if ts_code and ts_code != args.symbol:
                continue

            rec_time = clean(row.get("rec_time"))
            ann_date = clean(row.get("ann_date"))

            announcements.append(
                {
                    "type": "ANNOUNCEMENT",
                    "published_at": rec_time or ann_date,
                    "ann_date": ann_date,
                    "source": "listed_company_announcement",
                    "company_name": clean(row.get("name")),
                    "title": clean(row.get("title")),
                    "url": clean(row.get("url")),
                }
            )

    result = {
        "symbol": args.symbol,
        "requested_as_of": args.as_of,
        "window_start": window_start.strftime("%Y%m%d"),
        "window_end": args.as_of,
        "coverage": {
            "announcements": True,
            "news": False,
            "policy": False,
        },
        "announcements": announcements,
        "coverage_note": (
            "Current provider covers Tushare listed-company announcements only. "
            "General news and policy sources are not covered."
        ),
    }

    json.dump(
        result,
        sys.stdout,
        ensure_ascii=False,
        separators=(",", ":"),
    )


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
