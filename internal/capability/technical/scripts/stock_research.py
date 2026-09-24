#!/usr/bin/env python3

import argparse
import json
import math
import os
import sys
from datetime import datetime, timedelta

import pandas as pd
import tushare as ts


REQUIRED_BARS = 120
FETCH_CALENDAR_DAYS = 420


def parse_args():
    parser = argparse.ArgumentParser(
        description="Deterministic point-in-time A-share technical research"
    )
    parser.add_argument("--symbol", required=True)
    parser.add_argument("--as-of", required=True, dest="as_of")
    return parser.parse_args()


def validate_date(value):
    try:
        datetime.strptime(value, "%Y%m%d")
    except ValueError as exc:
        raise ValueError(
            f"invalid YYYYMMDD date: {value}"
        ) from exc


def exchange_for_symbol(symbol):
    symbol = symbol.upper()
    if symbol.endswith(".SH"):
        return "SSE"
    if symbol.endswith(".SZ"):
        return "SZSE"
    if symbol.endswith(".BJ"):
        return "BSE"
    return ""


def finite(value, digits=4):
    if value is None:
        return None
    try:
        number = float(value)
    except (TypeError, ValueError):
        return None
    if not math.isfinite(number):
        return None
    return round(number, digits)


def pct_return(series, periods):
    if len(series) <= periods:
        return None
    previous = series.iloc[-1 - periods]
    current = series.iloc[-1]
    if previous == 0:
        return None
    return finite((current / previous - 1.0) * 100.0)


def wilder_rsi(series, period=14):
    delta = series.diff()
    gains = delta.clip(lower=0.0)
    losses = -delta.clip(upper=0.0)

    result = pd.Series(index=series.index, dtype="float64")
    if len(series) <= period:
        return result

    avg_gain = gains.iloc[1 : period + 1].mean()
    avg_loss = losses.iloc[1 : period + 1].mean()

    if avg_loss == 0:
        result.iloc[period] = 100.0
    else:
        rs = avg_gain / avg_loss
        result.iloc[period] = 100.0 - 100.0 / (1.0 + rs)

    for i in range(period + 1, len(series)):
        avg_gain = (
            avg_gain * (period - 1) + gains.iloc[i]
        ) / period
        avg_loss = (
            avg_loss * (period - 1) + losses.iloc[i]
        ) / period

        if avg_loss == 0:
            result.iloc[i] = 100.0
        else:
            rs = avg_gain / avg_loss
            result.iloc[i] = 100.0 - 100.0 / (1.0 + rs)

    return result


def stock_basic(pro, symbol):
    fields = (
        "ts_code,symbol,name,exchange,list_status,"
        "list_date,delist_date"
    )
    frames = []

    for status in ("L", "D", "P", "G", "UN"):
        try:
            frame = pro.stock_basic(
                ts_code=symbol,
                list_status=status,
                fields=fields,
            )
        except Exception:
            continue
        if frame is not None and not frame.empty:
            frames.append(frame)

    if not frames:
        return None

    frame = pd.concat(frames, ignore_index=True)
    frame = frame.drop_duplicates(subset=["ts_code"])
    if frame.empty:
        return None

    return frame.iloc[0].to_dict()


def classify_listing(info, requested_as_of):
    if info is None:
        return None

    list_date = str(info.get("list_date") or "").strip()
    delist_date = str(info.get("delist_date") or "").strip()
    list_status = str(info.get("list_status") or "").strip()

    if list_date and requested_as_of < list_date:
        return "NOT_LISTED"

    if delist_date and requested_as_of > delist_date:
        return "DELISTED"

    if list_status in ("G", "UN") and (
        not list_date or requested_as_of < list_date
    ):
        return "NOT_LISTED"

    if list_status == "D" and (
        not delist_date or requested_as_of > delist_date
    ):
        return "DELISTED"

    return None


def calendar_row(pro, exchange, requested_as_of):
    kwargs = {
        "start_date": requested_as_of,
        "end_date": requested_as_of,
    }
    if exchange:
        kwargs["exchange"] = exchange

    frame = pro.trade_cal(**kwargs)
    if frame is None or frame.empty:
        return None

    row = frame.iloc[0]
    return {
        "is_open": str(row.get("is_open", "")),
        "pretrade_date": str(
            row.get("pretrade_date", "") or ""
        ),
    }


def load_daily(pro, symbol, start_date, end_date):
    frame = pro.daily(
        ts_code=symbol,
        start_date=start_date,
        end_date=end_date,
    )
    if frame is None or frame.empty:
        return pd.DataFrame()

    return (
        frame.sort_values("trade_date")
        .reset_index(drop=True)
    )


def last_trade_date(pro, symbol, end_date):
    start = (
        datetime.strptime(end_date, "%Y%m%d")
        - timedelta(days=120)
    ).strftime("%Y%m%d")

    frame = load_daily(
        pro,
        symbol,
        start,
        end_date,
    )
    if frame.empty:
        return ""

    return str(frame.iloc[-1]["trade_date"])


def status_result(
    symbol,
    requested_as_of,
    status,
    last_trade="",
    warning="",
):
    warnings = []
    if warning:
        warnings.append(warning)

    return {
        "symbol": symbol,
        "requested_as_of": requested_as_of,
        "effective_as_of": "",
        "last_trade_date": last_trade,
        "status": status,
        "price_basis": {
            "snapshot": "RAW",
            "technical": "PIT_QFQ",
            "pit_anchor": "",
        },
        "snapshot": None,
        "returns": {},
        "moving_averages": {},
        "momentum": {
            "rsi14": None,
            "macd": {
                "dif": None,
                "dea": None,
                "hist": None,
            },
        },
        "volume": {},
        "coverage": {
            "bars": 0,
            "required_bars": REQUIRED_BARS,
            "sufficient_history": False,
        },
        "warnings": warnings,
    }


def main():
    args = parse_args()
    symbol = args.symbol.strip().upper()
    requested_as_of = args.as_of.strip()

    if not symbol:
        raise ValueError("symbol is required")
    validate_date(requested_as_of)

    token = os.getenv("TUSHARE_TOKEN") or ts.get_token()
    if not token:
        raise RuntimeError("TUSHARE_TOKEN is required")

    pro = ts.pro_api(token)
    exchange = exchange_for_symbol(symbol)

    info = stock_basic(
        pro,
        symbol,
    )
    listing_status = classify_listing(
        info,
        requested_as_of,
    )

    if listing_status is not None:
        result = status_result(
            symbol,
            requested_as_of,
            listing_status,
            warning=(
                "listing status determined from stock_basic"
            ),
        )
        print(
            json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return

    cal = calendar_row(
        pro,
        exchange,
        requested_as_of,
    )
    if cal is None:
        result = status_result(
            symbol,
            requested_as_of,
            "NO_DATA",
            warning="trade calendar unavailable",
        )
        print(
            json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return

    if cal["is_open"] == "1":
        effective_as_of = requested_as_of
    else:
        effective_as_of = cal["pretrade_date"]

    if not effective_as_of:
        result = status_result(
            symbol,
            requested_as_of,
            "NO_DATA",
            warning=(
                "cannot resolve effective trading date"
            ),
        )
        print(
            json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return

    target = pro.daily(
        ts_code=symbol,
        trade_date=effective_as_of,
    )

    if target is None or target.empty:
        if cal["is_open"] == "1":
            last_trade = last_trade_date(
                pro,
                symbol,
                requested_as_of,
            )

            status = "SUSPENDED" if info else "NO_DATA"
            warning = (
                "exchange is open but daily has no row; "
                "Tushare daily omits suspended trading days"
                if status == "SUSPENDED"
                else "daily row unavailable and listing metadata missing"
            )

            result = status_result(
                symbol,
                requested_as_of,
                status,
                last_trade=last_trade,
                warning=warning,
            )
            print(
                json.dumps(
                    result,
                    ensure_ascii=False,
                    separators=(",", ":"),
                )
            )
            return

        result = status_result(
            symbol,
            requested_as_of,
            "NO_DATA",
            warning=(
                "no daily row for resolved effective_as_of"
            ),
        )
        print(
            json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return

    fetch_start = (
        datetime.strptime(
            effective_as_of,
            "%Y%m%d",
        )
        - timedelta(days=FETCH_CALENDAR_DAYS)
    ).strftime("%Y%m%d")

    daily = load_daily(
        pro,
        symbol,
        fetch_start,
        effective_as_of,
    )
    if daily.empty:
        result = status_result(
            symbol,
            requested_as_of,
            "NO_DATA",
            warning="historical daily data unavailable",
        )
        print(
            json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return

    adj = pro.adj_factor(
        ts_code=symbol,
        start_date=fetch_start,
        end_date=effective_as_of,
    )
    if adj is None or adj.empty:
        result = status_result(
            symbol,
            requested_as_of,
            "NO_DATA",
            warning="adjustment factors unavailable",
        )
        print(
            json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return

    adj = (
        adj[["ts_code", "trade_date", "adj_factor"]]
        .sort_values("trade_date")
        .reset_index(drop=True)
    )

    daily = daily.merge(
        adj,
        on=["ts_code", "trade_date"],
        how="left",
    )

    if daily["adj_factor"].isna().any():
        daily = daily.dropna(
            subset=["adj_factor"]
        ).reset_index(drop=True)

    if daily.empty:
        result = status_result(
            symbol,
            requested_as_of,
            "NO_DATA",
            warning=(
                "daily and adjustment factor series do not overlap"
            ),
        )
        print(
            json.dumps(
                result,
                ensure_ascii=False,
                separators=(",", ":"),
            )
        )
        return

    effective_as_of = str(
        daily.iloc[-1]["trade_date"]
    )
    anchor_factor = float(
        daily.iloc[-1]["adj_factor"]
    )

    daily["qfq"] = (
        daily["close"]
        * daily["adj_factor"]
        / anchor_factor
    )

    qfq = daily["qfq"].astype(float)

    for period in (5, 10, 20, 60):
        daily[f"ma{period}"] = (
            qfq.rolling(period).mean()
        )

    rsi14 = wilder_rsi(
        qfq,
        14,
    )
    daily["rsi14"] = rsi14

    ema12 = qfq.ewm(
        span=12,
        adjust=False,
    ).mean()
    ema26 = qfq.ewm(
        span=26,
        adjust=False,
    ).mean()
    dif = ema12 - ema26
    dea = dif.ewm(
        span=9,
        adjust=False,
    ).mean()

    daily["dif"] = dif
    daily["dea"] = dea
    daily["hist"] = 2.0 * (
        dif - dea
    )

    daily["volume_shares"] = (
        daily["vol"].astype(float) * 100.0
    )
    daily["amount_yuan"] = (
        daily["amount"].astype(float) * 1000.0
    )

    daily["volume_ma5"] = (
        daily["volume_shares"]
        .rolling(5)
        .mean()
    )
    daily["volume_ma20"] = (
        daily["volume_shares"]
        .rolling(20)
        .mean()
    )
    daily["amount_ma5"] = (
        daily["amount_yuan"]
        .rolling(5)
        .mean()
    )
    daily["amount_ma20"] = (
        daily["amount_yuan"]
        .rolling(20)
        .mean()
    )

    latest = daily.iloc[-1]

    daily_basic = pro.daily_basic(
        ts_code=symbol,
        trade_date=effective_as_of,
        fields=(
            "ts_code,trade_date,turnover_rate,"
            "volume_ratio"
        ),
    )

    turnover_rate = None
    volume_ratio = None
    if (
        daily_basic is not None
        and not daily_basic.empty
    ):
        db = daily_basic.iloc[0]
        turnover_rate = finite(
            db.get("turnover_rate")
        )
        volume_ratio = finite(
            db.get("volume_ratio")
        )

    volume_ma5 = finite(
        latest.get("volume_ma5")
    )
    volume_ma20 = finite(
        latest.get("volume_ma20")
    )
    amount_ma5 = finite(
        latest.get("amount_ma5")
    )
    amount_ma20 = finite(
        latest.get("amount_ma20")
    )

    current_volume = float(
        latest["volume_shares"]
    )
    current_amount = float(
        latest["amount_yuan"]
    )

    warnings = []
    sufficient_history = (
        len(daily) >= REQUIRED_BARS
    )
    if not sufficient_history:
        warnings.append(
            f"insufficient_history: {len(daily)} "
            f"trading bars available; "
            f"{REQUIRED_BARS} required"
        )

    result = {
        "symbol": symbol,
        "requested_as_of": requested_as_of,
        "effective_as_of": effective_as_of,
        "last_trade_date": effective_as_of,
        "status": "NORMAL",
        "price_basis": {
            "snapshot": "RAW",
            "technical": "PIT_QFQ",
            "pit_anchor": effective_as_of,
            "anchor_adj_factor": finite(
                anchor_factor,
                6,
            ),
        },
        "snapshot": {
            "open": finite(latest["open"]),
            "high": finite(latest["high"]),
            "low": finite(latest["low"]),
            "close": finite(latest["close"]),
            "pre_close": finite(
                latest["pre_close"]
            ),
            "change_percent": finite(
                latest["pct_chg"]
            ),
            "volume": finite(
                current_volume,
                2,
            ),
            "amount": finite(
                current_amount,
                2,
            ),
            "turnover_rate": turnover_rate,
            "volume_ratio": volume_ratio,
        },
        "returns": {
            "return_1d": pct_return(qfq, 1),
            "return_5d": pct_return(qfq, 5),
            "return_10d": pct_return(qfq, 10),
            "return_20d": pct_return(qfq, 20),
            "return_60d": pct_return(qfq, 60),
        },
        "moving_averages": {
            "ma5": finite(latest.get("ma5")),
            "ma10": finite(latest.get("ma10")),
            "ma20": finite(latest.get("ma20")),
            "ma60": finite(latest.get("ma60")),
        },
        "momentum": {
            "rsi14": finite(
                latest.get("rsi14")
            ),
            "macd": {
                "dif": finite(
                    latest.get("dif")
                ),
                "dea": finite(
                    latest.get("dea")
                ),
                "hist": finite(
                    latest.get("hist")
                ),
            },
        },
        "volume": {
            "volume_ma5": volume_ma5,
            "volume_ma20": volume_ma20,
            "volume_vs_ma5": (
                finite(
                    current_volume
                    / volume_ma5
                )
                if volume_ma5
                else None
            ),
            "volume_vs_ma20": (
                finite(
                    current_volume
                    / volume_ma20
                )
                if volume_ma20
                else None
            ),
            "amount_ma5": amount_ma5,
            "amount_ma20": amount_ma20,
            "amount_vs_ma5": (
                finite(
                    current_amount
                    / amount_ma5
                )
                if amount_ma5
                else None
            ),
            "amount_vs_ma20": (
                finite(
                    current_amount
                    / amount_ma20
                )
                if amount_ma20
                else None
            ),
        },
        "coverage": {
            "bars": int(len(daily)),
            "required_bars": REQUIRED_BARS,
            "sufficient_history": (
                sufficient_history
            ),
            "start_trade_date": str(
                daily.iloc[0]["trade_date"]
            ),
            "end_trade_date": effective_as_of,
        },
        "warnings": warnings,
    }

    print(
        json.dumps(
            result,
            ensure_ascii=False,
            separators=(",", ":"),
            allow_nan=False,
        )
    )


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(
            f"stock_research failed: {exc}",
            file=sys.stderr,
        )
        sys.exit(1)
