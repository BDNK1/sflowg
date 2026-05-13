#!/usr/bin/env bash

assert_file_executable() {
  local file="$1"

  if [[ ! -x "$file" ]]; then
    echo "expected executable file: $file" >&2
    return 1
  fi
}

assert_equals() {
  local want="$1"
  local got="$2"

  if [[ "$got" != "$want" ]]; then
    echo "expected '$want', got '$got'" >&2
    return 1
  fi
}
