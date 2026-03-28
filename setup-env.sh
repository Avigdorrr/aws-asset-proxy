#!/bin/bash
set -euo pipefail

# Variables are sourced from config.env
if [ -f "config.env" ]; then
    export $(grep -v '^#' config.env | xargs)
else
    echo "Error: config.env file not found."
    exit 1
fi

echo "=== Starting Environment Setup ==="
echo "Region: $AWS_REGION"
echo "ECR Repository: $ECR_REPOSITORY"
echo "Application Stack: $STACK_NAME"
echo "OIDC Stack: $OIDC_STACK_NAME"
echo "GitHub: $GITHUB_ORG/$GITHUB_REPO"
echo "----------------------------------"

# 1. Deploy OIDC Stack
echo "[1/6] Deploying GitHub Actions OIDC Stack..."
aws --region "$AWS_REGION" cloudformation deploy \
  --template-file infra/github-oidc.yaml \
  --stack-name "$OIDC_STACK_NAME" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides GitHubOrg="$GITHUB_ORG" GitHubRepo="$GITHUB_REPO" \
  --no-fail-on-empty-changeset

# Extract the Role ARN
AWS_ROLE_ARN=$(aws cloudformation describe-stacks --region "$AWS_REGION" --stack-name "$OIDC_STACK_NAME" --query "Stacks[0].Outputs[?OutputKey=='GitHubActionsRoleArn'].OutputValue" --output text)
echo "      Output - GitHubActionsRoleArn: $AWS_ROLE_ARN"

# For the very first manual deploy, we use a 'bootstrap' tag with a timestamp
IMAGE_TAG="bootstrap-$(date +%s)"

# 2. Get the URIs dynamically
echo "[2/6] Fetching AWS Account ID..."
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
ECR_URI="$ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
FULL_IMAGE_URI="$ECR_URI/$ECR_REPOSITORY:$IMAGE_TAG"

echo "      Account ID: $ACCOUNT_ID"
echo "      Image URI:  $FULL_IMAGE_URI"

# 3. Ensure ECR exists
echo "[3/6] Ensuring ECR repository '$ECR_REPOSITORY' exists..."

if ! aws ecr describe-repositories --repository-names "$ECR_REPOSITORY" --region "$AWS_REGION" > /dev/null 2>&1; then
    echo "      Repository not found. Creating new repository..."
    aws ecr create-repository --repository-name "$ECR_REPOSITORY" --region "$AWS_REGION" > /dev/null
else
    echo "      Repository already exists."
fi

# 4. Authenticate Docker to ECR
echo "[4/6] Authenticating Docker to ECR..."
aws ecr get-login-password --region "$AWS_REGION" | docker login --username AWS --password-stdin "$ECR_URI"

# 5. Build specifically for the Lambda architecture (ARM64 for Graviton2) and push to ECR
echo "[5/6] Building and pushing bootstrap Docker image for ARM64 architecture..."
docker buildx build --platform linux/arm64 -t "$FULL_IMAGE_URI" ./app
docker push "$FULL_IMAGE_URI"

# 6. Deploy CloudFormation
echo "[6/6] Deploying initial CloudFormation stack '$STACK_NAME'..."
aws --region "$AWS_REGION" cloudformation deploy \
  --template-file infra/app.yaml \
  --stack-name "$STACK_NAME" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ImageUri="$FULL_IMAGE_URI" \
  --no-fail-on-empty-changeset

echo "=== Setup complete! ==="
echo ""
echo "ACTION REQUIRED: Configure GitHub Actions variables"
echo "--------------------------------------------------------"
echo "Please add the following variable to your GitHub Repository ($GITHUB_ORG/$GITHUB_REPO):"
echo "  Name:  AWS_ROLE_ARN"
echo "  Value: $AWS_ROLE_ARN"
echo ""
echo "Future updates to the application should be handled exclusively via GitHub Actions."