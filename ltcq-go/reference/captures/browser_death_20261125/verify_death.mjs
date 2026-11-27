import fs from 'node:fs';
import path from 'node:path';

const outDir = 'reference/captures/browser_death_20261125';
const monsterHandle = 1073741824;
const playerHandle = 65537;
const loginAccount = 'haohaiyao';
const url = `http://localhost/?uid=aabb&trace=${Date.now()}&death=1`;
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
    wsConnections.push({ requestId: message.params.requestId, timestamp: message.params.timestamp, url: message.params.url });
  } else if (message.method === 'Network.webSocketFrameReceived') {
    wsFrames.push({ requestId: message.params.requestId, direction: 'received', timestamp: message.params.timestamp, payload: message.params.response.payloadData });
  } else if (message.method === 'Network.webSocketFrameSent') {
    wsFrames.push({ requestId: message.params.requestId, direction: 'sent', timestamp: message.params.timestamp, payload: message.params.response.payloadData });
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
  if (window.__deathProof) return;
  window.__deathProof = {sent: [], received: [], hp: [], hpCalls: 0, damage: [], removed: [], playerRemoved: [], dieSounds: [], goldChanges: [], expChanges: [], errors: []};
  let logicPatched = false;
  let actorPatched = false;
  let socketPatched = false;
  let roleAIPatched = false;
  const hex = value => Array.from(new Uint8Array(value)).map(v => v.toString(16).padStart(2, '0')).join('');
  const patchLogic = () => {
    if (logicPatched || typeof GameLogic === 'undefined' || !GameLogic.ins) return;
    const logic = GameLogic.ins();
    const hp = logic.postHpChange;
    logic.postHpChange = function(packet) {
      __deathProof.hpCalls++;
      const result = hp.apply(this, arguments);
      try {
        if (Array.isArray(result)) __deathProof.hp.push({handle: result[0] && result[0].handle, current: result[1], previous: result[2]});
      } catch (_) {}
      return result;
    };
    const damage = logic.postEntityHpChange;
    logic.postEntityHpChange = function(attacker, target, type, amount) {
      try {
        const handle = entity => entity?.handle ?? entity?.infoModel?.handle;
        __deathProof.damage.push({attacker: handle(attacker), target: handle(target), type, amount});
      } catch (_) {}
      return damage.apply(this, arguments);
    };
    const removed = logic.postRemoveEntity;
    logic.postRemoveEntity = function(handle, entity) {
      try {
        __deathProof.removed.push({handle, type: entity && entity.infoModel && entity.infoModel.type});
        if (handle === ${playerHandle}) __deathProof.playerRemoved.push({handle, type: entity && entity.infoModel && entity.infoModel.type});
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
      try { __deathProof.goldChanges.push({value, gold: actor.gold}); } catch (_) {}
      return result;
    };
    const exp = actor.postExp;
    actor.postExp = function(packet) {
      const result = exp.apply(this, arguments);
      try { __deathProof.expChanges.push({level: actor.level, exp: actor.exp}); } catch (_) {}
      return result;
    };
    actorPatched = true;
  };
  const patchRoleAI = () => {
    if (roleAIPatched || typeof RoleAI === 'undefined' || !RoleAI.ins) return;
    const roleAI = RoleAI.ins();
    const check = roleAI.checkPlayDieSound;
    roleAI.checkPlayDieSound = function(entity) {
      try { __deathProof.dieSounds.push(entity?.handle ?? entity?.infoModel?.handle); } catch (_) {}
      return check.apply(this, arguments);
    };
    roleAIPatched = true;
  };
  const patchSocket = () => {
    if (socketPatched || typeof GameSocket === 'undefined' || !GameSocket.ins) return;
    const wrapper = GameSocket.ins().socket_;
    const raw = wrapper && wrapper.socket && wrapper.socket.socket;
    if (!raw || typeof raw.send !== 'function') return;
    const send = raw.send.bind(raw);
    raw.send = function(data) {
      try { __deathProof.sent.push(hex(data)); } catch (_) {}
      return send(data);
    };
    raw.addEventListener('message', async event => {
      try {
        let data = event.data;
        if (data instanceof Blob) data = await data.arrayBuffer();
        __deathProof.received.push(hex(data));
      } catch (error) {
        __deathProof.errors.push(String(error));
      }
    });
    socketPatched = true;
  };
  setInterval(() => {
    try { patchLogic(); patchActor(); patchRoleAI(); patchSocket(); } catch (error) { __deathProof.errors.push(String(error)); }
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
  const proof = gameWindow.__deathProof || window.__deathProof;
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
      hpCalls: proof.hpCalls,
      damage: proof.damage.length,
      removed: proof.removed.length,
      playerRemoved: proof.playerRemoved.length,
      dieSounds: proof.dieSounds.length,
      goldChanges: proof.goldChanges,
      expChanges: proof.expChanges,
      errors: proof.errors
    } : null
  };
  if (GameLogic && GameLogic.ins && GameLogic.ins().gamescene) out.scene = egret.getQualifiedClassName(GameLogic.ins().gamescene);
  if (Actor && Actor.ins) out.actor = {level: Actor.level, exp: Actor.exp, gold: Actor.gold, hp: Actor.hp, maxHp: Actor.maxHp, x: Actor.x, y: Actor.y};
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

function parseFrames() {
  return wsFrames.map((frame, index) => {
    const bytes = Buffer.from(frame.payload, 'base64');
    const headerOffset = frame.direction === 'sent' ? 8 : 4;
    return {
      index,
      requestId: frame.requestId,
      direction: frame.direction,
      timestamp: frame.timestamp,
      hex: bytes.toString('hex'),
      sysid: bytes.length >= headerOffset + 2 ? bytes[headerOffset] : null,
      cmdid: bytes.length >= headerOffset + 2 ? bytes[headerOffset + 1] : null
    };
  });
}

function loginSkey() {
  const login = parseFrames().find(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
  if (!login) throw new Error('尚未捕获登录帧，无法构造移动请求');
  return Buffer.from(login.hex, 'hex').readUInt32LE(0);
}

async function sendMovement(x, y) {
  const skey = loginSkey();
  return evaluate(`(() => {
    const gameWindow = document.querySelector('iframe')?.contentWindow || window;
    const raw = gameWindow.GameSocket.ins().socket_.socket.socket;
    const bytes = new Uint8Array(30);
    const view = new DataView(bytes.buffer);
    view.setUint32(0, ${skey}, true);
    view.setUint32(4, 9001, true);
    bytes[8] = 0;
    bytes[9] = 31;
    view.setFloat64(10, ${playerHandle}, true);
    view.setUint32(18, 1, true);
    view.setUint32(22, ${x}, true);
    view.setUint32(26, ${y}, true);
    raw.send(bytes.buffer);
    return Array.from(bytes).map(v => v.toString(16).padStart(2, '0')).join('');
  })()`);
}

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
      if (!focused.focused) throw new Error('登录账号输入框聚焦失败');
      await send('Input.dispatchKeyEvent', { type: 'rawKeyDown', key: 'a', code: 'KeyA', windowsVirtualKeyCode: 65, modifiers: 2 });
      await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'a', code: 'KeyA', windowsVirtualKeyCode: 65, modifiers: 2 });
      await send('Input.insertText', { text: loginAccount });
      await waitForState(current => current.loginAccount === loginAccount, '账号输入未生效', 5000);
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
  await clickAt(canvas.x + canvas.width * xRatio, canvas.y + canvas.height * yRatio);
  if (ui.noticeVisible) {
    await sleep(300);
    noticeClosed = !(await evaluate(`!!document.querySelector('iframe')?.contentWindow?.Main?.init?.createGongGaoGroup?.visible`));
  } else {
    started = true;
  }
  return true;
}

async function refreshState() {
  state = await evaluate(stateExpression);
  return state;
}

async function waitForState(check, label, timeout = 50000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    await sleep(100);
    await refreshState();
    if (check(state)) return state;
  }
  throw new Error(label + ' 超时: ' + JSON.stringify(state));
}

async function waitForScene() {
  const deadline = Date.now() + 80000;
  while (Date.now() < deadline) {
    await sleep(300);
    await refreshState();
    if (!proofInjected && await evaluate(`!!document.querySelector('iframe')?.contentWindow`)) {
      await evaluate(`document.querySelector('iframe').contentWindow.eval(${JSON.stringify(init)})`);
      proofInjected = true;
    }
    if (!noticeClosed || !started) await clickStart();
    if (state.scene && state.proof && state.map?.id === 2) return state;
  }
  throw new Error('进入场景超时: ' + JSON.stringify(state));
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
  await waitForState(current => current.entities.some(entity => entity.handle === monsterHandle), '等待首只怪物');
  const initialState = state;
  const initialPlayer = initialState.entities.find(entity => entity.handle === playerHandle);
  if (!initialPlayer) throw new Error('初始角色实体不存在: ' + JSON.stringify(initialState));
  await capture('death_initial_full_hp.png');

  const liveMoveHex = await sendMovement(384, 384);
  await waitForState(current => current.entities.some(entity => entity.handle === playerHandle && entity.x === 384 && entity.y === 384), '等待接近怪物');
  const preDeathState = state;
  await capture('death_before_lethal_hit.png');

  await waitForState(current => current.proof?.playerRemoved >= 1 && !current.entities.some(entity => entity.handle === playerHandle), '等待玩家致死');
  const deathState = state;
  const deathClientProof = await evaluate(`document.querySelector('iframe')?.contentWindow.__deathProof || window.__deathProof`);
  await capture('death_player_removed.png');
  const deadMoveFrameIndex = wsFrames.length;
  const deadMoveHex = await sendMovement(448, 384);
  await sleep(700);
  const frozenState = await refreshState();

  await waitForState(current => current.entities.some(entity => entity.handle === playerHandle && entity.hp > 0 && entity.parent), '等待安全复活', 10000);
  const revivedState = state;
  await capture('death_player_revived.png');
  const framesAtRevival = parseFrames();
  const baseAtRevival = framesAtRevival.filter(frame => frame.direction === 'received' && frame.sysid === 0);
  const playerDamageAtRevival = baseAtRevival.filter(frame => frame.cmdid === 32 && Buffer.from(frame.hex, 'hex').readDoubleLE(6) === playerHandle).length;
  await sleep(2200);
  const finalState = await refreshState();
  const clientProof = await evaluate(`document.querySelector('iframe')?.contentWindow.__deathProof || window.__deathProof`);

  const parsedFrames = parseFrames();
  const loginFrames = parsedFrames.filter(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
  const baseFrames = parsedFrames.filter(frame => frame.direction === 'received' && frame.sysid === 0);
  const entityFrames = baseFrames.filter(frame => frame.cmdid === 4).map(frame => {
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
  });
  const playerEntityFrames = entityFrames.filter(entity => entity.type === 1 && entity.handle === playerHandle);
  const monsterEntityFrames = entityFrames.filter(entity => entity.type === 2 && entity.handle === monsterHandle);
  const hpFrames = baseFrames.filter(frame => frame.cmdid === 9).map(frame => {
    const bytes = Buffer.from(frame.hex, 'hex');
    return { timestamp: frame.timestamp, handle: bytes.readDoubleLE(6), hp: bytes.readDoubleLE(14) };
  });
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
    return { timestamp: frame.timestamp, handle: bytes.readDoubleLE(6) };
  }).filter(frame => frame.handle === playerHandle);
  const positionFrames = baseFrames.filter(frame => frame.cmdid === 13).map(frame => {
    const bytes = Buffer.from(frame.hex, 'hex');
    return { index: frame.index, timestamp: frame.timestamp, handle: bytes.readDoubleLE(6), x: bytes.readInt32LE(16), y: bytes.readInt32LE(20) };
  }).filter(frame => frame.handle === playerHandle);
  const deathHp = playerHpFrames.find(frame => frame.hp === 0);
  const deathHpIndex = baseFrames.findIndex(frame => frame.timestamp === deathHp?.timestamp && frame.cmdid === 9);
  const deathDamage = baseFrames[deathHpIndex + 1];
  const deathRemove = baseFrames[deathHpIndex + 2];
  const revive = playerEntityFrames.find(entity => entity.timestamp > deathHp?.timestamp);
  const deadWindow = baseFrames.filter(frame => frame.timestamp > deathRemove?.timestamp && frame.timestamp < revive?.timestamp);
  const deadWindowDamage = deadWindow.filter(frame => frame.cmdid === 32);
  const deadWindowHp = deadWindow.filter(frame => frame.cmdid === 9);
  const deadMovePositionFrames = positionFrames.filter(frame => frame.index >= deadMoveFrameIndex && frame.timestamp < revive?.timestamp);
  const resumedPlayerDamage = playerDamageFrames.filter(frame => frame.timestamp > revive?.timestamp);
  const resumedMonsterDamage = monsterDamageFrames.filter(frame => frame.timestamp > revive?.timestamp);
  const gameworldLog = fs.readFileSync('../.tmp_m5f/gameworld.log', 'utf8');
  const validation = {
    cleanFixture: initialState.actor?.exp === 0 && initialState.actor?.gold === 0,
    singleLoginAttempt: wsConnections.filter(item => item.url.includes(':19002')).length === 1 && loginFrames.length === 1,
    sceneEntered: initialState.scene && initialState.map?.id === 2,
    initialFullHp: initialPlayer?.hp === 100 && initialPlayer?.max === 100 && initialPlayer?.parent,
    monsterCreated: monsterEntityFrames.length === 1,
    liveMovementAccepted: positionFrames.some(frame => frame.x === 384 && frame.y === 384),
    lethalHpZero: playerHpFrames.some(frame => frame.hp === 0),
    deathPacketOrder: deathDamage?.cmdid === 32 && monsterDamageFrames.some(frame => frame.timestamp === deathDamage.timestamp && frame.damage === 100) && deathRemove?.cmdid === 10,
    clientDeathObserved: deathClientProof.playerRemoved.length === 1 && deathClientProof.dieSounds.includes(playerHandle),
    deadPlayerHidden: !deathState.entities.some(entity => entity.handle === playerHandle),
    deadMovementRejected: deadMovePositionFrames.length === 0 && gameworldLog.includes('玩家已死亡，不能移动: 65537'),
    noDamageWhileDead: deadWindowDamage.length === 0 && deadWindowHp.length === 0,
    revivalDelayDefault: Boolean(revive && deathHp && revive.timestamp - deathHp.timestamp >= 2.9 && revive.timestamp - deathHp.timestamp <= 4.0),
    revivalSameEntityPosition: revive?.handle === playerHandle && revive.x === preDeathState.entities.find(entity => entity.handle === playerHandle).x && revive.y === preDeathState.entities.find(entity => entity.handle === playerHandle).y && revive.hp === 100 && revive.max === 100,
    revivedClientEntity: revivedState.entities.some(entity => entity.handle === playerHandle && entity.hp === 100 && entity.max === 100 && entity.parent),
    combatResumedAfterRevival: resumedPlayerDamage.length > 0 && resumedMonsterDamage.length > 0,
    rewardsUnchanged: finalState.actor?.exp === 0 && finalState.actor?.gold === 0,
    noClientErrors: clientProof.errors.length === 0
  };
  if (Object.values(validation).some(value => !value)) {
    throw new Error('death validation failed: ' + JSON.stringify({ validation, wsConnections, loginFrames, initialState, preDeathState, deathState, frozenState, revivedState, finalState, playerEntityFrames, playerHpFrames, playerDamageFrames, monsterDamageFrames, removeFrames, positionFrames, deathHp, deathDamage, deathRemove, revive, deadWindowDamage, deadWindowHp, deadMovePositionFrames, resumedPlayerDamage, resumedMonsterDamage, clientProof }));
  }
  const proof = {
    date: '2026-11-25',
    url,
    gateway: '127.0.0.1:19002',
    gameworld: '127.0.0.1:16001',
    database: 'actor_s1_m5e_20261125_f1',
    account: loginAccount,
    actorID: playerHandle,
    revivalDelayMs: (revive.timestamp - deathHp.timestamp) * 1000,
    initialState,
    preDeathState,
    deathState,
    frozenState,
    revivedState,
    finalState,
    deathClientProof,
    clientProof,
    validation,
    wsConnections,
    loginFrames,
    liveMoveHex,
    deadMoveHex,
    playerEntityFrames,
    monsterEntityFrames,
    playerHpFrames,
    playerDamageFrames,
    monsterDamageFrames,
    removeFrames,
    positionFrames,
    deathHp,
    deathDamage,
    deathRemove,
    revive,
    deadWindowDamage,
    deadWindowHp,
    deadMovePositionFrames,
    resumedPlayerDamage,
    resumedMonsterDamage,
    playerDamageAtRevival,
    wsFrameCount: parsedFrames.length
  };
  fs.writeFileSync(path.join(outDir, 'death_browser_proof.json'), JSON.stringify(proof, null, 2));
  console.log(JSON.stringify({
    validation,
    revivalDelayMs: proof.revivalDelayMs,
    playerHpFrames,
    removeFrames,
    playerEntityFrames,
    deadMovePositionFrames,
    resumedPlayerDamageCount: resumedPlayerDamage.length,
    resumedMonsterDamageCount: resumedMonsterDamage.length,
    finalActor: finalState.actor,
    wsFrameCount: parsedFrames.length
  }, null, 2));
} finally {
  await fetch(`http://127.0.0.1:9223/json/close/${target.id}`).catch(() => {});
  ws.close();
}
