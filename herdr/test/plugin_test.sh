#!/usr/bin/env bash

set -euo pipefail

plugin_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly plugin_root
test_directory="$(mktemp -d)"
readonly test_directory
readonly fake_bin_directory="$test_directory/bin"

cleanup() {
    rm -rf "$test_directory"
}

fail() {
    printf 'FAIL: %s\n' "$1" >&2
    exit 1
}

assert_file_equals() {
    local expected="$1"
    local actual_file="$2"
    local actual

    actual=$(<"$actual_file")
    [[ "$actual" == "$expected" ]] || fail "Unexpected content in $actual_file"
}

assert_file_contains() {
    local expected="$1"
    local actual_file="$2"

    grep -Fq "$expected" "$actual_file" || fail "Missing '$expected' in $actual_file"
}

assert_file_not_contains() {
    local expected="$1"
    local actual_file="$2"

    grep -Fq "$expected" "$actual_file" && fail "Unexpected '$expected' in $actual_file"
    return 0
}

install_fake_commands() {
    mkdir -p "$fake_bin_directory" "$test_directory/home"

    cat >"$fake_bin_directory/timber" <<'FAKE_TIMBER'
#!/usr/bin/env bash
set -eu
if [[ "${1:-}" == "remove" ]]; then
    printf 'timber remove cwd=%s\n' "$PWD" >>"$DELETE_LOG_FILE"
    if [[ "${FAKE_REMOVE_FAILURE:-}" == "1" ]]; then
        printf 'worktree "x" is not clean\n' >&2
        exit 1
    fi
    exit 0
fi
printf '%s\n' "$@" >"$CREATE_ARGUMENTS_FILE"
if [[ "${1:-} ${2:-} ${3:-}" == "tui --herdr --no-title" ]]; then
    [[ "${FAKE_CREATE_FAILURE:-}" != "1" ]] || exit 1
    exit 0
fi
exit 1
FAKE_TIMBER

    chmod +x "$fake_bin_directory/timber"

    cat >"$fake_bin_directory/herdr" <<'FAKE_HERDR'
#!/usr/bin/env bash
set -eu
printf 'herdr %s\n' "$*" >>"$DELETE_LOG_FILE"
case "${1:-} ${2:-}" in
    "workspace get")
        if [[ "${FAKE_NOT_LINKED:-}" == "1" ]]; then
            printf '{"result":{"workspace":{"workspace_id":"w1","worktree":{"checkout_path":"/bare.git","is_linked_worktree":false,"repo_key":"k","repo_name":"r","repo_root":"/r"}}}}'
        else
            printf '{"result":{"workspace":{"workspace_id":"w1","worktree":{"checkout_path":"%s","is_linked_worktree":true,"repo_key":"k","repo_name":"r","repo_root":"/r"}}}}' "$FAKE_CHECKOUT_PATH"
        fi
        ;;
    "pane list")
        cat "$FAKE_PANE_LIST_FILE"
        ;;
    "pane get")
        printf '{"result":{"pane":{"pane_id":"%s"}}}' "$3"
        ;;
    "agent send-keys" | "pane close")
        exit 0
        ;;
    *)
        printf '{"result":{}}'
        ;;
esac
FAKE_HERDR

    chmod +x "$fake_bin_directory/herdr"
}

run_create_command() (
    cd "$plugin_root"
    env -i \
        HOME="$test_directory/home" \
        PATH="$fake_bin_directory:/usr/bin:/bin" \
        FAKE_CREATE_FAILURE="${FAKE_CREATE_FAILURE:-}" \
        CREATE_ARGUMENTS_FILE="$test_directory/create-arguments" \
        DELETE_LOG_FILE="$test_directory/delete-log" \
        /bin/bash bin/create
)

run_delete_command() (
    cd "$plugin_root"
    env -i \
        HOME="$test_directory/home" \
        PATH="$fake_bin_directory:/usr/bin:/bin" \
        HERDR_BIN_PATH="$fake_bin_directory/herdr" \
        HERDR_WORKSPACE_ID="${HERDR_WORKSPACE_ID:-w1}" \
        HERDR_PLUGIN_CONTEXT_JSON="${HERDR_PLUGIN_CONTEXT_JSON:-}" \
        FAKE_CHECKOUT_PATH="$test_directory/worktree" \
        FAKE_PANE_LIST_FILE="$test_directory/pane-list.json" \
        FAKE_NOT_LINKED="${FAKE_NOT_LINKED:-}" \
        FAKE_REMOVE_FAILURE="${FAKE_REMOVE_FAILURE:-}" \
        DELETE_LOG_FILE="$test_directory/delete-log" \
        CREATE_ARGUMENTS_FILE="$test_directory/create-arguments" \
        /bin/bash bin/delete
)

test_invokes_tui_create_with_herdr() {
    run_create_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"

    assert_file_equals $'tui\n--herdr\n--no-title' \
        "$test_directory/create-arguments"
}

test_reports_create_failure() {
    rm -f "$test_directory/create-arguments"

    if printf '\n' | FAKE_CREATE_FAILURE=1 run_create_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"; then
        fail 'Create failure returned success'
    fi

    assert_file_equals $'tui\n--herdr\n--no-title' \
        "$test_directory/create-arguments"
    assert_file_contains 'timber could not create or open the worktree.' "$test_directory/stderr"
}

setup_delete_fixtures() {
    mkdir -p "$test_directory/worktree"
    cat >"$test_directory/pane-list.json" <<EOF
{"result":{"panes":[
{"pane_id":"w1:p1","cwd":"$test_directory/worktree","foreground_cwd":"$test_directory/worktree","agent":"pi"},
{"pane_id":"w1:p2","cwd":"$test_directory/worktree","foreground_cwd":"$test_directory/worktree"},
{"pane_id":"w1:p3","cwd":"/other","foreground_cwd":"/other"}
]}}
EOF
    rm -f "$test_directory/delete-log"
}

test_delete_closes_only_worktree_panes() {
    setup_delete_fixtures

    run_delete_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"

    assert_file_contains "timber remove cwd=$test_directory/worktree" "$test_directory/delete-log"
    assert_file_contains "herdr agent send-keys w1:p1 ctrl+c" "$test_directory/delete-log"
    assert_file_contains "herdr agent send-keys w1:p1 ctrl+d" "$test_directory/delete-log"
    assert_file_contains "herdr pane close w1:p1" "$test_directory/delete-log"
    assert_file_contains "herdr pane close w1:p2" "$test_directory/delete-log"
    assert_file_not_contains "w1:p3" "$test_directory/delete-log"

    local remove_line close_line
    remove_line="$(grep -Fn "timber remove" "$test_directory/delete-log" | cut -d: -f1)"
    close_line="$(grep -Fn "herdr pane close" "$test_directory/delete-log" | head -1 | cut -d: -f1)"
    [[ "$remove_line" -lt "$close_line" ]] || fail "timber remove ran after pane close"
}

test_delete_refuses_when_remove_fails() {
    setup_delete_fixtures

    if FAKE_REMOVE_FAILURE=1 run_delete_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"; then
        fail 'Remove failure returned success'
    fi

    assert_file_contains 'is not clean' "$test_directory/stderr"
    assert_file_contains 'timber remove cwd=' "$test_directory/delete-log"
    assert_file_not_contains "pane close" "$test_directory/delete-log"
    assert_file_not_contains "send-keys" "$test_directory/delete-log"
}

test_delete_refuses_non_linked_workspace() {
    setup_delete_fixtures

    if FAKE_NOT_LINKED=1 run_delete_command \
        >"$test_directory/stdout" 2>"$test_directory/stderr"; then
        fail 'Non-linked workspace returned success'
    fi

    assert_file_not_contains "timber remove" "$test_directory/delete-log"
    assert_file_not_contains "pane close" "$test_directory/delete-log"
}

main() {
    trap cleanup EXIT
    install_fake_commands
    test_invokes_tui_create_with_herdr
    test_reports_create_failure
    test_delete_closes_only_worktree_panes
    test_delete_refuses_when_remove_fails
    test_delete_refuses_non_linked_workspace
    printf 'All plugin tests passed.\n'
}

main "$@"
