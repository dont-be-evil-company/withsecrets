#!/usr/bin/env bash

if [ -z "$VERSION" ]; then echo "Error: VERSION is not set"; exit 1; fi

GH_TAG="v$VERSION"
FILES=()

LINUX_FILES=(
  "dist/ws-linux-amd64"
)

MACOS_FILES=(
  "dist/ws-darwin-arm64"
  "dist/ws-darwin-amd64"
)

WINDOWS_FILES=(
  "dist/ws-windows-amd64.exe"
)

check_files_exist() {
  files=()
  for file in "${FILES[@]}"; do
    if [ ! -f "$file" ]; then
      files+=("$file")
    fi
  done
  if [ ${#files[@]} -gt 0 ]; then
    echo "Error: the following files do not exist:"
    for file in "${files[@]}"; do
      printf " - %s\n" "$file"
    done
    echo "This is the content of the dist directory:"
    ls -l dist/
    exit 1
  fi
}

merge_all_platform_files() {
  FILES=(
    "${LINUX_FILES[@]}"
    "${MACOS_FILES[@]}"
    "${WINDOWS_FILES[@]}"
  )
}

print_files() {
  echo "Files to upload:"
  for file in "${FILES[@]}"; do
    printf " - %s\n" "$file"
  done
}

do_gh_release() {
  echo "Creating new release $GH_TAG"
  print_files
  gh release create --generate-notes "$GH_TAG" "${FILES[@]}"
}

release() {
  merge_all_platform_files
  check_files_exist
  do_gh_release
}

release
