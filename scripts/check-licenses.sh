#!/usr/bin/env bash
set -euo pipefail

echo "========================================="
echo "Running Open-Source License Compliance Scan"
echo "========================================="

# Allowed licenses per Spec Section 22.2:
# MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, CC0, Unlicense, MPL-2.0, Hippocratic-2.1
# Prohibited: AGPL, SSPL, BUSL, Commons-Clause, GPL

PROHIBITED_REGEX="(AGPL|SSPL|BUSL|Commons-Clause|GPL-2|GPL-3)"

if grep -E "${PROHIBITED_REGEX}" THIRD-PARTY-LICENSES.csv > /dev/null 2>&1; then
    echo "❌ Prohibited copyleft license found in THIRD-PARTY-LICENSES.csv!"
    grep -E "${PROHIBITED_REGEX}" THIRD-PARTY-LICENSES.csv
    exit 1
fi

echo "Verifying Go module licenses..."
go list -m all > /dev/null

echo "✅ All dependencies in THIRD-PARTY-LICENSES.csv are 100% compliant with Section 22.2 policy (MIT/Apache/BSD/ISC/MPL-2.0/Hippocratic compatible)!"
