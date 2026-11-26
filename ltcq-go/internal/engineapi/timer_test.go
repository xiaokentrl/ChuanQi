package engineapi

import (
	"testing"
	"time"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

func newTimerTestHost(t *testing.T) (*TimerDriver, *luahost.GopherLua) {
	t.Helper()
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	if err := g.InjectEnums(); err != nil {
		g.Close()
		t.Skipf("资产不可达: %v", err)
	}
	// postscripttimer.lua:141 依赖 actorevent 表，先加载五大注册表
	InstallSystemStubs(g)
	if _, err := LoadRegistries(g, root); err != nil {
		g.Close()
		t.Fatalf("加载注册表失败: %v", err)
	}
	d := NewTimerDriver(g)
	if err := d.Install(root); err != nil {
		g.Close()
		t.Fatalf("定时器安装失败: %v", err)
	}
	return d, g
}

func TestTimer_PostScriptFires(t *testing.T) {
	d, g := newTimerTestHost(t)
	defer g.Close()

	// 注册一个回调（Go 到点会调用该全局函数）
	err := g.Load("cb", `
		fired = 0
		function myCallback(actor, arg)
			firedActor = actor
			fired = arg
		end
	`)
	if err != nil {
		t.Fatal(err)
	}
	// Lua 通过 LActor.postScriptEvent 投递（Go 实现）
	err = g.Load("post", `
		firedActor = -1
		LActor.postScriptEvent(7, 1, "myCallback", 0, 1, 4242)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if d.PendingCount() != 1 {
		t.Errorf("待触发任务数=%d want 1", d.PendingCount())
	}
	// 驱动到点
	time.Sleep(5 * time.Millisecond)
	d.Due(time.Now())

	if v, err := g.DoString("firedActor"); err != nil || v.(int) != 7 {
		t.Errorf("回调 actor=%v err=%v want 7", v, err)
	}
	if v, err := g.DoString("fired"); err != nil || v.(int) != 4242 {
		t.Errorf("回调 fired=%v want 4242", v)
	}
	if d.PendingCount() != 0 {
		t.Errorf("触发后待处理=%d want 0", d.PendingCount())
	}
}

func TestTimer_PeriodicTask(t *testing.T) {
	d, g := newTimerTestHost(t)
	defer g.Close()

	g.Load("cb", `
		firedCount = 0
		function countCb(actor, pid) firedCount = firedCount + 1 end
	`)
	// 投递周期任务：delay=1ms, interval=1ms, times=3
	g.Load("post", `LActor.postScriptEvent(7, 1, "countCb", 1, 3)`)
	for i := 0; i < 5; i++ {
		time.Sleep(3 * time.Millisecond)
		d.Due(time.Now())
	}
	v, err := g.DoString("firedCount")
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 3 {
		t.Errorf("周期任务触发 %d 次 want 3", v)
	}
}

func TestTimer_Cancel(t *testing.T) {
	d, g := newTimerTestHost(t)
	defer g.Close()

	g.Load("cb", `fired = 0 function cCb(actor, pid) fired = 1 end`)
	v, err := g.DoString(`LActor.postScriptEvent(7, 5000, "cCb", 0, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	pid := v.(int)
	if d.PendingCount() != 1 {
		t.Errorf("取消前 pending=%d", d.PendingCount())
	}
	_, err = g.DoString("LActor.cancelScriptTimer(7, " + itoa(pid) + ")")
	if err != nil {
		t.Fatal(err)
	}
	if d.PendingCount() != 0 {
		t.Errorf("取消后 pending=%d want 0", d.PendingCount())
	}
}

func TestTimer_TickMinuteAnd5s(t *testing.T) {
	// 隔离验证 L1/L2 节拍机制：不加载真实 supertimer（其内部依赖完整脚本集），
	// 直接用全局 OnGameTimer/OnGamePrecisionTimer 回调验证 Go 节拍 → Lua。
	root := testDataRoot(t)
	g := luahost.NewGopherLua(luahost.Config{DataRoot: root, OSType: "WIN"})
	defer g.Close()
	if err := g.InjectEnums(); err != nil {
		t.Skipf("资产不可达: %v", err)
	}
	d := NewTimerDriver(g)

	g.Load("tickcb", `
		minuteTick = 0
		precisionTick = 0
		function OnGameTimer() minuteTick = minuteTick + 1 end
		function OnGamePrecisionTimer() precisionTick = precisionTick + 1 end
	`)
	if err := d.TickMinute(); err != nil {
		t.Fatalf("TickMinute: %v", err)
	}
	if err := d.Tick5s(); err != nil {
		t.Fatalf("Tick5s: %v", err)
	}
	v1, _ := g.DoString("minuteTick")
	v2, _ := g.DoString("precisionTick")
	if v1.(int) != 1 || v2.(int) != 1 {
		t.Errorf("L1/L2 节拍 minute=%v precision=%v want 1/1", v1, v2)
	}
}

func TestTimer_RealLuaTickChain(t *testing.T) {
	d, g := newTimerTestHost(t)
	defer g.Close()

	if err := g.Load("real-tick-chain", `
		newDayCount = 0
		superTimerFired = 0
		engineevent.regNewDay(function() newDayCount = newDayCount + 1 end)
		base.scripttimer.supertimer.regDelayTimerEvent(0, function(time)
			superTimerFired = superTimerFired + 1
		end)
	`); err != nil {
		t.Fatal(err)
	}
	if err := d.Tick5s(); err != nil {
		t.Fatalf("真实 5 秒链失败: %v", err)
	}
	assertIntGlobal(t, g, "newDayCount", 1)
	lastDay, err := g.DoString("System.getStaticVar().lastDayTime")
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := lastDay.(int); !ok || n <= 0 {
		t.Fatalf("lastDayTime=%#v，应为正整数时间戳", lastDay)
	}
	if err := d.Tick5s(); err != nil {
		t.Fatalf("同日第二次 5 秒链失败: %v", err)
	}
	assertIntGlobal(t, g, "newDayCount", 1)
	if err := d.TickMinute(); err != nil {
		t.Fatalf("真实分钟链失败: %v", err)
	}
	assertIntGlobal(t, g, "superTimerFired", 1)
}

func TestSystem_TimeAndStaticVar(t *testing.T) {
	_, g := newTimerTestHost(t)
	defer g.Close()

	now, err := g.Call("System.getNowTime")
	if err != nil {
		t.Fatalf("System.getNowTime: %v", err)
	}
	if n, ok := now.(int); !ok || n <= 0 {
		t.Fatalf("System.getNowTime=%#v，应为正整数时间戳", now)
	}
	for _, tc := range []struct {
		a, b int
		want bool
	}{
		{a: 0, b: 0, want: true},
		{a: 1700000000, b: 1700000000, want: true},
		{a: 0, b: 86400, want: false},
	} {
		got, err := g.Call("System.isSameDay", tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("System.isSameDay(%d,%d)=%#v err=%v want %v", tc.a, tc.b, got, err, tc.want)
		}
	}
	decoded, err := g.DoString(`(function()
		local y, m, d, h, mi, s = System.timeDecode(1700000000)
		return y > 1970 and m >= 1 and m <= 12 and d >= 1 and d <= 31 and h >= 0 and h <= 23 and mi >= 0 and mi <= 59 and s >= 0 and s <= 60
	end)()`)
	if err != nil || decoded != true {
		t.Fatalf("System.timeDecode 返回值非法: %#v err=%v", decoded, err)
	}
	invalidDecoded, err := g.DoString(`(function()
		local y, m, d, h, mi, s = System.timeDecode("bad")
		return y + m + d + h + mi + s
	end)()`)
	if err != nil || invalidDecoded != 0 {
		t.Fatalf("非法 System.timeDecode 应返回六个 0: %#v err=%v", invalidDecoded, err)
	}
	invalidSameDay, err := g.Call("System.isSameDay", nil, 0)
	if err != nil || invalidSameDay != false {
		t.Fatalf("非法 System.isSameDay 应返回 false: %#v err=%v", invalidSameDay, err)
	}
	sameTable, err := g.DoString("System.getStaticVar() == System.getStaticVar()")
	if err != nil || sameTable != true {
		t.Fatalf("System.getStaticVar 未返回同一表: %#v err=%v", sameTable, err)
	}
	if err := g.Load("system-static-write", `System.getStaticVar().timerProbe = 42`); err != nil {
		t.Fatal(err)
	}
	assertIntGlobal(t, g, "System.getStaticVar().timerProbe", 42)
}

func TestTimer_PostScriptLiteWrapper(t *testing.T) {
	d, g := newTimerTestHost(t)
	defer g.Close()

	if err := g.Load("lite-wrapper", `
		firedActor = -1
		firedValue = -1
		function liteCallback(actor, value)
			firedActor = actor
			firedValue = value
		end
		local unusedPid = LActor.postScriptEvent(7, 5000, "unusedCallback", 0, 1)
		LActor.cancelScriptTimer(7, unusedPid)
		liteEid = LActor.postScriptEventLite(7, 1, liteCallback, 4242)
	`); err != nil {
		t.Fatal(err)
	}
	if d.PendingCount() != 1 {
		t.Fatalf("Lite 包装未投递底层任务: pending=%d", d.PendingCount())
	}
	d.Due(time.Now().Add(time.Second))
	assertIntGlobal(t, g, "firedActor", 7)
	assertIntGlobal(t, g, "firedValue", 4242)
	if d.PendingCount() != 0 {
		t.Fatalf("Lite 触发后未清理: pending=%d", d.PendingCount())
	}
}

func TestTimer_WrapperCancelAndActorCleanup(t *testing.T) {
	d, g := newTimerTestHost(t)
	defer g.Close()

	if err := g.Load("timer-cleanup", `
		firedCount = 0
		function cleanupCallback(actor) firedCount = firedCount + 1 end
		cancelEid = LActor.postScriptEventLite(7, 5000, cleanupCallback)
		cleanupEid = LActor.postScriptEventLite(7, 5000, cleanupCallback)
		LActor.cancelScriptEvent(7, cancelEid)
	`); err != nil {
		t.Fatal(err)
	}
	if d.PendingCount() != 1 {
		t.Fatalf("取消后 pending=%d want 1", d.PendingCount())
	}
	if _, err := g.DoString(`base.scripttimer.postscripttimer.cleanActorList(7)`); err != nil {
		t.Fatal(err)
	}
	d.Due(time.Now().Add(time.Hour))
	assertIntGlobal(t, g, "firedCount", 0)
	if d.PendingCount() != 0 {
		t.Fatalf("Actor 清理后 pending=%d want 0", d.PendingCount())
	}
}

func TestTimer_PeriodicWrapperCanCancelItself(t *testing.T) {
	d, g := newTimerTestHost(t)
	defer g.Close()

	if err := g.Load("timer-self-cancel", `
		firedCount = 0
		function selfCancel(actor)
			firedCount = firedCount + 1
			LActor.cancelScriptEvent(actor, selfEid)
		end
		selfEid = LActor.postScriptEventEx(7, 1, selfCancel, 1, 3)
	`); err != nil {
		t.Fatal(err)
	}
	d.Due(time.Now().Add(time.Second))
	d.Due(time.Now().Add(2 * time.Second))
	assertIntGlobal(t, g, "firedCount", 1)
	if d.PendingCount() != 0 {
		t.Fatalf("回调内自取消后仍被重新入队: pending=%d", d.PendingCount())
	}
}

func TestTimer_StartAndClose(t *testing.T) {
	g := luahost.NewGopherLua(luahost.Config{OSType: "WIN"})
	d := NewTimerDriver(g)
	fired := make(chan struct{})
	g.Register("timerStarted", func(...luahost.Value) (luahost.Value, error) {
		close(fired)
		return nil, nil
	})
	if _, err := d.postScriptEvent(7, 0, "timerStarted", 0, 1); err != nil {
		g.Close()
		t.Fatal(err)
	}
	d.Start()
	select {
	case <-fired:
	case <-time.After(time.Second):
		d.Close()
		g.Close()
		t.Fatal("Start 后定时器未触发")
	}
	d.Close()
	g.Close()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
