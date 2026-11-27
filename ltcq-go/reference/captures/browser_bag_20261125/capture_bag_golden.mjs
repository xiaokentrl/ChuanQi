import fs from 'node:fs';
import path from 'node:path';

const outDir = 'reference/captures/browser_bag_20261125';
const loginAccount = 'haohaiyao';
const url = `http://localhost/?uid=aabb&trace=${Date.now()}&bagGolden=1`;
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

const stateExpression = `(() => {
  const gameWindow = document.querySelector('iframe')?.contentWindow || window;
  const GameLogic = gameWindow.GameLogic;
  const GameMap = gameWindow.GameMap;
  const egret = gameWindow.egret;
  const bag = gameWindow.UserBag?.ins?.();
  const items = bag ? Object.fromEntries(Object.entries(bag.bagModel || {}).map(([type, list]) => [type, list.map(item => ({
    handle: item.handle,
    configID: item.configID,
    count: item.count,
    att: item.att?.map(att => [att.type, att.value]),
    extAtt: item.extAtt?.map(att => [att.type, att.value])
  }))])) : null;
  return {
    url: location.href,
    ready: document.readyState,
    loginAccount: gameWindow.Main?.init?.editableText?.text ?? null,
    loginLoading: gameWindow.Main?.init?.loadingGroup?.visible ?? null,
    scene: GameLogic && GameLogic.ins && GameLogic.ins().gamescene ? egret.getQualifiedClassName(GameLogic.ins().gamescene) : false,
    map: GameMap ? {id: GameMap.mapID, fuben: GameMap.fubenID} : null,
    bag: bag ? {bagNum: bag.bagNum, itemCount: bag.itemCount, items} : null
  };
})()`;

function parseFrames() {
  return wsFrames.map((frame, index) => {
    const bytes = Buffer.from(frame.payload, 'base64');
    const offset = frame.direction === 'sent' ? 8 : 4;
    return { index, direction: frame.direction, timestamp: frame.timestamp, hex: bytes.toString('hex'), sysid: bytes[offset], cmdid: bytes[offset + 1] };
  });
}

function parseItem(bytes, offset) {
  return {
    handle: bytes.readDoubleLE(offset),
    configID: bytes.readInt32LE(offset + 8),
    count: bytes.readInt32LE(offset + 12),
    att: Array.from({length: 8}, (_, index) => [bytes.readInt32LE(offset + 16 + index * 8), bytes.readInt32LE(offset + 20 + index * 8)]),
    extAtt: Array.from({length: 8}, (_, index) => [bytes.readInt32LE(offset + 80 + index * 8), bytes.readInt32LE(offset + 84 + index * 8)])
  };
}

function decodeBagFrame(frame) {
  if (frame.direction !== 'received' || frame.sysid !== 3) return null;
  const bytes = Buffer.from(frame.hex, 'hex');
  const common = {index: frame.index, timestamp: frame.timestamp, cmdid: frame.cmdid, frameBytes: bytes.length};
  if (frame.cmdid === 1) {
    const resetFlag = bytes.readUInt8(6);
    const bagType = bytes.readUInt8(7);
    const itemCount = bytes.readInt32LE(8);
    const items = Array.from({length: itemCount}, (_, index) => parseItem(bytes, 12 + index * 144));
    return {...common, resetFlag, bagType, itemCount, items};
  }
  if (frame.cmdid === 4) {
    return {...common, bagType: bytes.readUInt8(6), item: parseItem(bytes, 7), tipFlag: bytes.readUInt8(151)};
  }
  if (frame.cmdid === 5) {
    return {...common, bagType: bytes.readUInt8(6), handle: bytes.readDoubleLE(7), newCount: bytes.readInt32LE(15), tipFlag: bytes.readUInt8(19)};
  }
  if (frame.cmdid === 3) {
    return {...common, bagType: bytes.readUInt8(6), handle: bytes.readDoubleLE(7)};
  }
  return {...common, businessHex: bytes.subarray(6).toString('hex')};
}

let noticeClosed = false;
let started = false;
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
    if (!noticeClosed || !started) await clickStart();
    if (state.scene && state.map?.id === 2) return state;
  }
  throw new Error('进入场景超时: ' + JSON.stringify(state));
}
async function waitForFrames(check, label, timeout = 20000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) { await sleep(100); if (check(parseFrames())) return; }
  throw new Error(label + ' 超时');
}

try {
  await send('Page.enable');
  await send('Runtime.enable');
  await send('Network.enable');
  await send('Emulation.setDeviceMetricsOverride', { width: 580, height: 930, deviceScaleFactor: 1, mobile: false });
  await send('Page.navigate', { url });
  await send('Page.bringToFront');
  await selectLoginAccount();
  await waitForScene();
  await waitForFrames(frames => frames.some(frame => frame.direction === 'received' && frame.sysid === 3 && frame.cmdid === 1), '等待 C++ 背包初始化', 20000);
  await sleep(1500);
  await refreshState();

  const frames = parseFrames();
  const bagFrames = frames.filter(frame => frame.direction === 'received' && frame.sysid === 3).map(decodeBagFrame);
  const bagInit = bagFrames.filter(frame => frame.cmdid === 1);
  const loginFrames = frames.filter(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
  const validation = {
    cppGatewayConnection: wsConnections.filter(item => item.url.includes(':9002')).length === 1,
    singleLoginAttempt: loginFrames.length === 1,
    sceneEntered: state.scene && state.map?.id === 2,
    bagTypesInitialized: [0, 1, 2].every(type => bagInit.some(frame => frame.bagType === type)),
    bagInitLayoutsValid: bagInit.every(frame => frame.frameBytes === 12 + frame.itemCount * 144),
    itemLayoutsValid: bagInit.every(frame => frame.items.every(item => Number.isFinite(item.handle) && Number.isInteger(item.configID) && Number.isInteger(item.count))),
    clientBagModelPresent: state.bag && Array.isArray(state.bag.items?.[0])
  };
  if (Object.values(validation).some(value => !value)) throw new Error('bag golden validation failed: ' + JSON.stringify({validation, state, bagFrames, wsConnections, loginFrames}));

  const proof = {
    date: '2026-11-25', url, gateway: '127.0.0.1:9002', account: loginAccount,
    state, validation, wsConnections, loginFrames, bagFrames, wsFrameCount: frames.length,
    notes: [
      '本次只读登录，不发放、不使用、不分解、不移动物品。',
      '[03][01] 已由真实 C++ 网关抓包；[03][03]/[03][04]/[03][05] 是否出现取决于账号当时行为。',
      'ItemData 按 handle:f64 + configID:i32 + count:i32 + 8×(type:i32,value:i32) + 8×(type:i32,value:i32) 解析。'
    ]
  };
  fs.writeFileSync(path.join(outDir, 'bag_cpp_golden.json'), JSON.stringify(proof, null, 2));
  console.log(JSON.stringify({validation, bagFrameCommands: bagFrames.map(frame => frame.cmdid), bagInit: bagInit.map(frame => ({bagType: frame.bagType, itemCount: frame.itemCount, frameBytes: frame.frameBytes})), clientBag: state.bag, wsFrameCount: frames.length}, null, 2));
} catch (error) {
  fs.writeFileSync(path.join(outDir, 'bag_cpp_failure.json'), JSON.stringify({error: String(error), state, frames: parseFrames()}, null, 2));
  throw error;
} finally {
  await fetch(`http://127.0.0.1:9223/json/close/${target.id}`).catch(() => {});
  ws.close();
}
