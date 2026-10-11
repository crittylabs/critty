#!/bin/sh
set -eu

root=$(CDPATH='' cd -P "$(dirname "$0")/.." && pwd)
mkdir -p "$root/.tmp"
test_dir=$(mktemp -d "$root/.tmp/upload-test.XXXXXX")
trap 'rm -rf "$test_dir"' 0
trap 'exit 1' HUP INT TERM
mkdir "$test_dir/bin"
export UPLOAD_TEST_DIR="$test_dir"
export PATH="$test_dir/bin:$PATH"
cat >"$test_dir/bin/gh" <<'MOCK'
#!/bin/sh
set -eu
state=$UPLOAD_TEST_DIR
printf '%s\n' "$*" >> "$state/calls"
if [ "$1 $2" = 'repo view' ]; then
    [ "$UPLOAD_TEST_CASE" != repo_failure ] || exit 1
    printf 'knu/tcrit\n'
    exit 0
fi
if [ "$1 $2" = 'release upload' ]; then
    [ "${4##*/}" = recording-v0.15.0.gif ]
    cp "$4" "$state/uploaded"
    case $UPLOAD_TEST_CASE in
        failure) exit 1 ;;
        race) cp "$state/after.json" "$state/before.json"; exit 1 ;;
    esac
    cp "$state/after.json" "$state/before.json"
    exit 0
fi
for arg in "$@"; do
    case $arg in repos/*) endpoint=$arg ;; esac
done

case $endpoint in
    */releases\?per_page=100)
        tag=${UPLOAD_TEST_TAG:-demo-assets}
        case $UPLOAD_TEST_CASE in
            missing) printf '[[{"id":41,"tag_name":"other","draft":false}]]\n' ;;
            ambiguous) jq -n --arg tag "$tag" '[[{id:42, tag_name:$tag, draft:true}], [{id:43, tag_name:$tag, draft:false}]]' ;;
            # The draft is on the second page, as drafts of a new tag would be.
            draft) jq -n --arg tag "$tag" '[[{id:41, tag_name:"other", draft:false}], [{id:42, tag_name:$tag, draft:true}]]' ;;
            *) jq -n --arg tag "$tag" '[[{id:42, tag_name:$tag, draft:false}]]' ;;
        esac
        ;;
    */releases/42/assets\?per_page=100)
        [ "$UPLOAD_TEST_CASE" != list_failure ] || exit 1
        cat "$state/before.json"
        ;;
    *) printf 'unexpected endpoint: %s\n' "$endpoint" >&2; exit 1 ;;
esac
MOCK
chmod +x "$test_dir/bin/gh"
printf 'recording bytes\n' >"$test_dir/input"
cp "$test_dir/input" "$test_dir/recording.gif"
if command -v sha256sum >/dev/null 2>&1; then
    digest=$(sha256sum <"$test_dir/input")
else
    digest=$(shasum -a 256 <"$test_dir/input")
fi
digest=${digest%% *}
size=$(wc -c <"$test_dir/input")
url=https://github.com/knu/tcrit/releases/download/demo-assets/recording-v0.15.0.gif
jq -n --arg digest "sha256:$digest" --arg url "$url" --argjson size "$size" \
    '[[{id:7, name:"recording-v0.15.0.gif", state:"uploaded", size:$size,
    digest:$digest, browser_download_url:$url}]]' >"$test_dir/after.json"

for scenario in duplicate new race failure missing ambiguous draft list_failure repo_failure conflict renamed; do
    export UPLOAD_TEST_CASE=$scenario
    : >"$test_dir/calls"
    rm -f "$test_dir/uploaded"
    case $scenario in
        duplicate)
            # The matching asset is on the second page and has the versioned name.
            jq '[[], .[0]]' "$test_dir/after.json" >"$test_dir/before.json"
            ;;
        conflict) jq '.[0][0].digest = "sha256:other"' "$test_dir/after.json" >"$test_dir/before.json" ;;
        renamed) jq '.[0][0].name = "old-name.gif"' "$test_dir/after.json" >"$test_dir/before.json" ;;
        *) printf '[[]]\n' >"$test_dir/before.json" ;;
    esac
    result=0
    sh "$root/demo/release.sh" upload "$test_dir/recording.gif" v0.15.0 >"$test_dir/out" 2>"$test_dir/err" || result=$?
    case $scenario in
        duplicate | new | race | draft | renamed)
            [ "$result" -eq 0 ]
            [ "$(cat "$test_dir/out")" = "$url" ]
            ;;
        *)
            [ "$result" -ne 0 ]
            [ ! -s "$test_dir/out" ]
            ;;
    esac
    case $scenario in
        new | race | failure | draft | renamed) cmp "$test_dir/input" "$test_dir/uploaded" ;;
        *) [ ! -e "$test_dir/uploaded" ] ;;
    esac
    printf 'PASS %s\n' "$scenario"
done

export UPLOAD_TEST_CASE=list
rm -f "$test_dir/uploaded"
jq '[[], .[0]]' "$test_dir/after.json" >"$test_dir/before.json"
sh "$root/demo/release.sh" list >"$test_dir/out"
printf 'recording-v0.15.0.gif\tsha256:%s\t%s\n' "$digest" "$url" >"$test_dir/expected"
cmp "$test_dir/expected" "$test_dir/out"
[ ! -e "$test_dir/uploaded" ]
printf 'PASS list\n'

printf '[[]]\n' >"$test_dir/before.json"
sh "$root/demo/release.sh" list >"$test_dir/out"
[ ! -s "$test_dir/out" ]
printf 'PASS empty list\n'

export UPLOAD_TEST_CASE=repo_failure UPLOAD_TEST_TAG=media
: >"$test_dir/calls"
sh "$root/demo/release.sh" -r other/project -t media list >"$test_dir/out"
grep -F 'repos/other/project/releases?per_page=100' "$test_dir/calls" >/dev/null
grep -F 'repos/other/project/releases/42/assets' "$test_dir/calls" >/dev/null
unset UPLOAD_TEST_TAG
printf 'PASS explicit repository and tag\n'

for arguments in '' 'list extra' 'upload' 'unknown'; do
    : >"$test_dir/calls"
    # These are fixed test arguments, not user input.
    # shellcheck disable=SC2086
    if sh "$root/demo/release.sh" $arguments >"$test_dir/out" 2>"$test_dir/err"; then
        exit 1
    fi
    [ ! -s "$test_dir/calls" ]
done
printf 'PASS invalid arguments\n'
