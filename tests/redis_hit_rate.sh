#!/usr/bin/env bash

set -e

REDIS_CLI="${REDIS_CLI:-redis-cli}"
BEFORE_FILE="${1:-redis_before.info}"
AFTER_FILE="${2:-redis_after.info}"

get_value() {
  local file="$1"
  local key="$2"

  grep "^${key}:" "${file}" | awk -F ':' '{print $2}' | tr -d '\r'
}

before_hits=$(get_value "${BEFORE_FILE}" "keyspace_hits")
before_misses=$(get_value "${BEFORE_FILE}" "keyspace_misses")

after_hits=$(get_value "${AFTER_FILE}" "keyspace_hits")
after_misses=$(get_value "${AFTER_FILE}" "keyspace_misses")

delta_hits=$((after_hits - before_hits))
delta_misses=$((after_misses - before_misses))
total=$((delta_hits + delta_misses))

echo "========== redis hit rate =========="
echo "before_hits:   ${before_hits}"
echo "before_misses: ${before_misses}"
echo "after_hits:    ${after_hits}"
echo "after_misses:  ${after_misses}"
echo "delta_hits:    ${delta_hits}"
echo "delta_misses:  ${delta_misses}"

if [ "${total}" -eq 0 ]; then
  echo "hit_rate:      N/A"
else
  awk -v h="${delta_hits}" -v t="${total}" 'BEGIN {
    printf("hit_rate:      %.2f%%\n", h / t * 100)
  }'
fi

echo "===================================="