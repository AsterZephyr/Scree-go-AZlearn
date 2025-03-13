// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ws

import (
	"sync"
	"sync/atomic"
)

// once 是一个简化版的sync.Once实现
// 用于确保某个操作只执行一次，即使在并发环境下也能保证
// 与标准库的sync.Once相比，这个实现更加轻量级
type once struct {
	m    sync.Mutex // 互斥锁，用于保护done变量的修改
	done uint32     // 标记操作是否已完成，使用uint32以便原子操作
}

// Do 执行传入的函数，但确保该函数只会被执行一次
// 如果函数已经执行过，则直接返回
// 参数f是要执行的函数
func (o *once) Do(f func()) {
	// 快速检查操作是否已完成，避免获取锁的开销
	if atomic.LoadUint32(&o.done) == 1 {
		return
	}
	// 尝试获取执行权限
	if o.mayExecute() {
		f() // 执行函数
	}
}

// mayExecute 检查并标记操作是否可以执行
// 返回true表示可以执行，false表示已经执行过
// 这是一个内部方法，用于实现Do方法的并发安全
func (o *once) mayExecute() bool {
	o.m.Lock()         // 获取互斥锁
	defer o.m.Unlock() // 确保锁最终会被释放
	if o.done == 0 {   // 检查操作是否已完成
		atomic.StoreUint32(&o.done, 1) // 标记操作已完成
		return true                     // 允许执行
	}
	return false // 操作已完成，不允许执行
}
