package engineapi

import (
	"sync/atomic"

	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/luahost"
	"github.com/xiaokentrl/ChuanQi/ltcq-go/internal/scene"
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
	Register(Entry{Name: "LActor.getActorId", Fn: realGetActorId, Meta: "原版: LActor::getActorId | Lua调用: 1402处 | 规格: engine-api.md#LActor.getActorId"})

	Register(Entry{Name: "LActor.getLevel", Fn: realGetLevel, Meta: "Lua调用: 100处 | 已接内存实体"})
	Register(Entry{Name: "LActor.getName", Fn: realGetName, Meta: "Lua调用: 99处 | 已接内存实体"})
	Register(Entry{Name: "LActor.getActorName", Fn: realGetName, Meta: "Lua调用: 99处 | 已接内存实体"})
	Register(Entry{Name: "LActor.getZhuanShengLevel", Fn: realGetZhuanShengLevel, Meta: "Lua调用: 96处 | 已接 actors.zhuansheng_lv"})

	Register(Entry{Name: "LActor.getActorById", Fn: realGetActorById, Meta: "Lua调用: 128处 | 仅返回在线 Actor，离线为 nil"})
	Register(Entry{Name: "LActor.getActorByName", Fn: realGetActorByName, Meta: "Lua调用: 2处 | 仅返回在线 Actor，重名为 nil"})
	Register(Entry{Name: "LActor.getActorIdByName", Fn: realGetActorIdByName, Meta: "Lua调用: 2处 | 在线角色名查询，未命中为 0"})
	Register(Entry{Name: "LActor.getActorByAccountName", Fn: realGetActorByAccountName, Meta: "Lua调用: 2处 | 仅返回在线 Actor，账号重名为 nil"})
	Register(Entry{Name: "LActor.getActorIdByAccountName", Fn: realGetActorIdByAccountName, Meta: "Lua调用: 1处 | 在线账号查询，未命中或重名为 0"})
	Register(Entry{Name: "LActor.isActor", Fn: realIsActor, Meta: "Lua调用: 5处 | 仅在线玩家实体为 true"})
	Register(Entry{Name: "LActor.getGmLevel", Fn: realGetGmLevel, Meta: "原版 gmsystem.lua:21 | 当前无 GM 模型，失败关闭返回 0"})
	Register(Entry{Name: "LActor.isImage", Fn: realIsImage, Meta: "原版 chapter.lua:490 | 当前仅真实 Actor，失败关闭返回 false"})
	Register(Entry{Name: "LActor.getChapterLevel", Fn: realGetChapterLevel, Meta: "Lua调用: 14处 | 已接 actors.chapterlevel"})
	Register(Entry{Name: "LActor.getMonthCard", Fn: realGetMonthCard, Meta: "Lua调用: 5处 | 已接 actors.monthcard"})
	Register(Entry{Name: "LActor.getReincarnateLv", Fn: realGetReincarnateLevel, Meta: "Lua调用: 5处 | 已接 actors.reincarnate_lv"})
	Register(Entry{Name: "LActor.getJob", Fn: realGetJob, Meta: "Lua调用: 高频 | 已接 actors.job"})
	Register(Entry{Name: "LActor.getActorJob", Fn: realGetJob, Meta: "Lua调用: 2处 | getJob 别名"})
	Register(Entry{Name: "LActor.getSex", Fn: realGetSex, Meta: "Lua调用: 高频 | 已接 actors.sex"})
	Register(Entry{Name: "LActor.getActorSex", Fn: realGetSex, Meta: "Lua调用: 2处 | getSex 别名"})
	Register(Entry{Name: "LActor.getSceneId", Fn: realGetSceneId, Meta: "Lua调用: 高频 | 已接 actors.sceneid"})
	Register(Entry{Name: "LActor.getExp", Fn: realGetExp, Meta: "Lua调用: 原版 actorexp/item | 已接 actors.exp"})
	Register(Entry{Name: "LActor.getActorCurExp", Fn: realGetExp, Meta: "Lua调用: 1处 | getExp 别名"})
	Register(Entry{Name: "LActor.getServerId", Fn: realGetServerId, Meta: "Lua调用: 67处 | 已接登录会话 ServerID"})
	Register(Entry{Name: "LActor.getAccountName", Fn: realGetAccountName, Meta: "Lua调用: 58处 | 已接 actors.accountname"})
	Register(Entry{Name: "LActor.getActorPower", Fn: realGetActorPower, Meta: "Lua调用: 15处 | actors.totalpower 登录快照"})
	Register(Entry{Name: "LActor.getAccountId", Fn: realGetAccountId, Meta: "Lua调用: 5处 | 已接 actors.accountid"})
	Register(Entry{Name: "LActor.getRecharge", Fn: realGetRecharge, Meta: "Lua调用: 12处 | 已接 actors.recharge"})
	Register(Entry{Name: "LActor.getCreateTime", Fn: realGetCreateTime, Meta: "Lua调用: 2处 | actors.createtime Unix 秒"})
	Register(Entry{Name: "LActor.getAppid", Fn: realGetAppID, Meta: "Lua调用: 原版 giftcode | 已接 actors.appid"})
	Register(Entry{Name: "LActor.getPf", Fn: realGetPfID, Meta: "Lua调用: 原版 giftcode/log | 已接 actors.pfid"})
	Register(Entry{Name: "LActor.getPfId", Fn: realGetPfID, Meta: "Lua调用: 原版 dkm/tlog | getPf 别名"})
	Register(Entry{Name: "LActor.getGamePf", Fn: realGetGamePf, Meta: "原版 actorfunc.lua:103-112 | union 前缀含 * 时取首个 * 后子串"})
	Register(Entry{Name: "LActor.getActorLevel", Fn: realGetLevel, Meta: "Lua调用: 5处 | getLevel 别名"})
	Register(Entry{Name: "LActor.getActorZhuanShengLevel", Fn: realGetZhuanShengLevel, Meta: "Lua调用: 3处 | getZhuanShengLevel 别名"})

	// === staticVar（T0 #13）===
	Register(Entry{Name: "LActor.getStaticVar", Fn: realGetStaticVar, Meta: "原版: Lua调用:163处 | 写时标脏 | 已接内存实体"})

	// === 货币（T0 #21,35,37）===
	Register(Entry{Name: "LActor.getCurrency", Fn: realGetCurrency, Meta: "Lua调用: 107处 | 已接内存实体"})
	Register(Entry{Name: "LActor.getMoneyCount", Fn: realGetCurrency, Meta: "Lua调用: 36处 | mt* 货币查询，离线为 0"})
	Register(Entry{Name: "LActor.changeCurrency", Fn: realChangeCurrency, Meta: "Lua调用: 69处 | 金币/元宝先落库后改内存，绑定币种仅内存"})
	Register(Entry{Name: "LActor.changeYuanBao", Fn: realChangeYuanBao, Meta: "Lua调用: 64处 | 元宝 changeCurrency 别名，先写 actors.yuanbao 再改内存"})

	// === 物品（T0 #15,18）===
	Register(Entry{Name: "LActor.getItemCount", Fn: realGetItemCount, Meta: "Lua调用: 134处 | Go原子读取 | 已接背包"})
	Register(Entry{Name: "LActor.getBagItemCount", Fn: realGetBagItemCount, Meta: "Lua调用: 5处 | 背包物品总数量 | 已接背包"})
	Register(Entry{Name: "LActor.costItem", Fn: realCostItem, Meta: "Lua调用: 126处 | Go原子扣减+存档+协议通知 | 已接背包"})
	Register(Entry{Name: "LActor.giveItem", Fn: realGiveItem, Meta: "Go发放: itemId,count | 已接背包"})

	// === 奖励（T0 #16,30）===
	Register(Entry{Name: "LActor.giveAwards", Fn: stubNoop, Meta: "Lua调用: 133处 | 占位"})
	Register(Entry{Name: "LActor.canGiveAwards", Fn: stubBool, Meta: "Lua调用: 77处 | 占位"})

	// === 事件/变量（T0 #14,29）===
	Register(Entry{Name: "LActor.postScriptEventLite", Fn: func(a ...luahost.Value) (luahost.Value, error) {
		return 0, nil // handle，待接定时器层（4.7）
	}, Meta: "Lua调用: 141处 | 占位（依赖 4.7 定时器）"})
	Register(Entry{Name: "LActor.cancelScriptEvent", Fn: stubNoop, Meta: "Lua调用: 84处 | 占位"})

	// === 角色（T0 #19,31）===
	Register(Entry{Name: "LActor.getRoleCount", Fn: realGetRoleCount, Meta: "Lua调用: 120处 | 仅主角色，在线为 1，离线为 0"})
	Register(Entry{Name: "LActor.getRole", Fn: realGetRole, Meta: "Lua调用: 75处 | index=0 返回主角色，其他索引或离线为 nil"})
	Register(Entry{Name: "LActor.getRoleSkillLevel", Fn: realGetRoleSkillLevel, Meta: "原版 skill.lua:18 | 主角色 roleid=0，技能槽 index=0..4，未学习或离线为 0"})
	Register(Entry{Name: "LActor.GetRoleByJob", Fn: realGetRoleByJob, Meta: "原版 zhanlingsystem.lua:318 | 在线主角色职业匹配返回 actorID，否则 nil"})

	// === 提示/日志（T0 #8,28）===
	Register(Entry{Name: "LActor.sendTipmsg", Fn: stubNoop, Meta: "Lua调用: 310处 | typeMask位掩码见 engine-api.md:134 | 占位"})
	Register(Entry{Name: "LActor.log", Fn: realActorLog, Meta: "Lua调用: 91处 | 本地 slog 结构化日志，不声称已接 7001"})

	// === 场景副本（T0 #34,40,41,48,50）===
	Register(Entry{Name: "LActor.stopAI", Fn: stubNoop, Meta: "原版 actorrole.lua:107 | 停止在线玩家自动战斗，保留显式目标；由运行时按 VM 绑定 Combat"})
	Register(Entry{Name: "LActor.setAITarget", Fn: stubNoop, Meta: "原版 ai_action.lua:108 | 仅支持玩家→怪物、怪物→玩家，同场景存活目标；由运行时按 VM 绑定 Combat"})
	Register(Entry{Name: "LActor.getAITarget", Fn: stubNoop, Meta: "原版 citysystem.lua:290 | 返回有效目标句柄，无目标或失效为 nil；由运行时按 VM 绑定 Combat"})
	Register(Entry{Name: "LActor.setAIAttackMonster", Fn: stubNoop, Meta: "原版 actorrole.lua:89 | 清显式目标并恢复在线玩家自动索敌；由运行时按 VM 绑定 Combat"})
	Register(Entry{Name: "LActor.setAITargetNull", Fn: stubNoop, Meta: "原版 citysystem.lua:455 | 清 AI 目标但保留停战；由运行时按 VM 绑定 Combat"})
	Register(Entry{Name: "LActor.getFubenHandle", Fn: realGetFubenHandle, Meta: "原版 citysystem.lua:733 | 当前无副本实例，失败关闭返回 0"})
	Register(Entry{Name: "LActor.getFubenId", Fn: realGetFubenID, Meta: "原版 miscfunc.lua:202 | 当前无副本实例，失败关闭返回 0"})
	Register(Entry{Name: "LActor.isInFuben", Fn: realIsInFuben, Meta: "原版 campbattlefb.lua:627 | 当前无副本实例，失败关闭返回 false"})
	Register(Entry{Name: "LActor.getActor", Fn: realGetActor, Meta: "原版 worldboss.lua:940 | 在线玩家解析，非法或未命中为 nil"})
	Register(Entry{Name: "LActor.getEntity", Fn: realGetEntity, Meta: "原版 citysystem.lua:285 | 场景实体句柄解析，非法或未命中为 nil"})
	Register(Entry{Name: "LActor.getActorByEt", Fn: realGetActorByEt, Meta: "原版 citysystem.lua:290 | 玩家实体转在线 Actor，怪物或未命中为 nil"})
	Register(Entry{Name: "LActor.getHandle", Fn: realGetHandle, Meta: "原版 postscripttimer.lua:20 | 返回正整数句柄，非法或空参为 0"})
	Register(Entry{Name: "LActor.getEntityPosition", FnMulti: realGetEntityPosition, Meta: "原版 test_gathersystem.lua:47 | 返回场景实体像素坐标，未命中为 0,0"})
	Register(Entry{Name: "LActor.getIntProperty", Fn: realGetIntProperty, Meta: "原版 _G.lua:1011-1124 | 仅读取已证实属性白名单，未支持为 0"})
	Register(Entry{Name: "LActor.getHp", Fn: realGetHp, Meta: "原版 ai_action.lua:88 | World 实体优先，Actor HP 回退，未命中为 0"})
	Register(Entry{Name: "LActor.getHpMax", Fn: realGetHpMax, Meta: "原版 ai_action.lua:91 | World 实体优先，Actor MaxHP 回退，未命中为 0"})
	Register(Entry{Name: "LActor.getPosition", FnMulti: realGetPosition, Meta: "原版 newworldboss.lua:541 | 返回实体像素坐标，未命中为 0,0"})
	Register(Entry{Name: "LActor.isDeath", Fn: realIsDeath, Meta: "原版 togetherhit.lua:126 | World/Actor HP<=0，未命中为 false"})
	Register(Entry{Name: "LActor.getId", Fn: realGetId, Meta: "原版 bossinfo.lua:136 | 玩家 actorID，怪物 ConfigID，未命中为 0"})
	Register(Entry{Name: "LActor.exitFuben", Fn: stubNoop, Meta: "Lua调用: 45处 | 占位"})
	Register(Entry{Name: "LActor.enterFuBen", Fn: stubNoop, Meta: "Lua调用: 43处 | 占位"})

	// === 属性/阵营/公会/VIP（T0 #36,38,42,44,46,47）===
	Register(Entry{Name: "LActor.setIntProperty", Fn: stubNoop, Meta: "占位 | property ID 待 Ghidra 提取"})
	Register(Entry{Name: "LActor.getGuildId", Fn: realGetGuildID, Meta: "Lua调用: 63处 | 已接 actors.guildid"})
	Register(Entry{Name: "LActor.getGuildName", Fn: realGetGuildName, Meta: "Lua调用: 2处 | actors.guild_name 登录快照"})
	Register(Entry{Name: "LActor.setCamp", Fn: stubNoop, Meta: "推断 | 占位"})
	Register(Entry{Name: "LActor.getGuildPtr", Fn: stubInt1, Meta: "推断 | 占位"})
	Register(Entry{Name: "LActor.getVipLevel", Fn: realGetVipLevel, Meta: "Lua调用: 47处 | 已接 actors.vip_level"})
	Register(Entry{Name: "LActor.setVipLevel", Fn: realSetVipLevel, Meta: "Lua调用: 4处 | 先原子存档再更新内存"})
	Register(Entry{Name: "LActor.getVIPLevel", Fn: realGetVipLevel, Meta: "Lua调用: 1处 | getVipLevel 别名"})
	Register(Entry{Name: "LActor.reCalcAttr", Fn: stubNoop, Meta: "Lua调用:46处 | ADR-004 Go收口 | 占位"})
}

var defaultWorld atomic.Pointer[scene.World]

func setDefaultWorld(world *scene.World) {
	defaultWorld.Store(world)
}

func realGetEntity(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return nil, nil
	}
	world := defaultWorld.Load()
	if world == nil {
		return nil, nil
	}
	entity, ok := world.Entity(entityID)
	if !ok {
		return nil, nil
	}
	return entity.ID, nil
}

func realGetActorByEt(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return nil, nil
	}
	world := defaultWorld.Load()
	if world == nil {
		return nil, nil
	}
	entity, ok := world.Entity(entityID)
	if !ok || entity.Kind != scene.EntityPlayer {
		return nil, nil
	}
	actor := DefaultActorManager.Get(entityID)
	if actor == nil {
		return nil, nil
	}
	return actor.ID, nil
}

const (
	propertyID       = 0
	propertyPosX     = 1
	propertyPosY     = 2
	propertyHP       = 7
	propertyMaxHP    = 10
	propertyLevel    = 6
	propertySex      = 55
	propertyVocation = 56
	propertyExp      = 57
	propertyGuildID  = 70
	propertyBindCoin = 62
	propertyCoin     = 63
	propertyBindYB   = 64
	propertyYB       = 65
	propertyFight    = 100
	propertyRecharge = 102
	propertyVIPLevel = 103
)

func realGetHandle(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	handle, ok := actorHandleArgument(a[0])
	if !ok {
		return 0, nil
	}
	return handle, nil
}

func realGetEntityPosition(a ...luahost.Value) ([]luahost.Value, error) {
	if len(a) == 0 {
		return []luahost.Value{0, 0}, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return []luahost.Value{0, 0}, nil
	}
	world := defaultWorld.Load()
	if world == nil {
		return []luahost.Value{0, 0}, nil
	}
	entity, ok := world.Entity(entityID)
	if !ok {
		return []luahost.Value{0, 0}, nil
	}
	return []luahost.Value{entity.X, entity.Y}, nil
}

func realGetHp(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return 0, nil
	}
	world := defaultWorld.Load()
	if world != nil {
		if entity, ok := world.Entity(entityID); ok {
			return entity.HP, nil
		}
	}
	actor := DefaultActorManager.Get(entityID)
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.HP, nil
}

func realGetHpMax(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return 0, nil
	}
	world := defaultWorld.Load()
	if world != nil {
		if entity, ok := world.Entity(entityID); ok {
			return entity.MaxHP, nil
		}
	}
	actor := DefaultActorManager.Get(entityID)
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.MaxHP, nil
}

func realGetPosition(a ...luahost.Value) ([]luahost.Value, error) {
	if len(a) == 0 {
		return []luahost.Value{0, 0}, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return []luahost.Value{0, 0}, nil
	}
	world := defaultWorld.Load()
	if world != nil {
		if entity, ok := world.Entity(entityID); ok {
			return []luahost.Value{entity.X, entity.Y}, nil
		}
	}
	actor := DefaultActorManager.Get(entityID)
	if actor == nil {
		return []luahost.Value{0, 0}, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return []luahost.Value{actor.X, actor.Y}, nil
}

func realIsDeath(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return false, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return false, nil
	}
	world := defaultWorld.Load()
	if world != nil {
		if entity, ok := world.Entity(entityID); ok {
			return entity.HP <= 0, nil
		}
	}
	actor := DefaultActorManager.Get(entityID)
	if actor == nil {
		return false, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	return actor.HP <= 0, nil
}

func realGetId(a ...luahost.Value) (luahost.Value, error) {
	if len(a) == 0 {
		return 0, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return 0, nil
	}
	world := defaultWorld.Load()
	if world != nil {
		if entity, ok := world.Entity(entityID); ok {
			if entity.Kind == scene.EntityMonster {
				return entity.ConfigID, nil
			}
			return entity.ID, nil
		}
	}
	actor := DefaultActorManager.Get(entityID)
	if actor == nil {
		return 0, nil
	}
	return actor.ID, nil
}

func realGetIntProperty(a ...luahost.Value) (luahost.Value, error) {
	if len(a) < 2 {
		return 0, nil
	}
	entityID, ok := actorHandleArgument(a[0])
	if !ok {
		return 0, nil
	}
	property, err := integerArgument(a[1])
	if err != nil {
		return 0, nil
	}
	world := defaultWorld.Load()
	if world != nil {
		if entity, ok := world.Entity(entityID); ok {
			switch property {
			case propertyID:
				if entity.Kind == scene.EntityMonster {
					return entity.ConfigID, nil
				}
				return entity.ID, nil
			case propertyPosX:
				return entity.X, nil
			case propertyPosY:
				return entity.Y, nil
			case propertyHP:
				return entity.HP, nil
			case propertyMaxHP:
				return entity.MaxHP, nil
			}
		}
	}
	actor := DefaultActorManager.Get(entityID)
	if actor == nil {
		return 0, nil
	}
	actor.mu.RLock()
	defer actor.mu.RUnlock()
	switch property {
	case propertyID:
		return actor.ID, nil
	case propertyLevel:
		return actor.Level, nil
	case propertySex:
		return actor.Sex, nil
	case propertyVocation:
		return actor.Job, nil
	case propertyExp:
		return actor.Exp, nil
	case propertyGuildID:
		return actor.GuildID, nil
	case propertyBindCoin:
		return actor.Currency[0], nil
	case propertyCoin:
		return actor.Currency[currencyGold], nil
	case propertyBindYB:
		return actor.Currency[2], nil
	case propertyYB:
		return actor.Currency[currencyYuanBao], nil
	case propertyFight:
		return actor.TotalPower, nil
	case propertyRecharge:
		return actor.Recharge, nil
	case propertyVIPLevel:
		return actor.VIPLevel, nil
	default:
		return 0, nil
	}
}

func realGetGmLevel(_ ...luahost.Value) (luahost.Value, error) {
	return 0, nil
}

func realIsImage(_ ...luahost.Value) (luahost.Value, error) {
	return false, nil
}

func realGetFubenHandle(_ ...luahost.Value) (luahost.Value, error) {
	return 0, nil
}

func realGetFubenID(_ ...luahost.Value) (luahost.Value, error) {
	return 0, nil
}

func realIsInFuben(_ ...luahost.Value) (luahost.Value, error) {
	return false, nil
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
