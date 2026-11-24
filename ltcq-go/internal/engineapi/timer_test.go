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
		function myCallback(pid, arg)
			fired = arg
		end
	`)
	if err != nil {
		t.Fatal(err)
	}
	// Lua 通过 LActor.postScriptEvent 投递（Go 实现）
	err = g.Load("post", `
		LActor.postScriptEvent(nil, 1, "myCallback", 0, 1, 4242)
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

	v, err := g.DoString("fired")
	if err != nil {
		t.Fatal(err)
	}
	if v.(int) != 4242 {
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
		function countCb(pid) firedCount = firedCount + 1 end
	`)
	// 投递周期任务：delay=1ms, interval=1ms, times=3
	g.Load("post", `LActor.postScriptEvent(nil, 1, "countCb", 1, 3)`)
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

	g.Load("cb", `fired = 0 function cCb(pid) fired = 1 end`)
	v, err := g.DoString(`LActor.postScriptEvent(nil, 5000, "cCb", 0, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	pid := v.(int)
	if d.PendingCount() != 1 {
		t.Errorf("取消前 pending=%d", d.PendingCount())
	}
	_, err = g.DoString("LActor.cancelScriptTimer(nil, " + itoa(pid) + ")")
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
