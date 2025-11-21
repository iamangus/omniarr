#!/bin/bash

# Check if a query was provided
if [ -z "$1" ]; then
    echo "Usage: $0 <series_name>"
    exit 1
fi

QUERY="$1"
API_URL="http://localhost:8080"

# 1. Lookup the series
echo "Searching for '$QUERY'..."
LOOKUP_RESPONSE=$(curl -s -G --data-urlencode "query=$QUERY" "$API_URL/catalog/lookup")

# Check if we got results
COUNT=$(echo "$LOOKUP_RESPONSE" | jq '. | length')
if [ "$COUNT" -eq "0" ]; then
    echo "No results found."
    exit 0
fi

# Display the first result
echo "Found $COUNT results. Selecting the first one:"
FIRST_RESULT=$(echo "$LOOKUP_RESPONSE" | jq '.[0]')
TITLE=$(echo "$FIRST_RESULT" | jq -r '.Title')
YEAR=$(echo "$FIRST_RESULT" | jq -r '.Year')
echo "Title: $TITLE"
echo "Year: $YEAR"

# 2. Add to library
echo "Adding to library..."
# Construct the payload using the metadata from the lookup
# We need to wrap it in the expected structure for POST /entities
PAYLOAD=$(echo "$FIRST_RESULT" | jq '{entity_type: "series", metadata: .}')

RESPONSE=$(curl -s -X POST -H "Content-Type: application/json" -d "$PAYLOAD" "$API_URL/entities")

# Check if successful
UUID=$(echo "$RESPONSE" | jq -r '.uuid')
if [ "$UUID" != "null" ]; then
    echo "Successfully added '$TITLE' to library!"
    echo "Entity UUID: $UUID"
    echo "Monitoring started."
else
    echo "Failed to add series."
    echo "Response: $RESPONSE"
fi