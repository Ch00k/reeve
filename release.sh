#!/bin/bash
set -e

# Tags are vX.Y.Z: Go takes the binary's version, and `go install` the
# module's, from them, and both recognise only v-prefixed semver tags.

if [ "$(git branch --show-current)" != main ]; then
    echo "Error: releases are tagged on main" >&2
    exit 1
fi

if ! git diff-index --quiet HEAD --; then
    echo "Error: You have uncommitted changes. Please commit or stash them first." >&2
    exit 1
fi

echo "Fetching latest remote tags and changes..."
git fetch --tags origin
git pull --ff-only origin main

LATEST_TAG=$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || echo "v0.0.0")
echo "Latest tag: $LATEST_TAG"

IFS='.' read -r MAJOR MINOR PATCH <<<"${LATEST_TAG#v}"

case ${1:-patch} in
major)
    NEW_VERSION="v$((MAJOR + 1)).0.0"
    ;;
minor)
    NEW_VERSION="v$MAJOR.$((MINOR + 1)).0"
    ;;
patch)
    NEW_VERSION="v$MAJOR.$MINOR.$((PATCH + 1))"
    ;;
*)
    if [[ $1 =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        NEW_VERSION="$1"
    else
        echo "Usage: $0 [major|minor|patch|v1.2.3]"
        exit 1
    fi
    ;;
esac

read -p "Create release $NEW_VERSION? [y/N] " -r
if [[ $REPLY =~ ^[Yy]$ ]]; then
    git tag -a "$NEW_VERSION" -m "Release $NEW_VERSION"
    git push origin main "$NEW_VERSION"
    echo "Release $NEW_VERSION tagged; GitHub Actions builds and publishes it."
else
    echo "Release cancelled"
fi
