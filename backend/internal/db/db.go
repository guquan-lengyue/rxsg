package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"rxsg/backend/internal/config"
)

// DB 包装 *sqlx.DB，并复刻 legacy server/lib/mysql.php 中各 sql_* helper 的语义。
type DB struct {
	*sqlx.DB
}

// Open 按 legacy server/config/db.php 的连接参数建立连接池。
func Open(cfg config.DBConfig) (*DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=false&loc=Local",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name, cfg.Charset)

	x, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	x.SetMaxOpenConns(50)
	x.SetMaxIdleConns(10)
	x.SetConnMaxLifetime(time.Hour)
	if err := x.Ping(); err != nil {
		return nil, err
	}
	return &DB{x}, nil
}

// normalizeRow 把驱动返回的 []byte 转为 string，便于 JSON 输出与比较。
func normalizeRow(row map[string]any) map[string]any {
	for k, v := range row {
		if b, ok := v.([]byte); ok {
			row[k] = string(b)
		}
	}
	return row
}

// FetchOne 对应 sql_fetch_one：单行 map；无行时返回 sql.ErrNoRows。
func (d *DB) FetchOne(ctx context.Context, query string, args ...any) (map[string]any, error) {
	rows, err := d.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	row := make(map[string]any)
	if err := rows.MapScan(row); err != nil {
		return nil, err
	}
	return normalizeRow(row), nil
}

// FetchRows 对应 sql_fetch_rows：多行 map。
func (d *DB) FetchRows(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := d.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		out = append(out, normalizeRow(row))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FetchCellString 对应 sql_fetch_one_cell：首行首列（字符串）。
func (d *DB) FetchCellString(ctx context.Context, query string, args ...any) (string, error) {
	var v sql.NullString
	if err := d.QueryRowxContext(ctx, query, args...).Scan(&v); err != nil {
		return "", err
	}
	return v.String, nil
}

// FetchCellInt64 对应 sql_fetch_one_cell：首行首列（整型）。
func (d *DB) FetchCellInt64(ctx context.Context, query string, args ...any) (int64, error) {
	var v sql.NullInt64
	if err := d.QueryRowxContext(ctx, query, args...).Scan(&v); err != nil {
		return 0, err
	}
	return v.Int64, nil
}

// Exec 对应 sql_query：返回影响行数。
func (d *DB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := d.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Insert 对应 sql_insert：返回 lastInsertId。
func (d *DB) Insert(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := d.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Exists 对应 sql_check：是否存在至少一行。
func (d *DB) Exists(ctx context.Context, query string, args ...any) (bool, error) {
	var one int
	err := d.QueryRowxContext(ctx, query, args...).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Now 返回数据库当前时间戳（对应 select unix_timestamp()）。
func (d *DB) Now(ctx context.Context) (int64, error) {
	return d.FetchCellInt64(ctx, "select unix_timestamp()")
}