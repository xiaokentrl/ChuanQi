package engineapi

import (
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
)

// actor.go：T0 批次 LActor API（身份/属性、货币、物品、事件、场景副本）。
//
// 当前为骨架实现：actor 句柄用整数表示，实际玩家实体属阶段4.6（场景实体），
// 尚未接入。签名对齐 engine-api.md，返回值用占位默认值，待 4.6 接入后替换。
// 注意：这些 stub 让加载链能跑通，但不能用于真实玩法验证。

// asActorID 把 actor 句柄（整数或 nil）转 ID。
func asActorID(v luahost.Value) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func init() {
	// === 身份/属性（T0 #2,23,24,25,27）===
	Register(Entry{Name: "LActor.getActorId", Fn: func(a ...luahost.Value) (luahost.Value, error) {
		return asActorID(a[0]), nil
	}, Meta: "原版: LActor::getActorId | Lua调用: 1402处 | 规格: engine-api.md#LActor.getActorId"})

	Register(Entry{Name: "LActor.getLevel", Fn: realGetLevel, Meta: "Lua调用: 100处 | 已接内存实体"})
	Register(Entry{Name: "LActor.getName", Fn: realGetName, Meta: "Lua调用: 99处 | 已接内存实体"})
	Register(Entry{Name: "LActor.getActorName", Fn: realGetName, Meta: "Lua调用: 99处 | 已接内存实体"})
	Register(Entry{Name: "LActor.getZhuanShengLevel", Fn: stubInt1, Meta: "Lua调用: 96处 | 占位"})

	Register(Entry{Name: "LActor.getActorById", Fn: func(a ...luahost.Value) (luahost.Value, error) {
		return asActorID(a[0]), nil
	}, Meta: "Lua调用: 128处 | 占位（返回同ID句柄）"})

	// === staticVar（T0 #13）===
	Register(Entry{Name: "LActor.getStaticVar", Fn: realGetStaticVar, Meta: "原版: Lua调用:163处 | 写时标脏 | 已接内存实体"})

	// === 货币（T0 #21,35,37）===
	Register(Entry{Name: "LActor.getCurrency", Fn: realGetCurrency, Meta: "Lua调用: 107处 | 已接内存实体"})
	Register(Entry{Name: "LActor.changeCurrency", Fn: realChangeCurrency, Meta: "Lua调用: 69处 | 已接内存实体，审计待接"})
	Register(Entry{Name: "LActor.changeYuanBao", Fn: stubNoop, Meta: "推断 | 占位"})

	// === 物品（T0 #15,18）===
	Register(Entry{Name: "LActor.getItemCount", Fn: realGetItemCount, Meta: "Lua调用: 134处 | 已接内存实体"})
	Register(Entry{Name: "LActor.costItem", Fn: realCostItem, Meta: "Lua调用: 126处 | Go扣减+DB脏标记 | 已接内存实体"})

	// === 奖励（T0 #16,30）===
	Register(Entry{Name: "LActor.giveAwards", Fn: stubNoop, Meta: "Lua调用: 133处 | 占位"})
	Register(Entry{Name: "LActor.canGiveAwards", Fn: stubBool, Meta: "Lua调用: 77处 | 占位"})

	// === 事件/变量（T0 #14,29）===
	Register(Entry{Name: "LActor.postScriptEventLite", Fn: func(a ...luahost.Value) (luahost.Value, error) {
		return 0, nil // handle，待接定时器层（4.7）
	}, Meta: "Lua调用: 141处 | 占位（依赖 4.7 定时器）"})
	Register(Entry{Name: "LActor.cancelScriptEvent", Fn: stubNoop, Meta: "Lua调用: 84处 | 占位"})

	// === 角色（T0 #19,31）===
	Register(Entry{Name: "LActor.getRoleCount", Fn: stubInt1, Meta: "Lua调用: 120处 | index0主角色 | 占位"})
	Register(Entry{Name: "LActor.getRole", Fn: stubInt2, Meta: "Lua调用: 75处 | 占位"})

	// === 提示/日志（T0 #8,28）===
	Register(Entry{Name: "LActor.sendTipmsg", Fn: stubNoop, Meta: "Lua调用: 310处 | typeMask位掩码见 engine-api.md:134 | 占位"})
	Register(Entry{Name: "LActor.log", Fn: stubNoop, Meta: "Lua调用: 91处 | →7001日志流 | 占位"})

	// === 场景副本（T0 #34,40,41,48,50）===
	Register(Entry{Name: "LActor.stopAI", Fn: stubNoop, Meta: "Lua调用: 71处 | 占位"})
	Register(Entry{Name: "LActor.getFubenHandle", Fn: stubInt1, Meta: "Lua调用: 54处 | 占位"})
	Register(Entry{Name: "LActor.getActor", Fn: stubInt1, Meta: "推断 | 占位"})
	Register(Entry{Name: "LActor.exitFuben", Fn: stubNoop, Meta: "Lua调用: 45处 | 占位"})
	Register(Entry{Name: "LActor.enterFuBen", Fn: stubNoop, Meta: "Lua调用: 43处 | 占位"})

	// === 属性/阵营/公会/VIP（T0 #36,38,42,44,46,47）===
	Register(Entry{Name: "LActor.setIntProperty", Fn: stubNoop, Meta: "占位 | property ID 待 Ghidra 提取"})
	Register(Entry{Name: "LActor.getGuildId", Fn: stubInt1, Meta: "推断 | 占位"})
	Register(Entry{Name: "LActor.setCamp", Fn: stubNoop, Meta: "推断 | 占位"})
	Register(Entry{Name: "LActor.getGuildPtr", Fn: stubInt1, Meta: "推断 | 占位"})
	Register(Entry{Name: "LActor.getVipLevel", Fn: stubInt1, Meta: "推断 | 占位"})
	Register(Entry{Name: "LActor.reCalcAttr", Fn: stubNoop, Meta: "Lua调用:46处 | ADR-004 Go收口 | 占位"})
}

// === 通用 stub 实现 ===
func stubNoop(a ...luahost.Value) (luahost.Value, error) { return nil, nil }
func stubBool(a ...luahost.Value) (luahost.Value, error) { return false, nil }
func stubInt1(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	return asActorID(a[0]), nil
}
func stubInt2(a ...luahost.Value) (luahost.Value, error) { return 0, nil }
func stubStr1(a ...luahost.Value) (luahost.Value, error) { return "", nil }
