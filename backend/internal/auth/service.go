package auth

import (
	"context"
	"database/sql"
	"math/rand"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

type Service struct {
	db      *db.DB
	jwt     *JWTManager
	session *SessionStore
}

func NewService(d *db.DB, jwt *JWTManager, session *SessionStore) *Service {
	return &Service{db: d, jwt: jwt, session: session}
}

type LoginRequest struct {
	Passport string `json:"passport"`
	Password string `json:"password"`
	Passtype string `json:"passtype"`
}

type LoginResponse struct {
	Token     string     `json:"token"`
	ExpiresIn int        `json:"expiresIn"`
	User      model.User `json:"user"`
}

// Login 对应 Login::doLogin 的关键路径，但数据源改为新库 users 表：
// passport 查用户 + bcrypt 校验密码，成功后建立内存会话并签发 JWT。
func (s *Service) Login(ctx context.Context, req LoginRequest, ipInt uint32, sip string) (*LoginResponse, error) {
	if req.Passport == "" || req.Password == "" {
		return nil, httpx.BadRequest("invalid_param", "passport 与 password 必填")
	}

	user, err := s.db.FetchOne(ctx, "select * from users where passport=?", req.Passport)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("account_not_found", "错误的帐号！请在首页注册！")
	}
	if err != nil {
		return nil, err
	}

	hash := model.Str(user, "password_hash")
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return nil, httpx.Unauthorized("invalid_user_pwd", "密码不正确！")
	}

	if model.Int(user, "state") == 5 {
		return nil, httpx.Forbidden("account_locked", "账号已被锁定")
	}

	uid := model.Int(user, "id")
	sid := int64(rand.Int31())
	s.session.Set(uid, sid)

	token, err := s.jwt.Sign(uid, sid)
	if err != nil {
		return nil, err
	}
	return &LoginResponse{Token: token, ExpiresIn: s.jwt.TTLSeconds(), User: model.UserFromMap(user)}, nil
}

// Logout 移除内存会话。
func (s *Service) Logout(ctx context.Context, uid int) error {
	s.session.Delete(uid)
	return nil
}

func (s *Service) Me(ctx context.Context, uid int) (model.User, error) {
	row, err := s.db.FetchOne(ctx, "select * from users where id=?", uid)
	if err == sql.ErrNoRows {
		return model.User{}, httpx.NotFound("user_not_found", "用户不存在")
	}
	if err != nil {
		return model.User{}, err
	}
	return model.UserFromMap(row), nil
}

// Announce 新库无公告表，返回空串。
func (s *Service) Announce(ctx context.Context) (string, error) {
	return "", nil
}

// IPToInt 复刻 server/game/common.php:12 的 IP 整数编码：
// ip = (o3<<24) + (o2<<16) + (o1<<8) + o0
func IPToInt(raw string) uint32 {
	parts := strings.Split(raw, ".")
	if len(parts) != 4 {
		return 0
	}
	var o [4]uint32
	for i := 0; i < 4; i++ {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil || n < 0 || n > 255 {
			return 0
		}
		o[i] = uint32(n)
	}
	return (o[3] << 24) | (o[2] << 16) | (o[1] << 8) | o[0]
}