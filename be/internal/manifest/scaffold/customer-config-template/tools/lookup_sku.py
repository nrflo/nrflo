#!/usr/bin/env python3
"""Look up product details by SKU. Reads JSON from stdin, writes JSON to stdout."""
import json
import sys


def main():
    data = json.load(sys.stdin)
    sku = data["sku"]

    # Sample product catalog — replace with your actual data source.
    catalog = {
        "SKU-001": {"name": "Widget A", "price": 9.99, "in_stock": True},
        "SKU-002": {"name": "Widget B", "price": 19.99, "in_stock": False},
        "SKU-003": {"name": "Widget C", "price": 4.99, "in_stock": True},
    }

    if sku not in catalog:
        print(json.dumps({"error": f"SKU not found: {sku}"}))
        sys.exit(1)

    print(json.dumps(catalog[sku]))


if __name__ == "__main__":
    main()
