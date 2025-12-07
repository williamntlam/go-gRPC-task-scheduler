#!/bin/bash

# Script to run tests and display a summary with counts

# Run tests and capture output
TEST_OUTPUT=$(go test ./tests/... -v 2>&1)
TEST_EXIT_CODE=$?

# Display the test output
echo "$TEST_OUTPUT"

# Count test results (main test functions, not subtests)
# Go test output format:
# --- PASS: TestName (0.01s)        <- main test
#     --- PASS: TestName/subtest     <- subtest (indented)
# --- FAIL: TestName (0.01s)
# --- SKIP: TestName (0.01s)
PASSED=$(echo "$TEST_OUTPUT" | grep -E "^--- PASS:" | wc -l | tr -d ' ')
FAILED=$(echo "$TEST_OUTPUT" | grep -E "^--- FAIL:" | wc -l | tr -d ' ')
SKIPPED=$(echo "$TEST_OUTPUT" | grep -E "^--- SKIP:" | wc -l | tr -d ' ')

# Handle empty counts
PASSED=${PASSED:-0}
FAILED=${FAILED:-0}
SKIPPED=${SKIPPED:-0}

# Calculate totals
TOTAL_RUN=$((PASSED + FAILED + SKIPPED))

# Determine success/failure status
if [ "$TEST_EXIT_CODE" -eq 0 ] && [ "$FAILED" -eq 0 ]; then
    STATUS_EMOJI="✅"
    STATUS_TEXT="SUCCESS"
else
    STATUS_EMOJI="❌"
    STATUS_TEXT="FAILED"
fi

# Display summary
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 TEST SUMMARY"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
printf "  ✅ Passed:  %3d\n" "$PASSED"
printf "  ❌ Failed:  %3d\n" "$FAILED"
if [ "$SKIPPED" -gt 0 ]; then
    printf "  ⏭️  Skipped: %3d\n" "$SKIPPED"
fi
echo "  ─────────────────────"
printf "  📝 Total:   %3d\n" "$TOTAL_RUN"
echo ""
printf "  %s Status: %s\n" "$STATUS_EMOJI" "$STATUS_TEXT"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Exit with the test exit code
exit $TEST_EXIT_CODE
