#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")"/.. && pwd)"
RELEASE_JSON="$REPO_ROOT/release.json"

if ! command -v jq &>/dev/null; then
  echo "ERROR: jq is required" >&2
  exit 1
fi

SCENARIO="${SCENARIO:-release}"

owner=$(jq -r '.owner' "$RELEASE_JSON")
repo=$(jq -r '.repo' "$RELEASE_JSON")
project=$(jq -r '.project_name' "$RELEASE_JSON")
binary=$(jq -r '.binary_name' "$RELEASE_JSON")
version_file=$(jq -r '.version_file' "$RELEASE_JSON")
ldflags=$(jq -r '.build.ldflags' "$RELEASE_JSON")
default_format=$(jq -r '.archive.default_format' "$RELEASE_JSON")
platform_count=$(jq '.platforms | length' "$RELEASE_JSON")

os_title_for() {
  jq -r --arg goos "$1" '.naming.os_map[$goos] // ""' "$RELEASE_JSON"
}

arch_display_for() {
  local goarch="$1" goarm="${2:-}"
  if [ "$goarch" = "arm" ] && [ -n "$goarm" ]; then
    echo "armv${goarm}"
  else
    jq -r --arg goarch "$goarch" '.naming.arch_map[$goarch] // $goarch' "$RELEASE_JSON"
  fi
}

archive_fmt_for() {
  local goos="$1"
  local fmt="$default_format"
  local n i
  n=$(jq '.archive.format_overrides | length' "$RELEASE_JSON")
  i=0
  while [ "$i" -lt "$n" ]; do
    if [ "$(jq -r ".archive.format_overrides[$i].goos" "$RELEASE_JSON")" = "$goos" ]; then
      fmt=$(jq -r ".archive.format_overrides[$i].format" "$RELEASE_JSON")
    fi
    i=$((i + 1))
  done
  echo "$fmt"
}

generate_goreleaser_yml() {
  local out="$1"

  {
    echo "version: 2"
    echo "before:"
    echo "  hooks:"
    echo "    - go mod tidy"
    echo "version_file: ${version_file}"
    echo "builds:"
    echo "  - env:"
    echo "      - CGO_ENABLED=0"

    echo "    goos:"
    jq -r '.build.goos_order[]' "$RELEASE_JSON" | while IFS= read -r g; do
      echo "      - ${g}"
    done

    echo "    goarch:"
    jq -r '.build.goarch_order[]' "$RELEASE_JSON" | while IFS= read -r g; do
      if [ "$g" = "386" ]; then
        echo "      - '${g}'"
      else
        echo "      - ${g}"
      fi
    done

    echo "    goarm:"
    jq -r '.build.goarm_order[]' "$RELEASE_JSON" | while IFS= read -r g; do
      echo "      - '${g}'"
    done

    local i=0
    local need_ignore=false
    while [ "$i" -lt "$platform_count" ]; do
      local p_goos p_goarch p_goarm
      p_goos=$(jq -r ".platforms[$i].goos" "$RELEASE_JSON")
      p_goarch=$(jq -r ".platforms[$i].goarch" "$RELEASE_JSON")
      p_goarm=$(jq -r ".platforms[$i].goarm // \"\"" "$RELEASE_JSON")
      if [ "$p_goos" = "windows" ] && [ "$p_goarch" = "arm" ] && [ "$p_goarm" = "6" ]; then
        need_ignore=false
        break
      fi
      need_ignore=true
      i=$((i + 1))
    done
    if $need_ignore; then
      echo "    ignore:"
      echo "      - goos: windows"
      echo "        goarm: '6'"
    fi

    local has_darwin_arm=false
    i=0
    while [ "$i" -lt "$platform_count" ]; do
      local p_goos p_goarch
      p_goos=$(jq -r ".platforms[$i].goos" "$RELEASE_JSON")
      p_goarch=$(jq -r ".platforms[$i].goarch" "$RELEASE_JSON")
      if [ "$p_goos" = "darwin" ] && [ "$p_goarch" = "arm" ]; then
        has_darwin_arm=true
        break
      fi
      i=$((i + 1))
    done
    if ! $has_darwin_arm; then
      if $need_ignore; then
        echo "      - goos: darwin"
        echo "        goarch: arm"
      else
        echo "    ignore:"
        echo "      - goos: darwin"
        echo "        goarch: arm"
      fi
    fi

    echo "    ldflags:"
    echo "      - ${ldflags}"

    echo "checksum:"
    echo "  name_template: 'checksums.txt'"
    echo "archives:"
    echo "  - name_template: >-"
    echo "      {{ .ProjectName }}_"
    echo "      {{- title .Os }}_"

    local arch_map_entries
    arch_map_entries=$(jq -r '.naming.arch_map | to_entries[] | "\(.key):\(.value)"' "$RELEASE_JSON")
    local first=true
    while IFS=: read -r k v; do
      [ -z "$k" ] && continue
      if $first; then
        echo "      {{- if eq .Arch \"${k}\" }}${v}"
        first=false
      else
        echo "      {{- else if eq .Arch \"${k}\" }}${v}"
      fi
    done <<< "$arch_map_entries"
    if ! $first; then
      echo "      {{- else }}{{ .Arch }}{{ end }}"
    else
      echo "      {{ .Arch }}"
    fi
    echo "      {{- if .Arm }}v{{ .Arm }}{{ end }}"

    echo "    format_overrides:"
    local n_overrides i2
    n_overrides=$(jq '.archive.format_overrides | length' "$RELEASE_JSON")
    i2=0
    while [ "$i2" -lt "$n_overrides" ]; do
      echo "      - goos: $(jq -r ".archive.format_overrides[$i2].goos" "$RELEASE_JSON")"
      echo "        formats: [$(jq -r ".archive.format_overrides[$i2].format" "$RELEASE_JSON")]"
      i2=$((i2 + 1))
    done

    echo "changelog:"
    echo "  groups:"
    echo "    - title: 'New Features'"
    echo "      regexp: \"^.*feat[(\\\\w)]*:+.*$\""
    echo "      order: 0"
    echo "    - title: 'Bug fixes'"
    echo "      regexp: \"^.*fix[(\\\\w)]*:+.*$\""
    echo "      order: 1"
    echo "    - title: 'Documentation updates'"
    echo "      regexp: \"^.*docs[(\\\\w)]*:+.*$\""
    echo "      order: 2"
    echo "    - title: 'Other'"
    echo "      order: 999"

    local docker_enabled
    docker_enabled=$(jq -r '.docker.enabled // "false"' "$RELEASE_JSON")
    if [ "$docker_enabled" = "true" ]; then
      local image_name dockerfile
      image_name=$(jq -r '.docker.image_name' "$RELEASE_JSON")
      dockerfile=$(jq -r '.docker.dockerfile' "$RELEASE_JSON")
      local docker_platform_count
      docker_platform_count=$(jq '.docker.platforms | length' "$RELEASE_JSON")

      local scenario_path skip_push update_latest latest_tag
      scenario_path=".docker.scenarios.\"${SCENARIO}\""
      skip_push=$(jq -r "${scenario_path}.skip_push // \"false\"" "$RELEASE_JSON")
      update_latest=$(jq -r "${scenario_path}.update_latest // \"false\"" "$RELEASE_JSON")
      latest_tag=$(jq -r "${scenario_path}.latest_tag // \"latest\"" "$RELEASE_JSON")

      echo "dockers:"
      local d=0
      while [ "$d" -lt "$docker_platform_count" ]; do
        local d_goos d_goarch d_goarm d_image_arch
        d_goos=$(jq -r ".docker.platforms[$d].goos" "$RELEASE_JSON")
        d_goarch=$(jq -r ".docker.platforms[$d].goarch" "$RELEASE_JSON")
        d_goarm=$(jq -r ".docker.platforms[$d].goarm // \"\"" "$RELEASE_JSON")
        if [ "$d_goarch" = "arm" ] && [ -n "$d_goarm" ]; then
          d_image_arch="arm/v${d_goarm}"
        else
          d_image_arch="$d_goarch"
        fi
        echo "  - image_templates:"
        local tag_count
        tag_count=$(jq "${scenario_path}.tag_templates | length" "$RELEASE_JSON")
        local t=0
        while [ "$t" -lt "$tag_count" ]; do
          local tag
          tag=$(jq -r "${scenario_path}.tag_templates[$t]" "$RELEASE_JSON")
          echo "      - ${image_name}:${tag}-${d_image_arch}"
          t=$((t + 1))
        done
        if [ "$update_latest" = "true" ]; then
          echo "      - ${image_name}:${latest_tag}-${d_image_arch}"
        fi
        echo "    goos: $d_goos"
        echo "    goarch: $d_goarch"
        if [ -n "$d_goarm" ]; then
          echo "    goarm: '$d_goarm'"
        fi
        echo "    dockerfile: $dockerfile"
        echo "    build_flag_templates:"
        echo "      - --platform=${d_goos}/${d_image_arch}"
        if [ "$skip_push" = "true" ]; then
          echo "    skip_push: true"
        fi
        d=$((d + 1))
      done

      echo "docker_manifests:"
      local manifest_tag_count
      manifest_tag_count=$(jq "${scenario_path}.tag_templates | length" "$RELEASE_JSON")
      local mt=0
      while [ "$mt" -lt "$manifest_tag_count" ]; do
        local mtag
        mtag=$(jq -r "${scenario_path}.tag_templates[$mt]" "$RELEASE_JSON")
        echo "  - name_template: ${image_name}:${mtag}"
        echo "    image_templates:"
        local md=0
        while [ "$md" -lt "$docker_platform_count" ]; do
          local md_goos md_goarch md_goarm md_image_arch
          md_goos=$(jq -r ".docker.platforms[$md].goos" "$RELEASE_JSON")
          md_goarch=$(jq -r ".docker.platforms[$md].goarch" "$RELEASE_JSON")
          md_goarm=$(jq -r ".docker.platforms[$md].goarm // \"\"" "$RELEASE_JSON")
          if [ "$md_goarch" = "arm" ] && [ -n "$md_goarm" ]; then
            md_image_arch="arm/v${md_goarm}"
          else
            md_image_arch="$md_goarch"
          fi
          echo "      - ${image_name}:${mtag}-${md_image_arch}"
          md=$((md + 1))
        done
        if [ "$skip_push" = "true" ]; then
          echo "    skip_push: true"
        fi
        mt=$((mt + 1))
      done
      if [ "$update_latest" = "true" ]; then
        echo "  - name_template: ${image_name}:${latest_tag}"
        echo "    image_templates:"
        local mdl=0
        while [ "$mdl" -lt "$docker_platform_count" ]; do
          local mdl_goos mdl_goarch mdl_goarm mdl_image_arch
          mdl_goos=$(jq -r ".docker.platforms[$mdl].goos" "$RELEASE_JSON")
          mdl_goarch=$(jq -r ".docker.platforms[$mdl].goarch" "$RELEASE_JSON")
          mdl_goarm=$(jq -r ".docker.platforms[$mdl].goarm // \"\"" "$RELEASE_JSON")
          if [ "$mdl_goarch" = "arm" ] && [ -n "$mdl_goarm" ]; then
            mdl_image_arch="arm/v${mdl_goarm}"
          else
            mdl_image_arch="$mdl_goarch"
          fi
          echo "      - ${image_name}:${latest_tag}-${mdl_image_arch}"
          mdl=$((mdl + 1))
        done
        if [ "$skip_push" = "true" ]; then
          echo "    skip_push: true"
        fi
      fi
    fi

    echo "release:"
    echo "  prerelease: auto"
    echo "  mode: append"
  } > "$out"
}

generate_install_sh_platforms() {
  echo "get_binaries() {"
  echo "  case \"\$PLATFORM\" in"
  local i=0
  while [ "$i" -lt "$platform_count" ]; do
    local goos goarch goarm plat_key
    goos=$(jq -r ".platforms[$i].goos" "$RELEASE_JSON")
    goarch=$(jq -r ".platforms[$i].goarch" "$RELEASE_JSON")
    goarm=$(jq -r ".platforms[$i].goarm // \"\"" "$RELEASE_JSON")
    if [ -n "$goarm" ]; then
      plat_key="${goos}/${goarch}v${goarm}"
    else
      plat_key="${goos}/${goarch}"
    fi
    echo "    ${plat_key}) BINARIES=\"${binary}\" ;;"
    i=$((i + 1))
  done
  echo "    *)"
  echo "      log_crit \"platform \$PLATFORM is not supported.  Make sure this script is up-to-date and file request at https://github.com/\${PREFIX}/issues/new\""
  echo "      exit 1"
  echo "      ;;"
  echo "  esac"
  echo "}"
}

generate_install_sh_adjust_os() {
  echo "adjust_os() {"
  echo "  case \${OS} in"
  jq -r '.naming.os_map | to_entries[] | "\(.key):\(.value)"' "$RELEASE_JSON" | while IFS=: read -r k v; do
    [ -z "$k" ] && continue
    echo "    ${k}) OS=${v} ;;"
  done
  echo "  esac"
  echo "  true"
  echo "}"
}

generate_install_sh_adjust_arch() {
  echo "adjust_arch() {"
  echo "  case \${ARCH} in"
  jq -r '.naming.arch_map | to_entries[] | "\(.key):\(.value)"' "$RELEASE_JSON" | while IFS=: read -r k v; do
    [ -z "$k" ] && continue
    echo "    ${k}) ARCH=${v} ;;"
  done
  echo "  esac"
  echo "  true"
  echo "}"
}

generate_install_sh_format() {
  echo "adjust_format() {"
  echo "  # change format (tar.gz or zip) based on OS"
  echo "  case \${OS} in"
  local n_overrides i
  n_overrides=$(jq '.archive.format_overrides | length' "$RELEASE_JSON")
  i=0
  while [ "$i" -lt "$n_overrides" ]; do
    local o_goos o_fmt
    o_goos=$(jq -r ".archive.format_overrides[$i].goos" "$RELEASE_JSON")
    o_fmt=$(jq -r ".archive.format_overrides[$i].format" "$RELEASE_JSON")
    echo "    ${o_goos}) FORMAT=${o_fmt} ;;"
    i=$((i + 1))
  done
  echo "  esac"
  echo "  true"
  echo "}"
}

generate_readme_snippet() {
  local out="$1"

  {
    echo "<!-- AUTO-GENERATED from release.json by scripts/release_gen.sh - DO NOT EDIT MANUALLY -->"
    echo "## Installation"
    echo ""

    local curl_cmd brew_cmd choco_cmd
    curl_cmd=$(jq -r '.install.curl_template' "$RELEASE_JSON" | sed "s/{owner}/${owner}/g; s/{repo}/${repo}/g")
    brew_cmd=$(jq -r '.install.brew_command' "$RELEASE_JSON")
    choco_cmd=$(jq -r '.install.choco_command' "$RELEASE_JSON")

    echo '```bash'
    echo "$curl_cmd"
    echo '```'
    echo ""
    echo "Or via Homebrew:"
    echo ""
    echo '```bash'
    echo "$brew_cmd"
    echo '```'
    echo ""
    echo "Or via Chocolatey (Windows):"
    echo ""
    echo '```powershell'
    echo "$choco_cmd"
    echo '```'
    echo ""
    echo "### Supported Platforms"
    echo ""
    echo "| OS | Architecture | Archive |"
    echo "|---|---|---|"

    local i=0
    while [ "$i" -lt "$platform_count" ]; do
      local goos goarch goarm os_title arch_display fmt archive_name arm_suffix
      goos=$(jq -r ".platforms[$i].goos" "$RELEASE_JSON")
      goarch=$(jq -r ".platforms[$i].goarch" "$RELEASE_JSON")
      goarm=$(jq -r ".platforms[$i].goarm // \"\"" "$RELEASE_JSON")
      os_title=$(os_title_for "$goos")
      arch_display=$(arch_display_for "$goarch" "$goarm")
      fmt=$(archive_fmt_for "$goos")
      archive_name="${project}_${os_title}_${arch_display}.${fmt}"
      arm_suffix=""
      if [ -n "$goarm" ]; then
        arm_suffix=" v${goarm}"
      fi
      echo "| ${goos} | ${goarch}${arm_suffix} | \`${archive_name}\` |"
      i=$((i + 1))
    done
    echo ""

    local docker_image
    docker_image=$(jq -r '.docker.image_name' "$RELEASE_JSON")
    echo "### Docker"
    echo ""
    echo '```bash'
    echo "docker pull ${docker_image}:latest"
    echo '```'
    echo ""
    echo "Available tags: \`latest\`, \`<major>.<minor>\`, \`<version>\`"
    echo "<!-- END AUTO-GENERATED -->"
  } > "$out"
}

case "${1:-}" in
  goreleaser)
    generate_goreleaser_yml "${2:-/dev/stdout}"
    ;;
  install-platforms)
    generate_install_sh_platforms
    ;;
  install-adjust-os)
    generate_install_sh_adjust_os
    ;;
  install-adjust-arch)
    generate_install_sh_adjust_arch
    ;;
  install-adjust-format)
    generate_install_sh_format
    ;;
  readme-snippet)
    generate_readme_snippet "${2:-/dev/stdout}"
    ;;
  all)
    generate_goreleaser_yml "$REPO_ROOT/.goreleaser.yml.gen"
    generate_readme_snippet "$REPO_ROOT/README.release-snippet.md"
    echo "Generated: .goreleaser.yml.gen, README.release-snippet.md"
    ;;
  *)
    echo "Usage: $0 {goreleaser|install-platforms|install-adjust-os|install-adjust-arch|install-adjust-format|readme-snippet|all} [output-file]"
    echo ""
    echo "Environment variables:"
    echo "  SCENARIO   release|snapshot|pr  (default: release)"
    echo ""
    echo "Examples:"
    echo "  SCENARIO=release $0 goreleaser .goreleaser.yml"
    echo "  SCENARIO=snapshot $0 goreleaser .goreleaser.yml"
    echo "  SCENARIO=pr PR_NUMBER=123 $0 goreleaser .goreleaser.yml"
    exit 1
    ;;
esac
