package common

import (
	"errors"
	"sync"
)

type IDAllocator struct {
	start  int
	count  int
	cursor int
	used   map[int]bool
	mu     sync.Mutex
}

func NewIDAllocator(start, count int) *IDAllocator {
	return &IDAllocator{
		start:  start,
		count:  count,
		cursor: 0,
		used:   make(map[int]bool),
	}
}

// 分配下一个可用 ID
func (a *IDAllocator) Next() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i := 0; i < a.count; i++ {
		id := a.start + (a.cursor+i)%a.count
		if !a.used[id] {
			a.used[id] = true
			a.cursor = (a.cursor + i + 1) % a.count
			return id, nil
		}
	}
	return -1, errors.New("no available ID in range")
}

// 手动释放 ID（用于失败回滚）
func (a *IDAllocator) Free(id int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.used, id)
}
