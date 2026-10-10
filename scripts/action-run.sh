#!/bin/sh
# The GitHub Action's main step (action.yml): runs
# `ai-flow standards check` on INPUT_PATH, writes the SARIF file, puts a
# readable verdict in the job summary, sets the step outputs and exits
# with the CLI's own exit code (6 while a must finding is open).
#
# Every input arrives as an environment variable set by action.yml's
# env: block, never interpolated into this script's text, and every
# expansion below is quoted: an input is always one argv element.
#
# Environment:
#   AI_FLOW_BIN                 the ai-flow binary to run
#   INPUT_PATH                  directory to check
#   INPUT_SARIF                 SARIF output file
#   INPUT_PACK                  optional --pack value
#   INPUT_REQUIRE_INSTRUCTIONS  "true" adds --require-instructions
#   GITHUB_OUTPUT, GITHUB_STEP_SUMMARY, RUNNER_TEMP   set by the runner

set -u

envelope="${RUNNER_TEMP}/ai-flow-envelope.json"

set -- standards check "${INPUT_PATH:?INPUT_PATH is required}" --sarif "${INPUT_SARIF:?INPUT_SARIF is required}"
if [ -n "${INPUT_PACK:-}" ]; then
	set -- "$@" --pack "$INPUT_PACK"
fi
if [ "${INPUT_REQUIRE_INSTRUCTIONS:-false}" = "true" ]; then
	set -- "$@" --require-instructions
fi

"$AI_FLOW_BIN" "$@" >"$envelope"
code=$?

{
	echo "exit-code=${code}"
	echo "sarif=${INPUT_SARIF}"
} >>"$GITHUB_OUTPUT"

{
	echo "### ai-flow standards check"
	echo
	if command -v jq >/dev/null 2>&1 && jq -e . "$envelope" >/dev/null 2>&1; then
		if [ "$code" -eq 0 ]; then
			echo "**passed** — $(jq -r '.result.open // 0' "$envelope") open finding(s), none of them must."
		else
			echo "**failed** (exit ${code}, \`$(jq -r '.error.code // "unknown"' "$envelope")\`): $(jq -r '.error.message // ""' "$envelope")"
			echo
			jq -r '.error.details // [] | .[] | "- " + .' "$envelope"
		fi
		skipped="$(jq -r '.result.skipped // [] | .[] | "- `" + .check_id + "` on `" + .target + "`: " + .reason' "$envelope")"
		if [ -n "$skipped" ]; then
			echo
			echo "Skipped (need a rendering, which only a ticket has):"
			echo "$skipped"
		fi
		warnings="$(jq -r '.warnings // [] | .[] | "- " + .' "$envelope")"
		if [ -n "$warnings" ]; then
			echo
			echo "Warnings:"
			echo "$warnings"
		fi
	else
		echo "exit ${code}"
	fi
	echo
	echo "<details><summary>Envelope</summary>"
	echo
	echo '```json'
	cat "$envelope"
	echo '```'
	echo "</details>"
} >>"$GITHUB_STEP_SUMMARY"

cat "$envelope"
exit "$code"
