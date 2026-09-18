/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package service

import "sync"

// 异步图片生成的「每用户并发闸门」。
//
// 上游图片接口是同步的：一次调用要占住 10~60 秒。批量用户同时点「生成」时，
// 不加限制就会把上游打爆（显存不足、429），而且是整体失败 —— 不是变慢。
// 这里不限制全局吞吐（队列由上游自己排），只限制**单个用户同时在跑几张**，
// 挡住「一个人开十个标签页把上游占满」。
//
// 等待发生在后台 goroutine 里，不占用 HTTP 连接：用户看到的是任务一直在排队中。
var imageTaskSlots = struct {
	mu    sync.Mutex
	slots map[int]chan struct{}
}{slots: make(map[int]chan struct{})}

// AcquireImageTaskSlot 申请一个用户并发名额。
//
// limit <= 0 表示不限并发，直接返回空释放函数。
// 返回的函数必须 defer 调用，用来释放名额。
func AcquireImageTaskSlot(userID int, limit int) func() {
	if limit <= 0 {
		return func() {}
	}

	imageTaskSlots.mu.Lock()
	slot, ok := imageTaskSlots.slots[userID]
	if !ok {
		slot = make(chan struct{}, limit)
		imageTaskSlots.slots[userID] = slot
	}
	imageTaskSlots.mu.Unlock()

	// 满了就在这里排队等待（调用方在后台 goroutine 里，不阻塞用户请求）
	slot <- struct{}{}

	return func() {
		<-slot
	}
}
