// Package service 定义了 auto 模块核心业务逻辑服务
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"smart-task-platform/internal/dto"
	"smart-task-platform/internal/model"
	"smart-task-platform/internal/pkg/jwt"
	"smart-task-platform/internal/pkg/password"
	"smart-task-platform/internal/pkg/validator"
	"smart-task-platform/internal/repository"
)

var (
	ErrUsernameExists        = errors.New("username already exists") // 用户名已存在错误消息
	ErrEmailExists           = errors.New("email already exists")    // 邮箱已存在错误消息
	ErrInvalidUsernameFormat = errors.New("invalid username")        // 无效的用户名错误消息
	ErrInvalidEmailFormat    = errors.New("invalid email")           // 无效的邮箱错误消息
	ErrInvalidPasswordFormat = errors.New("invalid password")        // 无效的密码错误消息
	ErrInvalidNicknameFormat = errors.New("invalid nickname")        // 无效的昵称错误消息
	ErrInvalidAccountFormat  = errors.New("invalid account")         // 无效的账户错误消息
	ErrUserDisabled          = errors.New("user is disabled")        // 用户被禁用错误消息
	ErrPasswordMismatch      = errors.New("password does not match") // 密码不匹配错误消息
	ErrUserNotFound          = errors.New("user not found")          // 用户未找到错误消息
	ErrInvalidToken          = errors.New("invalid token")           // 无效的 Token 错误消息
	ErrExpiredToken          = errors.New("refresh token expired")   // 刷新令牌过期错误消息
	ErrInternal              = errors.New("internal server error")   // 内部服务器错误消息
)

// AuthService 认证服务
type AuthService struct {
	userRepo repository.UserRepository // 用户仓库接口
	txMgr    *repository.TxManager     // 事务管理器
	jwtMgr   *jwt.Manager              // JWT 管理器
}

// NewAuthService 创建认证服务
func NewAuthService(
	userRepo repository.UserRepository,
	txMgr *repository.TxManager,
	jwtMgr *jwt.Manager,
) *AuthService {
	return &AuthService{
		userRepo: userRepo,
		txMgr:    txMgr,
		jwtMgr:   jwtMgr,
	}
}

// Register 注册
func (s *AuthService) Register(ctx context.Context, req *dto.RegisterReq) (*dto.RegisterResp, error) {
	// 确保用户名、邮箱和昵称没有前后空格，邮箱统一小写，避免重复注册问题
	req.Username = strings.TrimSpace(req.Username)
	req.Nickname = strings.TrimSpace(req.Nickname)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	// 1、先判断格式是否正确，格式不正确直接返回错误，不需要查询数据库了
	if !validator.IsValidUsername(req.Username) {
		zap.L().Warn("invalid username format",
			zap.String("username", req.Username))
		return nil, ErrInvalidUsernameFormat // 用户名格式无效
	}

	if !validator.IsValidEmail(req.Email) {
		zap.L().Warn("invalid email format",
			zap.String("email", req.Email))
		return nil, ErrInvalidEmailFormat // 邮箱格式无效
	}

	if !validator.IsValidPassword(req.Password) {
		zap.L().Warn("invalid password format",
			zap.String("password", req.Password))
		return nil, ErrInvalidPasswordFormat // 密码格式无效
	}

	if req.Nickname != "" && !validator.IsValidNickname(req.Nickname) {
		zap.L().Warn("invalid nickname format",
			zap.String("nickname", req.Nickname))
		return nil, ErrInvalidNicknameFormat // 昵称格式无效
	}

	// 2、判断用户名和邮箱是否已经存在了
	existsUsername, errUser := s.userRepo.ExistsByUsername(ctx, req.Username)
	existsEmail, errEmail := s.userRepo.ExistsByEmail(ctx, req.Email)
	if errUser != nil || errEmail != nil {
		zap.L().Error("failed to check username existence",
			zap.String("username", req.Username),
			zap.String("email", req.Email),
			zap.NamedError("errUser", errUser),
			zap.NamedError("errEmail", errEmail))

		// 判断是否是用户不存在的错误，如果是用户不存在的错误则说明用户名或邮箱不存在，可以继续注册；如果是其他错误则说明查询过程中发生了错误，应该返回错误
		if !errors.Is(errUser, repository.ErrUserNotFound) && !errors.Is(errEmail, repository.ErrUserNotFound) {
			// 这里的错误可能是数据库连接错误或者其他查询错误，应该记录日志并返回错误
			zap.L().Error("failed to check username or email existence",
				zap.String("username", req.Username),
				zap.String("email", req.Email),
				zap.NamedError("errUser", errUser),
				zap.NamedError("errEmail", errEmail))
			return nil, ErrInternal // 服务器内部错误
		}
		return nil, ErrUserNotFound // 用户不存在
	}

	if existsUsername || existsEmail {
		zap.L().Warn("username or email already exists",
			zap.String("username", req.Username),
			zap.String("email", req.Email))
		return nil, ErrUsernameExists // 用户名或邮箱已存在
	}

	// 3、构造HashedPassword
	hashed, err := password.HashPassword(req.Password)
	if err != nil {
		zap.L().Error("failed to hash password",
			zap.Error(err))
		return nil, err
	}

	// 构造用户对象
	user := &model.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashed,
		Nickname:     req.Nickname,
		Status:       model.UserStatusActive,
	}

	// 这里单表操作就不需要事务了，如果后续注册流程复杂了再加事务
	if err := s.userRepo.Create(ctx, user); err != nil {
		zap.L().Error("failed to create user", zap.Error(err))
		return nil, err
	}

	// 返回响应
	return &dto.RegisterResp{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Nickname: user.Nickname,
	}, nil
}

// Login 登录
func (s *AuthService) Login(ctx context.Context, req *dto.LoginReq) (*dto.LoginResp, error) {
	req.Account = strings.TrimSpace(req.Account)

	// 1、先检查登录的账户是邮箱还是用户名，判断格式是否正确，格式不正确直接返回错误，不需要查询数据库了
	isVaildEmail := validator.IsValidEmail(req.Account)
	isValidUsername := validator.IsValidUsername(req.Account)
	if !isVaildEmail && !isValidUsername {
		zap.L().Warn("invalid account format",
			zap.String("account", req.Account))
		return nil, ErrInvalidAccountFormat // 账户格式无效
	}

	// 如果是合法的邮箱格式，则将账户转换为小写，确保邮箱登录不区分大小写
	if isVaildEmail {
		req.Account = strings.ToLower(req.Account) // 邮箱统一小写
	}
	// 进行登录的时候不需要对密码进行格式检查

	// 2、通过账户（用户名或邮箱）查询用户，得到用户信息
	user, err := s.userRepo.GetByAccount(ctx, req.Account)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			zap.L().Warn("invalid login attempt: account not found",
				zap.String("account", req.Account))
			return nil, ErrUserNotFound // 账户未找到
		}
		zap.L().Error("failed to get user by account",
			zap.String("account", req.Account),
			zap.Error(err))
		return nil, err
	}

	// 3、验证密码是否正确
	if ok := password.CheckPasswordHash(req.Password, user.PasswordHash); !ok {
		zap.L().Warn("invalid login attempt: incorrect password",
			zap.String("account", req.Account),
			zap.Uint64("user_id", user.ID))
		return nil, ErrPasswordMismatch // 密码错误，返回密码不匹配错误
	}

	// 4、查看用户状态，如果用户被禁用则返回错误
	if user.Status != model.UserStatusActive {
		zap.L().Warn("login attempt for disabled user",
			zap.String("account", req.Account),
			zap.Uint64("user_id", user.ID))
		return nil, ErrUserDisabled // 用户被禁用，返回用户被禁用错误
	}

	now := time.Now()                    // 获取当前时间
	var accessToken, refreshToken string // 访问令牌和刷新令牌
	var expiresIn int64                  // 过期时间，单位秒

	// 使用事务更新最后登录时间和生成 Token，确保原子性
	// 更新登录时间和发放 Token 是登录流程中两个重要的步骤，必须保证它们要么同时成功，要么同时失败，不能出现更新了登录时间但没有发放 Token 的情况，也不能出现发放了 Token 但没有更新登录时间的情况，这样才能保证系统状态的一致性和安全性
	err = s.txMgr.Transaction(ctx,
		func(tx *gorm.DB) error {
			// 更新最后登录时间
			if err := s.userRepo.UpdateLastLoginAtWithTx(ctx, tx, user.ID, now); err != nil {
				zap.L().Error("failed to update last login time",
					zap.String("account", req.Account),
					zap.Uint64("user_id", user.ID),
					zap.Error(err))
				return err
			}

			// 生成 Token
			var tokenErr error
			accessToken, refreshToken, expiresIn, tokenErr = s.jwtMgr.GenerateToken(user.ID, user.Username)
			if tokenErr != nil {
				zap.L().Error("failed to generate token",
					zap.String("account", req.Account),
					zap.Uint64("user_id", user.ID),
					zap.Error(tokenErr))
				return tokenErr
			}
			return nil
		})
	// 更新登录时间和生成 Token 可能会失败，如果失败了就记录错误日志并返回错误
	if err != nil {
		zap.L().Error("failed to complete login transaction",
			zap.String("account", req.Account),
			zap.Uint64("user_id", user.ID),
			zap.Error(err))
		return nil, err
	}
	// 完成登录事务
	// 返回登录响应，包含 Token 和用户信息摘要
	return &dto.LoginResp{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		User: dto.UserSummary{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Avatar:   user.Avatar,
		},
	}, nil
}

// Logout 退出登录（后续加入 Redis 登录状态再进行调整）
func (s *AuthService) Logout(ctx context.Context, userID uint64) (*dto.LogoutResp, error) {
	// 目前没有实际的退出登录操作，因为我们使用的是 JWT 无状态认证，前端只需要删除 Token 就行了
	// 如果后续需要实现 Token 黑名单或者其他退出登录机制，可以在这里添加相关逻辑
	zap.L().Info("user logged out",
		zap.Uint64("user_id", userID))
	return &dto.LogoutResp{
		Logout: true,
	}, nil
}

// Me 获取当前用户信息
func (s *AuthService) Me(ctx context.Context, userID uint64) (*dto.MeResp, error) {
	// 通过用户 ID 查询用户信息
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		zap.L().Error("failed to get user by ID",
			zap.Uint64("user_id", userID),
			zap.Error(err))
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserNotFound // 用户未找到
		}
		return nil, err
	}

	// 查看用户状态，如果用户被禁用则返回错误
	if user.Status != model.UserStatusActive {
		zap.L().Warn("attempt to access info of disabled user",
			zap.Uint64("user_id", userID))
		return nil, ErrUserDisabled
	}

	// 返回用户信息响应
	return &dto.MeResp{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Email:    user.Email,
		Avatar:   user.Avatar,
	}, nil
}

// RefreshToken 刷新 Token
func (s *AuthService) RefreshToken(ctx context.Context, req *dto.RefreshTokenReq) (*dto.RefreshTokenResp, error) {
	// 1、验证刷新 Token 的格式是否正确，格式不正确直接返回错误，不需要解析 Token 了
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.RefreshToken == "" {
		zap.L().Warn("empty refresh token")
		return nil, ErrInvalidToken // 刷新令牌不能为空，返回无效账户错误
	}

	// 2、解析刷新 Token 检验是否过期和有效，解析 Token 获取用户信息
	claims, err := s.jwtMgr.ParseToken(req.RefreshToken)
	if err != nil {
		switch {
		// 无效的签名方法和无效的 Token 都说明刷新令牌无效，返回无效账户错误
		case errors.Is(err, jwt.InvalidSigningMethodError),
			errors.Is(err, jwt.InvalidTokenError):
			return nil, ErrInvalidToken // 刷新令牌无效，返回无效 Token 错误
		// 过期的 Token
		case errors.Is(err, jwt.ExpiredTokenError):
			return nil, ErrExpiredToken // 刷新令牌过期，返回刷新令牌过期错误
		// 其他错误
		default:
			zap.L().Error("failed to parse refresh token",
				zap.String("refresh_token", req.RefreshToken),
				zap.Error(err))
			return nil, err // 解析 Token 过程中发生了其他错误，返回错误
		}
	}
	// 解析成功
	userID := claims.UserID     // 获取用户ID
	username := claims.Username // 获取用户名
	// 3、生成新的访问 Token 和刷新 Token
	accessToken, _, expiresIn, err := s.jwtMgr.GenerateToken(userID, username) // TODO：目前先不实现刷新令牌乱转，后续再完善
	if err != nil {
		zap.L().Error("failed to refresh token",
			zap.Uint64("user_id", userID),
			zap.Error(err))
		return nil, err
	}

	// 返回新的 Token 响应
	return &dto.RefreshTokenResp{
		AccessToken: accessToken,
		// RefreshToken: refreshToken,
		TokenType: "Bearer",
		ExpiresIn: expiresIn,
	}, nil
}
