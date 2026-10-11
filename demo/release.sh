#!/bin/sh
# List or upload demo assets in an existing GitHub release.

usage() {
    printf 'Usage: %s [-r OWNER/REPO] [-t TAG] list\n' "$0"
    printf '       %s [-r OWNER/REPO] [-t TAG] upload FILE VERSION\n' "$0"
}

fail() {
    printf 'release.sh: %s\n' "$*" >&2
    exit 1
}

hash_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        hash_output=$(sha256sum <"$1")
    else
        hash_output=$(shasum -a 256 <"$1")
    fi
    printf '%s\n' "${hash_output%% *}"
}

list_assets() {
    gh api --hostname github.com --paginate --slurp \
        "repos/$repo/releases/$release_id/assets?per_page=100" >"$work/pages.json"
    jq -e 'add // []' "$work/pages.json" >"$work/assets.json"
}

matching_url() {
    jq -r --arg digest "sha256:$digest" --argjson size "$size" --arg name "$name" \
        '[.[] | select(.name == $name and .state == "uploaded" and .size == $size and .digest == $digest)]
        | first | .browser_download_url // empty' "$work/assets.json"
}

load_release() {
    # The releases/tags endpoint omits drafts whose tag does not exist yet,
    # so search the release list, which includes drafts for writers.
    gh api --hostname github.com --paginate --slurp \
        "repos/$repo/releases?per_page=100" >"$work/releases.json"
    matches=$(jq --arg tag "$tag" '[add // [] | .[] | select(.tag_name == $tag)]' "$work/releases.json")
    case $(printf '%s' "$matches" | jq 'length') in
        1) release_id=$(printf '%s' "$matches" | jq -er '.[0].id') ;;
        0) fail "no release for tag: $tag" ;;
        *) fail "more than one release for tag: $tag" ;;
    esac
}

upload_asset() {
    if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
        fail 'sha256sum or shasum is required'
    fi
    case $1 in
        /*) file=$1 ;;
        *) file=$PWD/$1 ;;
    esac
    [ -f "$file" ] || fail "not a regular file: $1"
    version=$2
    jq -en --arg version "$version" '$version | test("^v[0-9]+\\.[0-9]+\\.[0-9]+$")' >/dev/null || fail 'expected version v0.xx.x'
    basename=${file##*/}
    case $basename in
        *.gif | *.mp4) name=${basename%.*}-$version.${basename##*.} ;;
        *) fail 'expected a GIF or MP4 recording' ;;
    esac
    # Hash and upload the same snapshot even if the recording is regenerated.
    cp "$file" "$work/input"
    digest=$(hash_file "$work/input")
    size=$(wc -c <"$work/input")
    load_release
    list_assets
    url=$(matching_url)
    if [ -n "$url" ]; then
        printf '%s\n' "$url"
        return
    fi

    if jq -e --arg name "$name" 'any(.[]; .name == $name)' "$work/assets.json" >/dev/null; then
        fail "asset already exists with different content: $name"
    fi
    mv "$work/input" "$work/$name"
    upload_status=0
    gh release upload "$tag" "$work/$name" --repo "https://github.com/$repo" >&2 || upload_status=$?
    # Recheck after upload, including a possible concurrent upload of the same file.
    list_assets
    url=$(matching_url)
    [ -n "$url" ] || fail "no matching asset after upload (exit $upload_status)"
    printf '%s\n' "$url"
}

main() {
    set -eu
    repo=
    tag=demo-assets
    while getopts 'r:t:h' option; do
        case $option in
            r) repo=$OPTARG ;;
            t) tag=$OPTARG ;;
            h)
                usage
                exit 0
                ;;
            *)
                usage >&2
                exit 2
                ;;
        esac
    done
    shift "$((OPTIND - 1))"
    [ "$#" -gt 0 ] || {
        usage >&2
        exit 2
    }
    subcommand=$1
    shift
    case $subcommand in
        list) [ "$#" -eq 0 ] || {
            usage >&2
            exit 2
        } ;;
        upload) [ "$#" -eq 2 ] || {
            usage >&2
            exit 2
        } ;;
        *)
            usage >&2
            exit 2
            ;;
    esac
    for tool in gh jq; do
        command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
    done
    [ -n "$repo" ] || repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
    jq -en --arg repo "$repo" '$repo | test("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$")' >/dev/null || fail 'expected OWNER/REPO'
    [ -n "$tag" ] || fail 'tag must not be empty'
    root=$(CDPATH='' cd -P "$(dirname "$0")/.." && pwd)
    mkdir -p "$root/.tmp"
    work=$(mktemp -d "$root/.tmp/release-demo.XXXXXX")
    trap 'rm -rf "$work"' 0
    trap 'exit 1' HUP INT TERM

    case $subcommand in
        list)
            load_release
            list_assets
            jq -r '.[] | [.name, (.digest // ""), .browser_download_url] | @tsv' "$work/assets.json"
            ;;
        upload) upload_asset "$1" "$2" ;;
    esac
}

main "$@"
