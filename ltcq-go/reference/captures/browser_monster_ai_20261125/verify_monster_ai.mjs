import fs from 'node:fs';
import path from 'node:path';

const outDir = 'reference/captures/browser_monster_ai_20261125';
const monsterHandle = 1073741824;
const playerHandle = 65537;
const loginAccount = 'haohaiyao';
const url = `http://localhost/?uid=aabb&trace=${Date.now()}&monsterAI=1`;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

const targets = await (await fetch('http://127.0.0.1:9223/json/list')).json();
let target = targets.find(item => item.type === 'page' && item.url === 'about:blank');
if (!target) target = await (await fetch('http://127.0.0.1:9223/json/new?about:blank', { method: 'PUT' })).json();

const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
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
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || JSON.stringify(result.exceptionDetails));
  return result.result.value;
}

const init = `(() => {
  if (window.__monsterAIProof) return;
  window.__monsterAIProof = {damage: [], errors: []};
  let patched = false;
  const patch = () => {
    if (patched || typeof GameLogic === 'undefined' || !GameLogic.ins) return;
    const logic = GameLogic.ins();
    const damage = logic.postEntityHpChange;
    logic.postEntityHpChange = function(attacker, target, type, amount) {
      try {
        const handle = entity => entity?.handle ?? entity?.infoModel?.handle;
        __monsterAIProof.damage.push({attacker: handle(attacker), target: handle(target), type, amount});
      } catch (_) {}
      return damage.apply(this, arguments);
    };
    patched = true;
  };
  setInterval(() => { try { patch(); } catch (error) { __monsterAIProof.errors.push(String(error)); } }, 5);
})();`;

const stateExpression = `(() => {
  const gameWindow = document.querySelector('iframe')?.contentWindow || window;
  const GameLogic = gameWindow.GameLogic;
  const GameMap = gameWindow.GameMap;
  const EntityManager = gameWindow.EntityManager;
  const AttributeType = gameWindow.AttributeType;
  const Actor = gameWindow.Actor;
  const egret = gameWindow.egret;
  const proof = gameWindow.__monsterAIProof || window.__monsterAIProof;
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
    proof: proof ? {damage: proof.damage, errors: proof.errors} : null
  };
  if (GameLogic && GameLogic.ins && GameLogic.ins().gamescene) out.scene = egret.getQualifiedClassName(GameLogic.ins().gamescene);
  if (Actor && Actor.ins) out.actor = {level: Actor.level, exp: Actor.exp, gold: Actor.gold, hp: Actor.hp, maxHp: Actor.maxHp, x: Actor.x, y: Actor.y};
  if (GameMap) out.map = {id: GameMap.mapID, fuben: GameMap.fubenID, mine: GameMap.sceneInMine()};
  if (EntityManager) {
    out.entities = Object.values(EntityManager.ins().getAllEntity()).map(entity => ({
      handle: entity.handle ?? entity.infoModel?.handle,
      type: entity.infoModel?.type,
      name: entity.infoModel?.name,
      configID: entity.infoModel?.configID,
      x: entity.x,
      y: entity.y,
      hp: entity.infoModel?.getAtt(AttributeType.atHp),
      max: entity.infoModel?.getAtt(AttributeType.atMaxHp),
      parent: !!entity.parent
    }));
  }
  if (egret?.lifecycle?.stage) {
    const walk = object => {
      if (!object) return;
      if (object.infoModel?.handle === ${monsterHandle}) out.monsterOnStage = true;
      if (object.numChildren) for (let i = 0; i < object.numChildren; i++) walk(object.getChildAt(i));
    };
    walk(egret.lifecycle.stage);
  }
  return out;
})()`;

function parseFrames() {
  return wsFrames.map((frame, index) => {
    const bytes = Buffer.from(frame.payload, 'base64');
    const offset = frame.direction === 'sent' ? 8 : 4;
    return { index, direction: frame.direction, timestamp: frame.timestamp, hex: bytes.toString('hex'), sysid: bytes[offset], cmdid: bytes[offset + 1] };
  });
}

function loginSkey() {
  const login = parseFrames().find(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
  if (!login) throw new Error('尚未捕获登录帧');
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
    bytes[8] = 0; bytes[9] = 31;
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
      const login = frame?.contentWindow?.Main?.init;
      const editable = login?.editableText;
      if (!frame || !editable?.visible || login.loadingGroup?.visible) return null;
      const rect = frame.getBoundingClientRect();
      if (login.createGongGaoGroup?.visible) return {notice: true, x: rect.left + rect.width * 0.88, y: rect.top + rect.height * 0.215};
      const point = login.group.localToGlobal(editable.x + editable.width / 2, editable.y + editable.height / 2);
      return {account: editable.text, x: rect.left + point.x, y: rect.top + point.y};
    })()`);
    if (input) {
      if (input.notice) { await clickAt(input.x, input.y); await sleep(300); continue; }
      if (input.account === loginAccount) return;
      await clickAt(input.x, input.y);
      const focused = await evaluate(`(() => {
        const frame = document.querySelector('iframe');
        const editable = frame?.contentWindow?.Main?.init?.editableText;
        editable?.inputUtils?.stageText?.$show();
        const input = frame?.contentDocument?.querySelector('#egretInput');
        input?.focus(); input?.select();
        return frame?.contentDocument?.activeElement?.id === 'egretInput';
      })()`);
      if (!focused) throw new Error('登录账号输入框聚焦失败');
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
    const login = frame?.contentWindow?.Main?.init;
    const socket = frame?.contentWindow?.GameSocket?.ins();
    if (!login) return null;
    return {ready: !!login.clickLabel?.touchEnabled && !!login.selectSerInfo, notice: !!login.createGongGaoGroup?.visible, status: socket?._socketStatus ?? 0, loading: !!login.loadingGroup?.visible};
  })()`);
  if (!ui?.ready) return;
  const canvas = await evaluate(`(() => {
    const frame = document.querySelector('iframe');
    const rect = frame?.getBoundingClientRect();
    const inner = frame?.contentDocument?.querySelector('canvas')?.getBoundingClientRect();
    return rect && inner ? {x: rect.left + inner.left, y: rect.top + inner.top, width: inner.width, height: inner.height} : null;
  })()`);
  if (!canvas) return;
  if (!ui.notice && (ui.status !== 0 || ui.loading)) { started = true; return; }
  await clickAt(canvas.x + canvas.width * (ui.notice ? 0.88 : 0.5), canvas.y + canvas.height * (ui.notice ? 0.215 : 0.744));
  if (ui.notice) { await sleep(300); noticeClosed = !(await evaluate(`!!document.querySelector('iframe')?.contentWindow?.Main?.init?.createGongGaoGroup?.visible`)); }
  else started = true;
}

async function refreshState() { state = await evaluate(stateExpression); return state; }
async function waitForState(check, label, timeout = 50000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) { await sleep(100); await refreshState(); if (check(state)) return state; }
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
async function waitForFrames(check, label, timeout = 20000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) { await sleep(100); if (check(parseFrames())) return; }
  throw new Error(label + ' 超时');
}

try {
  await send('Page.navigate', { url });
  await send('Page.bringToFront');
  await selectLoginAccount();
  await waitForScene();
  const initialState = state;
  await capture('monster_ai_initial.png');

  await waitForState(current => current.entities.some(entity => entity.handle === monsterHandle && entity.x === 384 && entity.y === 384), '等待怪物主动接近');
  const chasedState = state;
  await capture('monster_ai_chased.png');
  await waitForState(current => current.entities.some(entity => entity.handle === playerHandle && entity.hp < 100), '等待怪物攻击玩家');
  const attackedState = state;

  const escapeMoveHex = await sendMovement(1200, 384);
  const escapeFrameIndex = wsFrames.length;
  await waitForState(current => current.entities.some(entity => entity.handle === playerHandle && entity.x === 1200 && entity.y === 384), '等待玩家脱离追击');
  const escapedState = state;
  await waitForFrames(frames => frames.some(frame => frame.direction === 'received' && frame.sysid === 0 && frame.cmdid === 13 && frame.index >= escapeFrameIndex && Buffer.from(frame.hex, 'hex').readDoubleLE(6) === monsterHandle && Buffer.from(frame.hex, 'hex').readInt32LE(16) > 960), '等待怪物越过归位范围', 15000);
  await capture('monster_ai_leash.png');
  await waitForFrames(frames => {
    const moves = frames.filter(frame => frame.direction === 'received' && frame.sysid === 0 && frame.cmdid === 13 && frame.index >= escapeFrameIndex && Buffer.from(frame.hex, 'hex').readDoubleLE(6) === monsterHandle);
    const maxIndex = moves.reduce((best, frame, index) => Buffer.from(frame.hex, 'hex').readInt32LE(16) > Buffer.from(moves[best].hex, 'hex').readInt32LE(16) ? index : best, 0);
    return moves.length > 0 && Buffer.from(moves[maxIndex].hex, 'hex').readInt32LE(16) > 960 && moves.slice(maxIndex + 1).some(frame => Buffer.from(frame.hex, 'hex').readInt32LE(16) <= 640);
  }, '等待怪物开始归位', 15000);
  await sleep(3500);
  const returnFrameIndex = wsFrames.length;
  const returnMoveHex = await sendMovement(1000, 384);
  await waitForFrames(frames => frames.some(frame => {
    if (frame.direction !== 'received' || frame.sysid !== 0 || frame.cmdid !== 4 || frame.index < returnFrameIndex) return false;
    const bytes = Buffer.from(frame.hex, 'hex');
    return bytes.readUInt16LE(6) === 2 && bytes.readDoubleLE(8) === monsterHandle && bytes.readInt32LE(28) === 448 && bytes.readInt32LE(32) === 384;
  }), '等待怪物归位后重新进入视野', 10000);
  const returnedState = await refreshState();
  await capture('monster_ai_returned.png');
  const stableFrameIndex = wsFrames.length;
  await sleep(1600);
  const finalState = await refreshState();
  const clientProof = await evaluate(`document.querySelector('iframe')?.contentWindow.__monsterAIProof || window.__monsterAIProof`);

  const frames = parseFrames();
  const base = frames.filter(frame => frame.direction === 'received' && frame.sysid === 0);
  const entities = base.filter(frame => frame.cmdid === 4).map(frame => {
    const bytes = Buffer.from(frame.hex, 'hex');
    return {index: frame.index, timestamp: frame.timestamp, type: bytes.readUInt16LE(6), handle: bytes.readDoubleLE(8), x: bytes.readInt32LE(28), y: bytes.readInt32LE(32), hp: bytes.readDoubleLE(38), max: bytes.readDoubleLE(54)};
  });
  const monsterEntities = entities.filter(entity => entity.type === 2 && entity.handle === monsterHandle);
  const playerEntities = entities.filter(entity => entity.type === 1 && entity.handle === playerHandle);
  const moves = base.filter(frame => frame.cmdid === 13).map(frame => {
    const bytes = Buffer.from(frame.hex, 'hex');
    return {index: frame.index, timestamp: frame.timestamp, handle: bytes.readDoubleLE(6), mode: bytes.readUInt16LE(14), x: bytes.readInt32LE(16), y: bytes.readInt32LE(20), duration: bytes.readUInt32LE(24)};
  });
  const monsterMoves = moves.filter(frame => frame.handle === monsterHandle);
  const playerMoves = moves.filter(frame => frame.handle === playerHandle);
  const escapeMoves = monsterMoves.filter(frame => frame.index >= escapeFrameIndex);
  const damages = base.filter(frame => frame.cmdid === 32).map(frame => {
    const bytes = Buffer.from(frame.hex, 'hex');
    return {index: frame.index, timestamp: frame.timestamp, attacker: bytes.readDoubleLE(6), target: bytes.readDoubleLE(14), damage: bytes.readDoubleLE(22)};
  });
  const monsterDamage = damages.filter(frame => frame.attacker === monsterHandle && frame.target === playerHandle);
  const firstMove = monsterMoves[0];
  const firstDamage = monsterDamage[0];
  const maxMove = escapeMoves.reduce((best, frame) => !best || frame.x > best.x ? frame : best, null);
  const returnedEntity = monsterEntities.find(entity => entity.index > (maxMove?.index ?? 0) && entity.index >= returnFrameIndex && entity.x === 448 && entity.y === 384);
  const loginFrames = frames.filter(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
  const validation = {
    cleanFixture: initialState.actor?.exp === 0 && initialState.actor?.gold === 0,
    singleLoginAttempt: wsConnections.filter(item => item.url.includes(':19002')).length === 1 && loginFrames.length === 1,
    sceneEntered: initialState.scene && initialState.map?.id === 2,
    initialMonsterSpawn: monsterEntities[0]?.x === 448 && monsterEntities[0]?.y === 384,
    playerStartedOutsideAttackRange: playerEntities[0]?.x === 256 && Math.abs(playerEntities[0].x - monsterEntities[0].x) > 128,
    activeChasePacket: firstMove?.x === 384 && firstMove.y === 384 && firstMove.mode === 1 && firstMove.duration === 16,
    chaseBeforeAttack: Boolean(firstMove && firstDamage && firstMove.index < firstDamage.index),
    clientMonsterMoved: chasedState.entities.some(entity => entity.handle === monsterHandle && entity.x === 384 && entity.parent) && chasedState.monsterOnStage,
    monsterAttackedPlayer: attackedState.entities.some(entity => entity.handle === playerHandle && entity.hp < 100) && firstDamage?.damage === 10,
    liveDisengage: escapedState.entities.some(entity => entity.handle === playerHandle && entity.x === 1200 && entity.y === 384),
    leashTriggered: Boolean(maxMove && maxMove.x > 960),
    returnedHome: Boolean(returnedEntity) && returnedState.entities.some(entity => entity.handle === monsterHandle && entity.x === 448 && entity.y === 384),
    aggroCleared: !base.some(frame => frame.index >= stableFrameIndex && frame.cmdid === 13 && Buffer.from(frame.hex, 'hex').readDoubleLE(6) === monsterHandle) && !damages.some(frame => frame.index >= escapeFrameIndex && frame.attacker === monsterHandle),
    rewardsUnchanged: finalState.actor?.exp === 0 && finalState.actor?.gold === 0,
    noClientErrors: clientProof.errors.length === 0
  };
  if (Object.values(validation).some(value => !value)) throw new Error('monster AI validation failed: ' + JSON.stringify({validation, initialState, chasedState, attackedState, escapedState, returnedState, finalState, monsterEntities, playerEntities, monsterMoves, monsterDamage, clientProof}));
  const proof = {
    date: '2026-11-25', url, gateway: '127.0.0.1:19002', gameworld: '127.0.0.1:16001',
    database: 'actor_s1_m5e_20261125_g1', account: loginAccount, actorID: playerHandle,
    initialState, chasedState, attackedState, escapedState, returnedState, finalState, clientProof, validation,
    wsConnections, loginFrames, escapeMoveHex, returnMoveHex, monsterEntities, playerEntities, monsterMoves, playerMoves, monsterDamage,
    firstMove, firstDamage, maxMove, returnedEntity, wsFrameCount: frames.length
  };
  fs.writeFileSync(path.join(outDir, 'monster_ai_browser_proof.json'), JSON.stringify(proof, null, 2));
  console.log(JSON.stringify({validation, firstMove, firstDamage, maxMove, returnedEntity, monsterMoveCount: monsterMoves.length, monsterDamageCount: monsterDamage.length, finalActor: finalState.actor, wsFrameCount: frames.length}, null, 2));
} catch (error) {
  fs.writeFileSync(path.join(outDir, 'monster_ai_browser_failure.json'), JSON.stringify({error: String(error), state, frames: parseFrames()}, null, 2));
  throw error;
} finally {
  await fetch(`http://127.0.0.1:9223/json/close/${target.id}`).catch(() => {});
  ws.close();
}
