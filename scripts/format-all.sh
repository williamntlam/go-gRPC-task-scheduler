#!/bin/bash

# ============================================================================
# Format All Files Script
# ============================================================================
# This script formats all files in the project:
# - Go files: uses gofmt
# - YAML/JSON/Markdown: uses Prettier
# 
# Usage:
#   ./scripts/format-all.sh
#   ./scripts/format-all.sh --check  (check only, don't modify)
# ============================================================================

set -e

CHECK_ONLY=false
if [ "$1" == "--check" ]; then
    CHECK_ONLY=true
fi

echo "=========================================="
echo "Formatting All Files"
echo "=========================================="
echo ""

# Check if Prettier is available
if ! command -v prettier &> /dev/null && ! command -v npx &> /dev/null; then
    echo "⚠️  Prettier not found. Installing..."
    if command -v npm &> /dev/null; then
        npm install
    else
        echo "❌ npm not found. Please install Node.js and npm first."
        echo "   Or install Prettier globally: npm install -g prettier"
        exit 1
    fi
fi

# Format Go files
echo "📝 Formatting Go files..."
if [ "$CHECK_ONLY" = true ]; then
    if [ "$(gofmt -s -l . | wc -l)" -gt 0 ]; then
        echo "❌ Go files are not formatted. Run 'gofmt -s -w .'"
        gofmt -s -d . | head -20
        exit 1
    else
        echo "✅ Go files are properly formatted"
    fi
else
    gofmt -s -w .
    echo "✅ Go files formatted"
fi
echo ""

# Format YAML, JSON, and Markdown files with Prettier
echo "📝 Formatting YAML, JSON, and Markdown files with Prettier..."

PRETTIER_CMD="prettier"
if ! command -v prettier &> /dev/null; then
    PRETTIER_CMD="npx prettier"
fi

if [ "$CHECK_ONLY" = true ]; then
    if $PRETTIER_CMD --check "**/*.{yml,yaml,json,md}" 2>/dev/null; then
        echo "✅ YAML/JSON/Markdown files are properly formatted"
    else
        echo "❌ Some files need formatting. Run './scripts/format-all.sh' to fix"
        exit 1
    fi
else
    $PRETTIER_CMD --write "**/*.{yml,yaml,json,md}" 2>/dev/null || true
    echo "✅ YAML/JSON/Markdown files formatted"
fi
echo ""

echo "=========================================="
if [ "$CHECK_ONLY" = true ]; then
    echo "✅ Format check complete - all files are properly formatted!"
else
    echo "✅ All files formatted successfully!"
fi
echo "=========================================="
