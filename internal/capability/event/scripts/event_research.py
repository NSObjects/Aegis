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

    text = str(value).strip()
    if text.lower() in {"nan", "nat", "none"}:
        return None

    return text


def resolve_entity_name(pro, symbol):
    try:
        df = pro.stock_basic(
            ts_code=symbol,
            fields="ts_code,name,fullname",
        )
    except Exception:
        return ""

    if df is None or df.empty:
        return ""

    row = df.iloc[0]
    return clean(row.get("name")) or clean(row.get("fullname")) or ""


def main():
    args = parse_args()

    try:
        as_of = dt.datetime.strptime(args.as_of, "%Y%m%d").date()
    except ValueError:
        raise RuntimeError("as_of must use YYYYMMDD format")

    if args.lookback_days <= 0:
        raise RuntimeError("lookback_days must be positive")

    token = os.environ.get("TUSHARE_TOKEN", "").strip()
    if not token:
        raise RuntimeError("TUSHARE_TOKEN is required")

    window_start = as_of - dt.timedelta(days=args.lookback_days - 1)

    pro = ts.pro_api(token)
    entity_name = resolve_entity_name(pro, args.symbol)

    df = pro.anns_d(
        ts_code=args.symbol,
        start_date=window_start.strftime("%Y%m%d"),
        end_date=as_of.strftime("%Y%m%d"),
    )

    items = []

    if df is not None and not df.empty:
        if "rec_time" in df.columns:
            df = df.sort_values("rec_time", ascending=False)
        elif "ann_date" in df.columns:
            df = df.sort_values("ann_date", ascending=False)

        for _, row in df.iterrows():
            ts_code = clean(row.get("ts_code"))
            if ts_code and ts_code != args.symbol:
                continue

            rec_time = clean(row.get("rec_time"))
            ann_date = clean(row.get("ann_date"))
            title = clean(row.get("title"))
            if not title:
                continue

            items.append(
                {
                    "published_at": rec_time or ann_date or args.as_of,
                    "title": title,
                    "url": clean(row.get("url")) or "",
                    "snippet": "",
                }
            )

    json.dump(
        {
            "entity_name": entity_name,
            "items": items,
        },
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
