(panelIds, maxPayloadBytes) => {
  'use strict';
  const PRIVATE = '__SVG_MODIFIER_RENDER_CAPTURE_V2__';
  const HOOK = '__SVG_MODIFIER_CAPTURE_V2__';
  const MAX_DEPTH = 64;
  const MAX_SESSIONS = 64;
  const freeze = Object.freeze;
  const create = Object.create;
  const define = Object.defineProperty;
  const descriptor = Object.getOwnPropertyDescriptor;
  const prototype = Object.getPrototypeOf;
  const ownKeys = Reflect.ownKeys;
  const isArray = Array.isArray;
  const safeInteger = Number.isSafeInteger;
  const finite = Number.isFinite;
  const objectPrototype = Object.prototype;
  const arrayPrototype = Array.prototype;
  const own = Function.prototype.call.bind(Object.prototype.hasOwnProperty);
  const immutable = (fields) => freeze(Object.assign(create(null), fields));
  const errors = new Set([
    'CAPTURE_INSTANCE_AMBIGUOUS',
    'CAPTURE_PAYLOAD_TOO_LARGE',
    'CAPTURE_PAYLOAD_INVALID',
    'CAPTURE_DATA_STATE_UNSUPPORTED',
    'CAPTURE_EXPORT_FAILED',
    'CAPTURE_PROTOCOL_UNSUPPORTED',
    'CAPTURE_FRAME_UNSUPPORTED',
    'CAPTURE_PRODUCER_MISSING',
    'CAPTURE_MODE_UNSUPPORTED',
    'CAPTURE_SVG_COMPLEXITY_LIMIT',
    'CAPTURE_SVG_TEXT_LIMIT',
    'CAPTURE_SVG_INVALID_GEOMETRY',
  ]);
  if (
    !isArray(panelIds) ||
    panelIds.length === 0 ||
    panelIds.some((id) => !safeInteger(id) || id < 0) ||
    new Set(panelIds).size !== panelIds.length ||
    !safeInteger(maxPayloadBytes) ||
    maxPayloadBytes < 1
  ) {
    throw new Error('CAPTURE_PROTOCOL_UNSUPPORTED');
  }
  const previous = descriptor(window, PRIVATE);
  if (previous) {
    if (
      own(previous, 'value') &&
      previous.value.protocolVersion === 2 &&
      isArray(previous.value.panelIds) &&
      previous.value.panelIds.length === panelIds.length &&
      panelIds.every((id, i) => previous.value.panelIds[i] === id) &&
      previous.value.maxPayloadBytes === maxPayloadBytes
    ) {
      return;
    }
    throw new Error('CAPTURE_PROTOCOL_UNSUPPORTED');
  }

  // Код ошибки не содержит исходного исключения или данных панели.
  const invalid = () => {
    throw 'CAPTURE_PAYLOAD_INVALID';
  };
  const tooLarge = () => {
    throw 'CAPTURE_PAYLOAD_TOO_LARGE';
  };
  const record = (value) =>
    typeof value === 'object' && value !== null && !isArray(value);

  // Считаем JSON UTF-8 до сериализации; не вызываем getters и toJSON.
  const boundedCopy = (input, limit) => {
    let bytes = 0;
    let values = 0;
    const ancestors = new Set();
    const spend = (count) => {
      if (count > limit - bytes) tooLarge();
      bytes += count;
    };
    const stringBytes = (value) => {
      if (value.length + 2 > limit - bytes) tooLarge();
      spend(2);
      for (let index = 0; index < value.length; index++) {
        const code = value.charCodeAt(index);
        if (code === 34 || code === 92) spend(2);
        else if (code < 32)
          spend(
            code === 8 ||
              code === 9 ||
              code === 10 ||
              code === 12 ||
              code === 13
              ? 2
              : 6,
          );
        else if (code < 0x80) spend(1);
        else if (code < 0x800) spend(2);
        else if (code >= 0xd800 && code <= 0xdbff) {
          const next = value.charCodeAt(index + 1);
          if (next >= 0xdc00 && next <= 0xdfff) {
            spend(4);
            index++;
          } else spend(6);
        } else spend(code >= 0xdc00 && code <= 0xdfff ? 6 : 3);
      }
    };
    const visit = (value, depth) => {
      if (++values > limit || depth > MAX_DEPTH) tooLarge();
      if (value === null) {
        spend(4);
        return null;
      }
      if (typeof value === 'string') {
        stringBytes(value);
        return value;
      }
      if (typeof value === 'boolean') {
        spend(value ? 4 : 5);
        return value;
      }
      if (typeof value === 'number') {
        if (!finite(value)) invalid();
        spend(String(value).length);
        return value;
      }
      if (typeof value !== 'object' || ancestors.has(value)) invalid();
      const array = isArray(value);
      const proto = prototype(value);
      if (
        array
          ? proto !== arrayPrototype
          : proto !== objectPrototype && proto !== null
      )
        invalid();
      let length;
      if (array) {
        length = descriptor(value, 'length')?.value;
        if (!safeInteger(length) || length < 0) invalid();
        if (
          length > limit - values ||
          (length ? length * 2 + 1 : 2) > limit - bytes
        )
          tooLarge();
      }
      // Полная проверка symbol/non-enumerable требует ownKeys: потокового API в JS нет.
      // Само перечисление ключей, Proxy traps и producer factory не прерываются
      // этими бюджетами. Ограничены копия и дальнейший обход, а не произвольный JS.
      const keys = ownKeys(value);
      const count = keys.length - (array ? 1 : 0);
      if (count > limit - values || count > limit - bytes) tooLarge();
      if (array && count !== length) invalid();
      const copy = array ? [] : create(null);
      // Сериализация массива также не должна увидеть изменённый Array.prototype.toJSON.
      if (array) Object.setPrototypeOf(copy, null);
      ancestors.add(value);
      spend(2);
      let entries = 0;
      for (const key of keys) {
        if (array && key === 'length') continue;
        if (typeof key !== 'string') invalid();
        const property = descriptor(value, key);
        if (!property || !property.enumerable || !own(property, 'value'))
          invalid();
        if (entries) spend(1);
        if (array) {
          if (key !== String(entries)) invalid();
        } else {
          stringBytes(key);
          spend(1);
        }
        define(copy, key, {
          value: visit(property.value, depth + 1),
          enumerable: true,
        });
        entries++;
      }
      ancestors.delete(value);
      return freeze(copy);
    };
    return { value: visit(input, 0), bytes };
  };
  const createReceiver = (panelId) => {
    const identityFrom = (input) => {
      try {
        const value = boundedCopy(input, 2048).value;
        if (
          !record(value) ||
          ownKeys(value).length !== 4 ||
          value.producerId !== 'svgmodifier-panel' ||
          value.panelId !== panelId ||
          typeof value.producerVersion !== 'string' ||
          !value.producerVersion.trim() ||
          value.producerVersion.length > 128 ||
          typeof value.instanceId !== 'string' ||
          !value.instanceId.trim() ||
          value.instanceId.length > 256
        )
          return null;
        return value;
      } catch {
        return null;
      }
    };
    const runFrom = (input) => {
      try {
        const value = boundedCopy(input, 256).value;
        if (
          !record(value) ||
          ownKeys(value).length !== 3 ||
          !safeInteger(value.generation) ||
          value.generation < 1 ||
          !safeInteger(value.effectiveFromMs) ||
          !safeInteger(value.effectiveToMs) ||
          value.effectiveToMs < value.effectiveFromMs
        )
          return null;
        return value;
      } catch {
        return null;
      }
    };
    const matchesRun = (value, identity, run) =>
      record(value) &&
      value.kind === 'svgmodifier' &&
      value.schemaVersion === 2 &&
      record(value.producer) &&
      value.producer.id === identity.producerId &&
      value.producer.version === identity.producerVersion &&
      record(value.panel) &&
      value.panel.id === panelId &&
      record(value.observed) &&
      value.observed.generation === run.generation &&
      value.observed.effectiveFromMs === run.effectiveFromMs &&
      value.observed.effectiveToMs === run.effectiveToMs &&
      safeInteger(value.observed.evaluatedAtMs) &&
      (value.observed.dataState === 'Done' ||
        value.observed.dataState === 'Error');

    const live = new Set();
    let overflowed = false;
    const idle = (identity = null) =>
      immutable({ status: 'idle', identity, run: null });
    let state = idle();
    const error = (code, session) => {
      state = immutable({
        status: 'terminal-error',
        identity: session?.identity ?? null,
        run: session?.run ?? null,
        error: immutable({ code }),
      });
    };
    const current = (session, generation) =>
      !overflowed &&
      live.size === 1 &&
      live.has(session) &&
      session.run !== null &&
      session.run.generation === generation;
    const membershipChanged = () => {
      // После неоднозначности нужен новый begin; старый успех не восстанавливается.
      for (const session of live) session.run = null;
      if (overflowed || live.size > 1) error('CAPTURE_INSTANCE_AMBIGUOUS');
      else state = idle(live.values().next().value?.identity ?? null);
    };
    const hook = immutable({
      connect(input) {
        if (overflowed) return null;
        const identity = identityFrom(input);
        if (!identity) return null;
        if (live.size >= MAX_SESSIONS || overflowed) {
          // После превышения лимита больше нельзя доказать единственность instance.
          overflowed = true;
          membershipChanged();
          return null;
        }
        const session = {
          identity,
          highestGeneration: 0,
          run: null,
          publishingRun: null,
        };
        live.add(session);
        membershipChanged();
        return immutable({
          protocolVersion: 2,
          maxPayloadBytes,
          begin(input) {
            if (overflowed || !live.has(session)) return;
            const before = state;
            const highestGeneration = session.highestGeneration;
            const run = runFrom(input);
            if (
              overflowed ||
              !live.has(session) ||
              state !== before ||
              session.highestGeneration !== highestGeneration
            )
              return;
            if (!run) {
              // Некорректное обновление не оставляет доступным предыдущий снимок.
              session.run = null;
              if (live.size === 1) error('CAPTURE_PAYLOAD_INVALID', session);
              return;
            }
            if (run.generation <= session.highestGeneration) return;
            session.highestGeneration = run.generation;
            if (live.size !== 1) return;
            session.run = run;
            state = immutable({ status: 'pending', identity, run });
          },
          publish(generation, build) {
            if (
              !current(session, generation) ||
              state.status !== 'pending' ||
              state.identity !== identity ||
              state.run !== session.run ||
              session.publishingRun === session.run
            )
              return;
            const run = session.run;
            const before = state;
            const unchanged = () =>
              current(session, generation) &&
              session.run === run &&
              state === before;
            // Один factory/copy на run; новый begin допускается даже внутри старого factory.
            session.publishingRun = run;
            try {
              let value;
              try {
                value = build();
              } catch {
                if (unchanged()) error('CAPTURE_EXPORT_FAILED', session);
                return;
              }
              if (!unchanged()) return;
              try {
                const copied = boundedCopy(value, maxPayloadBytes);
                if (!matchesRun(copied.value, identity, run)) invalid();
                // Proxy traps также способны синхронно вызвать begin/fail/close.
                if (!unchanged()) return;
                state = immutable({
                  status: 'terminal-ok',
                  identity,
                  run,
                  snapshot: copied.value,
                  payloadBytes: copied.bytes,
                });
              } catch (caught) {
                if (unchanged())
                  error(
                    caught === 'CAPTURE_PAYLOAD_TOO_LARGE'
                      ? caught
                      : 'CAPTURE_PAYLOAD_INVALID',
                    session,
                  );
              }
            } finally {
              if (session.publishingRun === run) session.publishingRun = null;
            }
          },
          fail(generation, input) {
            if (!current(session, generation)) return;
            const run = session.run;
            const before = state;
            let code = 'CAPTURE_EXPORT_FAILED';
            try {
              const property = descriptor(input, 'code');
              if (
                property &&
                own(property, 'value') &&
                typeof property.value === 'string' &&
                errors.has(property.value)
              ) {
                code = property.value;
              }
            } catch {
              /* Сохраняется безопасный код; message не читается. */
            }
            if (
              current(session, generation) &&
              session.run === run &&
              state === before
            )
              error(code, session);
          },
          close() {
            if (!live.delete(session)) return;
            session.run = null;
            membershipChanged();
          },
        });
      },
    });
    return { hook, read: () => state };
  };
  const receivers = new Map(panelIds.map((id) => [id, createReceiver(id)]));
  const hook = immutable({
    connect(input) {
      try {
        const property = descriptor(input, 'panelId');
        if (
          !property ||
          !own(property, 'value') ||
          !safeInteger(property.value)
        )
          return null;
        return receivers.get(property.value)?.hook.connect(input) ?? null;
      } catch {
        return null;
      }
    },
  });
  const receiver = immutable({
    protocolVersion: 2,
    panelIds: freeze([...panelIds]),
    maxPayloadBytes,
    read: (id) =>
      receivers.get(id)?.read() ??
      immutable({
        status: 'terminal-error',
        identity: null,
        run: null,
        error: immutable({ code: 'CAPTURE_PAYLOAD_INVALID' }),
      }),
  });
  define(window, PRIVATE, { value: receiver });
  define(window, HOOK, { value: hook });
}
