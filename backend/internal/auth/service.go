package auth

import (
	"context"
	"database/sql"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

type Service struct {
	db         *db.DB
	jwt        *JWTManager
	sessionDir string
}

func NewService(d *db.DB, jwt *JWTManager, sessionDir string) *Service {
	return &Service{db: d, jwt: jwt, sessionDir: sessionDir}
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

// Login 复刻 Login::doLogin 的关键路径（server/game/Login.php:28-334）与 passport/myrxsg.php。
func (s *Service) Login(ctx context.Context, req LoginRequest, ipInt uint32, sip string) (*LoginResponse, error) {
	if req.Passport == "" || req.Password == "" {
		return nil, httpx.BadRequest("invalid_param", "passport 与 password 必填")
	}
	if len(req.Password) < 4 {
		return nil, httpx.BadRequest("password_too_short", "密码不能少于4个字符！")
	}
	passtype := req.Passtype
	if passtype == "" {
		passtype = "my"
	}

	// passport/myrxsg.php:4-11 —— legacy 为明文比较
	pass, err := s.db.FetchOne(ctx, "select * from test_passport where passport=?", req.Passport)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("account_not_found", "错误的帐号！请在首页注册！")
	}
	if err != nil {
		return nil, err
	}
	if model.Str(pass, "password") != req.Password {
		return nil, httpx.Unauthorized("invalid_user_pwd", "密码不正确！")
	}

	// Login.php:132 查角色
	user, err := s.db.FetchOne(ctx, "select * from sys_user where passport=? and passtype=?", req.Passport, passtype)
	if err == sql.ErrNoRows {
		// Login.php:155-158 首次登录即建号
		uid, ierr := s.db.Insert(ctx,
			"insert into sys_user (`passtype`,`passport`,`group`,`state`,`money`,`regtime`,`domainid`,`honour`) "+
				"values (?,?,0,3,88888,unix_timestamp(),0,1000)", passtype, req.Passport)
		if ierr != nil {
			return nil, ierr
		}
		if _, ierr = s.db.Exec(ctx,
			"INSERT INTO sys_user_comming (`uid`,`site_id`,`page_id`,`sub_page_id`) values (?,'0','0','0')", uid); ierr != nil {
			return nil, ierr
		}
		user, ierr = s.db.FetchOne(ctx, "select * from sys_user where uid=?", uid)
		if ierr != nil {
			return nil, ierr
		}
	} else if err != nil {
		return nil, err
	}

	if model.Int(user, "state") == 5 {
		// Login.php:190
		return nil, httpx.Forbidden("account_locked", "账号已被锁定")
	}

	uid := model.Int(user, "uid")
	sid := rand.Int31()

	if err := s.realLoginCore(ctx, uid, int64(sid), ipInt, sip); err != nil {
		return nil, err
	}

	token, err := s.jwt.Sign(uid, int64(sid))
	if err != nil {
		return nil, err
	}
	return &LoginResponse{Token: token, ExpiresIn: s.jwt.TTLSeconds(), User: model.UserFromMap(user)}, nil
}

// realLoginCore 是 realLogin（utils.php:393）的核心子集：
// sys_sessions upsert、sessions/<uid> 文件、sys_online 更新、log_login 记录。
// TODO(phase2): rank_user 排行、giveDalibao 礼包、updateUnionRank、sendSysInform、updateFcmTime、cfg_baned_ip 判定。
func (s *Service) realLoginCore(ctx context.Context, uid int, sid int64, ipInt uint32, sip string) error {
	if _, err := s.db.Exec(ctx,
		"insert into sys_sessions(uid, sid, ip) values(?,?,?) on duplicate key update `sid`=values(`sid`),`ip`=values(`ip`)",
		uid, sid, ipInt); err != nil {
		return err
	}

	if s.sessionDir != "" {
		if err := os.MkdirAll(s.sessionDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(s.sessionDir, strconv.Itoa(uid)),
			[]byte(strconv.FormatInt(sid, 10)), 0o644); err != nil {
			return err
		}
	}

	if _, err := s.db.Exec(ctx,
		"update sys_online set onlinetime=onlinetime+GREATEST(0,lastupdate-onlineupdate),"+
			"onlineupdate=unix_timestamp(),`lastupdate`=unix_timestamp() where uid=?", uid); err != nil {
		return err
	}

	// legacy 失败不致命；此处同样忽略错误
	_, _ = s.db.Exec(ctx,
		"insert into log_login (uid,ip,time,sip) values (?,?,unix_timestamp(),?)", uid, ipInt, sip)

	return nil
}

// Logout 删除 sys_sessions 行与 sessions/<uid> 文件。
func (s *Service) Logout(ctx context.Context, uid int) error {
	if _, err := s.db.Exec(ctx, "delete from sys_sessions where uid=?", uid); err != nil {
		return err
	}
	if s.sessionDir != "" {
		_ = os.Remove(filepath.Join(s.sessionDir, strconv.Itoa(uid)))
	}
	return nil
}

func (s *Service) Me(ctx context.Context, uid int) (model.User, error) {
	row, err := s.db.FetchOne(ctx, "select * from sys_user where uid=?", uid)
	if err == sql.ErrNoRows {
		return model.User{}, httpx.NotFound("user_not_found", "用户不存在")
	}
	if err != nil {
		return model.User{}, err
	}
	return model.UserFromMap(row), nil
}

// Announce 对应 Login::getLoginAnnouncement（Login.php:11-27，读 sys_announce id=1）。
func (s *Service) Announce(ctx context.Context) (string, error) {
	c, err := s.db.FetchCellString(ctx, "select content from sys_announce where id=1")
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return c, nil
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