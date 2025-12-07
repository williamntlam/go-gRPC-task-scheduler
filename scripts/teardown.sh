#!/bin/bash

# Teardown script - wrapper for shutdown.sh
# This script is called by the Makefile for consistency

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"$SCRIPT_DIR/shutdown.sh" "$@"
