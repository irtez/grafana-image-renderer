(request) => {
  'use strict';
  const error = (code) => ({ status: 'error', error: { code } });
  const pending = () => ({ status: 'pending', contextKey: '', panels: [] });
  try {
    const scene = window.__grafanaSceneContext;
    if (!scene) return pending();
    if (!scene.state || typeof scene.getDashboardPanels !== 'function')
      return error('CAPTURE_LAYOUT_UNSUPPORTED');
    if (!scene.state.uid) return pending();
    if (scene.state.uid !== request.dashboardUID)
      return error('CAPTURE_CONTEXT_CHANGED');
    const vars = scene.state.$variables?.state?.variables ?? [];
    if (!Array.isArray(vars)) return error('CAPTURE_LAYOUT_UNSUPPORTED');
    if (
      scene.state.defaultVariablesLoading ||
      vars.some((v) => v.state?.loading === true || v.state?.isLoading === true)
    )
      return pending();
    const time = scene.state.$timeRange?.state;
    if (!time?.value) return pending();
    const from = time.value.from?.valueOf(),
      to = time.value.to?.valueOf();
    if (!Number.isFinite(from) || !Number.isFinite(to))
      return error('CAPTURE_LAYOUT_UNSUPPORTED');
    const picker = scene.state.controls?.state?.refreshPicker;
    if (picker?.state?.refresh) {
      if (typeof picker.onIntervalChanged !== 'function')
        return error('CAPTURE_LAYOUT_UNSUPPORTED');
      picker.onIntervalChanged('');
    }
    const key = '__SVG_MODIFIER_LAYOUT_V2__';
    if (!window[key])
      Object.defineProperty(window, key, {
        value: { scene, epoch: 0, token: Date.now() + ':' + Math.random() },
      });
    const local = window[key];
    if (local.scene !== scene) {
      local.scene = scene;
      local.epoch++;
    }
    const contextKey = JSON.stringify([
      local.token,
      local.epoch,
      scene.state.uid,
      scene.state.version,
      time.from,
      time.to,
      time.timeZone,
      from,
      to,
      vars.map((v) => [v.state?.name, v.state?.value]),
    ]);
    if (contextKey.length > 262144) return error('CAPTURE_LAYOUT_UNSUPPORTED');
    const all = scene.getDashboardPanels();
    if (!Array.isArray(all) || all.length > 10000)
      return error('CAPTURE_LAYOUT_UNSUPPORTED');
    const ancestry = (panel) => {
      const chain = [],
        seen = new Set();
      for (let n = panel; n && n !== scene; n = n.parent) {
        if (seen.has(n) || chain.length >= 64 || !n.state)
          throw Error('layout');
        seen.add(n);
        chain.push(n);
      }
      return chain;
    };
    const legacyId = (panel) => {
      if (typeof panel.getLegacyPanelId === 'function')
        return panel.getLegacyPanelId();
      const match = /^panel-(\d+)(?:$|-)/.exec(panel.state?.key ?? '');
      return match ? Number(match[1]) : null;
    };
    const rows = request.panelIds.map((panelId) => {
      const matches = all.filter((p) => legacyId(p) === panelId);
      const fail = (code) => ({ panelId, ...error(code) });
      if (!matches.length) return fail('CAPTURE_PANEL_NOT_FOUND');
      if (matches.length !== 1) return fail('CAPTURE_REPEAT_UNSUPPORTED');
      const panel = matches[0],
        chain = ancestry(panel);
      if (
        chain.some(
          (n) =>
            !!n.state.repeatByVariable ||
            !!n.state.variableName ||
            n.isRepeated?.() === true,
        )
      )
        return fail('CAPTURE_REPEAT_UNSUPPORTED');
      if (panel.state.pluginId !== 'svgmodifier-panel')
        return fail('CAPTURE_PANEL_UNSUPPORTED');
      const tabs = chain.filter(
        (n) =>
          Array.isArray(n.parent?.state?.tabs) &&
          n.parent.state.tabs.includes(n),
      );
      if (
        tabs.some(
          (n) =>
            typeof n.getSlug !== 'function' ||
            typeof n.parent.switchToTab !== 'function' ||
            typeof n.parent.getCurrentTab !== 'function',
        )
      )
        return fail('CAPTURE_LAYOUT_UNSUPPORTED');
      if (panelId === request.focusID) {
        for (const n of chain.slice().reverse()) {
          if (tabs.includes(n) && n.parent.getCurrentTab() !== n)
            n.parent.switchToTab(n);
          if (
            typeof n.getCollapsedState === 'function' &&
            typeof n.setCollapsedState === 'function'
          ) {
            if (n.getCollapsedState()) n.setCollapsedState(false);
          } else if (
            n.state.isCollapsed === true &&
            Array.isArray(n.state.children) &&
            typeof n.setState === 'function'
          ) {
            n.setState({ isCollapsed: false });
          } else if (n.state.collapse === true || n.state.isCollapsed === true)
            return fail('CAPTURE_LAYOUT_UNSUPPORTED');
        }
        const panelKey = panel.state.key;
        if (panelKey !== 'panel-' + panelId)
          return fail('CAPTURE_REPEAT_UNSUPPORTED');
        const elements = document.querySelectorAll(
          '[data-viz-panel-key="' + panelKey + '"]',
        );
        if (elements.length > 1) return fail('CAPTURE_REPEAT_UNSUPPORTED');
        if (elements.length)
          elements[0].scrollIntoView({ block: 'center', behavior: 'instant' });
        else if (typeof panel.parent?.scrollIntoView === 'function')
          panel.parent.scrollIntoView();
      }
      return {
        panelId,
        status: 'ready',
        active:
          panel.isActive !== false &&
          tabs.every((n) => n.parent.getCurrentTab() === n),
      };
    });
    return { status: 'ready', contextKey, panels: rows };
  } catch {
    return error('CAPTURE_LAYOUT_UNSUPPORTED');
  }
}
