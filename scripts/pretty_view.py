#!/usr/bin/env python3
"""
Pretty structured view for oi-assistant /analyse output.
Reads JSON from stdin and prints human-friendly summary.
Usage:
  curl -s 'http://localhost:8080/analyse?symbol=NIFTY' | python3 scripts/pretty_view.py
"""
import sys
import json
from collections import Counter

def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        print("Error parsing JSON:", e)
        sys.exit(1)

    sigs = data.get("signals", [])
    symbol = sys.argv[1] if len(sys.argv) > 1 else "NIFTY"
    print(f"{symbol} OI Analysis")
    print("=" * 55)
    print(f"Total significant OI moves (>=20% change): {len(sigs)}")
    print(f"Cached from Redis: {data.get('cached')}")
    actions = Counter(s.get("action", "WATCH") for s in sigs)
    print(f"  BUY: {actions.get('BUY', 0):2d}   SELL: {actions.get('SELL', 0):2d}   WATCH: {actions.get('WATCH', 0):2d}")
    print()

    buys = [s for s in sigs if s.get("action") == "BUY"]
    sells = [s for s in sigs if s.get("action") == "SELL"]

    if buys:
        print("🟢 BUY signals (put writing / bullish PE build-up or bullish CE setup)")
        for s in buys:
            strike = int(s["strike_price"])
            print(f"   {strike:5d} {s['option_type']:2}  {s['confidence']:6}  {s['rationale']}")
        print()

    if sells:
        print("🔴 SELL signals (bearish setups — PE unwinding or CE buying exhaustion)")
        for s in sells:
            strike = int(s["strike_price"])
            print(f"   {strike:5d} {s['option_type']:2}  {s['confidence']:6}  {s['rationale']}")
        print()

    if not buys and not sells:
        print("No strong BUY or SELL signals (only WATCH).")

    print("TRADE BIAS & CONCLUSION")
    print("-" * 55)
    buy_count = len(buys)
    sell_count = len(sells)

    # OI interpretation:
    # BUY = put writing (PE OI build-up) = bullish support
    # SELL = bearish setups (e.g. PE unwinding = exits from protection = bearish)
    if buy_count > sell_count * 1.2:
        print("  Overall: Bullish bias — significant put writing (PE OI build-up).")
        print("  Institutions appear to be selling puts = expecting support at key strikes.")
        print("  Watch for sustained move up or range-bound with support holding.")
    elif sell_count > buy_count * 1.2:
        print("  Overall: Bearish bias — put longs exiting / call side dominant.")
        print("  Watch lower strikes for breakdown; consider hedges on longs.")
    else:
        print("  Overall: Mixed / range-bound — balanced CE and PE activity.")
        print("  Wait for clearer conviction on one side before taking directional trades.")

    print()
    print("Tip: Focus on MEDIUM confidence strikes with largest % OI change.")
    print("     Always use proper risk management — this is analysis, not advice.")
    print("     Re-upload fresh CSV and run 'make view' after new download.")

if __name__ == "__main__":
    main()
