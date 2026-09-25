(panelId) => {
  'use strict';
  const PRIVATE = '__SVG_MODIFIER_RENDER_CAPTURE_V2__';
  const MAX_FRAMES = 64;
  const MAX_DEPTH = 16;
  const error = (code) => Object.assign(Object.create(null), {
    status: 'terminal-error', identity: null, run: null,
    error: Object.assign(Object.create(null), { code }),
  });
  const idle = () => Object.assign(Object.create(null), { status: 'idle', identity: null, run: null });
  try {
    const root = window[PRIVATE];
    if (!root || root.protocolVersion !== 2 || typeof root.read !== 'function' ||
        !Array.isArray(root.panelIds) || !root.panelIds.includes(panelId) || !Number.isSafeInteger(root.maxPayloadBytes) || root.maxPayloadBytes < 1) {
      return error('CAPTURE_PROTOCOL_UNSUPPORTED');
    }
    // Методы берём из parent realm: sandbox подменяет document/frames и DOM-прототипы
    // дочернего window. Native ссылки живут только внутри этого renderer-вызова.
    const queryFrames = Document.prototype.querySelectorAll;
    const countFrames = Object.getOwnPropertyDescriptor(window, 'length').get;
    const frameAccess = {
      iframe: {
        window: Object.getOwnPropertyDescriptor(HTMLIFrameElement.prototype, 'contentWindow').get,
        document: Object.getOwnPropertyDescriptor(HTMLIFrameElement.prototype, 'contentDocument').get,
      },
      frame: {
        window: Object.getOwnPropertyDescriptor(HTMLFrameElement.prototype, 'contentWindow').get,
        document: Object.getOwnPropertyDescriptor(HTMLFrameElement.prototype, 'contentDocument').get,
      },
    };
    const seen = new Set();
    const pending = [{ frame: window, document: window.document, depth: 0 }];
    const receivers = [];
    let edges = 0;
    while (pending.length) {
      const { frame, document, depth } = pending.pop();
      if (seen.has(frame)) continue;
      if (depth > MAX_DEPTH || seen.size >= MAX_FRAMES) return error('CAPTURE_FRAME_UNSUPPORTED');
      seen.add(frame);
      let receiver;
      try {
        receiver = frame[PRIVATE];
      } catch { return error('CAPTURE_FRAME_UNSUPPORTED'); }
      if (!document || !receiver) return error('CAPTURE_FRAME_UNSUPPORTED');
      if (receiver.protocolVersion !== 2 || !Array.isArray(receiver.panelIds) || receiver.panelIds.length !== root.panelIds.length || receiver.panelIds.some((id,index)=>id!==root.panelIds[index]) ||
          receiver.maxPayloadBytes !== root.maxPayloadBytes || typeof receiver.read !== 'function') {
        return error('CAPTURE_PROTOCOL_UNSUPPORTED');
      }
      receivers.push(receiver);
      const enqueue = (child, childDocument) => {
        if (++edges > MAX_FRAMES * 4) throw 'CAPTURE_FRAME_UNSUPPORTED';
        // Native contentDocument равен null для недоступного origin.
        if (!child || !childDocument) throw 'CAPTURE_FRAME_UNSUPPORTED';
        if (!seen.has(child)) pending.push({ frame: child, document: childDocument, depth: depth + 1 });
      };
      try {
        const length = countFrames.call(frame);
        const elements = queryFrames.call(document, 'iframe,frame');
        // Не пропускаем неизвестные child contexts, например iframe в закрытом shadow root.
        if (!Number.isSafeInteger(length) || length < 0 || length > MAX_FRAMES || elements.length !== length) {
          return error('CAPTURE_FRAME_UNSUPPORTED');
        }
        for (let index = 0; index < elements.length; index++) {
          const element = elements[index];
          const host=element.closest?.('[data-viz-panel-key]')?.getAttribute('data-viz-panel-key');
          if (host && /^panel-\d+$/.test(host) && Number(host.slice(6))!==panelId) continue;
          const access = frameAccess[element.localName];
          enqueue(access.window.call(element), access.document.call(element));
        }
      } catch { return error('CAPTURE_FRAME_UNSUPPORTED'); }
    }
    // Сначала завершаем обход DOM: его getters могут синхронно обновить producer.
    // Далее читаются только атомарные неизменяемые состояния receiver.
    const states = receivers.map((receiver) => receiver.read(panelId));
    const equivalent = (left, right) => {
      let work = 0;
      const visit = (a, b, depth) => {
        if (++work > root.maxPayloadBytes + 2048 || depth > 68) return false;
        if (a === b) return true;
        if (a === null || b === null || typeof a !== 'object' || typeof b !== 'object' ||
            Array.isArray(a) !== Array.isArray(b)) return false;
        const keys = Object.keys(a);
        if (keys.length !== Object.keys(b).length) return false;
        for (const key of keys) {
          if (!Object.prototype.hasOwnProperty.call(b, key) || !visit(a[key], b[key], depth + 1)) return false;
        }
        return true;
      };
      return visit(left, right, 0);
    };
    let selected = null;
    for (const state of states) {
      if (!state || !['idle', 'pending', 'terminal-ok', 'terminal-error'].includes(state.status)) {
        return error('CAPTURE_PAYLOAD_INVALID');
      }
      if (state.identity === null) {
        if (state.status === 'terminal-error') return state;
        if (state.status !== 'idle') return error('CAPTURE_PAYLOAD_INVALID');
        continue;
      }
      if (!state.identity || state.identity.producerId !== 'svgmodifier-panel' || state.identity.panelId !== panelId ||
          typeof state.identity.producerVersion !== 'string' || typeof state.identity.instanceId !== 'string') {
        return error('CAPTURE_PAYLOAD_INVALID');
      }
      if (!selected) { selected = state; continue; }
      if (state === selected) continue;
      const first = selected.identity;
      const next = state.identity;
      if (first.instanceId !== next.instanceId || first.producerId !== next.producerId ||
          first.producerVersion !== next.producerVersion || first.panelId !== next.panelId) {
        return error('CAPTURE_INSTANCE_AMBIGUOUS');
      }
      // Одинаковый instance через aliases допустим только с одним атомарным состоянием.
      if (!equivalent(selected, state)) return error('CAPTURE_PAYLOAD_INVALID');
    }
    return selected ?? idle();
  } catch { return error('CAPTURE_FRAME_UNSUPPORTED'); }
}
