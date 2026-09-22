const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { test } = require('node:test');

const scripts = JSON.parse(fs.readFileSync(0, 'utf8'));
const name = '__SVG_MODIFIER_RENDER_CAPTURE_V1__';
function context(bootstrap = scripts.bootstrap) {
  const realm = vm.createContext({});
  vm.runInContext(`
    window = globalThis;
    frames = [];
    frameElements = [];
    class HTMLIFrameElement {
      constructor(child) { this.child = child; this.localName = 'iframe'; }
      get contentWindow() { return this.child; }
      get contentDocument() { return this.child.__nativeDocument; }
    }
    HTMLFrameElement = HTMLIFrameElement;
    class Document {
      querySelectorAll() { return this.actualElements ?? this.elements(); }
    }
    document = new Document();
    document.elements = () => [...new Set([...frames, ...frameElements.map(e => e.contentWindow)])]
      .map(child => new HTMLIFrameElement(child));
    __nativeDocument = document;
    Object.defineProperty(window, 'length', { get() {
      return this.__nativeFrameCount ?? Document.prototype.querySelectorAll.call(this.__nativeDocument).length;
    } });
    setTimeout = setInterval = requestAnimationFrame = function () { throw Error('timer'); };
    MutationObserver = function () { throw Error('observer'); };
    function identity(instanceId = 'instance-a') {
      return { producerId: 'svgmodifier-panel', producerVersion: '1.0.0', panelId: 7, instanceId };
    }
    function run(generation = 1) {
      return { generation, effectiveFromMs: 1000, effectiveToMs: 2000 };
    }
    function snapshot(generation = 1) {
      return { kind: 'svgmodifier', schemaVersion: 1,
        producer: { id: 'svgmodifier-panel', version: '1.0.0' }, panel: { id: 7 },
        observed: { generation, effectiveFromMs: 1000, effectiveToMs: 2000,
          evaluatedAtMs: 2001, dataState: 'Done' } };
    }
    function connect(instanceId) { return __SVG_MODIFIER_CAPTURE_V1__.connect(identity(instanceId)); }
    function publish(instanceId = 'instance-a', generation = 1) {
      const session = connect(instanceId);
      session.begin(run(generation));
      session.publish(generation, () => snapshot(generation));
      return session;
    }
  `, realm);
  vm.runInContext(bootstrap, realm);
  return realm;
}
function execute(realm, code) { return vm.runInContext(code, realm); }
function read(realm) { return JSON.parse(JSON.stringify(execute(realm, scripts.read))); }
function local(realm) { return JSON.parse(JSON.stringify(execute(realm, `${name}.read()`))); }
function code(realm, expected) {
  const state = read(realm);
  assert.equal(state.status, 'terminal-error');
  assert.equal(state.error.code, expected);
  assert.equal(state.snapshot, undefined);
}

test('installs in a child context without timers and accepts only the requested producer', () => {
  const realm = context('');
  realm.top = {};
  execute(realm, scripts.bootstrap);
  assert.equal(execute(realm, '__SVG_MODIFIER_CAPTURE_V1__.connect({...identity(),panelId:8})'), null);
  assert.equal(execute(realm, '__SVG_MODIFIER_CAPTURE_V1__.connect({...identity(),producerId:"other"})'), null);
  assert.equal(execute(realm, '__SVG_MODIFIER_CAPTURE_V1__.connect({...identity(),instanceId:""})'), null);
  assert.equal(read(realm).status, 'idle');
  execute(realm, 'handle = publish()');
  assert.equal(read(realm).status, 'terminal-ok');
  assert.equal(execute(realm, 'handle.maxPayloadBytes'), 1048576);
  assert.equal(execute(realm, 'handle.protocolVersion'), 1);
});

test('one atomic immutable copy retains original values and exact byte count', () => {
  const realm = context();
  execute(realm, `handle = connect(); handle.begin(run()); original = snapshot();
    original.extra = { text: 'é漢😀\\n\\t"\\\\\\ud800', numbers: [-0, 1.25, 1e-20], nested: [null, true] };
    expected = JSON.stringify(original);
    handle.publish(1, () => original);
    original.extra.nested[0] = 'changed'; original.observed.generation = 8;`);
  const state = read(realm);
  assert.equal(state.status, 'terminal-ok');
  assert.deepEqual(state.snapshot, JSON.parse(execute(realm, 'expected')));
  assert.equal(state.payloadBytes, Buffer.byteLength(execute(realm, 'expected')));
  assert.deepEqual(state.run, { generation: 1, effectiveFromMs: 1000, effectiveToMs: 2000 });
  assert.equal(execute(realm, `Object.isFrozen(${name}.read().snapshot.extra.nested)`), true);
});

test('begin clears success; late publish and fail cannot replace the current generation', () => {
  const realm = context();
  execute(realm, 'handle = publish(); handle.begin(run(2)); called = false; handle.publish(1, () => { called = true; return snapshot(); }); handle.fail(1,{code:"CAPTURE_EXPORT_FAILED"});');
  assert.equal(read(realm).status, 'pending');
  assert.equal(read(realm).snapshot, undefined);
  assert.equal(execute(realm, 'called'), false);
  execute(realm, 'handle.publish(2, () => snapshot(2)); handle.begin(run(1));');
  assert.equal(read(realm).snapshot.observed.generation, 2);
});

test('invalid newly announced run removes success and rejects late callbacks from the previous run', () => {
  const realm = context();
  execute(realm, 'handle=publish();handle.begin({...run(2),effectiveFromMs:NaN});called=false;');
  code(realm, 'CAPTURE_PAYLOAD_INVALID');
  assert.equal(read(realm).run, null);
  execute(realm, 'handle.publish(1,()=>{called=true;return snapshot();});handle.fail(1,{code:"CAPTURE_EXPORT_FAILED"});');
  assert.equal(execute(realm, 'called'), false);
  code(realm, 'CAPTURE_PAYLOAD_INVALID');
});

test('valid old and duplicate begin preserve the current terminal result', () => {
  const realm = context();
  execute(realm, 'handle=publish("instance-a",2);');
  const before = read(realm);
  execute(realm, 'handle.begin(run(1));handle.begin(run(2));');
  assert.deepEqual(read(realm), before);
});

test('invalid begin does not execute time getters or leave the old snapshot', () => {
  const realm = context();
  execute(realm, `handle=publish();called=false;handle.begin({...run(2),get effectiveFromMs(){called=true;return 1000;}});`);
  assert.equal(execute(realm, 'called'), false);
  code(realm, 'CAPTURE_PAYLOAD_INVALID');
});

test('run validation cannot overwrite a new begin triggered by descriptor reentrancy', () => {
  const realm = context();
  execute(realm, `handle=publish();handle.begin(new Proxy({...run(3),effectiveFromMs:NaN},{
    getOwnPropertyDescriptor(value,key){
      if(key==='effectiveFromMs')handle.begin(run(2));
      return Object.getOwnPropertyDescriptor(value,key);
    }
  }));`);
  assert.equal(read(realm).status, 'pending');
  assert.equal(read(realm).run.generation, 2);
});

test('run validation cannot overwrite a newer terminal error triggered by descriptor reentrancy', () => {
  const realm = context();
  execute(realm, `handle=publish();handle.begin(new Proxy({...run(2),effectiveFromMs:NaN},{
    getOwnPropertyDescriptor(value,key){
      if(key==='effectiveFromMs')handle.fail(1,{code:'CAPTURE_DATA_STATE_UNSUPPORTED'});
      return Object.getOwnPropertyDescriptor(value,key);
    }
  }));`);
  code(realm, 'CAPTURE_DATA_STATE_UNSUPPORTED');
});

test('even a valid outer begin cannot replace a run begun later during validation', () => {
  const realm = context();
  execute(realm, `handle=publish();handle.begin(new Proxy(run(3),{
    getOwnPropertyDescriptor(value,key){
      if(key==='effectiveFromMs'){handle.begin(run(2));handle.publish(2,()=>snapshot(2));}
      return Object.getOwnPropertyDescriptor(value,key);
    }
  }));`);
  assert.equal(read(realm).status, 'terminal-ok');
  assert.equal(read(realm).snapshot.observed.generation, 2);
});

test('duplicate publish after success does not call the factory or replace the snapshot', () => {
  const realm = context();
  execute(realm, 'handle=publish();called=false;');
  const before = read(realm);
  execute(realm, 'handle.publish(1,()=>{called=true;return {...snapshot(),extra:"replacement"};});');
  assert.equal(execute(realm, 'called'), false);
  assert.deepEqual(read(realm), before);
});

test('publish after failure does not call the factory or resurrect success', () => {
  const realm = context();
  execute(realm, 'handle=connect();handle.begin(run());handle.fail(1,{code:"CAPTURE_EXPORT_FAILED"});called=false;');
  execute(realm, 'handle.publish(1,()=>{called=true;return snapshot();});');
  assert.equal(execute(realm, 'called'), false);
  code(realm, 'CAPTURE_EXPORT_FAILED');
});

test('recursive publish in the same factory runs the factory only once', () => {
  const realm = context();
  execute(realm, `handle=connect();handle.begin(run());called=0;
    handle.publish(1,function build(){called++;if(called<5)handle.publish(1,build);return snapshot();});`);
  assert.equal(execute(realm, 'called'), 1);
  assert.equal(read(realm).status, 'terminal-ok');
});

test('copy-time reentrant publish cannot replace the outer snapshot', () => {
  const realm = context();
  execute(realm, `handle=connect();handle.begin(run());called=false;
    handle.publish(1,()=>new Proxy(snapshot(),{ownKeys(value){
      handle.publish(1,()=>{called=true;return {...snapshot(),extra:'replacement'};});
      return Reflect.ownKeys(value);
    }}));`);
  assert.equal(execute(realm, 'called'), false);
  assert.equal(read(realm).status, 'terminal-ok');
  assert.equal(read(realm).snapshot.extra, undefined);
});

test('a new generation may publish while an older generation factory is returning', () => {
  const realm = context();
  execute(realm, `handle=connect();handle.begin(run());
    handle.publish(1,()=>{handle.begin(run(2));handle.publish(2,()=>snapshot(2));return snapshot();});`);
  assert.equal(read(realm).status, 'terminal-ok');
  assert.equal(read(realm).snapshot.observed.generation, 2);
});

test('unmount and remount cannot restore a closed session', () => {
  const realm = context();
  execute(realm, 'old = publish(); old.close(); handle = connect("instance-b"); old.publish(1, () => snapshot());');
  assert.equal(read(realm).status, 'idle');
  execute(realm, 'handle.begin(run()); handle.publish(1, () => snapshot());');
  assert.equal(read(realm).identity.instanceId, 'instance-b');
});

test('two live connections fail ambiguous even if their identities are equal', () => {
  const realm = context();
  execute(realm, 'first = publish(); second = connect();');
  code(realm, 'CAPTURE_INSTANCE_AMBIGUOUS');
  execute(realm, 'second.close(); first.publish(1, () => snapshot());');
  assert.equal(read(realm).status, 'idle');
  execute(realm, 'first.begin(run(2)); first.publish(2, () => snapshot(2));');
  assert.equal(read(realm).snapshot.observed.generation, 2);
});

test('excess simultaneous connections stay bounded and cannot later revive a hidden instance', () => {
  const realm = context();
  execute(realm, `handles=Array.from({length:1000},(_,i)=>connect('instance-'+i));`);
  assert.ok(execute(realm, 'handles.filter(Boolean).length') <= 64);
  execute(realm, 'handles.filter(Boolean).forEach(handle=>handle.close());');
  code(realm, 'CAPTURE_INSTANCE_AMBIGUOUS');
});

for (const action of ['handle.begin(run(2))', 'handle.fail(1,{code:"CAPTURE_DATA_STATE_UNSUPPORTED"})', 'handle.close()']) {
  test(`factory reentrancy preserves the newer state: ${action}`, () => {
    const realm = context();
    execute(realm, `handle = connect(); handle.begin(run()); handle.publish(1, () => { ${action}; return snapshot(); });`);
    assert.notEqual(read(realm).status, 'terminal-ok');
    assert.equal(read(realm).snapshot, undefined);
  });
}

test('copy-time proxy reentrancy cannot overwrite a new run', () => {
  const realm = context();
  execute(realm, `handle = connect(); handle.begin(run());
    handle.publish(1, () => new Proxy(snapshot(), { ownKeys(value) { handle.begin(run(2)); return Reflect.ownKeys(value); } }));`);
  assert.equal(read(realm).status, 'pending');
  assert.equal(read(realm).run.generation, 2);
});

test('fail-time proxy reentrancy cannot overwrite a newer error in the same run', () => {
  const realm = context();
  execute(realm, `handle = connect(); handle.begin(run());
    handle.fail(1, new Proxy({}, { getOwnPropertyDescriptor() {
      handle.fail(1, {code:'CAPTURE_DATA_STATE_UNSUPPORTED'});
      return {value:'CAPTURE_EXPORT_FAILED',configurable:true};
    } }));`);
  code(realm, 'CAPTURE_DATA_STATE_UNSUPPORTED');
});

test('factory exceptions and unknown producer failures expose only safe codes', () => {
  const realm = context();
  execute(realm, 'handle = connect(); handle.begin(run()); handle.publish(1, () => { throw Error("secret"); });');
  code(realm, 'CAPTURE_EXPORT_FAILED');
  execute(realm, `handle.begin(run(2)); touched = false; handle.fail(2, {code:'secret',get message(){touched=true;throw Error('secret')}});`);
  code(realm, 'CAPTURE_EXPORT_FAILED');
  assert.equal(execute(realm, 'touched'), false);
  assert.equal(JSON.stringify(read(realm)).includes('secret'), false);
});

for (const mutate of [
  'value.extra = undefined', 'value.extra = NaN', 'value.extra = Infinity',
  'value.extra = () => 1', 'value.extra = value', 'value.extra = new Date()',
  'value.extra = [1,,3]', 'value.extra = {[Symbol("key")]:1}',
  'Object.defineProperty(value,"extra",{value:1,enumerable:false})',
  'Object.defineProperty(value,"extra",{get(){ touched=true;return 1; },enumerable:true})',
  'value.toJSON = () => { touched=true;return {}; }',
  'value.panel.id=8', 'value.producer.version="2"', 'value.observed.generation=2',
  'value.observed.effectiveFromMs=999', 'value.observed.dataState="Loading"',
]) {
  test(`rejects invalid snapshot without executing accessors: ${mutate}`, () => {
    const realm = context();
    execute(realm, `handle=connect();handle.begin(run());value=snapshot();touched=false;${mutate};handle.publish(1,()=>value);`);
    code(realm, 'CAPTURE_PAYLOAD_INVALID');
    assert.equal(execute(realm, 'touched'), false);
  });
}

test('safe data keys cannot pollute the copied object prototype', () => {
  const realm = context();
  execute(realm, `handle=connect();handle.begin(run());value=snapshot();
    value.extra=JSON.parse('{"__proto__":{"polluted":true},"constructor":{"prototype":{"polluted":true}}}');
    handle.publish(1,()=>value);`);
  assert.equal(read(realm).status, 'terminal-ok');
  assert.equal(read(realm).snapshot.extra.__proto__.polluted, true);
  assert.equal(execute(realm, `Object.getPrototypeOf(${name}.read().snapshot.extra)`), null);
  assert.equal(execute(realm, '({}).polluted'), undefined);
});

test('an inherited toJSON added after publication is never called on the stored copy', () => {
  const realm = context();
  execute(realm, `handle=connect();handle.begin(run());value=snapshot();value.extra=[1];handle.publish(1,()=>value);
    Object.prototype.toJSON = Array.prototype.toJSON = function(){throw Error('unsafe serialization')};`);
  assert.equal(execute(realm, `JSON.parse(JSON.stringify(${name}.read())).snapshot.extra[0]`), 1);
});

test('exact UTF-8 byte limit permits the complete snapshot and rejects the next byte', () => {
  const realm = context(scripts.limited);
  execute(realm, `handle=connect();handle.begin(run());value=snapshot();value.extra='';
    value.extra='x'.repeat(512-JSON.stringify(value).length);handle.publish(1,()=>value);`);
  assert.equal(read(realm).status, 'terminal-ok');
  assert.equal(read(realm).payloadBytes, 512);
  execute(realm, `handle.begin(run(2));value.observed.generation=2;value.extra+='x';handle.publish(2,()=>value);`);
  code(realm, 'CAPTURE_PAYLOAD_TOO_LARGE');
});

for (const expression of ['"x".repeat(2000000)', 'new Array(100000000)', 'Array.from({length:100001},()=>0)', 'Object.fromEntries(Array.from({length:100001},(_,i)=>["key"+i,0]))', 'Array.from({length:70}).reduce(v=>({child:v}),null)']) {
  test(`copy returns an explicit size error without a partial snapshot: ${expression}`, () => {
    const realm = context();
    execute(realm, `handle=connect();handle.begin(run());value=snapshot();value.extra=${expression};handle.publish(1,()=>value);`);
    code(realm, 'CAPTURE_PAYLOAD_TOO_LARGE');
  });
}

test('bootstrap is idempotent and does not reset an active instance', () => {
  const realm = context();
  execute(realm, 'handle=publish();');
  execute(realm, scripts.bootstrap);
  assert.equal(read(realm).status, 'terminal-ok');
});

test('reader searches nested and hidden frame contexts', () => {
  const root = context();
  const child = context();
  const hidden = context();
  root.frames = [child];
  child.frameElements = [{ contentWindow: hidden }];
  execute(hidden, 'handle=publish();');
  assert.equal(read(root).status, 'terminal-ok');
  assert.equal(local(root).status, 'idle');
});

test('reader deduplicates aliases for one validated instance', () => {
  const root = context();
  const child = context();
  execute(child, 'handle=publish();');
  root.frames = [child, child];
  root.frameElements = [{ contentWindow: child }];
  assert.equal(read(root).status, 'terminal-ok');
});

test('same identity and snapshot in distinct alias objects is one producer', () => {
  const root = context();
  const child = context();
  execute(root, 'handle=publish();');
  execute(child, 'handle=publish();');
  root.frames = [child];
  assert.equal(read(root).status, 'terminal-ok');
});

test('alias equivalence compares JSON values independently of object key insertion order', () => {
  const root = context();
  const child = context();
  execute(root, 'handle=publish();');
  execute(child, `handle=connect();handle.begin(run());handle.publish(1,()=>{
    const value=snapshot();value.panel={id:7};
    return Object.fromEntries(Object.entries(value).reverse());
  });`);
  root.frames = [child];
  assert.equal(read(root).status, 'terminal-ok');
});

test('different matching instances in separate frames fail ambiguous', () => {
  const root = context();
  const child = context();
  execute(root, 'handle=publish();');
  execute(child, 'handle=publish("instance-b");');
  root.frames = [child];
  code(root, 'CAPTURE_INSTANCE_AMBIGUOUS');
});

test('an errored producer does not hide ambiguity with another matching instance', () => {
  const root = context();
  const child = context();
  execute(root, 'handle=publish();handle.fail(1,{code:"CAPTURE_EXPORT_FAILED"});');
  execute(child, 'handle=publish("instance-b");');
  root.frames = [child];
  code(root, 'CAPTURE_INSTANCE_AMBIGUOUS');
});

test('producer updates during frame discovery invalidate the previous success before reading', () => {
  const root = context();
  execute(root, 'handle=publish();document.elements=()=>{handle.begin(run(2));return [];};');
  assert.equal(read(root).status, 'pending');
  assert.equal(read(root).snapshot, undefined);
});

test('native document traversal reads a sandbox producer whose visible frames and document are virtualized', () => {
  const root = context();
  const child = context();
  execute(child, 'handle=publish();document.actualElements=[];');
  root.frames = [child];
  child.frames = { length: 1 };
  child.document = { querySelectorAll() { throw Error('iframe.contentWindow is not allowed'); } };
  assert.equal(read(root).status, 'terminal-ok');
  assert.equal(read(root).identity.instanceId, 'instance-a');
});

test('an undiscovered native child frame fails closed instead of hiding a second instance', () => {
  const root = context();
  execute(root, 'handle=publish();');
  root.__nativeFrameCount = 1;
  code(root, 'CAPTURE_FRAME_UNSUPPORTED');
});

test('a native inaccessible child document cannot be replaced by a readable virtual document', () => {
  const root = context();
  const child = context();
  execute(child, 'handle=publish();');
  root.frames = [child];
  child.__nativeDocument = null;
  code(root, 'CAPTURE_FRAME_UNSUPPORTED');
});

test('aliases disagreeing about generation never return stale success', () => {
  const root = context();
  const child = context();
  execute(root, 'handle=publish();');
  execute(child, 'handle=connect();handle.begin(run(2));');
  root.frames = [child];
  code(root, 'CAPTURE_PAYLOAD_INVALID');
});

test('same-generation alias payload disagreement fails closed', () => {
  const root = context();
  const child = context();
  execute(root, 'handle=publish();');
  execute(child, 'handle=connect();handle.begin(run());handle.publish(1,()=>({...snapshot(),extra:1}));');
  root.frames = [child];
  code(root, 'CAPTURE_PAYLOAD_INVALID');
});

test('inaccessible frames cannot be hidden by a successful top-level producer', () => {
  const root = context();
  execute(root, 'handle=publish();');
  root.frames = [new Proxy({}, { get() { throw Error('cross origin'); } })];
  code(root, 'CAPTURE_FRAME_UNSUPPORTED');
});

test('unsupported child and overwritten receiver return explicit safe errors', () => {
  const root = context();
  root.frames = [context('')];
  code(root, 'CAPTURE_FRAME_UNSUPPORTED');
  const incompatible = context('');
  incompatible[name] = { protocolVersion: 2, read() { throw Error('secret'); } };
  code(incompatible, 'CAPTURE_PROTOCOL_UNSUPPORTED');
});

test('frame count and depth are bounded; cyclic aliases do not loop', () => {
  const root = context();
  root.frames = [root];
  assert.equal(read(root).status, 'idle');
  root.frames = Array.from({ length: 70 }, () => context());
  code(root, 'CAPTURE_FRAME_UNSUPPORTED');
  let parent = root;
  root.frames = [];
  for (let i = 0; i < 20; i++) { const child = context(); parent.frames = [child]; parent = child; }
  code(root, 'CAPTURE_FRAME_UNSUPPORTED');
});
