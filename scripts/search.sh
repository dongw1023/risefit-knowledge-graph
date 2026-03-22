#!/bin/bash

# Configuration
# Default to localhost for testing, or set via environment variable
BASE_URL="${SEARCH_API_URL:-http://localhost:8080}"
QUERY="${1:-muscle recovery protein}"
NUM_RESULTS="${2:-3}"

echo "Searching for: $QUERY"
echo "Target URL: $BASE_URL/search"

# Perform the request and capture the response and HTTP code
# We use a temp file to safely separate the body and the status code
TEMP_FILE=$(mktemp)
HTTP_CODE=$(curl -s -o "$TEMP_FILE" -w "%{http_code}" -X POST "$BASE_URL/search" \
     -H "Content-Type: application/json" \
     -d "{
           \"query\": \"$QUERY\",
           \"num_results\": $NUM_RESULTS
         }")

BODY=$(cat "$TEMP_FILE")
rm "$TEMP_FILE"

if [ "$HTTP_CODE" -eq 200 ]; then
    echo "$BODY" | jq .
else
    echo "Error: Server returned HTTP $HTTP_CODE"
    echo "Response body:"
    echo "$BODY"
    
    if [ "$HTTP_CODE" -eq 404 ] && [[ "$BASE_URL" == *".run.app" ]]; then
        echo -e "\nTip: The staging service is currently set to 'Internal Ingress'. It cannot be reached directly from the public internet."
        echo "Try running it locally with './scripts/run.sh server' or check if you need to use a proxy/VPN."
    fi
fi
