# User 模块接口测试文档模板

## 1. 文档说明

本文档用于测试 `User` 用户模块接口是否符合设计预期，覆盖以下接口：

- 获取用户公开信息
- 更新当前用户资料
- 修改当前用户密码
- 搜索用户列表

使用 Postman 完成测试。

---

## 2. 通用规范

### 2.1 接口前缀

```http
/api/v1
```

### 2.2 认证方式

需要登录的接口统一使用 JWT：

```http
Authorization: Bearer <access_token>
```

### 2.3 统一成功响应

```json
{
  "code": 0,
  "message": "Success",
  "data": {}
}
```

### 2.4 统一失败响应

```json
{
  "code": 10001,
  "message": "invalid params",
  "data": null
}
```

---

## 3. 测试环境

### 3.1 服务地址

```text
http://127.0.0.1:8080
```

### 3.2 测试变量

| 变量名 | 示例值 | 说明 |
|---|---|---|
| `base_url` | `http://127.0.0.1:8080` | 服务地址 |
| `access_token` | 登录后获取 | 当前用户访问令牌 |
| `user_id` | `1` | 目标用户 ID |
| `nickname` | `张三` | 用户昵称 |
| `new_nickname` | `新的昵称` | 更新后的昵称 |
| `avatar` | `https://example.com/avatar.png` | 用户头像 |
| `new_avatar` | `https://example.com/new_avatar.png` | 更新后的头像 |
| `old_password` | `12345678ll` | 旧密码 |
| `new_password` | `NewPassword123456!` | 新密码 |
| `keyword` | `zhang` | 搜索关键字 |
| `page` | `1` | 页码 |
| `page_size` | `10` | 每页数量 |

---

## 4. DTO 对象说明

### 4.1 UserPublicInfoResp

```json
{
  "id": 1,
  "username": "zhangsan",
  "nickname": "张三",
  "avatar": "https://example.com/avatar.png"
}
```

### 4.2 UpdateUserProfileReq

```json
{
  "nickname": "新的昵称",
  "avatar": "https://example.com/new_avatar.png"
}
```

### 4.3 UpdateUserProfileResp

```json
{
  "id": 1,
  "username": "zhangsan",
  "nickname": "新的昵称",
  "avatar": "https://example.com/new_avatar.png"
}
```

### 4.4 UpdateUserPasswordReq

```json
{
  "old_password": "12345678ll",
  "new_password": "NewPassword123456!"
}
```

### 4.5 UpdateUserPasswordResp

```json
{
  "updated": true
}
```

> 如果当前 DTO 实际不是该结构，请以项目中的真实 DTO 为准。

### 4.6 SearchUserListResp

```json
{
  "list": [
    {
      "id": 1,
      "username": "zhangsan",
      "nickname": "张三",
      "avatar": "https://example.com/avatar.png"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 10
}
```

---

# 5. 接口测试用例

---

## 5.1 获取用户公开信息

### 接口信息

- **接口名称**：获取用户公开信息
- **请求方式**：`GET`
- **请求路径**：`/api/v1/users/{id}`

### 请求头

无

### 路径参数

| 参数名 | 示例值 | 必填 | 说明 |
|---|---|---|---|
| `id` | `1` | 是 | 用户 ID |

### 请求示例

```http
GET /api/v1/users/1
```

### 成功响应示例

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "id": 1,
    "username": "zhangsan",
    "nickname": "张三",
    "avatar": "https://example.com/avatar.png"
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `message = Success`
- `data.id` 存在
- `data.username` 存在
- `data.nickname` 存在
- `data.avatar` 为合法 URL 或为空字符串（按实际业务）

### 异常测试

#### 用户不存在

```http
GET /api/v1/users/999999
```

#### 用户 ID 非法

```http
GET /api/v1/users/abc
```

### 异常响应示例

```json
{
  "code": 20001,
  "message": "user not found",
  "data": null
}
```

或参数错误：

```json
{
  "code": 10001,
  "message": "invalid params",
  "data": null
}
```

---

## 5.2 更新当前用户资料

### 接口信息

- **接口名称**：更新当前用户资料
- **请求方式**：`PUT`
- **请求路径**：`/api/v1/users/me`

### 请求头

```http
Authorization: Bearer <access_token>
Content-Type: application/json
```

### 请求体

```json
{
  "nickname": "新的昵称",
  "avatar": "https://example.com/new_avatar.png"
}
```

### 成功响应示例

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "id": 1,
    "username": "zhangsan",
    "nickname": "新的昵称",
    "avatar": "https://example.com/new_avatar.png"
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `message = Success`
- `data.id` 存在
- `data.username` 存在
- `data.nickname = 新的昵称`
- `data.avatar = https://example.com/new_avatar.png`

### 异常测试

#### 不带 Token

无 `Authorization` 请求头

#### Token 非法

```http
Authorization: Bearer invalid_token
```

#### 昵称格式不合法

```json
{
  "nickname": "@",
  "avatar": "https://example.com/new_avatar.png"
}
```

#### 头像 URL 非法

```json
{
  "nickname": "新的昵称",
  "avatar": "avatar.png"
}
```

#### 用户不存在

当前 token 对应用户已被删除

#### 用户被禁用

当前 token 对应用户状态为禁用

### 异常响应示例

未登录：

```json
{
  "code": 10003,
  "message": "unauthorized",
  "data": null
}
```

参数错误：

```json
{
  "code": 10001,
  "message": "invalid params",
  "data": null
}
```

业务错误：

```json
{
  "code": 20001,
  "message": "user not found",
  "data": null
}
```

```json
{
  "code": 20002,
  "message": "user disabled",
  "data": null
}
```

```json
{
  "code": 20004,
  "message": "invalid avatar format",
  "data": null
}
```

---

## 5.3 修改当前用户密码

### 接口信息

- **接口名称**：修改当前用户密码
- **请求方式**：`PATCH`
- **请求路径**：`/api/v1/users/me/password`

### 请求头

```http
Authorization: Bearer <access_token>
Content-Type: application/json
```

### 请求体

```json
{
  "old_password": "12345678ll",
  "new_password": "NewPassword123456!"
}
```

### 成功响应示例

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "updated": true
  }
}
```

> 如果当前 DTO 返回空对象或其他字段，请以实际 DTO 为准。

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `message = Success`
- `data.updated = true` 或返回结构符合 DTO 定义
- 修改成功后，旧密码不能再登录
- 修改成功后，新密码可以登录

### 异常测试

#### 不带 Token

无 `Authorization` 请求头

#### Token 非法

```http
Authorization: Bearer invalid_token
```

#### 旧密码错误

```json
{
  "old_password": "wrongpassword",
  "new_password": "NewPassword123456!"
}
```

#### 新密码格式不合法

```json
{
  "old_password": "12345678ll",
  "new_password": "123"
}
```

#### 新密码与旧密码相同

```json
{
  "old_password": "12345678ll",
  "new_password": "12345678ll"
}
```

#### 用户不存在

当前 token 对应用户已被删除

#### 用户被禁用

当前 token 对应用户状态为禁用

### 异常响应示例

未登录：

```json
{
  "code": 10003,
  "message": "unauthorized",
  "data": null
}
```

旧密码错误：

```json
{
  "code": 20005,
  "message": "old password mismatch",
  "data": null
}
```

新密码非法：

```json
{
  "code": 10001,
  "message": "invalid params",
  "data": null
}
```

或：

```json
{
  "code": 20006,
  "message": "invalid password format",
  "data": null
}
```

新旧密码相同：

```json
{
  "code": 20007,
  "message": "new password same as old",
  "data": null
}
```

---

## 5.4 搜索用户列表

### 接口信息

- **接口名称**：搜索用户列表
- **请求方式**：`GET`
- **请求路径**：`/api/v1/users`

### 请求头

无

### Query 参数

| 参数名 | 示例值 | 必填 | 说明 |
|---|---|---|---|
| `keyword` | `zhang` | 否 | 搜索关键字，用户名或昵称模糊匹配 |
| `page` | `1` | 否 | 页码，默认 1 |
| `page_size` | `10` | 否 | 每页数量，默认值以项目配置为准 |

### 请求示例

```http
GET /api/v1/users/list?keyword=zhang&page=1&page_size=10
```

### 成功响应示例

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "list": [
      {
        "id": 1,
        "username": "zhangsan",
        "nickname": "张三",
        "avatar": "https://example.com/avatar.png"
      },
      {
        "id": 2,
        "username": "zhangwuji",
        "nickname": "张无忌",
        "avatar": "https://example.com/avatar2.png"
      }
    ],
    "total": 2,
    "page": 1,
    "page_size": 10
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `message = Success`
- `data.list` 为数组
- `data.total >= 0`
- `data.page = 请求值或默认值`
- `data.page_size = 请求值或默认值`
- `list` 中每个元素包含：
  - `id`
  - `username`
  - `nickname`
  - `avatar`

### 异常测试

#### 空关键字

```http
GET /api/v1/users/list?keyword=   &page=1&page_size=10
```

> 预期一般为成功返回空列表，不应报错。

#### page 非法

```http
GET /api/v1/users/list?keyword=zhang&page=0&page_size=10
```

#### page_size 非法

```http
GET /api/v1/users/list?keyword=zhang&page=1&page_size=0
```

#### page_size 超过最大值

```http
GET /api/v1/users/list?keyword=zhang&page=1&page_size=9999
```

### 异常/边界响应示例

空关键字返回空列表：

```json
{
  "code": 0,
  "message": "Success",
  "data": {
    "list": [],
    "total": 0,
    "page": 1,
    "page_size": 10
  }
}
```

---

# 6. 推荐测试顺序

```text
1. 注册并登录，获取 access_token
2. 获取用户公开信息
3. 更新当前用户资料
4. 再次获取用户公开信息，验证资料已更新
5. 搜索用户列表，验证用户名搜索命中
6. 修改当前用户密码
7. 使用旧密码登录，预期失败
8. 使用新密码登录，预期成功
```

---

# 7. 测试检查清单

## 7.1 获取用户公开信息

- [x] 能根据用户 ID 正常查询
- [x] 用户不存在时能正确返回错误
- [x] 非法用户 ID 能正确返回参数错误

## 7.2 更新当前用户资料

- [x] 合法 token 可更新资料
- [x] 非法 token 被拒绝
- [x] 未登录被拒绝
- [x] 昵称校验正确
- [x] 头像 URL 校验正确

## 7.3 修改当前用户密码

- [x] 合法 token 可修改密码
- [x] 旧密码错误时被拒绝
- [x] 新密码格式错误时被拒绝
- [x] 新旧密码相同时被拒绝
- [x] 修改成功后旧密码失效
- [x] 修改成功后新密码生效

## 7.4 搜索用户列表

- [x] 关键字搜索正常
- [x] 昵称搜索正常
- [x] 空关键字返回空列表
- [x] page 默认值正确
- [x] page_size 默认值正确
- [x] page_size 超最大值时自动兜底

---

# 8. 当前接口一致性说明

## 8.1 获取用户公开信息接口

当前接口返回用户公开字段，一般包含：

- `id`
- `username`
- `nickname`
- `avatar`

不应返回敏感字段，例如：

- `password_hash`
- `email`（如果设计上属于隐私字段）
- 其他内部状态字段

## 8.2 更新资料接口

当前接口应以 DTO 为准，通常返回更新后的用户公开摘要信息：

- `id`
- `username`
- `nickname`
- `avatar`

## 8.3 修改密码接口

当前接口只表示密码是否更新成功，不应返回密码相关敏感信息。

## 8.4 搜索用户列表接口

当前搜索接口应返回用户公开信息列表，不应返回敏感数据字段。

---

# 9. 逻辑结构展示

```text
User 模块接口测试逻辑
/api/v1/users
├── GET    /{id}
├── PUT    /profile
├── PUT    /password
└── GET    /list
```

---

# 10. 备注

- 成功响应统一格式：

```json
{
  "code": 0,
  "message": "Success",
  "data": {}
}
```

- 失败响应统一格式：

```json
{
  "code": 10001,
  "message": "invalid params",
  "data": null
}
```

- 需要登录的接口统一使用：

```http
Authorization: Bearer <access_token>
```