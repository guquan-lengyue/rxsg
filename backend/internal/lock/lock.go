package lock

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Locker 用 MySQL GET_LOCK/RELEASE_LOCK 替代 legacy 的 userlock/*.lock 文件锁
// （见 server/game/utils.php:1201 的 newLockUser/unlockUser）。
// GET_LOCK 是连接级的，因此必须固定在同一条 *sqlx.Conn 上获取与释放。
type Locker struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) *Locker {
	return &Locker{db: db}
}

// WithUserLock 以 (uid, key) 为粒度加锁执行 fn；获取不到返回 lock_busy 错误。
func (l *Locker) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	conn, err := l.db.Connx(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	name := fmt.Sprintf("rxsg:user:%d:%s", uid, key)

	var got sql.NullInt64
	if err := conn.QueryRowxContext(ctx, "select GET_LOCK(?, ?)", name, 5).Scan(&got); err != nil {
		return err
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("lock_busy")
	}

	var dummy sql.NullInt64
	defer conn.QueryRowxContext(context.Background(), "select RELEASE_LOCK(?)", name).Scan(&dummy)

	return fn(ctx)
}