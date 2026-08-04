#!/usr/bin/env bash
# version.sh - Versioning & Tag Release Helper

set -e

TYPE=${1:-patch}
MESSAGE=${2:-""}

if [ -n "$(git status --porcelain)" ]; then
  echo "⚠️ Working tree has uncommitted changes. Please commit or stash them first."
  git status -s
  exit 1
fi

LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
echo "📌 Current Latest Tag: $LATEST_TAG"

CLEAN_TAG="${LATEST_TAG#v}"
IFS='.' read -r MAJOR MINOR PATCH <<< "$CLEAN_TAG"

MAJOR=${MAJOR:-0}
MINOR=${MINOR:-0}
PATCH=${PATCH:-0}

case "$TYPE" in
  patch)
    PATCH=$((PATCH + 1))
    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
    ;;
  minor)
    MINOR=$((MINOR + 1))
    PATCH=0
    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
    ;;
  major)
    MAJOR=$((MAJOR + 1))
    MINOR=0
    PATCH=0
    NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"
    ;;
  v*|*)
    if [[ "$TYPE" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]]; then
      if [[ "$TYPE" == v* ]]; then
        NEW_TAG="$TYPE"
      else
        NEW_TAG="v$TYPE"
      fi
    else
      echo "❌ Invalid version type: '$TYPE'. Use: patch, minor, major, or a version string like v1.1.0"
      exit 1
    fi
    ;;
esac

if [ -z "$MESSAGE" ]; then
  MESSAGE="Release $NEW_TAG"
fi

echo "🚀 Bumping version: $LATEST_TAG ➡️ $NEW_TAG"

git tag -a "$NEW_TAG" -m "$MESSAGE"
echo "✅ Created Git Tag: $NEW_TAG"
echo "💡 Push to GitHub to trigger automatic release build:"
echo "   git push origin $NEW_TAG"
