#!/bin/bash
set -euo pipefail

# Variables - Change these to match your setup
REGION="eu-central-1"
REPO_NAME="asset-proxy"
STACK_NAME="asset-proxy-stack"

echo "=== Starting Environment Setup ==="
echo "Region: $REGION"
echo "Repository: $REPO_NAME"
echo "Stack: $STACK_NAME"
echo "----------------------------------"

# For the very first manual deploy, we use a 'bootstrap' tag
IMAGE_TAG="bootstrap-$(date +%s)"

# 1. Get the URIs dynamically
echo "[1/5] Fetching AWS Account ID..."
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR_URI="$ACCOUNT_ID.dkr.ecr.$REGION.amazonaws.com"
FULL_IMAGE_URI="$ECR_URI/$REPO_NAME:$IMAGE_TAG"

echo "      Account ID: $ACCOUNT_ID"
echo "      Image URI:  $FULL_IMAGE_URI"

# 2. Ensure ECR exists
echo "[2/5] Ensuring ECR repository '$REPO_NAME' exists..."
if ! aws ecr describe-repositories --repository-names "$REPO_NAME" --region "$REGION" > /dev/null 2>&1; then
    echo "      Repository not found. Creating new repository..."
    aws ecr create-repository --repository-name "$REPO_NAME" --region "$REGION" > /dev/null
else
    echo "      Repository already exists."
fi

# 3. Authenticate Docker to ECR
echo "[3/5] Authenticating Docker to ECR..."
aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$ECR_URI"

# 4. Build specifically for the Lambda architecture (ARM64 for Graviton2) and push to ECR
echo "[4/5] Building and pushing Docker image for ARM64 architecture..."
docker build --platform linux/arm64 -t "$FULL_IMAGE_URI" ./app
docker push "$FULL_IMAGE_URI"

# 5. Deploy CloudFormation
echo "[5/5] Deploying CloudFormation stack '$STACK_NAME'..."
aws --region "$REGION" cloudformation deploy \
  --template-file infra/template.yaml \
  --stack-name $STACK_NAME \
  --capabilities CAPABILITY_IAM \
  --parameter-overrides ImageUri=$FULL_IMAGE_URI

echo "=== Setup complete! ==="
echo "Future updates should be handled via GitHub Actions."