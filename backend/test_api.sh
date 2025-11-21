#!/bin/bash

# Build the application
echo "Building application..."
go build -o omniarr cmd/omniarr/main.go
if [ $? -ne 0 ]; then
    echo "Build failed"
    exit 1
fi

# Start the application in MOCK_MODE
echo "Starting application in MOCK_MODE..."
export MOCK_MODE=true
export PORT=8081
./omniarr &
PID=$!

# Wait for server to start
echo "Waiting for server to start..."
sleep 2

# Function to check if a command succeeded
check_status() {
    if [ $1 -ne 0 ]; then
        echo "Error: $2 failed"
        kill $PID
        exit 1
    fi
}

# 1. Get System Config
echo "Testing GET /system/config..."
curl -s http://localhost:8081/system/config | jq .
check_status $? "GET /system/config"

# 2. Lookup Catalog
echo "Testing GET /catalog/lookup..."
curl -s "http://localhost:8081/catalog/lookup?query=Breaking%20Bad" | jq .
check_status $? "GET /catalog/lookup"

# 3. Create Entity
echo "Testing POST /entities..."
RESPONSE=$(curl -s -X POST -H "Content-Type: application/json" -d '{"entity_type": "series", "metadata": {"title": "Breaking Bad"}}' http://localhost:8081/entities)
echo $RESPONSE | jq .
UUID=$(echo $RESPONSE | jq -r .uuid)

if [ "$UUID" == "null" ] || [ -z "$UUID" ]; then
    echo "Error: Failed to get UUID from create entity response"
    kill $PID
    exit 1
fi
echo "Created Entity UUID: $UUID"

# 4. Force Search
echo "Testing POST /acquisition/search/$UUID..."
curl -s -X POST http://localhost:8081/acquisition/search/$UUID | jq .
check_status $? "POST /acquisition/search/$UUID"

# 5. Delete Entity
echo "Testing DELETE /entities/$UUID..."
curl -s -X DELETE http://localhost:8081/entities/$UUID
check_status $? "DELETE /entities/$UUID"

# Cleanup
echo "Stopping server..."
kill $PID
echo "Test completed successfully."