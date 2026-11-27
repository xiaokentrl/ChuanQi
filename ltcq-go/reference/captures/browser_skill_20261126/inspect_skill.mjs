import fs from 'node:fs';

const outDir = 'reference/captures/browser_skill_20261126';
const mode = 'inspect';
const actorID = 65537;
const itemID = 200013;
const loginAccount = 'haohaiyao';
const proofDate = new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Shanghai' });
const url = `http://localhost/?uid=aabb&trace=${Date.now()}&bagVerify=${mode}`;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const stage = label => console.log(`[${new Date().toISOString()}] ${label}`);

const target = await (await fetch('http://127.0.0.1:9223/json/new?about:blank', { method: 'PUT' })).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
let nextID = 0;
const pending = new Map();
const wsFrames = [];
const wsConnections = [];
const runtimeEvents = [];

ws.onmessage = event => {
  const message = JSON.parse(event.data);
  if (message.id && pending.has(message.id)) {
    const { resolve, reject } = pending.get(message.id);
    pending.delete(message.id);
    message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result);
    return;
  }
  if (message.method === 'Network.webSocketCreated') {
    wsConnections.push(message.params.request?.url ?? message.params.url);
  } else if (message.method === 'Network.webSocketFrameReceived') {
    wsFrames.push({ direction: 'received', timestamp: message.params.timestamp, payload: message.params.response.payloadData });
  } else if (message.method === 'Network.webSocketFrameSent') {
    wsFrames.push({ direction: 'sent', timestamp: message.params.timestamp, payload: message.params.response.payloadData });
  } else if (message.method === 'Runtime.exceptionThrown') {
    runtimeEvents.push(message.params.exceptionDetails);
  }
};

function send(method, params = {}) {
  return new Promise((resolve, reject) => {
    const id = ++nextID;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error(`CDP 超时: ${method}`)); }, 15000);
    pending.set(id, {
      resolve: value => { clearTimeout(timer); resolve(value); },
      reject: error => { clearTimeout(timer); reject(error); }
    });
    ws.send(JSON.stringify({ id, method, params }));
  });
}

async function evaluate(expression) {
  const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
  if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || JSON.stringify(result.exceptionDetails));
  return result.result.value;
}

const errorInit = `(() => {
  window.__bagProofErrors = [];
  window.addEventListener('error', event => __bagProofErrors.push(String(event.message || event.error)));
  window.addEventListener('unhandledrejection', event => __bagProofErrors.push(String(event.reason)));
})();`;

const stateExpression = `(() => {
  const gameWindow = document.querySelector('iframe')?.contentWindow || window;
  const bag = gameWindow.UserBag?._instance || null;
  const items = bag?.bagModel?.[0] || [];
  const AttributeType = gameWindow.AttributeType;
  return {
    url: location.href,
    loginAccount: gameWindow.Main?.init?.editableText?.text ?? null,
    loginLoading: gameWindow.Main?.init?.loadingGroup?.visible ?? null,
    scene: gameWindow.GameLogic?.ins?.().gamescene ? gameWindow.egret.getQualifiedClassName(gameWindow.GameLogic.ins().gamescene) : false,
    map: gameWindow.GameMap ? {id: gameWindow.GameMap.mapID, fuben: gameWindow.GameMap.fubenID} : null,
    actor: gameWindow.Actor?.ins ? {level: gameWindow.Actor.level, exp: gameWindow.Actor.exp, gold: gameWindow.Actor.gold, hp: gameWindow.Actor.hp, maxHp: gameWindow.Actor.maxHp, x: gameWindow.Actor.x, y: gameWindow.Actor.y} : null,
    entities: gameWindow.EntityManager ? Object.values(gameWindow.EntityManager.ins().getAllEntity()).map(entity => ({handle: entity.handle ?? entity.infoModel?.handle, x: entity.x, y: entity.y, hp: entity.infoModel?.getAtt(AttributeType.atHp)})) : [],
    socketStatus: gameWindow.GameSocket?.ins?.()._socketStatus ?? null,
    bag: bag ? {bagNum: bag.bagNum, items: items.map(item => ({handle: item.handle, configID: item.configID, count: item.count}))} : null,
    errors: gameWindow.__bagProofErrors || window.__bagProofErrors || []
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
  const handle = bytes.readDoubleLE(offset);
  const handleBuffer = Buffer.alloc(8);
  handleBuffer.writeDoubleLE(handle);
  return {
    handle,
    handleBits: handleBuffer.readBigUInt64LE().toString(16),
    configID: bytes.readInt32LE(offset + 8),
    count: bytes.readInt32LE(offset + 12)
  };
}

function decodeBagFrame(frame) {
  if (frame.direction !== 'received' || frame.sysid !== 3) return null;
  const bytes = Buffer.from(frame.hex, 'hex');
  const common = {index: frame.index, timestamp: frame.timestamp, cmdid: frame.cmdid, frameBytes: bytes.length};
  if (frame.cmdid === 1) {
    if (bytes.length < 12) return null;
    const resetFlag = bytes.readUInt8(6);
    const bagType = bytes.readUInt8(7);
    const itemCount = bytes.readInt32LE(8);
    if (itemCount < 0 || bytes.length !== 12 + itemCount * 144) return null;
    const items = Array.from({length: itemCount}, (_, index) => parseItem(bytes, 12 + index * 144));
    return {...common, resetFlag, bagType, itemCount, items};
  }
  if (frame.cmdid === 4 && bytes.length === 152) {
    return {...common, bagType: bytes.readUInt8(6), item: parseItem(bytes, 7), tipFlag: bytes.readUInt8(151)};
  }
  if (frame.cmdid === 5 && bytes.length === 20) {
    return {...common, bagType: bytes.readUInt8(6), handle: bytes.readDoubleLE(7), newCount: bytes.readInt32LE(15), tipFlag: bytes.readUInt8(19)};
  }
  if (frame.cmdid === 3 && bytes.length === 15) {
    return {...common, bagType: bytes.readUInt8(6), handle: bytes.readDoubleLE(7)};
  }
  if (frame.cmdid === 6 && bytes.length === 7) {
    return {...common, result: bytes.readUInt8(6)};
  }
  return common;
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
    view.setFloat64(10, ${actorID}, true);
    view.setUint32(18, 1, true);
    view.setUint32(22, ${x}, true);
    view.setUint32(26, ${y}, true);
    raw.send(bytes.buffer);
    return Array.from(bytes).map(value => value.toString(16).padStart(2, '0')).join('');
  })()`);
}

await send('Page.enable');
await send('Runtime.enable');
await send('Network.enable');
await send('Emulation.setDeviceMetricsOverride', { width: 580, height: 930, deviceScaleFactor: 1, mobile: false });
await send('Page.addScriptToEvaluateOnNewDocument', { source: errorInit });

let noticeClosed = false;
let started = false;
let state = {};
let passed = false;
let drainCall = null;
let useCall = null;

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
  if (ui.notice) {
    await sleep(300);
    noticeClosed = !(await evaluate(`!!document.querySelector('iframe')?.contentWindow?.Main?.init?.createGongGaoGroup?.visible`));
  } else {
    started = true;
  }
}

async function refreshState() { state = await evaluate(stateExpression); return state; }
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
    if (!noticeClosed || !started) await clickStart();
    if (state.scene && state.map?.id === 2) return state;
  }
  throw new Error('进入场景超时: ' + JSON.stringify(state));
}
async function capture(name) {
  const shot = await send('Page.captureScreenshot', { format: 'png' });
  fs.writeFileSync(pathJoin(name), Buffer.from(shot.data, 'base64'));
}
function pathJoin(name) { return `${outDir}/${name}`; }
async function waitForBag(check, label, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    await sleep(100);
    const frames = parseFrames().map(decodeBagFrame).filter(Boolean);
    if (check(frames)) return frames;
  }
  throw new Error(label + ' 超时');
}
const itemCount = current => (current.bag?.items || []).filter(item => item.configID === itemID).reduce((sum, item) => sum + item.count, 0);

try {
  stage(`${mode}: 导航`);
  await send('Page.navigate', { url });
  await send('Page.bringToFront');
  stage(`${mode}: 选择账号`);
  await selectLoginAccount();
  stage(`${mode}: 进入场景`);
  await waitForScene();
  stage(`${mode}: 读取初始状态`);
  const initialState = await refreshState();
  stage(`${mode}: 等待背包初始化`);
  const initialBagFrames = await waitForBag(frames => frames.filter(frame => frame.cmdid === 1).length === 3, '等待三类背包初始化');
  if (false) await waitForState(current => itemCount(current) === 1, '等待客户端背包状态');
  stage(`${mode}: 初始截图`);
  await capture(`bag_${mode}_initial.png`);

  stage(`${mode}: 检查技能客户端对象`);
  const skillInspection = await evaluate(`(() => {
    const w = document.querySelector('iframe')?.contentWindow || window;
    const names = Object.getOwnPropertyNames(w).filter(name => /skill/i.test(name)).sort();
    const objects = names.map(name => {
      let value;
      try { value = w[name]; } catch { return {name, error: true}; }
      const info = {name, type: typeof value};
      if (value && (typeof value === 'object' || typeof value === 'function')) {
        try { info.keys = Object.keys(value).slice(0, 80); } catch {}
        try { info.methods = Object.getOwnPropertyNames(value).filter(key => typeof value[key] === 'function').slice(0, 120); } catch {}
        try { if (value._instance) info.instanceKeys = Object.keys(value._instance); } catch {}
        try { if (value.ins) { const inst = value.ins(); info.instanceKeys = Object.keys(inst); info.instanceMethods = Object.getOwnPropertyNames(Object.getPrototypeOf(inst)).filter(key => typeof inst[key] === 'function'); } } catch (error) { info.insError = String(error); }
      }
      return info;
    });
    const actor = w.Actor;
    const userSkill = w.UserSkill?.ins?.();
    const methodNames = ['sendUseSkill','doSkillResult','sendGrewUpSkill','postSkillUpgrade','sendGrewUpAllSkill','postSkillUpgradeAll','canGrewupSkill','canGrewupAllSkills','getSingleSkillGrewUpCost'];
    const methodSources = Object.fromEntries(methodNames.map(name => [name, userSkill ? String(userSkill[name]) : null]));
    let actorProto = actor;
    const actorProperties = [];
    while (actorProto && actorProperties.length < 200) {
      actorProperties.push(...Object.getOwnPropertyNames(actorProto));
      actorProto = Object.getPrototypeOf(actorProto);
    }
    const subRoles = w.SubRoles?.ins?.();
    const subRole = subRoles?.getSubRoleByIndex?.(0) ?? null;
    return {names, objects, actorKeys: actor ? Object.keys(actor).filter(key => /skill/i.test(key)) : [], actorProperties: [...new Set(actorProperties)].filter(key => /skill/i.test(key)), subRolesKeys: subRoles ? Object.keys(subRoles) : [], subRoleKeys: subRole ? Object.keys(subRole) : [], subRoleData: subRole ? JSON.parse(JSON.stringify(subRole)) : null, userSkillData: userSkill ? JSON.parse(JSON.stringify(userSkill)) : null, skillByJob: w.SkillData?.getSkillByJob?.(1) ?? null, methodSources};
  })()`);
  fs.writeFileSync(pathJoin('skill_client_inspection.json'), JSON.stringify(skillInspection, null, 2));
  console.log(JSON.stringify(skillInspection, null, 2));
  throw new Error('INSPECT_DONE');

  let drainedState = null;
  let stackedState = null;
  let usedState = null;
  let moveHex = null;
  let mutationFrameStart = parseFrames().length;
  if (mode === 'mutate') {
    if (itemCount(initialState) > 0) {
      stage(`${mode}: 清理前次验收遗留物品`);
      drainCall = await evaluate(`(() => {
        const gameWindow = document.querySelector('iframe')?.contentWindow || window;
        const bag = gameWindow.UserBag?.ins?.();
        return {result: bag?.sendUseItem(${itemID}, ${itemCount(initialState)}), socketStatus: gameWindow.GameSocket?.ins?.()._socketStatus};
      })()`);
      drainedState = await waitForState(current => itemCount(current) === 0, '等待遗留物品扣减');
      mutationFrameStart = parseFrames().length;
    }
    stage(`${mode}: 发送移动`);
    moveHex = await sendMovement(320, 384);
    stage(`${mode}: 等待玩家移动`);
    await waitForState(current => current.entities.some(entity => entity.handle === actorID && entity.x === 320 && entity.y === 384), '等待玩家进入攻击距离');
    stage(`${mode}: 等待首次物品发放`);
    await waitForBag(frames => frames.some(frame => frame.index >= mutationFrameStart && frame.cmdid === 4 && frame.item?.configID === itemID && frame.item?.count === 1), '等待首次物品发放');
    stage(`${mode}: 等待物品叠加`);
    await waitForBag(frames => frames.some(frame => frame.index >= mutationFrameStart && frame.cmdid === 5 && frame.newCount === 2), '等待物品叠加');
    stackedState = await waitForState(current => itemCount(current) === 2, '等待客户端物品叠加');
    stage(`${mode}: 叠加截图`);
    await capture('bag_mutate_stacked.png');
    stage(`${mode}: 发送使用物品命令`);
    useCall = await evaluate(`(() => {
      const gameWindow = document.querySelector('iframe')?.contentWindow || window;
      const bag = gameWindow.UserBag?.ins?.();
      return {result: bag?.sendUseItem(${itemID}, 1), socketStatus: gameWindow.GameSocket?.ins?.()._socketStatus};
    })()`);
    stage(`${mode}: 等待使用成功`);
    await waitForBag(frames => frames.some(frame => frame.index >= mutationFrameStart && frame.cmdid === 6 && frame.result === 0), '等待使用物品成功回包');
    usedState = await waitForState(current => itemCount(current) === 1, '等待客户端扣减物品');
    stage(`${mode}: 使用后截图`);
    await capture('bag_mutate_used.png');
  }

  stage(`${mode}: 汇总验收证据`);
  const finalState = await refreshState();
  const frames = parseFrames();
  const bagFrames = frames.map(decodeBagFrame).filter(Boolean);
  const bagInit = bagFrames.filter(frame => frame.cmdid === 1);
  const otherBag = bagInit.find(frame => frame.bagType === 0);
  const loginFrames = frames.filter(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
  const useCommands = frames.filter(frame => frame.index >= mutationFrameStart && frame.direction === 'sent' && frame.sysid === 3 && frame.cmdid === 6);
  const clientItem = current => (current.bag?.items || []).find(item => item.configID === itemID);
  const validation = {
    singleLoginAttempt: wsConnections.filter(item => item.includes(':19002')).length === 1 && loginFrames.length === 1,
    sceneEntered: initialState.scene && initialState.map?.id === 2,
    bagTypesInitialized: [0, 1, 2].every(type => bagInit.some(frame => frame.bagType === type)),
    bagInitLayoutsValid: initialBagFrames.length === 3 && initialBagFrames.every(frame => frame.frameBytes === 12 + frame.itemCount * 144),
    initialItemLayoutValid: initialBagFrames.filter(frame => frame.bagType === 0).flatMap(frame => frame.items).every(item => Number.isFinite(item.handle) && item.handle !== 0 && item.configID === itemID && item.count > 0),
    persistedOrMutatedCount: itemCount(finalState) === 1,
    noClientErrors: finalState.errors.length === 0 && runtimeEvents.length === 0
  };
  if (mode === 'mutate') {
    const mutations = bagFrames.filter(frame => frame.index >= mutationFrameStart);
    const add = mutations.find(frame => frame.cmdid === 4 && frame.item?.configID === itemID);
    const update = mutations.find(frame => frame.cmdid === 5 && frame.newCount === 2);
    validation.firstDropPacket = !!add && add.item.count === 1;
    validation.stackPacket = !!update;
    validation.clientStacked = itemCount(stackedState) === 2;
    validation.stableItemHandle = !!add && !!update && !!clientItem(stackedState) && add.item.handle === update.handle && update.handle === clientItem(stackedState).handle;
    validation.existingUseCommand = useCommands.length === 1 && useCall?.result === false && (() => {
      const bytes = Buffer.from(useCommands[0].hex, 'hex');
      return bytes.readInt32LE(10) === itemID && bytes.readInt32LE(14) === 1;
    })();
    validation.useSucceeded = mutations.some(frame => frame.cmdid === 6 && frame.result === 0);
    validation.clientConsumed = itemCount(usedState) === 1;
    validation.survivedVerification = finalState.entities.some(entity => entity.handle === actorID && entity.hp > 0);
  } else {
    validation.persistedInitItem = otherBag?.itemCount === 1 && otherBag?.items[0]?.configID === itemID && otherBag?.items[0]?.count === 1;
    validation.noBagMutations = !bagFrames.some(frame => [3, 4, 5, 6].includes(frame.cmdid));
  }
  if (Object.values(validation).some(value => !value)) throw new Error('bag validation failed: ' + JSON.stringify({validation, initialState, drainedState, stackedState, usedState, finalState, bagFrames}));
  const proof = {
    date: proofDate, mode, url, gateway: '127.0.0.1:19002', gameworld: '127.0.0.1:16001',
    database: 'actor_s1_m5e_m5h_20261125', account: loginAccount, actorID, itemID,
    initialState, drainedState, stackedState, usedState, drainCall, useCall, finalState, validation, wsConnections, loginFrames, useCommands, bagFrames, moveHex, wsFrameCount: frames.length
  };
  fs.writeFileSync(pathJoin(`bag_${mode}_browser_proof.json`), JSON.stringify(proof, null, 2));
  passed = true;
  console.log(JSON.stringify({mode, validation, finalActor: finalState.actor, finalBag: finalState.bag, wsFrameCount: frames.length}, null, 2));
} catch (error) {
  fs.writeFileSync(pathJoin(`bag_${mode}_failure.json`), JSON.stringify({error: String(error), state, runtimeEvents, drainCall, useCall, frames: parseFrames()}, null, 2));
  throw error;
} finally {
  await fetch(`http://127.0.0.1:9223/json/close/${target.id}`).catch(() => {});
  ws.close();
}
