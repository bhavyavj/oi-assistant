#!/usr/bin/env python3
"""Generates a realistic sample_options.xlsx for testing the OI Assistant."""
import random
import openpyxl
from pathlib import Path

symbols = ["NIFTY", "BANKNIFTY", "RELIANCE", "TCS"]
option_types = ["CE", "PE"]
expiries = ["27-Jun-2024", "25-Jul-2024"]

rows = [["Symbol", "Strike Price", "Option Type", "Expiry", "OI", "Prev OI", "Volume", "LTP"]]

for symbol in symbols:
    base_strike = {"NIFTY": 22000, "BANKNIFTY": 48000, "RELIANCE": 2900, "TCS": 3800}[symbol]
    for i in range(-5, 6):
        for ot in option_types:
            strike = base_strike + i * 100
            prev_oi = random.randint(50000, 5000000)
            # Make some rows have significant OI change (>20%) to trigger analysis
            change_factor = random.choice([0.5, 0.8, 1.0, 1.3, 1.6, 2.0, 0.3])
            oi = int(prev_oi * change_factor)
            rows.append([
                symbol,
                strike,
                ot,
                random.choice(expiries),
                oi,
                prev_oi,
                random.randint(1000, 500000),
                round(random.uniform(10, 500), 2),
            ])

wb = openpyxl.Workbook()
ws = wb.active
ws.title = "Options OI Data"
for row in rows:
    ws.append(row)

out = Path(__file__).parent / "sample_options.xlsx"
wb.save(out)
print(f"Generated {len(rows)-1} rows → {out}")
