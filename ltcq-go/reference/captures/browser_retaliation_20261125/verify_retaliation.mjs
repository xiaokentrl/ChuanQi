import fs from 'node:fs';
import path from 'node:path';

const outDir = 'reference/captures/browser_retaliation_20261125';
const monsterHandle = 1073741824;
const playerHandle = 65537;
const loginAccount = 'abc';
const url = `http://localhost/?uid=aabb&trace=${Date.now()}&retaliation=1`;
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
const wsConnections = [];
ws.onmessage = event => {
  const message = JSON.parse(event.data);
  if (message.id && pending.has(message.id)) {
    const { resolve, reject } = pending.get(message.id);
    pending.delete(message.id);
    message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result);
    return;
  }
  if (message.method === 'Network.webSocketCreated') {
    wsConnections.push({requestId: message.params.requestId, timestamp: message.params.timestamp, url: message.params.url});
  } else if (message.method === 'Network.webSocketFrameReceived') {
    wsFrames.push({requestId: message.params.requestId, direction: 'received', timestamp: message.params.timestamp, payload: message.params.response.payloadData});
  } else if (message.method === 'Network.webSocketFrameSent') {
    wsFrames.push({requestId: message.params.requestId, direction: 'sent', timestamp: message.params.timestamp, payload: message.params.response.payloadData});
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
  window.__retaliationProof = {sent: [], received: [], hp: [], damage: [], removed: [], goldChanges: [], expChanges: [], errors: []};
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
        if (Array.isArray(result)) __retaliationProof.hp.push({handle: result[0] && result[0].handle, current: result[1], previous: result[2]});
      } catch (_) {}
      return result;
    };
    const damage = logic.postEntityHpChange;
    logic.postEntityHpChange = function(attacker, target, type, amount) {
      try {
        const handle = entity => entity?.handle ?? entity?.infoModel?.handle;
        __retaliationProof.damage.push({attacker: handle(attacker), target: handle(target), type, amount});
      } catch (_) {}
      return damage.apply(this, arguments);
    };
    const removed = logic.postRemoveEntity;
    logic.postRemoveEntity = function(handle, entity) {
      try {
        __retaliationProof.removed.push({handle, type: entity && entity.infoModel && entity.infoModel.type});
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
      try { __retaliationProof.goldChanges.push({value, gold: actor.gold}); } catch (_) {}
      return result;
    };
    const exp = actor.postExp;
    actor.postExp = function(packet) {
      const result = exp.apply(this, arguments);
      try { __retaliationProof.expChanges.push({level: actor.level, exp: actor.exp}); } catch (_) {}
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
      try { __retaliationProof.sent.push(hex(data)); } catch (_) {}
      return send(data);
    };
    raw.addEventListener('message', async event => {
      try {
        let data = event.data;
        if (data instanceof Blob) data = await data.arrayBuffer();
        __retaliationProof.received.push(hex(data));
      } catch (error) {
        __retaliationProof.errors.push(String(error));
      }
    });
    socketPatched = true;
  };
  setInterval(() => {
    try { patchLogic(); patchActor(); patchSocket(); } catch (error) { __retaliationProof.errors.push(String(error)); }
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
  const proof = gameWindow.__retaliationProof || window.__retaliationProof;
  const out = {
    url: location.href,
    ready: document.readyState,
    loginAccount: gameWindow.Main?.init?.editableText?.text ?? null,
    loginLoading: gameWindow.Main?.init?.loadingGroup?.visible ?? null,
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
  if (Actor && Actor.ins) out.actor = {level: Actor.level, exp: Actor.exp, gold: Actor.gold, hp: Actor.hp, maxHp: Actor.maxHp};
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

async function clickAt(x, y) {
  await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x, y });
  await send('Input.dispatchMouseEvent', { type: 'mousePressed', x, y, button: 'left', clickCount: 1 });
  await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x, y, button: 'left', clickCount: 1 });
}

async function selectLoginAccount() {
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    const input = await evaluate(`(() => {
      const frame = document.querySelector('iframe');
      const gameWindow = frame?.contentWindow;
      const login = gameWindow?.Main?.init;
      const editable = login?.editableText;
      if (!frame || !editable?.visible || login.loadingGroup?.visible) return null;
      const rect = frame.getBoundingClientRect();
      if (login.createGongGaoGroup?.visible) {
        return {notice: true, x: rect.left + rect.width * 0.88, y: rect.top + rect.height * 0.215};
      }
      const point = login.group.localToGlobal(editable.x + editable.width / 2, editable.y + editable.height / 2);
      return {account: editable.text, x: rect.left + point.x, y: rect.top + point.y};
    })()`);
    if (input) {
      if (input.notice) {
        await clickAt(input.x, input.y);
        await sleep(300);
        continue;
      }
      if (input.account === loginAccount) return;
      await clickAt(input.x, input.y);
      const focused = await evaluate(`(() => {
        const frame = document.querySelector('iframe');
        const editable = frame?.contentWindow?.Main?.init?.editableText;
        editable?.inputUtils?.stageText?.$show();
        const input = frame?.contentDocument?.querySelector('#egretInput');
        input?.focus();
        input?.select();
        const active = frame?.contentDocument?.activeElement;
        return {focused: active?.id === 'egretInput', active: active?.id, value: active?.value};
      })()`);
      console.log('login-focus', JSON.stringify(focused));
      if (!focused.focused) throw new Error('登录账号输入框聚焦失败');
      await send('Input.dispatchKeyEvent', { type: 'rawKeyDown', key: 'a', code: 'KeyA', windowsVirtualKeyCode: 65, modifiers: 2 });
      await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'a', code: 'KeyA', windowsVirtualKeyCode: 65, modifiers: 2 });
      await send('Input.insertText', { text: loginAccount });
      await waitFor(current => current.loginAccount === loginAccount, '账号输入未生效', 5000);
      return;
    }
    await sleep(300);
  }
  throw new Error('登录账号输入框未就绪');
}

async function clickStart() {
  const ui = await evaluate(`(() => {
    const frame = document.querySelector('iframe');
    const login = frame && frame.contentWindow && frame.contentWindow.Main && frame.contentWindow.Main.init;
    const gameSocket = frame && frame.contentWindow && frame.contentWindow.GameSocket && frame.contentWindow.GameSocket.ins();
    if (!login) return null;
    return {
      ready: !!login.clickLabel && login.clickLabel.touchEnabled && !!login.selectSerInfo,
      noticeVisible: !!login.createGongGaoGroup && login.createGongGaoGroup.visible,
      socketStatus: gameSocket?._socketStatus ?? 0,
      loading: !!login.loadingGroup && login.loadingGroup.visible
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
  if (!ui.noticeVisible && (ui.socketStatus !== 0 || ui.loading)) {
    started = true;
    return true;
  }
  const xRatio = ui.noticeVisible ? 0.88 : 0.5;
  const yRatio = ui.noticeVisible ? 0.215 : 0.744;
  const x = canvas.x + canvas.width * xRatio;
  const y = canvas.y + canvas.height * yRatio;
  await clickAt(x, y);
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

try {
await send('Page.navigate', { url });
await send('Page.bringToFront');
await selectLoginAccount();
await waitForScene();
await waitFor(current => current.entities.some(entity => entity.handle === monsterHandle), '等待首只怪物');
const initialState = state;
await capture('retaliation_monster_initial.png');

await waitFor(current => current.proof?.damage >= 6, '等待双方伤害飘字');
const midState = state;
await capture('retaliation_damage_visible.png');

await waitFor(current =>
  current.proof?.removed >= 1 &&
  current.actor?.gold >= 1000 && current.actor?.exp >= 100,
  '等待首次击杀奖励');
const firstKillState = state;
await capture('retaliation_first_kill.png');
const clientProof = await evaluate(`document.querySelector('iframe')?.contentWindow.__retaliationProof || window.__retaliationProof`);

const parsedFrames = wsFrames.map(frame => {
  const bytes = Buffer.from(frame.payload, 'base64');
  const headerOffset = frame.direction === 'sent' ? 8 : 4;
  return {
    requestId: frame.requestId,
    direction: frame.direction,
    timestamp: frame.timestamp,
    hex: bytes.toString('hex'),
    sysid: bytes.length >= headerOffset + 2 ? bytes[headerOffset] : null,
    cmdid: bytes.length >= headerOffset + 2 ? bytes[headerOffset + 1] : null
  };
});
const loginFrames = parsedFrames.filter(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
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
});
const monsterHpFrames = hpFrames.filter(frame => frame.handle === monsterHandle);
const playerHpFrames = hpFrames.filter(frame => frame.handle === playerHandle);
const damageFrames = baseFrames.filter(frame => frame.cmdid === 32).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {
    timestamp: frame.timestamp,
    attacker: bytes.readDoubleLE(6),
    target: bytes.readDoubleLE(14),
    damage: bytes.readDoubleLE(22),
    damageType: bytes.readUInt32LE(30)
  };
});
const playerDamageFrames = damageFrames.filter(frame => frame.attacker === playerHandle && frame.target === monsterHandle);
const monsterDamageFrames = damageFrames.filter(frame => frame.attacker === monsterHandle && frame.target === playerHandle);
const removeFrames = baseFrames.filter(frame => frame.cmdid === 10).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {timestamp: frame.timestamp, handle: bytes.readDoubleLE(6)};
}).filter(frame => frame.handle === monsterHandle);
const retaliationGaps = monsterDamageFrames.slice(1).map((frame, index) => frame.timestamp - monsterDamageFrames[index].timestamp);
const firstKillPlayer = firstKillState.entities.find(entity => entity.handle === playerHandle);
const validation = {
  cleanFixture: initialState.actor?.exp === 0 && initialState.actor?.gold === 0,
  singleLoginAttempt: wsConnections.filter(item => item.url.includes(':19002')).length === 1 && loginFrames.length === 1,
  sceneEntered: initialState.scene && initialState.map?.id === 2,
  monsterCreated: monsterEntityFrames.length === 1,
  playerDamageFloating: playerDamageFrames.length === 20 && clientProof.damage.filter(item => item.attacker === playerHandle && item.target === monsterHandle).length === 20,
  monsterDamageFloating: monsterDamageFrames.length === 5 && clientProof.damage.filter(item => item.attacker === monsterHandle && item.target === playerHandle && item.amount === -10).length === 5,
  playerHpPacketSequence: playerHpFrames.length === 5 && playerHpFrames.every((frame, index) => frame.hp === 90 - index * 10),
  retaliationIntervalRespected: retaliationGaps.length === 4 && retaliationGaps.every(gap => gap >= 1.8),
  monsterHpReachedZero: monsterHpFrames.filter(frame => frame.hp === 0).length === 1,
  clientPlayerHpUpdated: firstKillPlayer?.hp === 50 && firstKillPlayer?.max === 100 && firstKillPlayer?.parent,
  firstRewardGoldPacket: goldFrames.length === 1 && goldFrames[0]?.gold === 1000,
  firstRewardExpPacket: expFrames.length === 1 && expFrames[0]?.added === 100 && expFrames[0]?.exp === 100,
  firstRemoval: removeFrames.length === 1,
  noClientErrors: clientProof.errors.length === 0
};
if (Object.values(validation).some(value => !value)) {
  throw new Error('retaliation validation failed: ' + JSON.stringify({validation, wsConnections, loginFrames, initialState, midState, firstKillState, monsterEntityFrames, monsterHpFrames, playerHpFrames, playerDamageFrames, monsterDamageFrames, goldFrames, expFrames, removeFrames, clientProof}));
}
const proof = {
  date: '2026-11-25',
  url,
  gateway: '127.0.0.1:19002',
  gameworld: '127.0.0.1:16001',
  database: 'actor_s1_m5e_20261125_01',
  initialState,
  midState,
  firstKillState,
  clientProof,
  validation,
  wsConnections,
  loginFrames,
  actorBaseFrames,
  goldFrames,
  expFrames,
  monsterEntityFrames,
  monsterHpFrames,
  playerHpFrames,
  playerDamageFrames,
  monsterDamageFrames,
  removeFrames,
  retaliationGaps,
  wsFrameCount: parsedFrames.length
};
fs.writeFileSync(path.join(outDir, 'retaliation_browser_proof.json'), JSON.stringify(proof, null, 2));
console.log(JSON.stringify({
  validation,
  wsConnections,
  loginFrameCount: loginFrames.length,
  monsterHpZeroCount: monsterHpFrames.filter(frame => frame.hp === 0).length,
  playerHpFrames,
  playerDamagePacketCount: playerDamageFrames.length,
  monsterDamagePacketCount: monsterDamageFrames.length,
  retaliationGaps,
  clientDamageCount: clientProof.damage.length,
  firstKillState,
  wsFrameCount: parsedFrames.length
}, null, 2));
} finally {
  await fetch(`http://127.0.0.1:9223/json/close/${target.id}`).catch(() => {});
  ws.close();
}
