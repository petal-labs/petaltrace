#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: $0 <petaltrace-binary>" >&2
	exit 2
fi

binary=$1
case "$binary" in
	/*) ;;
	*) binary="$(pwd)/$binary" ;;
esac

if [ ! -f "$binary" ]; then
	echo "release binary not found: $binary" >&2
	exit 1
fi

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/petaltrace-release-smoke.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

cat >"$tmp_dir/petaltrace.yaml" <<'EOF'
storage:
  sqlite:
    path: "petaltrace-release-smoke.db"
    wal_mode: false
EOF

(
	cd "$tmp_dir"
	"$binary" --config "$tmp_dir/petaltrace.yaml" stats --json >/dev/null
)

if [ ! -s "$tmp_dir/petaltrace-release-smoke.db" ]; then
	echo "release binary did not create a SQLite database" >&2
	exit 1
fi
