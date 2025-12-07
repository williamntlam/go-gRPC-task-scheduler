#!/bin/bash

# Script to run tests and display a summary with counts

# Function to update progress bar
update_progress() {
    local current=$1
    local total=$2
    local width=50
    local percentage=$((current * 100 / total))
    local filled=$((current * width / total))
    local empty=$((width - filled))
    
    # Build progress bar
    local bar=""
    for ((i=0; i<filled; i++)); do
        bar="${bar}█"
    done
    for ((i=0; i<empty; i++)); do
        bar="${bar}░"
    done
    
    # Print progress bar (overwrite same line)
    printf "\r  Progress: [%s] %3d%% (%d/%d tests)" "$bar" "$percentage" "$current" "$total"
}

# Count total number of test functions first (for progress bar)
echo "🔍 Discovering tests..."
TOTAL_TESTS=$(go test ./tests/... -list . 2>/dev/null | grep -E "^Test" | wc -l | tr -d ' ')
TOTAL_TESTS=${TOTAL_TESTS:-1}  # Default to 1 if count fails

if [ "$TOTAL_TESTS" -eq 0 ]; then
    TOTAL_TESTS=1
fi

echo "📊 Found $TOTAL_TESTS test functions"
echo "🧪 Running tests..."
echo ""

# Use a temporary file to track progress
PROGRESS_FILE=$(mktemp)
trap "rm -f $PROGRESS_FILE" EXIT
echo "0" > "$PROGRESS_FILE"

# Use a temporary file to capture output
TEMP_OUTPUT=$(mktemp)
trap "rm -f $TEMP_OUTPUT $PROGRESS_FILE" EXIT

# Run tests and process output line by line
go test ./tests/... -v 2>&1 | tee "$TEMP_OUTPUT" | while IFS= read -r line; do
    # Echo the line to show test output
    echo "$line"
    
    # Check if this is a test completion line (PASS, FAIL, or SKIP)
    if echo "$line" | grep -qE "^--- (PASS|FAIL|SKIP):"; then
        # Increment counter in file (to work around subshell limitation)
        CURRENT=$(cat "$PROGRESS_FILE")
        echo $((CURRENT + 1)) > "$PROGRESS_FILE"
        update_progress $((CURRENT + 1)) "$TOTAL_TESTS"
    fi
done

TEST_EXIT_CODE=${PIPESTATUS[0]}

# Read the full output from temp file
TEST_OUTPUT=$(cat "$TEMP_OUTPUT")
rm -f "$TEMP_OUTPUT" "$PROGRESS_FILE"

# Clear the progress line and add newline
printf "\n\n"

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

# Extract failed test names (main tests only, not subtests)
# Format: --- FAIL: TestName (0.01s)
FAILED_TEST_NAMES=$(echo "$TEST_OUTPUT" | grep -E "^--- FAIL:" | sed -E 's/^--- FAIL: ([^ ]+).*/\1/')

# Extract error details for each failed test
# Go test output format:
# --- FAIL: TestName (0.01s)
#     test_file.go:123: Error message
#     Expected: ...
#     Actual: ...
extract_failed_test_errors() {
    local test_name="$1"
    # Find the line number of the FAIL line for this test
    local fail_line_num=$(echo "$TEST_OUTPUT" | grep -n "^--- FAIL: $test_name " | head -1 | cut -d: -f1)
    
    if [ -z "$fail_line_num" ]; then
        return
    fi
    
    # Extract lines starting from the line after FAIL until we hit another test boundary
    # Look for next "---" (PASS/FAIL/SKIP) or "===" (RUN) lines
    echo "$TEST_OUTPUT" | awk -v start_line="$fail_line_num" '
        NR > start_line {
            # Stop if we hit another test result or new test run
            if (/^--- (PASS|FAIL|SKIP):/ || /^=== RUN/) {
                exit
            }
            # Stop if we hit the summary section
            if (/^FAIL/ || /^ok/ || /^PASS/) {
                exit
            }
            print
        }
    ' | head -15
}

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

# Show progress bar in summary
if [ "$TOTAL_RUN" -gt 0 ] && [ "$TOTAL_TESTS" -gt 0 ]; then
    width=50
    percentage=$((TOTAL_RUN * 100 / TOTAL_TESTS))
    filled=$((TOTAL_RUN * width / TOTAL_TESTS))
    empty=$((width - filled))
    
    # Build progress bar
    bar=""
    for ((i=0; i<filled; i++)); do
        bar="${bar}█"
    done
    for ((i=0; i<empty; i++)); do
        bar="${bar}░"
    done
    
    printf "  Progress: [%s] %3d%% (%d/%d tests completed)\n" "$bar" "$percentage" "$TOTAL_RUN" "$TOTAL_TESTS"
    echo ""
fi

printf "  ✅ Passed:  %3d\n" "$PASSED"
printf "  ❌ Failed:  %3d\n" "$FAILED"
if [ "$SKIPPED" -gt 0 ]; then
    printf "  ⏭️  Skipped: %3d\n" "$SKIPPED"
fi
echo "  ─────────────────────"
printf "  📝 Total:   %3d\n" "$TOTAL_RUN"
echo ""

# Display failed test cases if any
if [ "$FAILED" -gt 0 ] && [ -n "$FAILED_TEST_NAMES" ]; then
    echo "  ❌ Failed Test Cases:"
    # Use a here-string to avoid subshell issues
    while IFS= read -r test_name; do
        if [ -n "$test_name" ]; then
            printf "     • %s\n" "$test_name"
            # Extract and display error details for this test
            ERROR_DETAILS=$(extract_failed_test_errors "$test_name")
            if [ -n "$ERROR_DETAILS" ]; then
                # Indent the error details
                echo "$ERROR_DETAILS" | sed 's/^/        /' | head -15
                # If there are more than 15 lines, indicate truncation
                LINE_COUNT=$(echo "$ERROR_DETAILS" | wc -l | tr -d ' ')
                if [ "$LINE_COUNT" -gt 15 ]; then
                    echo "        ... (output truncated, see full test output above)"
                fi
                echo ""
            fi
        fi
    done <<< "$FAILED_TEST_NAMES"
    echo ""
fi

printf "  %s Status: %s\n" "$STATUS_EMOJI" "$STATUS_TEXT"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Exit with the test exit code
exit $TEST_EXIT_CODE
