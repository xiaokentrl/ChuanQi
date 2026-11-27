import fs from 'node:fs';
import path from 'node:path';

const outDir = 'reference/captures/browser_attack_20261125';
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

await send('Page.enable');
await send('Runtime.enable');
await send('Network.enable');
await send('Emulation.setDeviceMetricsOverride', { width: 580, height: 930, deviceScaleFactor: 1, mobile: false });

const init = `(() => {
  window.__combatProof = {sent: [], received: [], hp: [], damage: [], removed: [], monsters: [], errors: []};
  let logicPatched = false;
  let socketPatched = false;
  const hex = value => Array.from(new Uint8Array(value)).map(v => v.toString(16).padStart(2, '0')).join('');
  const patchLogic = () => {
    if (logicPatched || typeof GameLogic === 'undefined' || !GameLogic.ins) return;
    const logic = GameLogic.ins();
    const hp = logic.postHpChange;
    logic.postHpChange = function(packet) {
      const result = hp.apply(this, arguments);
      try {
        if (Array.isArray(result)) __combatProof.hp.push({handle: result[0] && result[0].handle, current: result[1], previous: result[2]});
      } catch (_) {}
      return result;
    };
    const damage = logic.postEntityHpChange;
    logic.postEntityHpChange = function(attacker, target, type, amount) {
      try {
        __combatProof.damage.push({attacker: attacker && attacker.handle, target: target && target.handle, type, amount});
      } catch (_) {}
      return damage.apply(this, arguments);
    };
    const removed = logic.postRemoveEntity;
    logic.postRemoveEntity = function(handle, entity) {
      try {
        __combatProof.removed.push({handle, type: entity && entity.infoModel && entity.infoModel.type});
      } catch (_) {}
      return removed.apply(this, arguments);
    };
    const created = logic.postCreateOtherEntity;
    logic.postCreateOtherEntity = function(packet) {
      const result = created.apply(this, arguments);
      try {
        if (result && result.infoModel && result.infoModel.type === 2) {
          __combatProof.monsters.push({handle: result.handle, hp: result.infoModel.getAtt(AttributeType.atHp), max: result.infoModel.getAtt(AttributeType.atMaxHp)});
        }
      } catch (_) {}
      return result;
    };
    logicPatched = true;
  };
  const patchSocket = () => {
    if (socketPatched || typeof GameSocket === 'undefined' || !GameSocket.ins) return;
    const wrapper = GameSocket.ins().socket_;
    const raw = wrapper && wrapper.socket && wrapper.socket.socket;
    if (!raw || typeof raw.send !== 'function') return;
    const send = raw.send.bind(raw);
    raw.send = function(data) {
      try { __combatProof.sent.push(hex(data)); } catch (_) {}
      return send(data);
    };
    raw.addEventListener('message', async event => {
      try {
        let data = event.data;
        if (data instanceof Blob) data = await data.arrayBuffer();
        __combatProof.received.push(hex(data));
      } catch (error) {
        __combatProof.errors.push(String(error));
      }
    });
    socketPatched = true;
  };
  setInterval(() => {
    try { patchLogic(); patchSocket(); } catch (error) { __combatProof.errors.push(String(error)); }
  }, 5);
})();`;

await send('Page.addScriptToEvaluateOnNewDocument', { source: init });
await send('Page.navigate', { url: 'http://localhost/?uid=aabb&trace=1795553070' });

const stateExpression = `(() => {
  const gameWindow = document.querySelector('iframe')?.contentWindow || window;
  const GameLogic = gameWindow.GameLogic;
  const GameMap = gameWindow.GameMap;
  const EntityManager = gameWindow.EntityManager;
  const AttributeType = gameWindow.AttributeType;
  const egret = gameWindow.egret;
  const proof = gameWindow.__combatProof || window.__combatProof;
  const out = {
    url: location.href,
    ready: document.readyState,
    role: typeof gameWindow.RoleMgr,
    game: typeof GameLogic,
    scene: false,
    map: null,
    entities: [],
    monsterOnStage: false,
    proof: proof ? {
      hp: proof.hp.length,
      damage: proof.damage.length,
      removed: proof.removed.length,
      monsters: proof.monsters,
      errors: proof.errors
    } : null
  };
  if (GameLogic && GameLogic.ins && GameLogic.ins().gamescene) out.scene = egret.getQualifiedClassName(GameLogic.ins().gamescene);
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
      if (object.infoModel && object.infoModel.handle === 1073741824) out.monsterOnStage = true;
      if (object.numChildren) for (let i = 0; i < object.numChildren; i++) walk(object.getChildAt(i));
    };
    walk(egret.lifecycle.stage);
  }
  return out;
})()`;

let noticeClosed = false;
let started = false;
let proofInjected = false;
let state = {};
const deadline = Date.now() + 80000;
while (Date.now() < deadline) {
  await sleep(500);
  state = await evaluate(stateExpression);
  if (!proofInjected && await evaluate(`!!document.querySelector('iframe')?.contentWindow`)) {
    await evaluate(`document.querySelector('iframe').contentWindow.eval(${JSON.stringify(init)})`);
    proofInjected = true;
  }
  if (!noticeClosed || !started) {
    const canvas = await evaluate(`(() => {
      const frame = document.querySelector('iframe');
      if (!frame) return null;
      const rect = frame.getBoundingClientRect();
      const canvas = frame.contentDocument && frame.contentDocument.querySelector('canvas');
      if (!canvas || rect.width < 100 || rect.height < 100) return null;
      const inner = canvas.getBoundingClientRect();
      return {x: rect.left + inner.left, y: rect.top + inner.top, width: inner.width, height: inner.height};
    })()`);
    if (canvas) {
      const xRatio = !noticeClosed ? 0.88 : 0.5;
      const yRatio = !noticeClosed ? 0.215 : 0.744;
      const x = canvas.x + canvas.width * xRatio;
      const y = canvas.y + canvas.height * yRatio;
      await send('Input.dispatchMouseEvent', {type: 'mouseMoved', x, y});
      await send('Input.dispatchMouseEvent', {type: 'mousePressed', x, y, button: 'left', clickCount: 1});
      await send('Input.dispatchMouseEvent', {type: 'mouseReleased', x, y, button: 'left', clickCount: 1});
      if (!noticeClosed) {
        noticeClosed = true;
        await sleep(800);
      } else {
        started = true;
      }
    }
  }
  if (state.proof && (state.proof.damage.length > 0 || state.entities.some(entity => entity.type === 2))) break;
}
if (!state.proof) throw new Error('client proof unavailable: ' + JSON.stringify(state));

await sleep(1200);
state = await evaluate(stateExpression);
const initialState = state;
const initial = await send('Page.captureScreenshot', { format: 'png' });
fs.writeFileSync(path.join(outDir, 'combat_monster_initial.png'), Buffer.from(initial.data, 'base64'));

while (Date.now() < deadline) {
  await sleep(300);
  state = await evaluate(stateExpression);
  if (state.proof.damage >= 6) break;
}
const midState = state;
const mid = await send('Page.captureScreenshot', { format: 'png' });
fs.writeFileSync(path.join(outDir, 'combat_damage_visible.png'), Buffer.from(mid.data, 'base64'));

while (Date.now() < deadline) {
  await sleep(300);
  state = await evaluate(stateExpression);
  if (state.proof && state.proof.removed > 0 && !state.entities.some(entity => entity.handle === 1073741824) && !state.monsterOnStage) break;
}
const finalShot = await send('Page.captureScreenshot', { format: 'png' });
fs.writeFileSync(path.join(outDir, 'combat_monster_removed.png'), Buffer.from(finalShot.data, 'base64'));

const fullProof = await evaluate(`document.querySelector('iframe')?.contentWindow.__combatProof || window.__combatProof`);
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
const combatFrames = parsedFrames.filter(frame => frame.direction === 'received' && (
  (frame.sysid === 0 && [4, 9, 10, 32].includes(frame.cmdid)) ||
  (frame.sysid === 255 && [1, 4, 5].includes(frame.cmdid))
));
const monsterEntityFrames = combatFrames.filter(frame => frame.sysid === 0 && frame.cmdid === 4).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {
    timestamp: frame.timestamp,
    type: bytes.readUInt16LE(6),
    handle: bytes.readDoubleLE(8),
    configID: bytes.readInt32LE(16),
    x: bytes.readInt32LE(28),
    y: bytes.readInt32LE(32)
  };
}).filter(entity => entity.type === 2);
const hpFrames = combatFrames.filter(frame => frame.sysid === 0 && frame.cmdid === 9).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {timestamp: frame.timestamp, handle: bytes.readDoubleLE(6), hp: bytes.readDoubleLE(14)};
});
const damageFrames = combatFrames.filter(frame => frame.sysid === 0 && frame.cmdid === 32).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {
    timestamp: frame.timestamp,
    attacker: bytes.readDoubleLE(6),
    target: bytes.readDoubleLE(14),
    damage: bytes.readDoubleLE(22),
    damageType: bytes.readUInt32LE(30)
  };
});
const removeFrames = combatFrames.filter(frame => frame.sysid === 0 && frame.cmdid === 10).map(frame => {
  const bytes = Buffer.from(frame.hex, 'hex');
  return {timestamp: frame.timestamp, handle: bytes.readDoubleLE(6)};
});
const monsterHandle = 1073741824;
const validation = {
  monsterCreated: monsterEntityFrames.some(entity => entity.handle === monsterHandle) || initialState.entities.some(entity => entity.handle === monsterHandle),
  clientHpDecreased: midState.entities.some(entity => entity.handle === monsterHandle && entity.hp < 2000),
  hpDecreased: hpFrames.some(frame => frame.handle === monsterHandle && frame.hp < 2000),
  damageFloating: fullProof.damage.length >= 3,
  hpReachedZero: hpFrames.some(frame => frame.handle === monsterHandle && frame.hp === 0),
  removalCallback: fullProof.removed.some(item => item.handle === monsterHandle),
  removalPacket: removeFrames.some(frame => frame.handle === monsterHandle),
  entityRemoved: !state.entities.some(entity => entity.handle === monsterHandle),
  stageRemoved: !state.monsterOnStage
};
if (Object.values(validation).some(value => !value)) {
  throw new Error('combat validation failed: ' + JSON.stringify({validation, initialState, midState, state, monsterEntityFrames, hpFrames, damageFrames, removeFrames}));
}
const proof = {
  date: '2026-11-25',
  url: 'http://localhost/?uid=aabb&trace=1795553070',
  gateway: '127.0.0.1:19002',
  gameworld: '127.0.0.1:16001',
  state,
  initialState,
  midState,
  clientProof: fullProof,
  validation,
  monsterEntityFrames,
  hpFrames,
  damageFrames,
  removeFrames,
  combatFrames,
  wsFrameCount: parsedFrames.length
};
fs.writeFileSync(path.join(outDir, 'combat_browser_proof.json'), JSON.stringify(proof, null, 2));
console.log(JSON.stringify({
  state,
  hp: fullProof.hp,
  damageCount: fullProof.damage.length,
  removed: fullProof.removed,
  monsters: monsterEntityFrames,
  validation,
  hpPacketCount: hpFrames.length,
  damagePacketCount: damageFrames.length,
  removalPacketCount: removeFrames.length,
  combatFrameCount: combatFrames.length,
  wsFrameCount: parsedFrames.length
}, null, 2));
ws.close();
