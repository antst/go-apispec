#!/usr/bin/env bash
# update-golden-files.sh — Regenerate golden files using the SAME code
# path as the golden comparison tests.
#
# Usage:
#   scripts/update-golden-files.sh

set -euo pipefail

cd "$(dirname "$0")/.."

echo "Updating golden files via TestUpdateGolden..."
echo "(Uses the SAME generateGoldenSpec() function as the comparison tests)"
echo ""

go test ./internal/engine/ -run TestUpdateGolden -v 2>&1 | grep -E "UPDATED|unchanged|PASS|FAIL"

echo ""
echo "Verifying golden tests pass..."
go test ./internal/engine/ -run "TestGolden_All" -v 2>&1 | grep -E "PASS|FAIL" | tail -3

echo ""
echo "Done. Review changes: git diff testdata/"
