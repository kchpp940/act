#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")"/.. && pwd)"
RELEASE_JSON="$REPO_ROOT/release.json"
GEN_SCRIPT="$REPO_ROOT/scripts/release_gen.sh"

SCENARIO="${SCENARIO:-release}"
CHECK_ALL_SCENARIOS="${CHECK_ALL_SCENARIOS:-false}"

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required" >&2
  exit 1
fi

check_goreleaser() {
  local scenario_to_check="${1:-$SCENARIO}"
  echo "=== Checking .goreleaser.yml (scenario: ${scenario_to_check}) ==="
  local generated
  generated=$(SCENARIO="$scenario_to_check" "$GEN_SCRIPT" goreleaser /dev/stdout)
  local actual
  actual=$(cat "$REPO_ROOT/.goreleaser.yml")
  if [ "$generated" = "$actual" ]; then
    echo "  PASSED: .goreleaser.yml matches release.json"
  else
    echo "  FAILED: .goreleaser.yml differs from release.json"
    diff <(echo "$actual") <(echo "$generated") || true
    return 1
  fi
}

check_install_sh_platforms() {
  echo "=== Checking install.sh get_binaries() ==="
  local generated
  generated=$("$GEN_SCRIPT" install-platforms)
  local actual
  actual=$(sed -n '/^get_binaries()/,/^}/p' "$REPO_ROOT/install.sh")
  if [ "$generated" = "$actual" ]; then
    echo "  PASSED: get_binaries() matches release.json"
  else
    echo "  FAILED: get_binaries() differs from release.json"
    diff <(echo "$actual") <(echo "$generated") || true
    return 1
  fi
}

check_install_sh_adjust_os() {
  echo "=== Checking install.sh adjust_os() ==="
  local generated
  generated=$("$GEN_SCRIPT" install-adjust-os)
  local actual
  actual=$(sed -n '/^adjust_os()/,/^}/p' "$REPO_ROOT/install.sh")
  if [ "$generated" = "$actual" ]; then
    echo "  PASSED: adjust_os() matches release.json"
  else
    echo "  FAILED: adjust_os() differs from release.json"
    diff <(echo "$actual") <(echo "$generated") || true
    return 1
  fi
}

check_install_sh_adjust_arch() {
  echo "=== Checking install.sh adjust_arch() ==="
  local generated
  generated=$("$GEN_SCRIPT" install-adjust-arch)
  local actual
  actual=$(sed -n '/^adjust_arch()/,/^}/p' "$REPO_ROOT/install.sh")
  if [ "$generated" = "$actual" ]; then
    echo "  PASSED: adjust_arch() matches release.json"
  else
    echo "  FAILED: adjust_arch() differs from release.json"
    diff <(echo "$actual") <(echo "$generated") || true
    return 1
  fi
}

check_install_sh_format() {
  echo "=== Checking install.sh adjust_format() ==="
  local generated
  generated=$("$GEN_SCRIPT" install-adjust-format)
  local actual
  actual=$(sed -n '/^adjust_format()/,/^}/p' "$REPO_ROOT/install.sh")
  if [ "$generated" = "$actual" ]; then
    echo "  PASSED: adjust_format() matches release.json"
  else
    echo "  FAILED: adjust_format() differs from release.json"
    diff <(echo "$actual") <(echo "$generated") || true
    return 1
  fi
}

check_readme() {
  echo "=== Checking README.md auto-generated section ==="
  local generated
  generated=$("$GEN_SCRIPT" readme-snippet /dev/stdout)
  local actual
  actual=$(sed -n '/AUTO-GENERATED from release.json/,/END AUTO-GENERATED/p' "$REPO_ROOT/README.md")
  if [ -z "$actual" ]; then
    echo "  FAILED: No AUTO-GENERATED section found in README.md"
    return 1
  fi
  if [ "$generated" = "$actual" ]; then
    echo "  PASSED: README.md auto-generated section matches release.json"
  else
    echo "  FAILED: README.md auto-generated section differs from release.json"
    diff <(echo "$actual") <(echo "$generated") || true
    return 1
  fi
}

check_makefile() {
  echo "=== Checking Makefile ==="
  local pass=true
  local version_file
  version_file=$(jq -r '.version_file' "$RELEASE_JSON")

  if grep -qE "VERSION.*cat.*${version_file}" "$REPO_ROOT/Makefile"; then
    echo "  OK VERSION reads from $version_file"
  else
    echo "  MISMATCH VERSION does not read from $version_file"
    pass=false
  fi

  if grep -qE 'LDFLAGS.*(RELEASE_JSON|release\.json)' "$REPO_ROOT/Makefile"; then
    echo "  OK LDFLAGS references release.json"
  else
    echo "  MISMATCH LDFLAGS does not reference release.json"
    pass=false
  fi

  if $pass; then echo "  PASSED"; else echo "  FAILED"; fi
  $pass
}

check_docker_tags() {
  local scenario_to_check="${1:-$SCENARIO}"
  echo "=== Checking Docker configuration (scenario: ${scenario_to_check}) ==="
  local docker_enabled
  docker_enabled=$(jq -r '.docker.enabled // "false"' "$RELEASE_JSON")
  if [ "$docker_enabled" != "true" ]; then
    echo "  SKIPPED: Docker not enabled in release.json"
    return 0
  fi

  local scenario_path
  scenario_path=".docker.scenarios.\"${scenario_to_check}\""
  if ! jq -e "${scenario_path}" "$RELEASE_JSON" > /dev/null 2>&1; then
    echo "  FAILED: Scenario '${scenario_to_check}' not defined in release.json"
    return 1
  fi

  local generated
  generated=$(SCENARIO="$scenario_to_check" "$GEN_SCRIPT" goreleaser /dev/stdout)

  local scenario_desc
  scenario_desc=$(jq -r "${scenario_path}.description // \"\"" "$RELEASE_JSON")
  echo "  Scenario: ${scenario_desc}"

  local push skip_push update_latest
  push=$(jq -r "${scenario_path}.push // \"false\"" "$RELEASE_JSON")
  skip_push=$(jq -r "${scenario_path}.skip_push // \"false\"" "$RELEASE_JSON")
  update_latest=$(jq -r "${scenario_path}.update_latest // \"false\"" "$RELEASE_JSON")
  echo "  push: ${push}, skip_push: ${skip_push}, update_latest: ${update_latest}"

  if echo "$generated" | grep -q "^dockers:"; then
    local gen_docker_count
    gen_docker_count=$(echo "$generated" | awk '/^dockers:/,/^docker_manifests:/' | grep -c '^  - image_templates:' || true)
    local expected_count
    expected_count=$(jq '.docker.platforms | length' "$RELEASE_JSON")
    echo "  OK dockers: ${gen_docker_count} platform images defined (expected ${expected_count})"
  else
    echo "  FAILED: dockers section not found in generated goreleaser.yml"
    return 1
  fi

  local tag_count
  tag_count=$(jq "${scenario_path}.tag_templates | length" "$RELEASE_JSON")
  echo "  OK tag_templates: ${tag_count} tags defined"

  local expected_manifest_count
  expected_manifest_count="${tag_count}"
  if [ "$update_latest" = "true" ]; then
    expected_manifest_count=$((expected_manifest_count + 1))
  fi

  if echo "$generated" | grep -q "^docker_manifests:"; then
    local gen_manifest_count
    gen_manifest_count=$(echo "$generated" | awk '/^docker_manifests:/,/^release:/' | grep -c '^  - name_template:' || true)
    echo "  OK docker_manifests: ${gen_manifest_count} manifests defined (expected ${expected_manifest_count})"
  else
    echo "  FAILED: docker_manifests section not found in generated goreleaser.yml"
    return 1
  fi

  if [ "$skip_push" = "true" ]; then
    local skip_push_count
    skip_push_count=$(echo "$generated" | grep -c "skip_push: true" || true)
    if [ "$skip_push_count" -gt 0 ]; then
      echo "  OK skip_push: ${skip_push_count} entries marked skip_push"
    else
      echo "  FAILED: skip_push is true in config but not found in generated goreleaser.yml"
      return 1
    fi
  fi

  if [ "$update_latest" = "true" ]; then
    local latest_tag
    latest_tag=$(jq -r "${scenario_path}.latest_tag // \"latest\"" "$RELEASE_JSON")
    if echo "$generated" | grep -q "name_template:.*:${latest_tag}"; then
      echo "  OK latest tag: ${latest_tag} manifest defined"
    else
      echo "  FAILED: update_latest is true but latest tag manifest not found"
      return 1
    fi
  fi

  local docker_image
  docker_image=$(jq -r '.docker.image_name' "$RELEASE_JSON")
  if echo "$generated" | grep -q "${docker_image}:"; then
    echo "  OK Docker image prefix: ${docker_image}"
  else
    echo "  FAILED: Docker image ${docker_image} not found in generated config"
    return 1
  fi

  echo "  PASSED"
}

check_all_scenarios() {
  echo "=== Checking all Docker scenarios ==="
  local scenarios
  scenarios=$(jq -r '.docker.scenarios | keys[]' "$RELEASE_JSON")
  local result=0
  for s in $scenarios; do
    if ! check_docker_tags "$s"; then
      result=1
    fi
    echo ""
  done
  return $result
}

case "${1:-check}" in
  check)
    result=0
    check_goreleaser || result=1
    check_install_sh_platforms || result=1
    check_install_sh_adjust_os || result=1
    check_install_sh_adjust_arch || result=1
    check_install_sh_format || result=1
    check_readme || result=1
    check_makefile || result=1
    if [ "$CHECK_ALL_SCENARIOS" = "true" ]; then
      check_all_scenarios || result=1
    else
      check_docker_tags || result=1
    fi
    if [ $result -eq 0 ]; then
      echo ""
      echo "All checks passed!"
    else
      echo ""
      echo "Some checks FAILED! Run 'make release-gen' to regenerate from release.json"
      exit 1
    fi
    ;;
  docker-scenarios)
    check_all_scenarios
    ;;
  *)
    echo "Usage: $0 {check|docker-scenarios}"
    echo ""
    echo "Environment variables:"
    echo "  SCENARIO             release|snapshot|pr  (default: release)"
    echo "  CHECK_ALL_SCENARIOS  true|false            (default: false)"
    echo ""
    echo "Examples:"
    echo "  $0 check"
    echo "  SCENARIO=snapshot $0 check"
    echo "  CHECK_ALL_SCENARIOS=true $0 check"
    echo "  $0 docker-scenarios"
    exit 1
    ;;
esac
