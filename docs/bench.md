## 测试环境

* 云服务器
* 4核
* 4G


## 第一版本测压（无缓存）


数据：
```sql
/*
  压测专用数据库初始化脚本

  数据库名称：
  smart_task_platform_benchmark

  数据规模：
  users:           100
  projects:        20
  project_members: 500
  tasks:           5,000
  task_comments:   20,000
*/
```

### 命令

```bash
wrk -t4 -c100 -d60s -L \
  -H "Authorization: Bearer <token>" \
  "http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10"
```

### 测试步骤

```txt
1. 重复 -c100 测试 2~3 次，确认结果稳定
2. 继续测试 -c200
3. 继续测试 -c300
4. 继续测试 -c500
5. 每次记录 QPS、Avg Latency、Max Latency、错误码
6. 同时观察数据库和应用服务资源
7. 完成无缓存基准后，再加入 Redis 权限缓存
8. 使用同样并发梯度重新压测
9. 对比优化前后结果
```

测试账号与 Token

|用途	|username	|user_id	|project_id	|role	|token 是否有效
|项目 Owner|	bench_user_001	|1	|1	|owner	|是 / 否
|项目成员	|bench_user_002	|2	|1	|admin/member|	是 / 否
|普通成员	|bench_user_006	|6	|1	|member	|是 / 否
|非项目成员	|bench_user_026	|26	|1	|非成员|	是 / 否

### 测试

#### -c100

##### 测试结果一

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  4 threads and 100 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    88.21ms   42.53ms 658.79ms   72.78%
    Req/Sec   288.45     37.78   470.00     70.52%
  Latency Distribution
     50%   87.62ms
     75%  111.33ms
     90%  135.18ms
     99%  211.42ms
  68985 requests in 1.00m, 224.67MB read
Requests/sec:   1148.60
Transfer/sec:      3.74MB
```

##### 测试结果二

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  4 threads and 100 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    88.11ms   40.41ms 451.28ms   71.78%
    Req/Sec   288.16     38.33   440.00     72.22%
  Latency Distribution
     50%   86.46ms
     75%  109.93ms
     90%  135.78ms
     99%  210.15ms
  68925 requests in 1.00m, 224.47MB read
Requests/sec:   1147.08
Transfer/sec:      3.74MB
```

#### -c200


##### 测试结果一

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   171.53ms   51.95ms 591.50ms   69.59%
    Req/Sec   146.38     28.32   232.00     71.70%
  Latency Distribution
     50%  165.66ms
     75%  202.12ms
     90%  240.19ms
     99%  318.87ms
  69958 requests in 1.00m, 227.84MB read
Requests/sec:   1164.32
Transfer/sec:      3.79MB
```


##### 测试结果二

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   171.81ms   52.74ms 507.05ms   69.49%
    Req/Sec   145.91     28.90   242.00     70.50%
  Latency Distribution
     50%  165.62ms
     75%  202.99ms
     90%  242.05ms
     99%  323.07ms
  69839 requests in 1.00m, 227.45MB read
Requests/sec:   1162.08
Transfer/sec:      3.78MB
```

##### 测试结果三（索引优化+查询优化）

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    99.30ms   34.06ms 366.26ms   69.55%
    Req/Sec   253.24     29.71   620.00     71.52%
  Latency Distribution
     50%   95.19ms
     75%  118.81ms
     90%  144.52ms
     99%  197.84ms
  121052 requests in 1.00m, 396.20MB read
Requests/sec:   2014.43
Transfer/sec:      6.59MB
```
#### 测试结果四（索引优化+查询优化）

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    95.47ms   32.91ms 359.92ms   70.07%
    Req/Sec   263.59     30.58   363.00     68.94%
  Latency Distribution
     50%   91.08ms
     75%  114.55ms
     90%  138.96ms
     99%  191.87ms
  125974 requests in 1.00m, 412.31MB read
Requests/sec:   2096.80
Transfer/sec:      6.86MB
```

#### -c300


##### 测试结果一

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 300 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   140.33ms   50.97ms 536.72ms   69.65%
    Req/Sec   265.43     35.31   390.00     67.99%
  Latency Distribution
     50%  134.07ms
     75%  169.91ms
     90%  207.84ms
     99%  288.11ms
  126787 requests in 1.00m, 414.98MB read
Requests/sec:   2110.53
Transfer/sec:      6.91MB
```


##### 测试结果二

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 300 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   145.72ms   52.56ms 517.47ms   69.57%
    Req/Sec   255.31     34.94   373.00     69.44%
  Latency Distribution
     50%  138.95ms
     75%  176.20ms
     90%  215.54ms
     99%  297.37ms
  122096 requests in 1.00m, 399.62MB read
Requests/sec:   2032.63
Transfer/sec:      6.65MB
```


#### -c500


##### 测试结果一

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 500 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   240.19ms   90.95ms 860.62ms   69.29%
    Req/Sec   259.83     41.26   424.00     71.08%
  Latency Distribution
     50%  228.51ms
     75%  293.43ms
     90%  361.60ms
     99%  498.99ms
  124110 requests in 1.00m, 406.21MB read
Requests/sec:   2065.73
Transfer/sec:      6.76MB
```


##### 测试结果二

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 500 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   247.62ms   94.80ms 978.85ms   69.76%
    Req/Sec   252.07     42.53   414.00     72.15%
  Latency Distribution
     50%  235.09ms
     75%  302.25ms
     90%  373.96ms
     99%  521.66ms
  120453 requests in 1.00m, 394.24MB read
Requests/sec:   2004.67
Transfer/sec:      6.56MB
```

#### -c800

##### 测试结果一

```txt
  "http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10"
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 800 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   399.14ms  155.87ms   1.52s    69.77%
    Req/Sec   252.27     50.79   464.00     70.10%
  Latency Distribution
     50%  378.88ms
     75%  489.47ms
     90%  606.52ms
     99%  853.31ms
  120376 requests in 1.00m, 393.99MB read
Requests/sec:   2002.92
Transfer/sec:      6.56MB
```

##### 测试结果二

```txt
Running 1m test @ http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10
  8 threads and 800 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   400.59ms  155.97ms   1.61s    69.63%
    Req/Sec   251.10     48.99   420.00     68.66%
  Latency Distribution
     50%  380.68ms
     75%  491.72ms
     90%  607.95ms
     99%  850.97ms
  119909 requests in 1.00m, 392.46MB read
Requests/sec:   1995.68
Transfer/sec:      6.53MB
```

### 结果说明

1. 当前系统健康承载能力：
   c200 左右，QPS 约 2000，P99 约 200ms。

2. 当前系统吞吐上限：
   约 2000 ~ 2100 QPS。

3. c300、c500、c800 并没有提升 QPS，只是增加延迟：
   说明系统已经进入吞吐瓶颈区。

4. 数据库当前不是瓶颈：
   DB 活跃连接低于 10，说明主要瓶颈大概率在应用层 CPU、GORM、JSON、JWT、中间件、日志或压测端。

5. 下一步最关键：
   用 pprof + CPU 监控确认热点，不要靠猜。

#### 测试方法

1. 程序绑定CPU(1-3):"sudo taskset -pc 1-3 $(pidof smart-task-platform)"
2. CPU观察：`pidstat -p $(pidof smart-task-platform),$(pidof mysqld) -u -r 1` 
3. 测压方式：
```bash
taskset -c 0 wrk -t8 -c200 -d60s -L \
  -H "Authorization: Bearer <token>" \
  "http://127.0.0.1:8080/api/v1/projects/1/tasks?page=1&page_size=10"
```
4. `ppof`观察：

  * `curl -o cpu_c200.nolog.pprof "http://127.0.0.1:6060/debug/pprof/profile?seconds=30"`
  * `go tool pprof cpu_c200.nolog.pprof`
  * `top`
  * `top cum`

例如：
```txt
(pprof) top
Showing nodes accounting for 19530ms, 39.81% of 49060ms total
Dropped 924 nodes (cum <= 245.30ms)
Showing top 10 nodes out of 315
      flat  flat%   sum%        cum   cum%
   12480ms 25.44% 25.44%    12480ms 25.44%  internal/runtime/syscall.Syscall6
     970ms  1.98% 27.42%      970ms  1.98%  runtime.(*mspan).base (inline)
     870ms  1.77% 29.19%     2510ms  5.12%  runtime.scanobject
     850ms  1.73% 30.92%     1020ms  2.08%  runtime.findObject
     800ms  1.63% 32.55%      800ms  1.63%  runtime.nextFreeFast
     800ms  1.63% 34.18%     1920ms  3.91%  runtime.selectgo
     740ms  1.51% 35.69%      800ms  1.63%  runtime.lock2
     710ms  1.45% 37.14%      710ms  1.45%  internal/runtime/atomic.(*Uint32).CompareAndSwap (inline)
     690ms  1.41% 38.54%      720ms  1.47%  runtime.unlock2
     620ms  1.26% 39.81%      680ms  1.39%  runtime.(*mspan).writeHeapBitsSmall
(pprof) top -cum
Showing nodes accounting for 0.43s, 0.88% of 49.06s total
Dropped 924 nodes (cum <= 0.25s)
Showing top 10 nodes out of 315
      flat  flat%   sum%        cum   cum%
     0.04s 0.082% 0.082%     40.05s 81.63%  net/http.(*conn).serve
         0     0% 0.082%     38.21s 77.88%  github.com/gin-gonic/gin.(*Engine).ServeHTTP
         0     0% 0.082%     38.21s 77.88%  net/http.serverHandler.ServeHTTP
     0.01s  0.02%   0.1%     38.17s 77.80%  github.com/gin-gonic/gin.(*Engine).handleHTTPRequest
     0.04s 0.082%  0.18%     38.13s 77.72%  github.com/gin-gonic/gin.(*Context).Next
     0.01s  0.02%   0.2%     38.13s 77.72%  github.com/gin-gonic/gin.CustomRecoveryWithWriter.func1
     0.03s 0.061%  0.26%     38.10s 77.66%  smart-task-platform/internal/api/router.RegisterTaskRoutes.JWTAuth.func1
     0.04s 0.082%  0.35%     35.16s 71.67%  smart-task-platform/internal/api/handler.(*TaskHandler).ListProjectTasks
     0.07s  0.14%  0.49%     31.19s 63.58%  smart-task-platform/internal/service.(*TaskService).ListProjectTasks
     0.19s  0.39%  0.88%     27.77s 56.60%  gorm.io/gorm.(*processor).Execute
```

程序CPU使用率：
```txt
Duration: 30.20s
Total samples: 49.06s
Go 应用 CPU 使用率: 162.46%
```

现象：
1. Go CPU 只有 162% 远没跑满 4 核
2. MySQL 活跃连接很低，但每秒 SQL 量巨大（单请求 6 条，QPS2000 → 12000 条 / 秒 SQL）
3. pprof 大量 syscall.Syscall6 都是等待 MySQL 网络 IO

结论：MySQL + Go 应用共同构成瓶颈，其中 MySQL 压力很明显。

* 瓶颈并非 Go 应用 CPU 算力不足，也不是 MySQL 慢查询 / 连接打满；而是MySQL 数据访问链路瓶颈 + Go 应用 GORM 查询处理开销。

* 当前接口 QPS 卡在 2000 左右，主要不是 Go 应用 CPU 打满，而是单机部署下 MySQL 承担了过多高频重复查询；单次请求 6 条 SQL 导致 MySQL 每秒处理约 1.2 万条 SQL，mysqld CPU 接近 180%，因此下一步应引入 Redis 缓存权限、用户和项目基础信息，把请求链路中的重复 SQL 降下来。

最后：
1. 不属于 CPU 瓶颈
2. 不属于 内存瓶颈
3. 不属于 网络瓶颈
4. **属于 高频短查询 IO 瓶颈 + 无缓存架构瓶颈**


## 第二版测压（缓存）

缓存命中率测试：

1. 0% ~ 10%   ：看 DB 回源能力、缓存重建能力、singleflight 效果
2. 50%        ：看混合场景性能
3. 80% ~ 90%  ：比较接近真实线上
4. 95% ~ 99%+ ：看缓存热数据场景上限

### 测试

#### -200

##### 第一次

```txt
Running 1m test @ http://127.0.0.1:8080
  8 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    38.32ms   15.75ms 172.86ms   72.77%
    Req/Sec   659.66     65.18     0.93k    72.40%
  Latency Distribution
     50%   36.34ms
     75%   46.64ms
     90%   58.57ms
     99%   86.06ms
  315436 requests in 1.00m, 1.32GB read
Requests/sec:   5252.69
Transfer/sec:     22.50MB
```

缓存命中率：

```txt
========== redis hit rate ==========
before_hits:   0
before_misses: 1
after_hits:    9388483
after_misses:  362023
delta_hits:    9388483
delta_misses:  362022
hit_rate:      96.29%
====================================
```

##### 第二次

```txt
Running 1m test @ http://127.0.0.1:8080
  8 threads and 200 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    38.34ms   15.89ms 181.40ms   72.48%
    Req/Sec   659.32     63.43     0.91k    69.77%
  Latency Distribution
     50%   36.36ms
     75%   46.85ms
     90%   58.89ms
     99%   86.24ms
  315283 requests in 1.00m, 1.32GB read
Requests/sec:   5249.33
Transfer/sec:     22.50MB
```

#### 描述

* 针对任务列表查询场景引入 Redis 缓存优化，采用短 TTL + 查询参数维度缓存方案降低重复查询开销；在单机压测环境下，混合热点流量场景 Redis 命中率稳定在 95% 左右，接口 QPS 提升约 2.6 倍，平均延迟降低约 60%。
* 针对项目任务列表等读多写少场景引入 Redis 缓存，基于查询参数组合缓存分页列表、任务项及用户简要信息，并通过 wrk 构造热点混合流量进行压测；在单机 200 并发下，Redis keyspace 命中率约 95%，接口 QPS 从约 2.0k 提升至约 5.2k，平均响应时间从 99ms 降低至 38ms，P99 从 197ms 降低至 86ms。


#### -300

##### 第一次

```txt
Running 1m test @ http://127.0.0.1:8080
  8 threads and 300 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    56.90ms   16.10ms 201.20ms   73.07%
    Req/Sec   653.61     64.86     0.92k    73.78%
  Latency Distribution
     50%   55.47ms
     75%   65.32ms
     90%   77.06ms
     99%  104.35ms
  312420 requests in 1.00m, 1.31GB read
Requests/sec:   5198.53
Transfer/sec:     22.29MB
````

1. 优化前后性能对比
2. Redis 缓存命中率
3. 200 并发下的稳定表现
4. 300 并发下 QPS 进入平台期
5. CPU 接近 3 核瓶颈
6. 能说明瓶颈从 MySQL 转移到了应用层 CPU


* 针对任务列表等读多写少接口引入 Redis 缓存，采用查询参数组合 key + 短 TTL 的方式缓存分页结果，并通过 wrk 构造热点混合流量进行压测；在 Redis 命中率约 95%~96% 的情况下，接口 QPS 从约 2.0k 提升至约 5.2k，平均响应时间从约 99ms 降至约 38ms，P99 从约 197ms 降至约 86ms。在 300 并发测试中观察到 QPS 基本进入平台期，结合 CPU 接近 3 核上限，判断当前单机瓶颈主要转移至应用层 CPU。

* 我没有继续盲目提高并发，因为在 200 到 400 并发之间，QPS 基本维持在 5.2k 左右，但平均延迟和 P99 明显上升，同时服务进程 CPU 已经接近 3 核打满。所以我判断当前单机吞吐已经接近上限，继续加并发不会带来有效吞吐提升，只会增加排队延迟。这个阶段优化目标已经从数据库访问优化转移到应用层 CPU 开销，比如 JSON 编解码、对象分配、日志和框架处理成本。