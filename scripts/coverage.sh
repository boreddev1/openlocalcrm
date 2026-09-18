#!/usr/bin/env bash
set -e

# ==============================================================================
# OpenLocalCRM Test Coverage Report Generator
# ==============================================================================

COVERAGE_DIR="coverage"
COVERAGE_OUT="${COVERAGE_DIR}/coverage.out"
COVERAGE_HTML="${COVERAGE_DIR}/coverage.html"

mkdir -p "${COVERAGE_DIR}"

echo "📊 Führe Tests mit Code-Coverage-Analyse aus..."
go test -coverprofile="${COVERAGE_OUT}" -covermode=atomic ./...

echo ""
echo "📈 Gesamtabdeckung nach Paketen:"
go tool cover -func="${COVERAGE_OUT}" | tail -n 15

TOTAL_COV=$(go tool cover -func="${COVERAGE_OUT}" | grep total | awk '{print $3}')
echo ""
echo "🎯 Gesamte Testabdeckung: ${TOTAL_COV}"

if [ "$1" = "--html" ]; then
    go tool cover -html="${COVERAGE_OUT}" -o "${COVERAGE_HTML}"
    echo "📄 HTML-Report erstellt: ${COVERAGE_HTML}"
    if command -v open >/dev/null 2>&1; then
        open "${COVERAGE_HTML}"
    elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "${COVERAGE_HTML}"
    fi
fi
