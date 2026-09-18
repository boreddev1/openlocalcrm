#!/usr/bin/env bash
set -e

# ==============================================================================
# OpenLocalCRM Test Coverage Report & Quality Enforcement
# ==============================================================================

COVERAGE_DIR="coverage"
COVERAGE_OUT="${COVERAGE_DIR}/coverage.out"
COVERAGE_HTML="${COVERAGE_DIR}/coverage.html"

MIN_COVERAGE=""
DIFF_MIN_COVERAGE=""
CHECK_DIFF=false
OPEN_HTML=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --html)
            OPEN_HTML=true
            shift
            ;;
        --min|--min-coverage)
            MIN_COVERAGE="$2"
            shift 2
            ;;
        --diff)
            CHECK_DIFF=true
            if [[ -n "$2" && "$2" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
                DIFF_MIN_COVERAGE="$2"
                shift 2
            else
                DIFF_MIN_COVERAGE="50.0"
                shift
            fi
            ;;
        *)
            shift
            ;;
    esac
done

mkdir -p "${COVERAGE_DIR}"

echo "📊 Führe Tests mit Code-Coverage-Analyse (inkl. Cross-Package-Tracking) aus..."
go test -coverpkg=./... -coverprofile="${COVERAGE_OUT}" -covermode=atomic ./...

echo ""
echo "📈 Gesamtabdeckung nach Paketen:"
go tool cover -func="${COVERAGE_OUT}" | tail -n 20

TOTAL_COV=$(go tool cover -func="${COVERAGE_OUT}" | grep total | awk '{print $3}')
echo ""
echo "🎯 Gesamte Testabdeckung: ${TOTAL_COV}"

# HTML Export if requested
if [ "$OPEN_HTML" = true ]; then
    go tool cover -html="${COVERAGE_OUT}" -o "${COVERAGE_HTML}"
    echo "📄 HTML-Report erstellt: ${COVERAGE_HTML}"
    if command -v open >/dev/null 2>&1; then
        open "${COVERAGE_HTML}"
    elif command -v xdg-open >/dev/null 2>&1; then
        xdg-open "${COVERAGE_HTML}"
    fi
fi

# Optional: Check Diff Coverage for newly added code lines
if [ "$CHECK_DIFF" = true ]; then
    echo ""
    echo "🔍 Prüfe Diff-Coverage auf neuem/geändertem Code (Mindestens ${DIFF_MIN_COVERAGE}%)..."
    go run ./scripts/diff-coverage.go -coverprofile="${COVERAGE_OUT}" -min="${DIFF_MIN_COVERAGE}"
fi

# Optional: Check Global Minimum Statement Coverage
if [ -n "$MIN_COVERAGE" ]; then
    TOTAL_NUM=$(echo "${TOTAL_COV}" | tr -d '%')
    MEETS_MIN=$(awk -v total="$TOTAL_NUM" -v min="$MIN_COVERAGE" 'BEGIN { if (total >= min) print "true"; else print "false" }')
    if [ "$MEETS_MIN" != "true" ]; then
        echo ""
        echo -e "\033[0;31m❌ FEHLER: Gesamte Testabdeckung von ${TOTAL_COV} liegt unter der Mindestanforderung von ${MIN_COVERAGE}%!\033[0m"
        exit 1
    else
        echo ""
        echo -e "\033[0;32m✅ Mindest-Testabdeckung von ${MIN_COVERAGE}% erfüllt (${TOTAL_COV})!\033[0m"
    fi
fi
