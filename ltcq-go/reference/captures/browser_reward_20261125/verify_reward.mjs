import fs from 'node:fs';
import path from 'node:path';

const outDir = 'reference/captures/browser_reward_20261125';
const monsterHandle = 1073741824;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

const targets = await (await fetch('http://127.0.0.1:9223/json/list')).json();
let target = targets.find(item => item.type === 'page' && item.url === 'about:blank');
if (!target) {
  target = await (await fetch('http://127.0.0.1:9223/json/new?about:blank', { method: 'PUT' })).json();
}

const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  ws.onopen = resolve;
  ws.onerror = reject;
});

let nextID = 0;
const pending = new Map();
const wsFrames = [];
ws.onmessage = event => {
  const message = JSON.parse(event.data);
  if (message.id && pending.has(message.id)) {
    const { resolve, reject } = pending.get(message.id);
    pending.delete(message.id);
    message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result);
    return;
  }
  if (message.method === 'Network.webSocketFrameReceived') {
    wsFrames.push({ direction: 'received', timestamp: message.params.timestamp, payload: message.params.response.payloadData });
  } else if (message.method === 'Network.webSocketFrameSent') {
    wsFrames.push({ direction: 'sent', timestamp: message.params.timestamp, payload: message.params.response.payloadData });
  }
};

function send(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++nextID;
    pending.set(id, { resolve, reject });
    ws.send(JSON.stringify({ id, method, params }));
  });
}

async function evaluate(expression) {
  const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.exception?.description || JSON.stringify(result.exceptionDetails));
  }
  return result.result.value;
}

const init = `(() => {
  window.__rewardProof = {sent: [], received: [], hp: [], damage: [], removed: [], goldChanges: [], expChanges: [], errors: []};
  let logicPatched = false;
  let actorPatched = false;
  let socketPatched = false;
  const hex = value => Array.from(new Uint8Array(value)).map(v => v.toString(16).padStart(2, '0')).join('');
  const patchLogic = () => {
    if (logicPatched || typeof GameLogic === 'undefined' || !GameLogic.ins) return;
    const logic = GameLogic.ins();
    const hp = logic.postHpChange;
    logic.postHpChange = function(packet) {
      const result = hp.apply(this, arguments);
      try {
        if (Array.isArray(result)) __rewardProof.hp.push({handle: result[0] && result[0].handle, current: result[1], previous: result[2]});
      } catch (_) {}
      return result;
    };
    const damage = logic.postEntityHpChange;
    logic.postEntityHpChange = function(attacker, target, type, amount) {
      try {
        __rewardProof.damage.push({attacker: attacker && attacker.handle, target: target && target.handle, type, amount});
      } catch (_) {}
      return damage.apply(this, arguments);
    };
    const removed = logic.postRemoveEntity;
    logic.postRemoveEntity = function(handle, entity) {
      try {
        __rewardProof.removed.push({handle, type: entity && entity.infoModel && entity.infoModel.type});
      } catch (_) {}
      return removed.apply(this, arguments);
    };
    logicPatched = true;
  };
  const patchActor = () => {
    if (actorPatched || typeof Actor === 'undefined' || !Actor.ins) return;
    const actor = Actor.ins();
    const gold = actor.postGoldChange;
    actor.postGoldChange = function(value) {
      const result = gold.apply(this, arguments);
      try { __rewardProof.goldChanges.push({value, gold: actor.gold}); } catch (_) {}
      return result;
    };
    const exp = actor.postExp;
    actor.postExp = function(packet) {
      const result = exp.apply(this, arguments);
      try { __rewardProof.expChanges.push({level: actor.level, exp: actor.exp}); } catch (_) {}
      return result;
    };
    actorPatched = true;
  };
  const patchSocket = () => {
    if (socketPatched || typeof GameSocket === 'undefined' || !GameSocket.ins) return;
    const wrapper = GameSocket.ins().socket_;
    const raw = wrapper && wrapper.socket && wrapper.socket.socket;
    if (!raw || typeof raw.send !== 'function') return;
    const send = raw.send.bind(raw);
    raw.send = function(data) {
      try { __rewardProof.sent.push(hex(data)); } catch (_) {}
      return send(data);
    };
    raw.addEventListener('message', async event => {
      try {
        let data = event.data;
        if (data instanceof Blob) data = await data.arrayBuffer();
        __rewardProof.received.push(hex(data));
      } catch (error) {
        __rewardProof.errors.push(String(error));
      }
    });
    socketPatched = true;
  };
  setInterval(() => {
    try { patchLogic(); patchActor(); patchSocket(); } catch (error) { __rewardProof.errors.push(String(error)); }
  }, 5);
})();`;

const stateExpression = `(() => {
  const gameWindow = document.querySelector('iframe')?.contentWindow || window;
  const GameLogic = gameWindow.GameLogic;
  const GameMap = gameWindow.GameMap;
  const EntityManager = gameWindow.EntityManager;
  const AttributeType = gameWindow.AttributeType;
  const Actor = gameWindow.Actor;
  const egret = gameWindow.egret;
  const proof = gameWindow.__rewardProof || window.__rewardProof;
  const out = {
    url: location.href,
    ready: document.readyState,
    scene: false,
    map: null,
    actor: null,
    entities: [],
    monsterOnStage: false,
    proof: proof ? {
      hp: proof.hp.length,
      damage: proof.damage.length,
      removed: proof.removed.length,
      goldChanges: proof.goldChanges,
      expChanges: proof.expChanges,
      errors: proof.errors
    } : null
  };
  if (GameLogic && GameLogic.ins && GameLogic.ins().gamescene) out.scene = egret.getQualifiedClassName(GameLogic.ins().gamescene);
  if (Actor && Actor.ins) out.actor = {level: Actor.level, exp: Actor.exp, gold: Actor.gold};
  if (GameMap) out.map = {id: GameMap.mapID, fuben: GameMap.fubenID, mine: GameMap.sceneInMine()};
  if (EntityManager) {
    out.entities = Object.values(EntityManager.ins().getAllEntity()).map(entity => ({
      handle: entity.handle ?? (entity.infoModel && entity.infoModel.handle),
      type: entity.infoModel && entity.infoModel.type,
      name: entity.infoModel && entity.infoModel.name,
      configID: entity.infoModel && entity.infoModel.configID,
      x: entity.x,
      y: entity.y,
      hp: entity.infoModel && entity.infoModel.getAtt(AttributeType.atHp),
      max: entity.infoModel && entity.infoModel.getAtt(AttributeType.atMaxHp),
      parent: !!entity.parent
    }));
  }
  if (egret && egret.lifecycle && egret.lifecycle.stage) {
    const walk = object => {
      if (!object) return;
      if (object.infoModel && object.infoModel.handle === ${monsterHandle}) out.monsterOnStage = true;
      if (object.numChildren) for (let i = 0; i < object.numChildren; i++) walk(object.getChildAt(i));
    };
    walk(egret.lifecycle.stage);
  }
  return out;
})()`;

await send('Page.enable');
await send('Runtime.enable');
await send('Network.enable');
await send('Emulation.setDeviceMetricsOverride', { width: 580, height: 930, deviceScaleFactor: 1, mobile: false });
await send('Page.addScriptToEvaluateOnNewDocument', { source: init });

let noticeClosed = false;
let started = false;
let proofInjected = false;
let state = {};

async function clickStart() {
  const ui = await evaluate(`(() => {
    const frame = document.querySelector('iframe');
    const login = frame && frame.contentWindow && frame.contentWindow.Main && frame.contentWindow.Main.init;
    if (!login) return null;
    return {
      ready: !!login.clickLabel && login.clickLabel.touchEnabled && !!login.selectSerInfo,
      noticeVisible: !!login.createGongGaoGroup && login.createGongGaoGroup.visible
    };
  })()`);
  if (!ui?.ready) return false;
  const canvas = await evaluate(`(() => {
    const frame = document.querySelector('iframe');
    if (!frame) return null;
    const rect = frame.getBoundingClientRect();
    const canvas = frame.contentDocument && frame.contentDocument.querySelector('canvas');
    if (!canvas || rect.width < 100 || rect.height < 100) return null;
    const inner = canvas.getBoundingClientRect();
    return {x: rect.left + inner.left, y: rect.top + inner.top, width: inner.width, height: inner.height};
  })()`);
  if (!canvas) return false;
  const xRatio = ui.noticeVisible ? 0.88 : 0.5;
  const yRatio = ui.noticeVisible ? 0.215 : 0.744;
  const x = canvas.x + canvas.width * xRatio;
  const y = canvas.y + canvas.height * yRatio;
  await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x, y });
  await send('Input.dispatchMouseEvent', { type: 'mousePressed', x, y, button: 'left', clickCount: 1 });
  await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x, y, button: 'left', clickCount: 1 });
  if (ui.noticeVisible) {
    await sleep(300);
    const noticeVisible = await evaluate(`(() => {
      const frame = document.querySelector('iframe');
      const login = frame && frame.contentWindow && frame.contentWindow.Main && frame.contentWindow.Main.init;
      return !!login?.createGongGaoGroup?.visible;
    })()`);
    noticeClosed = !noticeVisible;
  } else {
    started = true;
  }
  return true;
}

async function waitForScene(timeout = 80000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    await sleep(500);
    state = await evaluate(stateExpression);
    if (!proofInjected && await evaluate(`!!document.querySelector('iframe')?.contentWindow`)) {
      await evaluate(`document.querySelector('iframe').contentWindow.eval(${JSON.stringify(init)})`);
      proofInjected = true;
    }
    if (!noticeClosed || !started) await clickStart();
    if (state.scene && state.proof && state.map?.id === 2) return state;
  }
  throw new Error('进入场景超时: ' + JSON.stringify(state));
}

async function waitFor(check, label, timeout = 50000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    await sleep(300);
    state = await evaluate(stateExpression);
    if (check(state)) return state;
  }
  throw new Error(label + ' 超时: ' + JSON.stringify(state));
}

async function capture(name) {
  const shot = await send('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(path.join(outDir, name), Buffer.from(shot.data, 'base64'));
}

await send('Page.navigate', { url: 'http://localhost/?uid=aabb&trace=1795553070&reward=1' });
await waitForScene();
await waitFor(current => current.entities.some(entity => entity.handle === monsterHandle), '等待首只怪物');
const initialState = state;
await capture('reward_monster_initial.png');

await waitFor(current => current.proof?.damage >= 6, '等待伤害飘字');
const midState = state;
await capture('reward_damage_visible.png');

await waitFor(current =>
  current.proof?.removed >= 1 &&
  current.actor?.gold >= 1000 && current.actor?.exp >= 100,
  '等待首次击杀奖励');
const firstRewardState = state;
await capture('reward_first_kill.png');

await waitFor(current => current.entities.some(entity =>
  entity.handle === monsterHandle && entity.hp === 2000 && entity.parent
), '等待怪物满血重生', 15000);
const respawnState = state;
await capture('reward_monster_respawned.png');

const secondDamageTarget = respawnState.proof.damage + 6;
await waitFor(current => current.proof?.damage >= secondDamageTarget, '等待重生后伤害飘字');
await waitFor(current =>
  current.proof?.removed >= 2 &&
  current.actor?.gold >= 2000 && current.actor?.exp >= 200,
  '等待第二次击杀奖励');
const secondRewardState = state;
await capture('reward_second_kill.png');
const cycleProof = await evaluate(`document.querySelector('iframe')?.contentWindow.__rewardProof || window.__rewardProof`);
const socketClosed = await evaluate(`(() => {
  const gameWindow = document.querySelector('iframe')?.contentWindow || window;
  const wrapper = gameWindow.GameSocket?.ins?.().socket_;
  const raw = wrapper?.socket?.socket;
  if (raw) raw.close();
  return !!raw;
})()`);
if (!socketClosed) throw new Error('无法关闭重登前的游戏 WebSocket');
await sleep(1000);
const reloginTimestamp = Date.now() / 1000;
await send('Page.navigate', { url: 'http://localhost/?uid=aabb&trace=1795553070&reward=1&relogin=1' });
noticeClosed = false;
started = false;
proofInjected = false;
await waitForScene();
await waitFor(current => current.actor?.gold === 2000 && current.actor?.exp === 200, '等待重登恢复奖励进度');
const reloginState = state;
await capture('reward_relogin_persisted.png');
const reloginProof = await evaluate(`document.querySelector('iframe')?.contentWindow.__rewardProof || window.__rewardProof`);

const parsedFrames = wsFrames.map(frame => {
  const bytes = Buffer.from(frame.payload, 'base64');
  return {
    direction: frame.direction,
    timestamp: frame.timestamp,
    hex: bytes.toString('hex'),
    sysid: bytes.length >= 5 ? bytes[4] : null,
    cmdid: bytes.length >= 6 ? bytes[5] : null
  };
});
const baseFrames = parsedFrames.filter(frame => frame.direction === 'received' && frame.sysid === 0);
const actorBaseFrames = baseFrames.filter(frame => frame.cmdid === 1).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {
    timestamp: frame.timestamp,
    actorID: bytes.readDoubleLE(6),
    level: bytes.readInt32LE(55),
    exp: bytes.readInt32LE(59),
    gold: Number(bytes.readBigUInt64LE(71))
  };
});
const goldFrames = baseFrames.filter(frame => frame.cmdid === 5).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {timestamp: frame.timestamp, moneyType: bytes.readInt16LE(6), gold: Number(bytes.readBigUInt64LE(8))};
});
const expFrames = baseFrames.filter(frame => frame.cmdid === 7).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {timestamp: frame.timestamp, level: bytes.readInt32LE(6), exp: bytes.readInt32LE(10), added: bytes.readInt32LE(14)};
});
const monsterEntityFrames = baseFrames.filter(frame => frame.cmdid === 4).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {
    timestamp: frame.timestamp,
    type: bytes.readUInt16LE(6),
    handle: bytes.readDoubleLE(8),
    configID: bytes.readInt32LE(16),
    x: bytes.readInt32LE(28),
    y: bytes.readInt32LE(32),
    hp: bytes.readDoubleLE(38),
    max: bytes.readDoubleLE(54)
  };
}).filter(entity => entity.type === 2 && entity.handle === monsterHandle);
const hpFrames = baseFrames.filter(frame => frame.cmdid === 9).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {timestamp: frame.timestamp, handle: bytes.readDoubleLE(6), hp: bytes.readDoubleLE(14)};
}).filter(frame => frame.handle === monsterHandle);
const damageFrames = baseFrames.filter(frame => frame.cmdid === 32).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {
    timestamp: frame.timestamp,
    attacker: bytes.readDoubleLE(6),
    target: bytes.readDoubleLE(14),
    damage: bytes.readDoubleLE(22),
    damageType: bytes.readUInt32LE(30)
  };
}).filter(frame => frame.target === monsterHandle);
const removeFrames = baseFrames.filter(frame => frame.cmdid === 10).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {timestamp: frame.timestamp, handle: bytes.readDoubleLE(6)};
}).filter(frame => frame.handle === monsterHandle);

const cycleMonsterFrames = monsterEntityFrames.filter(frame => frame.timestamp < reloginTimestamp);
const firstRespawn = cycleMonsterFrames[1];
const reloginActorBase = actorBaseFrames.at(-1);
const firstCycleGold = goldFrames.filter(frame => frame.timestamp < firstRespawn?.timestamp);
const firstCycleExp = expFrames.filter(frame => frame.timestamp < firstRespawn?.timestamp);
const validation = {
  sceneEntered: initialState.scene && initialState.map?.id === 2,
  monsterCreated: monsterEntityFrames.length >= 3,
  damageFloating: cycleProof.damage.length >= 40,
  hpReachedZeroTwice: hpFrames.filter(frame => frame.hp === 0).length >= 2,
  firstRewardGoldPacket: firstCycleGold.length === 1 && firstCycleGold[0]?.gold === 1000,
  firstRewardExpPacket: firstCycleExp.length === 1 && firstCycleExp[0]?.added === 100 && firstCycleExp[0]?.exp === 100,
  clientGoldUpdated: cycleProof.goldChanges.some(item => item.value === 1000) && cycleProof.goldChanges.some(item => item.value === 2000),
  clientExpUpdated: firstRewardState.actor?.exp === 100 && secondRewardState.actor?.exp === 200,
  firstRemoval: removeFrames.filter(frame => frame.timestamp < firstRespawn?.timestamp).length === 1,
  respawnSameHandleFullHP: firstRespawn?.hp === 2000 && firstRespawn?.max === 2000,
  respawnClientVisible: respawnState.entities.some(entity => entity.handle === monsterHandle && entity.hp === 2000 && entity.parent) && respawnState.monsterOnStage,
  secondRewardGoldPacket: goldFrames.length === 2 && goldFrames[1]?.gold === 2000,
  secondRewardExpPacket: expFrames.length === 2 && expFrames[1]?.added === 100 && expFrames[1]?.exp === 200,
  secondRemoval: removeFrames.filter(frame => frame.timestamp < reloginTimestamp).length === 2,
  reloginActorBasePersisted: actorBaseFrames.length === 2 && reloginActorBase?.level === 51 && reloginActorBase?.exp === 200 && reloginActorBase?.gold === 2000,
  reloginClientPersisted: reloginState.actor?.level === 51 && reloginState.actor?.exp === 200 && reloginState.actor?.gold === 2000,
  noClientErrors: cycleProof.errors.length === 0 && reloginProof.errors.length === 0
};
if (Object.values(validation).some(value => !value)) {
  throw new Error('reward validation failed: ' + JSON.stringify({validation, initialState, firstRewardState, respawnState, secondRewardState, reloginState, actorBaseFrames, goldFrames, expFrames, monsterEntityFrames, hpFrames, damageFrames, removeFrames}));
}
const proof = {
  date: '2026-11-25',
  url: 'http://localhost/?uid=aabb&trace=1795553070&reward=1',
  gateway: '127.0.0.1:19002',
  gameworld: '127.0.0.1:16001',
  database: 'actor_s1_m5d_20261125',
  initialState,
  midState,
  firstRewardState,
  respawnState,
  secondRewardState,
  reloginState,
  cycleClientProof: cycleProof,
  reloginClientProof: reloginProof,
  validation,
  actorBaseFrames,
  goldFrames,
  expFrames,
  monsterEntityFrames,
  hpFrames,
  damageFrames,
  removeFrames,
  wsFrameCount: parsedFrames.length
};
fs.writeFileSync(path.join(outDir, 'reward_browser_proof.json'), JSON.stringify(proof, null, 2));
console.log(JSON.stringify({
  validation,
  actorBaseFrames,
  goldFrames,
  expFrames,
  monsterEntityFrames,
  hpZeroCount: hpFrames.filter(frame => frame.hp === 0).length,
  damagePacketCount: damageFrames.length,
  removalPacketCount: removeFrames.length,
  clientDamageCount: cycleProof.damage.length,
  clientGoldChanges: cycleProof.goldChanges,
  clientExpChanges: cycleProof.expChanges,
  reloginState,
  wsFrameCount: parsedFrames.length
}, null, 2));
ws.close();
