#!/bin/bash

# Build script that embeds .env content into the binary
# Usage: ./build.sh [output_name] [target_cmd]
#
# Examples:
#   ./build.sh                           # Build o2stock-crawler
#   ./build.sh o2stock-api api           # Build o2stock-api
#   ./build.sh myapp crawler             # Build crawler as myapp


# --- 加载系统和用户的环境变量 ---
source /etc/profile
source ~/.bashrc
# ---------------------------------------

set -e

OUTPUT_NAME="${1:-o2stock-crawler-ol2}"
TARGET_CMD="${2:-o2stock-crawler-ol2}"

ENV_FILE=".env"
if [ ! -f "$ENV_FILE" ]; then
    echo "Warning: .env file not found, building without embedded config"
    ENV_CONTENT=""
else
    # Read .env content and escape for ldflags
    ENV_CONTENT=$(cat "$ENV_FILE" | tr '\n' '\\' | sed 's/\\/\\n/g' | sed 's/\\n$//')
fi

# Build with embedded env
echo "Building ${OUTPUT_NAME} from cmd/${TARGET_CMD}..."
go build -ldflags "-X 'o2stock-crawler/internal/config.EmbeddedEnv=${ENV_CONTENT}'" \
    -o "${OUTPUT_NAME}" \
    "./cmd/${TARGET_CMD}"

echo "Build complete: ${OUTPUT_NAME}"
echo "The binary now contains embedded configuration from .env"
