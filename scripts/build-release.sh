#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/build-release.sh [options]

Examples:
  scripts/build-release.sh
  scripts/build-release.sh --target linux/amd64
  scripts/build-release.sh --target linux/arm64 --output-name dr600ab
  scripts/build-release.sh --all

Options:
  --target GOOS/GOARCH  Build one target. Default: current Go host target.
  --all                Build common release packages for Windows, macOS, and Linux.
  --output-name NAME    Binary base name. Default: dr600ab
  --skip-frontend       Reuse backend/internal/webassets/dist instead of rebuilding frontend.
  -h, --help            Show this help.

Environment:
  CGO_ENABLED           Passed to go build when set. Default: Go toolchain default.
  CC                    Passed to go build when set.
  CXX                   Passed to go build when set.
EOF
}

die() {
  echo "$1" >&2
  exit "${2:-1}"
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"
WEBASSETS_DIR="$ROOT_DIR/backend/internal/webassets"
OUTPUT_DIR="$ROOT_DIR/dist"
OUTPUT_NAME="${OUTPUT_NAME:-dr600ab}"
TARGET=""
BUILD_ALL=false
SKIP_FRONTEND=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      if [[ $# -lt 2 || "$2" == -* ]]; then
        die "--target requires GOOS/GOARCH." 2
      fi
      TARGET="$2"
      shift 2
      ;;
    --all)
      BUILD_ALL=true
      shift
      ;;
    --output-name)
      if [[ $# -lt 2 || "$2" == -* ]]; then
        die "--output-name requires NAME." 2
      fi
      OUTPUT_NAME="$2"
      shift 2
      ;;
    --skip-frontend)
      SKIP_FRONTEND=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "$BUILD_ALL" == true && -n "$TARGET" ]]; then
  die "--all and --target cannot be used together." 2
fi

validate_target() {
  local target="$1"
  if [[ "$target" != */* || "$target" == */ || "$target" == /* ]]; then
    die "Invalid target: $target. Expected GOOS/GOARCH, for example linux/amd64." 2
  fi
}

mkdir -p "$OUTPUT_DIR"

build_frontend() {
  if [[ "$SKIP_FRONTEND" == true ]]; then
    if [[ ! -f "$WEBASSETS_DIR/dist/index.html" ]]; then
      die "--skip-frontend requires an existing $WEBASSETS_DIR/dist/index.html. Run without --skip-frontend first." 1
    fi
    echo "Skipping frontend build."
    return
  fi

  echo "Building frontend..."
  (
    cd "$FRONTEND_DIR"
    npm run build
  )

  echo "Embedding frontend assets..."
  rm -rf "$WEBASSETS_DIR/dist"
  mkdir -p "$WEBASSETS_DIR"
  cp -R "$FRONTEND_DIR/dist" "$WEBASSETS_DIR/dist"
  printf 'placeholder for go:embed before release build\n' > "$WEBASSETS_DIR/dist/placeholder.txt"
}

host_target() {
  echo "$(go env GOOS)/$(go env GOARCH)"
}

target_os() {
  local target="$1"
  echo "${target%%/*}"
}

target_arch() {
  local target="$1"
  echo "${target##*/}"
}

binary_name() {
  local goos="$1"
  if [[ "$goos" == "windows" ]]; then
    echo "$OUTPUT_NAME.exe"
    return
  fi
  echo "$OUTPUT_NAME"
}

package_ext() {
  local goos="$1"
  if [[ "$goos" == "windows" ]]; then
    echo "zip"
    return
  fi
  echo "tar.gz"
}

append_go_env_args() {
  local goos="$1"
  local goarch="$2"
  env_args=("GOOS=$goos" "GOARCH=$goarch")
  if [[ -n "${CGO_ENABLED:-}" ]]; then
    env_args+=("CGO_ENABLED=$CGO_ENABLED")
  fi
  if [[ -n "${CC:-}" ]]; then
    env_args+=("CC=$CC")
  fi
  if [[ -n "${CXX:-}" ]]; then
    env_args+=("CXX=$CXX")
  fi
}

build_binary() {
  local target="$1"
  local goos goarch output env_args
  validate_target "$target"
  goos="$(target_os "$target")"
  goarch="$(target_arch "$target")"
  output="$OUTPUT_DIR/$(binary_name "$goos")"

  echo "Building backend binary for $target..."
  (
    cd "$ROOT_DIR"
    append_go_env_args "$goos" "$goarch"
    echo "Go target: ${env_args[*]}"
    env "${env_args[@]}" go build -o "$output" ./backend/cmd/api
  )
  echo "Built $output"
}

build_package() {
  local target="$1"
  local goos goarch package_dir binary package env_args
  validate_target "$target"
  goos="$(target_os "$target")"
  goarch="$(target_arch "$target")"
  package_dir="$OUTPUT_DIR/packages/$OUTPUT_NAME-$goos-$goarch"
  binary="$(binary_name "$goos")"
  package="$OUTPUT_DIR/$OUTPUT_NAME-$goos-$goarch.$(package_ext "$goos")"

  echo "Building package for $target..."
  rm -rf "$package_dir"
  mkdir -p "$package_dir"
  (
    cd "$ROOT_DIR"
    append_go_env_args "$goos" "$goarch"
    echo "Go target: ${env_args[*]}"
    env "${env_args[@]}" go build -o "$package_dir/$binary" ./backend/cmd/api
  )
  cat > "$package_dir/README.txt" <<EOF
DR600AB release package

Target: $goos/$goarch

Run:
  ./$binary

Default URL:
  http://127.0.0.1:18080/#/screen

Linux SSH deployment:
  scripts/deploy-release.sh <user@host> --binary dist/packages/$OUTPUT_NAME-$goos-$goarch/$binary
EOF

  rm -f "$package"
  if [[ "$goos" == "windows" ]]; then
    (
      cd "$OUTPUT_DIR/packages"
      zip -qr "$package" "$OUTPUT_NAME-$goos-$goarch"
    )
  else
    (
      cd "$OUTPUT_DIR/packages"
      tar -czf "$package" "$OUTPUT_NAME-$goos-$goarch"
    )
  fi
  echo "Built $package"
}

build_frontend

if [[ "$BUILD_ALL" == true ]]; then
  targets=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
  )
  for target in "${targets[@]}"; do
    build_package "$target"
  done
else
  build_binary "${TARGET:-$(host_target)}"
fi
