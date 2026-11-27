import fs from 'node:fs';

const outDir = 'reference/captures/browser_skill_20261126';
const mode = process.argv.includes('--restart') ? 'restart' : process.argv.includes('--relogin') ? 'relogin' : 'mutate';
const actorID = 65537;
const monsterID = 1 << 30;
const loginAccount = 'haohaiyao';
const database = 'actor_s1_m5e_skill_20261126';
const proofDate = new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Shanghai' });
const url = `http://localhost/?uid=aabb&trace=${Date.now()}&skillVerify=${mode}`;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const stage = label => console.log(`[${new Date().toISOString()}] ${label}`);
const pathJoin = name => `${outDir}/${name}`;

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
  window.__skillProofErrors = [];
  window.addEventListener('error', event => __skillProofErrors.push(String(event.message || event.error)));
  window.addEventListener('unhandledrejection', event => __skillProofErrors.push(String(event.reason)));
})()`;

const stateExpression = `(() => {
  const w = document.querySelector('iframe')?.contentWindow || window;
  const role = w.SubRoles?.ins?.().getSubRoleByIndex?.(0) ?? null;
  const AttributeType = w.AttributeType;
  return {
    url: location.href,
    loginAccount: w.Main?.init?.editableText?.text ?? null,
    loginLoading: w.Main?.init?.loadingGroup?.visible ?? null,
    scene: w.GameLogic?.ins?.().gamescene ? w.egret.getQualifiedClassName(w.GameLogic.ins().gamescene) : false,
    map: w.GameMap ? {id: w.GameMap.mapID, fuben: w.GameMap.fubenID} : null,
    actor: w.Actor ? {level: w.Actor.level, gold: w.Actor.gold, hp: w.Actor.hp, maxHp: w.Actor.maxHp} : null,
    skills: role?.skillsData?.map(skill => ({id: skill._id, configID: skill.configID, lv: skill.lv})) ?? [],
    entities: w.EntityManager ? Object.values(w.EntityManager.ins().getAllEntity()).map(entity => ({handle: entity.handle ?? entity.infoModel?.handle, x: entity.x, y: entity.y, hp: entity.infoModel?.getAtt?.(AttributeType.atHp)})) : [],
    socketStatus: w.GameSocket?.ins?.()._socketStatus ?? null,
    errors: w.__skillProofErrors || window.__skillProofErrors || []
  };
})()`;

function parseFrames() {
  return wsFrames.map((frame, index) => {
    const bytes = Buffer.from(frame.payload, 'base64');
    const offset = frame.direction === 'sent' ? 8 : 4;
    return { index, direction: frame.direction, timestamp: frame.timestamp, hex: bytes.toString('hex'), sysid: bytes[offset], cmdid: bytes[offset + 1] };
  });
}

function decodeRoleFrame(frame) {
  if (frame.direction !== 'received' || frame.sysid !== 0 || frame.cmdid !== 2) return null;
  const bytes = Buffer.from(frame.hex, 'hex');
  const body = bytes.subarray(4);
  if (body.length !== 3677 - 4 + 436) return null;
  return {
    index: frame.index,
    frameBytes: bytes.length,
    levels: Array.from({length: 5}, (_, index) => body.readInt32LE(28 + index * 4)),
    trailing: Array.from({length: 5}, (_, index) => body.readInt32LE(48 + index * 4))
  };
}

function decodeSkillFrame(frame) {
  if (frame.direction !== 'received' || frame.sysid !== 2) return null;
  const bytes = Buffer.from(frame.hex, 'hex');
  const common = {index: frame.index, timestamp: frame.timestamp, cmdid: frame.cmdid, frameBytes: bytes.length};
  if (frame.cmdid === 1 && bytes.length === 36) {
    return {...common, attacker: bytes.readDoubleLE(6), skillID: bytes.readInt32LE(14), target: bytes.readDoubleLE(18), damageCount: bytes.readUInt16LE(26), damage: bytes.readDoubleLE(28)};
  }
  if (frame.cmdid === 4 && bytes.length === 12) {
    return {...common, roleID: bytes.readUInt16LE(6), slot: bytes.readUInt16LE(8), level: bytes.readUInt16LE(10)};
  }
  if (frame.cmdid === 5 && bytes.length === 18) {
    return {...common, roleID: bytes.readUInt16LE(6), levels: Array.from({length: 5}, (_, index) => bytes.readUInt16LE(8 + index * 2))};
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
    const w = document.querySelector('iframe')?.contentWindow || window;
    const raw = w.GameSocket.ins().socket_.socket.socket;
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
let singleCall = null;
let allCall = null;
let firstCastCall = null;
let cooldownCastCall = null;
let secondCastCall = null;
let moveHex = null;

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
async function waitForFrames(check, label, timeout = 30000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    await sleep(100);
    const frames = parseFrames();
    if (check(frames)) return frames;
  }
  throw new Error(label + ' 超时');
}
const skillFrames = (frames, start = 0) => frames.map(decodeSkillFrame).filter(frame => frame && frame.index >= start);
const roleFrames = frames => frames.map(decodeRoleFrame).filter(Boolean);
const skillSignature = current => JSON.stringify(current.skills.map(skill => [skill.configID, skill.lv]));

try {
  stage(`${mode}: 导航`);
  await send('Page.navigate', { url });
  await send('Page.bringToFront');
  stage(`${mode}: 选择账号`);
  await selectLoginAccount();
  stage(`${mode}: 进入场景`);
  await waitForScene();
  stage(`${mode}: 等待 RoleData 技能段`);
  await waitForFrames(frames => roleFrames(frames).length === 1, '等待 RoleData');
  const initialState = await waitForState(current => current.skills.length === 5 && current.skills.every(skill => skill.configID > 0 && skill.lv > 0), '等待客户端五槽技能');
  const initialRole = roleFrames(parseFrames())[0];
  await capture(`skill_${mode}_initial.png`);

  let upgradedState = null;
  let allUpgradedState = null;
  let beforeCast = null;
  let afterFirstCast = null;
  let afterSecondCast = null;
  let mutationFrameStart = parseFrames().length;
  let castFrameStart = parseFrames().length;

  if (mode === 'mutate') {
    stage(`${mode}: 单技能升级`);
    singleCall = await evaluate(`(() => {
      const w = document.querySelector('iframe')?.contentWindow || window;
      const skill = w.UserSkill?.ins?.();
      return {available: typeof skill?.sendGrewUpSkill === 'function', result: skill?.sendGrewUpSkill(0, 0)};
    })()`);
    await waitForFrames(frames => skillFrames(frames, mutationFrameStart).some(frame => frame.cmdid === 4 && frame.slot === 0 && frame.level === 2), '等待单技能升级回包');
    upgradedState = await waitForState(current => current.skills[0].lv === 2 && current.actor.gold === 95000, '等待客户端技能与金币更新');

    stage(`${mode}: 一键升级选中技能`);
    allCall = await evaluate(`(() => {
      const w = document.querySelector('iframe')?.contentWindow || window;
      const skill = w.UserSkill?.ins?.();
      return {available: typeof skill?.sendGrewUpAllSkill === 'function', result: skill?.sendGrewUpAllSkill(0, false, 0)};
    })()`);
    await waitForFrames(frames => skillFrames(frames, mutationFrameStart).some(frame => frame.cmdid === 5), '等待一键升级回包');
    allUpgradedState = await waitForState(current => current.skills[0].lv > 2 && current.actor.gold < 95000, '等待一键升级客户端状态');

    stage(`${mode}: 移动到怪物攻击距离`);
    moveHex = await sendMovement(384, 384);
    await waitForState(current => current.entities.some(entity => entity.handle === actorID && entity.x === 384 && entity.y === 384), '等待玩家移动');
    beforeCast = await waitForState(current => current.entities.some(entity => entity.handle === monsterID && entity.hp > 0), '等待目标怪物');
    const configID = beforeCast.skills[0].configID;
    castFrameStart = parseFrames().length;
    stage(`${mode}: 释放基础技能`);
    firstCastCall = await evaluate(`(() => {
      const w = document.querySelector('iframe')?.contentWindow || window;
      const skill = w.UserSkill?.ins?.();
      return {available: typeof skill?.sendUseSkill === 'function', result: skill?.sendUseSkill(${actorID}, ${monsterID}, ${configID})};
    })()`);
    await waitForFrames(frames => skillFrames(frames, castFrameStart).some(frame => frame.cmdid === 1), '等待技能伤害回包');
    afterFirstCast = await waitForState(current => current.entities.some(entity => entity.handle === monsterID && entity.hp < beforeCast.entities.find(item => item.handle === monsterID).hp), '等待怪物掉血');

    stage(`${mode}: 验证冷却拒绝重复释放`);
    cooldownCastCall = await evaluate(`(() => {
      const w = document.querySelector('iframe')?.contentWindow || window;
      return w.UserSkill?.ins?.().sendUseSkill(${actorID}, ${monsterID}, ${configID});
    })()`);
    await sleep(250);
    const cooldownResults = skillFrames(parseFrames(), castFrameStart).filter(frame => frame.cmdid === 1);
    if (cooldownResults.length !== 1) throw new Error(`冷却期内收到 ${cooldownResults.length} 个技能结果`);

    stage(`${mode}: 冷却结束后再次释放`);
    await sleep(850);
    secondCastCall = await evaluate(`(() => {
      const w = document.querySelector('iframe')?.contentWindow || window;
      return w.UserSkill?.ins?.().sendUseSkill(${actorID}, ${monsterID}, ${configID});
    })()`);
    await waitForFrames(frames => skillFrames(frames, castFrameStart).filter(frame => frame.cmdid === 1).length === 2, '等待第二次技能伤害回包');
    afterSecondCast = await waitForState(current => current.entities.some(entity => entity.handle === monsterID && entity.hp < afterFirstCast.entities.find(item => item.handle === monsterID).hp), '等待第二次怪物掉血');
    await capture('skill_mutate_cast.png');
  }

  stage(`${mode}: 汇总验收证据`);
  const finalState = await refreshState();
  const frames = parseFrames();
  const skills = skillFrames(frames);
  const role = roleFrames(frames)[0];
  const loginFrames = frames.filter(frame => frame.direction === 'sent' && frame.sysid === 255 && frame.cmdid === 1);
  const mutationSkills = skillFrames(frames, mutationFrameStart);
  const validation = {
    singleLoginAttempt: wsConnections.filter(item => item.includes(':19002')).length === 1 && loginFrames.length === 1,
    sceneEntered: initialState.scene && initialState.map?.id === 2,
    roleDataLengthValid: role?.frameBytes === 4 + 3677 - 4 + 436,
    roleSkillLevelsValid: role?.levels.every((level, index) => level === initialState.skills[index].lv) && role?.trailing.every(value => value === 0),
    fiveSkillsParsed: initialState.skills.length === 5 && initialState.skills.every((skill, index) => skill.configID === 11001 + index * 1000 + skill.lv - 1),
    noClientErrors: finalState.errors.length === 0 && runtimeEvents.length === 0,
    playerAlive: finalState.entities.some(entity => entity.handle === actorID && entity.hp > 0)
  };
  if (mode === 'mutate') {
    const single = mutationSkills.find(frame => frame.cmdid === 4);
    const all = mutationSkills.find(frame => frame.cmdid === 5);
    const casts = skillFrames(frames, castFrameStart).filter(frame => frame.cmdid === 1);
    validation.singleUpgradePacket = single?.roleID === 0 && single?.slot === 0 && single?.level === 2;
    validation.singleUpgradeCost = upgradedState?.actor.gold === 95000;
    validation.allUpgradePacket = all?.roleID === 0 && all?.levels[0] > 2 && all.levels.slice(1).every(level => level === 1);
    validation.allUpgradeClientState = allUpgradedState?.skills[0].lv === all?.levels[0] && allUpgradedState.actor.gold < 95000;
    validation.firstCastLayout = casts[0]?.attacker === actorID && casts[0]?.target === monsterID && casts[0]?.skillID === finalState.skills[0].configID && casts[0]?.damageCount === 1 && casts[0]?.damage > 0;
    validation.firstCastDamage = afterFirstCast?.entities.find(entity => entity.handle === monsterID).hp < beforeCast.entities.find(entity => entity.handle === monsterID).hp;
    validation.cooldownRejected = casts.length === 2;
    validation.secondCastDamage = afterSecondCast?.entities.find(entity => entity.handle === monsterID).hp < afterFirstCast.entities.find(entity => entity.handle === monsterID).hp;
  } else {
    const expected = JSON.parse(fs.readFileSync(pathJoin('skill_mutate_browser_proof.json'), 'utf8'));
    validation.persistedSkills = skillSignature(finalState) === skillSignature(expected.finalState);
    validation.persistedGold = finalState.actor.gold === expected.finalState.actor.gold;
    validation.noSkillMutations = mutationSkills.length === 0;
  }
  if (Object.values(validation).some(value => !value)) throw new Error('skill validation failed: ' + JSON.stringify({validation, initialState, upgradedState, allUpgradedState, beforeCast, afterFirstCast, afterSecondCast, finalState, skills, role}));
  const proof = {
    date: proofDate, mode, url, gateway: '127.0.0.1:19002', gameworld: '127.0.0.1:16001',
    database, account: loginAccount, actorID, monsterID,
    initialState, upgradedState, allUpgradedState, beforeCast, afterFirstCast, afterSecondCast,
    singleCall, allCall, firstCastCall, cooldownCastCall, secondCastCall, moveHex,
    finalState, validation, wsConnections, loginFrames, skillFrames: skills, role, wsFrameCount: frames.length
  };
  fs.writeFileSync(pathJoin(`skill_${mode}_browser_proof.json`), JSON.stringify(proof, null, 2));
  passed = true;
  console.log(JSON.stringify({mode, validation, finalActor: finalState.actor, finalSkills: finalState.skills, wsFrameCount: frames.length}, null, 2));
} catch (error) {
  fs.writeFileSync(pathJoin(`skill_${mode}_failure.json`), JSON.stringify({error: String(error), state, runtimeEvents, singleCall, allCall, firstCastCall, cooldownCastCall, secondCastCall, frames: parseFrames()}, null, 2));
  throw error;
} finally {
  await fetch(`http://127.0.0.1:9223/json/close/${target.id}`).catch(() => {});
  ws.close();
}
