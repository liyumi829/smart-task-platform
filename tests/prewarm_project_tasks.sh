#!/usr/bin/env bash

set -e

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
TOKEN="${TOKEN:-<token>}"
PROJECT_ID="${PROJECT_ID:-1}"
PAGE_SIZE="${PAGE_SIZE:-20}"

# 热点请求列表，需要和 wrk Lua 里的 hot_paths 保持一致
HOT_PATHS=(
  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}"
  "/api/v1/projects/${PROJECT_ID}/tasks?page=2&page_size=${PAGE_SIZE}"
  "/api/v1/projects/${PROJECT_ID}/tasks?page=3&page_size=${PAGE_SIZE}"

  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&status=todo"
  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&status=in_progress"
  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&status=done"

  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&priority=low"
  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&priority=medium"
  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&priority=high"

  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&sort_by=created_at&sort_order=desc"
  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&sort_by=due_date&sort_order=desc"

  "/api/v1/projects/${PROJECT_ID}/tasks?page=1&page_size=${PAGE_SIZE}&assignee_id=0"
)

echo "start prewarm..."

# 多轮预热，保证 page cache、task item cache、user brief cache 都写入 Redis
for round in $(seq 1 5); do
  echo "prewarm round: ${round}"

  for path in "${HOT_PATHS[@]}"; do
    curl -s -o /dev/null \
      -H "Authorization: Bearer ${TOKEN}" \
      "${BASE_URL}${path}"
  done
done

echo "prewarm done."