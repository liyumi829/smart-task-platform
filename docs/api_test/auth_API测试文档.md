# Auth 模块接口测试文档模板

## 1. 文档说明

本文档用于测试 `Auth` 认证模块接口是否符合设计预期，覆盖以下接口：

- 用户注册
- 用户登录
- 退出登录
- 获取当前登录用户信息
- 刷新访问令牌

使用 postman 完成测试

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
  "message": "success",
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

本地 postman 测试，在云服务器上的服务器监听地址打开

```text
http://127.0.0.1:8080
```

### 3.2 测试变量

| 变量名 | 示例值 | 说明 |
|---|---|---|
| `base_url` | `http://127.0.0.1:8080` | 服务地址 |
| `username` | `zhangsan` | 测试用户名 |
| `email` | `zhangsan@example.com` | 测试邮箱 |
| `password` | `12345678ll` | 测试密码 |
| `access_token` | 登录后获取 | 访问令牌 |
| `refresh_token` | 登录后获取 | 刷新令牌 |

---

## 4. DTO 对象说明

### 4.1 RegisterReq

```json
{
  "username": "zhangsan",
  "email": "zhangsan@example.com",
  "password": "12345678ll",
  "nickname": "张三"
}
```

### 4.2 RegisterResp

```json
{
  "id": 1,
  "username": "zhangsan",
  "email": "zhangsan@example.com",
  "nickname": "张三"
}
```

### 4.3 LoginReq

```json
{
  "account": "zhangsan",
  "password": "12345678ll"
}
```

### 4.4 LoginResp

```json
{
  "access_token": "jwt_access_token",
  "refresh_token": "jwt_refresh_token",
  "token_type": "Bearer",
  "expires_in": 3600,
  "user": {
    "id": 1,
    "username": "zhangsan",
    "nickname": "张三",
    "avatar": "https://xxx.com/avatar.png"
  }
}
```

### 4.5 LogoutReq

```json
{}
```

### 4.6 LogoutResp

```json
{
  "logged_out": true
}
```

### 4.7 MeResp

```json
{
  "id": 1,
  "username": "zhangsan",
  "nickname": "张三",
  "email": "zhangsan@example.com",
  "avatar": "https://xxx.com/avatar.png"
}
```

### 4.8 RefreshTokenReq

```json
{
  "refresh_token": "jwt_refresh_token"
}
```

### 4.9 RefreshTokenResp


```json
{
  "access_token": "new_jwt_access_token",
  "refresh_token":"new_jwt_refresh_token",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

---

# 5. 接口测试用例

---

## 5.1 用户注册

### 接口信息

- **接口名称**：用户注册
- **请求方式**：`POST`
- **请求路径**：`/api/v1/auth/register`

### 请求头

```http
Content-Type: application/json
```

### 请求体

```json
{
  "username": "zhangsan",
  "email": "zhangsan@example.com",
  "password": "12345678ll",
  "nickname": "张三"
}
```

### 成功响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1,
    "username": "zhangsan",
    "email": "zhangsan@example.com",
    "nickname": "张三"
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `message = success`
- `data.id` 存在
- `data.username` 与请求一致
- `data.email` 与请求一致

### 异常测试

#### 用户名太短

```json
{
  "username": "ab",
  "email": "zhangsan@example.com",
  "password": "12345678ll",
  "nickname": "张三"
}
```

#### 邮箱格式错误

```json
{
  "username": "zhangsan",
  "email": "invalid_email",
  "password": "12345678ll",
  "nickname": "张三"
}
```

#### 密码太短

```json
{
  "username": "zhangsan",
  "email": "zhangsan@example.com",
  "password": "123",
  "nickname": "张三"
}
```

### 异常响应示例

```json
{
  "code": 10001,
  "message": "invalid params",
  "data": null
}
```

---

## 5.2 用户登录

### 接口信息

- **接口名称**：用户登录
- **请求方式**：`POST`
- **请求路径**：`/api/v1/auth/login`

### 请求头

```http
Content-Type: application/json
```

### 请求体

```json
{
  "account": "zhangsan",
  "password": "12345678ll"
}
```

> `account` 支持用户名或邮箱

### 成功响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "jwt_access_token",
    "refresh_token": "jwt_refresh_token",
    "token_type": "Bearer",
    "expires_in": 3600,
    "user": {
      "id": 1,
      "username": "zhangsan",
      "nickname": "张三",
      "avatar": "https://xxx.com/avatar.png"
    }
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `data.access_token` 存在
- `data.refresh_token` 存在
- `data.token_type = Bearer`
- `data.expires_in > 0`
- `data.user.id` 存在
- 登录成功后保存：
  - `access_token`
  - `refresh_token`

### 异常测试

#### 用户名不存在

```json
{
  "account": "not_exists_user",
  "password": "12345678ll"
}
```

#### 密码错误

```json
{
  "account": "zhangsan",
  "password": "wrongpassword"
}
```

### 异常响应示例

```json
{
  "code": 20003,
  "message": "invalid account or password",
  "data": null
}
```

---

## 5.3 获取当前登录用户信息

### 接口信息

- **接口名称**：获取当前用户
- **请求方式**：`GET`
- **请求路径**：`/api/v1/auth/me`

### 请求头

```http
Authorization: Bearer <access_token>
```

### 请求体

无

### 成功响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1,
    "username": "zhangsan",
    "nickname": "张三",
    "email": "zhangsan@example.com",
    "avatar": "https://xxx.com/avatar.png"
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `data.id` 存在
- `data.username` 存在
- `data.email` 存在

### 异常测试

#### 不带 Token

无 `Authorization` 请求头

#### Token 非法

```http
Authorization: Bearer invalid_token
```

#### Token 过期

使用过期的 `access_token`

### 异常响应示例

```json
{
  "code": 10003,
  "message": "unauthorized",
  "data": null
}
```

---

## 5.4 刷新访问令牌

### 接口信息

- **接口名称**：刷新访问令牌
- **请求方式**：`POST`
- **请求路径**：`/api/v1/auth/refresh`

### 请求头

```http
Content-Type: application/json
```

### 请求体

```json
{
  "refresh_token": "jwt_refresh_token"
}
```

### 成功响应示例

> 当前按 DTO 为准，不返回新的 `refresh_token`

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "new_jwt_access_token",
    "refresh_token": "new_jwt_refresh_token",
    "token_type": "Bearer",
    "expires_in": 3600
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `data.access_token` 存在
- `data.refresh_token` 存在
- `data.token_type = Bearer`
- `data.expires_in > 0`
- 刷新成功后更新本地 `access_token`

### 异常测试

#### refresh_token 为空

```json
{
  "refresh_token": ""
}
```

#### refresh_token 非法

```json
{
  "refresh_token": "invalid_refresh_token"
}
```

#### refresh_token 过期

使用过期的 `refresh_token`

### 异常响应示例

```json
{
  "code": 10002,
  "message": "refresh token invalid or expired",
  "data": null
}
```

---

## 5.5 退出登录

### 接口信息

- **接口名称**：退出登录
- **请求方式**：`POST`
- **请求路径**：`/api/v1/auth/logout`

### 请求头

```http
Authorization: Bearer <access_token>
Content-Type: application/json
```

### 请求体

```json
{}
```

### 成功响应示例

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "logged_out": true
  }
}
```

### 断言点

- HTTP 状态码为 `200`
- `code = 0`
- `data.logged_out = true`
- 客户端清理：
  - `access_token`
  - `refresh_token`

### 异常测试

#### 不带 Token

无 `Authorization` 请求头

#### Token 非法

```http
Authorization: Bearer invalid_token
```

### 异常响应示例

```json
{
  "code": 10002,
  "message": "unauthorized",
  "data": null
}
```

---

# 6. 推荐测试顺序

```text
1. 注册
2. 登录
3. 获取当前用户
4. 刷新访问令牌
5. 再次获取当前用户
6. 退出登录
7. 退出后再次获取当前用户
```

---

# 7. 测试检查清单

## 7.1 注册

- [x] 用户名校验正确
- [x] 邮箱校验正确
- [x] 密码长度校验正确
- [x] 重复用户能正确提示

## 7.2 登录

- [x] 支持用户名登录
- [x] 支持邮箱登录
- [x] 返回 access_token
- [x] 返回 refresh_token
- [x] 返回用户摘要信息

## 7.3 鉴权

- [x] 合法 access_token 可访问受保护接口
- [x] 非法 access_token 被拒绝
- [x] 过期 access_token 被拒绝

## 7.4 刷新令牌

- [x] 合法 refresh_token 能刷新 access_token
- [x] 非法 refresh_token 被拒绝
- [x] 过期 refresh_token 被拒绝

## 7.5 退出登录

- [x] 退出接口能正常返回成功
- [ ] 退出后本地 token 被清理
- [ ] 若后期接入 Redis，退出后服务端会话应立即失效

---

# 8. 逻辑结构展示

```text
Auth 模块接口测试逻辑
/api/v1/auth
├── POST /register
├── POST /login
├── GET /me
├── POST /refresh
└── POST /logout
```

---

# 9. 备注

- 成功响应统一格式：

```json
{
  "code": 0,
  "message": "success",
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
