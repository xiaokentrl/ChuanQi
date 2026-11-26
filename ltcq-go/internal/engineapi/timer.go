package engineapi

import (
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// timer.go：定时器四层（ROADMAP 4.7）。
// L1 OnGameTimer（分钟级）/ L2 5秒精度：Go 节拍 → Lua 全局回调。
// L3 supertimer / L4 postscripttimer：纯 Lua 模块挂在 L1/L4 底层上。
// L4 底层毫秒调度器在此实现（对齐原版 LActor.postScriptEvent/cancelScriptTimer）。

// TimerDriver 持有 Go 侧定时器调度器与 Lua 宿主引用。
type TimerDriver struct {
	L         luahost.Interpreter
	mu        sync.Mutex
	tasks     []*timerTask
	cancelled map[int64]bool
	running   map[int64]bool
	nextPID   int64
	stop      chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
	wg        sync.WaitGroup
}

type timerTask struct {
	pid      int64
	actorID  int
	at       time.Time
	interval time.Duration
	times    int
	callback string
	args     []luahost.Value
}

// NewTimerDriver 创建定时器驱动。
func NewTimerDriver(L luahost.Interpreter) *TimerDriver {
	return &TimerDriver{L: L, stop: make(chan struct{}), cancelled: map[int64]bool{}, running: map[int64]bool{}}
}

// Install 注册 L4 底层 LActor API + 加载定时器 Lua 模块。
func (d *TimerDriver) Install(dataRoot string) error {
	d.L.Register("LActor.getHandle", realGetHandle)
	d.L.Register("LActor.postScriptEvent", d.postScriptEvent)
	d.L.Register("LActor.cancelScriptTimer", d.cancelScriptTimer)

	// 加载三层 Lua 模块（L1/L2 engineevent、L3 supertimer、L4 postscripttimer）
	mods := []string{
		"functions/base/engineevent/engineevent.lua",
		"functions/base/scripttimer/supertimer.lua",
		"functions/base/scripttimer/postscripttimer.lua",
	}
	for _, rel := range mods {
		path := filepath.Join(dataRoot, filepath.FromSlash(rel))
		if err := d.L.LoadFile(path); err != nil {
			return err
		}
	}
	return nil
}

func (d *TimerDriver) postScriptEvent(args ...luahost.Value) (luahost.Value, error) {
	// 签名：postScriptEvent(entity, delay, callbackName, interval, times, ...)
	if len(args) < 3 {
		return nil, nil
	}
	delay := asInt(args[1])
	callback := asString(args[2])
	interval := 0
	times := 1
	if len(args) >= 4 {
		interval = asInt(args[3])
	}
	if len(args) >= 5 {
		times = asInt(args[4])
	}
	pid := atomic.AddInt64(&d.nextPID, 1)
	task := &timerTask{
		pid:      pid,
		actorID:  asActorID(args[0]),
		at:       time.Now().Add(time.Duration(delay) * time.Millisecond),
		interval: time.Duration(interval) * time.Millisecond,
		times:    times,
		callback: callback,
		args:     append([]luahost.Value(nil), args[5:]...),
	}
	d.mu.Lock()
	d.tasks = append(d.tasks, task)
	d.mu.Unlock()
	return int(pid), nil
}

func (d *TimerDriver) cancelScriptTimer(args ...luahost.Value) (luahost.Value, error) {
	if len(args) < 2 {
		return nil, nil
	}
	pid := int64(asInt(args[1]))
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cancelled[pid] = true
	found := d.running[pid]
	for i, t := range d.tasks {
		if t.pid == pid {
			found = true
			d.tasks = append(d.tasks[:i], d.tasks[i+1:]...)
			break
		}
	}
	if !found {
		delete(d.cancelled, pid)
	}
	return nil, nil
}

// Due 驱动一次到点任务（供主循环调用，避免后台 goroutine 与 Lua VM 并发）。
// 返回本次触发的任务数。
func (d *TimerDriver) Due(now time.Time) int {
	d.mu.Lock()
	var due []*timerTask
	var keep []*timerTask
	for _, t := range d.tasks {
		if t != nil && !t.at.After(now) {
			due = append(due, t)
		} else {
			keep = append(keep, t)
		}
	}
	d.tasks = keep
	for _, t := range due {
		d.running[t.pid] = true
	}
	d.mu.Unlock()

	for _, t := range due {
		args := append([]luahost.Value{t.actorID}, t.args...)
		if _, err := d.L.Call(t.callback, args...); err != nil {
			slog.Error("定时器回调失败", "pid", t.pid, "callback", t.callback, "err", err)
		}
		d.mu.Lock()
		cancelled := d.cancelled[t.pid]
		if !cancelled && (t.times == -1 || t.times > 1) && t.interval > 0 {
			if t.times > 1 {
				t.times--
			}
			t.at = now.Add(t.interval)
			d.tasks = append(d.tasks, t)
		}
		delete(d.running, t.pid)
		delete(d.cancelled, t.pid)
		d.mu.Unlock()
	}
	return len(due)
}

// PendingCount 返回待触发任务数（测试/监控）。
func (d *TimerDriver) PendingCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.tasks)
}

func (d *TimerDriver) Start() {
	d.startOnce.Do(func() {
		d.wg.Add(1)
		go d.Run()
	})
}

func (d *TimerDriver) Run() {
	defer d.wg.Done()
	dueTicker := time.NewTicker(10 * time.Millisecond)
	minuteTicker := time.NewTicker(time.Minute)
	precisionTicker := time.NewTicker(5 * time.Second)
	defer dueTicker.Stop()
	defer minuteTicker.Stop()
	defer precisionTicker.Stop()
	for {
		select {
		case now := <-dueTicker.C:
			d.Due(now)
		case <-minuteTicker.C:
			if err := d.TickMinute(); err != nil {
				slog.Warn("分钟定时器回调失败", "err", err)
			}
		case <-precisionTicker.C:
			if err := d.Tick5s(); err != nil {
				slog.Warn("5秒定时器回调失败", "err", err)
			}
		case <-d.stop:
			return
		}
	}
}

func (d *TimerDriver) Close() {
	d.closeOnce.Do(func() { close(d.stop) })
	d.wg.Wait()
}

// L1/L2 引擎节拍：调用 Lua 全局回调。
func (d *TimerDriver) TickMinute() error {
	_, err := d.L.Call("OnGameTimer")
	return err
}
func (d *TimerDriver) Tick5s() error {
	_, err := d.L.Call("OnGamePrecisionTimer")
	return err
}
