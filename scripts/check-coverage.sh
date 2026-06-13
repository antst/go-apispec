#!/usr/bin/env bash
#
# Per-package statement-coverage gate.
#
# Enforces the repo's documented ≥95%-per-package target (see CLAUDE.md) from a
# Go coverage profile, so a change that drops a package below its floor fails CI
# rather than silently eroding coverage.
#
# Usage:
#   scripts/check-coverage.sh [coverage-profile]
# The profile defaults to coverage.out and must be produced with
# `go test -coverprofile=<profile> ./...`.
#
# The CLI entrypoint packages (cmd/*) carry their own lower floors: they are
# thin main()/flag glue, not the analysis library the 95% target is about.
# These floors are a no-regression ratchet — raise them as coverage improves,
# never lower them.

set -euo pipefail

PROFILE="${1:-coverage.out}"
DEFAULT_MIN=95.0

# Package import path -> minimum percent. Anything not listed uses DEFAULT_MIN.
declare -A FLOOR=(
	["github.com/antst/go-apispec/cmd/apispec"]=90.0
	["github.com/antst/go-apispec/cmd/apidiag"]=92.0
)

if [[ ! -f "$PROFILE" ]]; then
	echo "coverage profile not found: $PROFILE" >&2
	echo "produce it with: go test -coverprofile=$PROFILE ./..." >&2
	exit 2
fi

# Aggregate covered/total statements per package from the profile. Each data
# line is "<importpath>/<file>.go:<pos> <numStatements> <hitCount>"; the package
# is the directory portion of the file path.
pkg_pcts="$(awk '
	/^mode:/ { next }
	{
		file = $1
		sub(/:[0-9].*$/, "", file)          # strip ":line.col,line.col"
		# Package import path = dirname(file). Built by re-joining all path
		# components except the trailing filename (avoids a slash inside a
		# regex class, which is non-portable across awk implementations).
		n = split(file, parts, "/")
		pkg = parts[1]
		for (i = 2; i < n; i++) pkg = pkg "/" parts[i]
		stmts = $(NF - 1)
		hits  = $NF
		total[pkg] += stmts
		if (hits > 0) covered[pkg] += stmts
	}
	END {
		for (p in total) {
			pct = (total[p] > 0) ? (covered[p] * 100.0 / total[p]) : 100.0
			printf "%s %.1f\n", p, pct
		}
	}
' "$PROFILE" | sort)"

if [[ -z "$pkg_pcts" ]]; then
	echo "no coverage data found in $PROFILE" >&2
	exit 2
fi

fail=0
printf '%-55s %8s %8s   %s\n' "PACKAGE" "COVERAGE" "FLOOR" "STATUS"
while read -r pkg pct; do
	min="${FLOOR[$pkg]:-$DEFAULT_MIN}"
	if awk "BEGIN{exit !($pct + 0 < $min + 0)}"; then
		status="FAIL"
		fail=1
	else
		status="ok"
	fi
	printf '%-55s %7s%% %7s%%   %s\n' "${pkg#github.com/antst/go-apispec/}" "$pct" "$min" "$status"
done <<<"$pkg_pcts"

echo
if [[ "$fail" -ne 0 ]]; then
	echo "coverage gate FAILED: one or more packages are below their floor (default ${DEFAULT_MIN}%)." >&2
	echo "Add tests for the net-new logic, or — for an intentional, justified floor change — update scripts/check-coverage.sh." >&2
	exit 1
fi
echo "coverage gate passed: every package meets its floor (default ${DEFAULT_MIN}%)."
