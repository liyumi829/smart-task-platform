-- project_tasks_90hit.lua
--
-- 目标：
--   使用 wrk 构造 90% 热点请求 + 10% 冷请求，
--   用于测试 Redis 缓存命中率接近 90% 时的接口 QPS。
--
-- 接口：
--   GET /api/v1/projects/{project_id}/tasks?page=1&page_size=20
--
-- 参数约束：
--   page >= 1
--   10 <= page_size <= 50
--
-- 使用方式：
--   export TOKEN="<token>"
--   export HOT_RATIO=90
--   export PROJECT_ID=1
--   export PAGE_SIZE=20
--
--   taskset -c 0 wrk -t8 -c200 -d45s -L \
--     -s project_tasks_90hit.lua \
--     -H "Authorization: Bearer ${TOKEN}" \
--     "http://127.0.0.1:8080"

local hot_ratio = tonumber(os.getenv("HOT_RATIO") or "90")
local hot_project_id = tonumber(os.getenv("PROJECT_ID") or "1")
local page_size = tonumber(os.getenv("PAGE_SIZE") or "20")

-- 控制 hot_ratio 范围，避免传入非法值
if hot_ratio < 0 then
    hot_ratio = 0
end

if hot_ratio > 100 then
    hot_ratio = 100
end

-- 控制 page_size 范围，必须满足项目约束：10 <= page_size <= 50
if page_size < 10 then
    page_size = 10
end

if page_size > 50 then
    page_size = 50
end

-- 控制 page 范围，必须满足项目约束：page >= 1
local function fix_page(page)
    if page == nil or page < 1 then
        return 1
    end
    return page
end

-- 每个 wrk 线程自己的计数器，用于生成冷请求唯一 keyword
local counter = 0

-- 热点请求池：
-- 这些请求会被提前预热，正式压测时 90% 的请求从这里随机选择。
-- 所有 page 都 >= 1，page_size 都在 10~50 范围内。
local hot_paths = {
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size,
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=2&page_size=" .. page_size,
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=3&page_size=" .. page_size,

    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&status=todo",
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&status=in_progress",
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&status=done",

    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&priority=low",
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&priority=medium",
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&priority=high",

    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&sort_by=created_at&sort_order=desc",
    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&sort_by=due_date&sort_order=desc",

    "/api/v1/projects/" .. hot_project_id .. "/tasks?page=1&page_size=" .. page_size .. "&assignee_id=0",
}

-- 构造热点请求：
-- 热点请求来自 hot_paths，这些 key 在正式压测前应该已经被预热。
local function build_hot_path()
    local idx = math.random(1, #hot_paths)
    return hot_paths[idx]
end

-- 构造冷请求：
-- 通过唯一 keyword 让每次冷请求产生一个新的缓存 key。
-- 这样冷请求大概率 Redis miss。
local function build_cold_path()
    counter = counter + 1

    local page = fix_page(math.random(1, 10))
    local keyword = "cold_" .. tostring(counter) .. "_" .. tostring(math.random(100000000))

    return "/api/v1/projects/" .. hot_project_id ..
        "/tasks?page=" .. page ..
        "&page_size=" .. page_size ..
        "&keyword=" .. keyword ..
        "&sort_by=due_date&sort_order=desc"
end

-- wrk 每次请求都会执行 request()
request = function()
    local n = math.random(1, 100)

    -- 90% 请求热点 key，理论上命中 Redis
    if n <= hot_ratio then
        return wrk.format("GET", build_hot_path())
    end

    -- 10% 请求冷 key，理论上 miss Redis
    return wrk.format("GET", build_cold_path())
end

-- 统计响应状态码，方便确认压测过程中是否有大量错误
local responses = {
    ok_2xx = 0,
    err_4xx = 0,
    err_5xx = 0,
    other = 0,
}

response = function(status, headers, body)
    if status >= 200 and status < 300 then
        responses.ok_2xx = responses.ok_2xx + 1
    elseif status >= 400 and status < 500 then
        responses.err_4xx = responses.err_4xx + 1
    elseif status >= 500 and status < 600 then
        responses.err_5xx = responses.err_5xx + 1
    else
        responses.other = responses.other + 1
    end
end

done = function(summary, latency, requests)
    io.write("\n========== custom stats ==========\n")
    io.write("hot_ratio: " .. tostring(hot_ratio) .. "%\n")
    io.write("hot_project_id: " .. tostring(hot_project_id) .. "\n")
    io.write("page_size: " .. tostring(page_size) .. "\n")
    io.write("2xx: " .. tostring(responses.ok_2xx) .. "\n")
    io.write("4xx: " .. tostring(responses.err_4xx) .. "\n")
    io.write("5xx: " .. tostring(responses.err_5xx) .. "\n")
    io.write("other: " .. tostring(responses.other) .. "\n")
    io.write("==================================\n")
end