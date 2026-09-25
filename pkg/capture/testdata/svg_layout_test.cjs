const { test } = require('node:test'),
  assert = require('node:assert/strict'),
  vm = require('node:vm'),
  fs = require('node:fs');
const script = fs.readFileSync(0, 'utf8');
function fixture() {
  const realm = vm.createContext({});
  vm.runInContext(
    `window=globalThis;location={pathname:'/d/example/map',search:'?from=now-3h&to=now'};
 let scrolled=[];
 document={querySelectorAll(selector){return [{scrollIntoView(){scrolled.push(selector)},getBoundingClientRect(){return {width:200,height:100}},getAttribute(){return 'panel-7'}}]}};
 refresh={state:{refresh:'5s'},onIntervalChanged(value){this.state.refresh=value}};
 root={state:{uid:'example',version:1,$timeRange:{state:{from:'now-3h',to:'now',timeZone:'UTC',value:{from:1000,to:2000}}},$variables:{state:{variables:[{state:{name:'node',value:['a','b'],type:'custom'}}]}},controls:{state:{refreshPicker:refresh}}},getDashboardPanels(){return panels}};
 tab1={state:{key:'tab-1'},getSlug(){return 'One'}},tab2={state:{key:'tab-2'},getSlug(){return 'Two'}};
 tabs={state:{tabs:[tab1,tab2],currentTabSlug:'One'},parent:root,getCurrentTab(){return this.state.tabs.find(t=>t.getSlug()===this.state.currentTabSlug)},switchToTab(tab){this.state.currentTabSlug=tab.getSlug()}};
 tab1.parent=tabs;tab2.parent=tabs;
 row={state:{collapse:true},parent:tab2,getCollapsedState(){return this.state.collapse},setCollapsedState(value){this.state.collapse=value}};
 function panel(id,parent=tab1){return {state:{key:'panel-'+id,pluginId:'svgmodifier-panel'},parent,getLegacyPanelId(){return id}}};
 panels=[panel(7),panel(8,row)];__grafanaSceneContext=root;`,
    realm,
  );
  return {
    realm,
    step: (focusID = 7) =>
      JSON.parse(
        JSON.stringify(
          vm.runInContext(
            `(${script})({panelIds:[7,8,9],dashboardUID:'example',focusID:${focusID}})`,
            realm,
          ),
        ),
      ),
    get: (code) => vm.runInContext(code, realm),
  };
}
test('known panel inventory preserves order and missing panel is explicit', () => {
  const f = fixture(),
    r = f.step();
  assert.equal(r.status, 'ready');
  assert.deepEqual(
    r.panels.map((p) => p.panelId),
    [7, 8, 9],
  );
  assert.equal(r.panels[2].error.code, 'CAPTURE_PANEL_NOT_FOUND');
  assert.equal(f.get('refresh.state.refresh'), '');
});
test('focus activates exact tab then opens its row and scrolls selected panel', () => {
  const f = fixture();
  f.step(8);
  assert.equal(f.get('tabs.state.currentTabSlug'), 'Two');
  assert.equal(f.get('row.state.collapse'), false);
  assert.match(f.get('scrolled[0]'), /panel-8/);
});
test('tab switches do not change context but variables and time do', () => {
  const f = fixture(),
    a = f.step().contextKey;
  assert.equal(f.step(8).contextKey, a);
  f.get("root.state.$variables.state.variables[0].state.value=['c']");
  const b = f.step(8).contextKey;
  assert.notEqual(b, a);
  f.get('root.state.$timeRange.state.value.to=3000');
  assert.notEqual(f.step().contextKey, b);
});
test('duplicate or authored repeat does not select a first instance', () => {
  const f = fixture();
  f.get('panels.push(panel(7));');
  assert.equal(f.step().panels[0].error.code, 'CAPTURE_REPEAT_UNSUPPORTED');
  const other = fixture();
  other.get("panels[0].parent={state:{repeatByVariable:'node'},parent:root}");
  assert.equal(other.step().panels[0].error.code, 'CAPTURE_REPEAT_UNSUPPORTED');
});
test('unsupported type and unknown layout fail per panel', () => {
  const f = fixture();
  f.get("panels[0].state.pluginId='text';delete tabs.switchToTab;");
  const result = f.step(8);
  assert.equal(result.panels[0].error.code, 'CAPTURE_PANEL_UNSUPPORTED');
  assert.equal(result.panels[1].error.code, 'CAPTURE_LAYOUT_UNSUPPORTED');
});
test('no scene waits; incompatible scene fails safely without echoing exception text', () => {
  const f = fixture();
  f.get('delete __grafanaSceneContext');
  assert.equal(f.step().status, 'pending');
  f.get('__grafanaSceneContext={state:{uid:"example"}}');
  assert.equal(f.step().error.code, 'CAPTURE_LAYOUT_UNSUPPORTED');
});

test('loading data can precede the plugin mount', () => {
  const f=fixture();
  f.get("panels[0].state.$data={state:{data:{state:'Loading'}}}");
  assert.equal(f.step().panels[0].dataPending,true);
  f.get("panels[0].state.$data.state.data.state='Done'");
  assert.equal(f.step().panels[0].dataPending,false);
});
