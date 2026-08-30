#!/bin/bash
# Deployment script for TestApp
# Last deployed: 2025-12-15

set -e

APP_NAME="testapp"
DOCKER_REGISTRY="registry.example.com"
VERSION=$(cat config.json | python3 -c "import sys,json; print(json.load(sys.stdin)['version'])")

echo "=== Deploying $APP_NAME v$VERSION ==="

# Build
echo "[1/4] Building Docker image..."
docker build -t $DOCKER_REGISTRY/$APP_NAME:$VERSION .
docker build -t $DOCKER_REGISTRY/$APP_NAME:latest .

# Push
echo "[2/4] Pushing to registry..."
docker push $DOCKER_REGISTRY/$APP_NAME:$VERSION
docker push $DOCKER_REGISTRY/$APP_NAME:latest

# Deploy
echo "[3/4] Deploying to Kubernetes..."
kubectl set image deployment/$APP_NAME $APP_NAME=$DOCKER_REGISTRY/$APP_NAME:$VERSION
kubectl rollout status deployment/$APP_NAME --timeout=120s

# Verify
echo "[4/4] Running health check..."
sleep 5
HEALTH=$(curl -s http://localhost:8080/health | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")
if [ "$HEALTH" != "ok" ]; then
    echo "ERROR: Health check failed! Rolling back..."
    kubectl rollout undo deployment/$APP_NAME
    exit 1
fi

echo "=== Deployment successful ==="
