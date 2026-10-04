package auth

import "sync"

// SessionStore 保存 uid -> 当前有效 sid 的映射。
// 新库没有 legacy sys_sessions 表，改为进程内内存态；
// 单实例部署下与原会话校验语义等价，多实例需换成共享存储。
type SessionStore struct {
	mu   sync.RWMutex
	data map[int]int64
}

func NewSessionStore() *SessionStore {
	return &SessionStore{data: make(map[int]int64)}
}

// Set 登录时写入（等价 legacy insert ... on duplicate key update）。
func (s *SessionStore) Set(uid int, sid int64) {
	s.mu.Lock()
	s.data[uid] = sid
	s.mu.Unlock()
}

// Valid 等价 checkUserAuth：uid 存在且 sid 一致。
func (s *SessionStore) Valid(uid int, sid int64) bool {
	s.mu.RLock()
	cur, ok := s.data[uid]
	s.mu.RUnlock()
	return ok && cur == sid
}

// Delete 登出时移除（等价 delete from sys_sessions）。
func (s *SessionStore) Delete(uid int) {
	s.mu.Lock()
	delete(s.data, uid)
	s.mu.Unlock()
}