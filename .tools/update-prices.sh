#!/bin/sh
# Refreshes prices.json from OpenRouter's public model list. Run it before a release,
# check the diff, then build: the file is embedded, so wj itself never goes online for prices.
set -eu
cd "$(dirname "$0")/.."
tmp=prices.json.tmp
curl -fsS -m 60 https://openrouter.ai/api/v1/models | jq --arg date "$(date -u +%F)" '
  def keep: with_entries(select(.key | IN("prompt", "completion", "input_cache_read",
    "input_cache_write", "input_cache_write_1h", "min_prompt_tokens")));
  {
    asOf: $date,
    source: "https://openrouter.ai/api/v1/models",
    models: ([.data[]
      | select((.id | test(":|^~|^openrouter/") | not)
          and ((.pricing.prompt | tonumber) > 0 or (.pricing.completion | tonumber) > 0))
      | {key: .id, value: ((.pricing | keep) + (if .pricing.overrides then {overrides: (.pricing.overrides | map(keep))} else {} end))}]
      | sort_by(.key) | from_entries)
  }' > "$tmp"
mv "$tmp" prices.json
jq -r '"\(.models | length) models, \(.asOf)"' prices.json
