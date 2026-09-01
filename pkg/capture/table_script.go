package capture

const tableCaptureScript = `async (args) => {
  const domain = (code, details = {}) => {
    const error = new Error(code);
    error.siamCapture = { code, ...details };
    throw error;
  };
  const twoFrames = async () => {
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
  };
  const integerAttribute = (element, name) => {
    const raw = element.getAttribute(name);
    if (raw === null || !/^[0-9]+$/.test(raw)) {
      domain("TABLE_GRID_INCONSISTENT");
    }
    const value = Number(raw);
    if (!Number.isSafeInteger(value) || value <= 0) {
      domain("TABLE_GRID_INCONSISTENT");
    }
    return value;
  };
  const normalizeText = (value) => String(value ?? "")
    .replace(/\r\n?/g, "\n")
    .replace(/\x1B(?:[@-_]|\[[0-?]*[ -/]*[@-~])/g, "")
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, "");
  const renderedText = (element) => {
    const hidden = Array.from(element.querySelectorAll('[aria-hidden="true"]'));
    const saved = hidden.map((node) => ({
      node,
      value: node.style.getPropertyValue("display"),
      priority: node.style.getPropertyPriority("display"),
    }));
    for (const item of saved) {
      item.node.style.setProperty("display", "none", "important");
    }
    try {
      return normalizeText(element.innerText);
    } finally {
      for (const item of saved) {
        if (item.value) {
          item.node.style.setProperty("display", item.value, item.priority);
        } else {
          item.node.style.removeProperty("display");
        }
      }
    }
  };
  const scrollPositions = (maximum, viewport) => {
    const max = Math.max(0, Math.floor(maximum));
    const step = Math.max(1, Math.floor(viewport * 0.8));
    const positions = [0];
    for (let position = step; position < max; position += step) {
      positions.push(position);
    }
    if (max > 0) {
      positions.push(max);
    }
    if (positions.length > Math.ceil(max / step) + 2) {
      domain("TABLE_GRID_INCONSISTENT");
    }
    return positions;
  };

  try {
    if (!Number.isSafeInteger(args.panelId) || args.panelId < 0 || !Number.isSafeInteger(args.maxJSONBytes) || args.maxJSONBytes <= 0) {
      domain("TABLE_GRID_INCONSISTENT");
    }

    const scene = window.__grafanaSceneContext;
    const panels = scene?.getDashboardPanels?.() ?? [];
    const matches = panels.filter((panel) => panel?.state?.key === "panel-" + args.panelId);
    if (matches.length !== 1) {
      domain("TABLE_PANEL_NOT_FOUND");
    }
    const panel = matches[0];
    if (panel?.state?.pluginId !== "table") {
      domain("TABLE_PANEL_TYPE_MISMATCH");
    }

    const panelData = panel?.state?.$data?.state?.data;
    if (!panelData) {
      domain("TABLE_DATA_NOT_READY");
    }
    if (Array.isArray(panelData.errors) && panelData.errors.length > 0) {
      domain("TABLE_QUERY_ERROR");
    }
    if (panelData.state !== "Done") {
      domain("TABLE_DATA_NOT_READY");
    }

    const frames = Array.isArray(panelData.series) ? panelData.series : [];
    const configuredFrameIndex = Number(panel?.state?.options?.frameIndex ?? 0);
    const frameIndex = configuredFrameIndex > 0 && configuredFrameIndex < frames.length ? configuredFrameIndex : 0;
    const frame = frames[frameIndex];
    if (!frame || !Array.isArray(frame.fields)) {
      domain("TABLE_FRAME_MISSING");
    }
    if (frame.fields.some((field) => field?.type === "nestedFrames")) {
      domain("TABLE_NESTED_FRAME_UNSUPPORTED");
    }
    const totalRows = Number(frame.length);
    if (!Number.isSafeInteger(totalRows) || totalRows < 0) {
      domain("TABLE_FRAME_MISSING");
    }

    const effectiveFromMs = Number(panelData?.timeRange?.from?.valueOf?.());
    const effectiveToMs = Number(panelData?.timeRange?.to?.valueOf?.());
    if (!Number.isFinite(effectiveFromMs) || !Number.isFinite(effectiveToMs) || effectiveFromMs > effectiveToMs) {
      domain("TABLE_DATA_NOT_READY");
    }

    let grids = [];
    for (let attempt = 0; attempt < 60; attempt++) {
      grids = Array.from(document.querySelectorAll('[role="grid"]'))
        .filter((grid) => !grid.parentElement?.closest('[role="grid"]'));
      if (grids.length !== 0) {
        break;
      }
      await twoFrames();
    }
    if (grids.length === 0) {
      domain("TABLE_GRID_NOT_FOUND");
    }
    if (grids.length !== 1) {
      domain("TABLE_GRID_INCONSISTENT");
    }
    const grid = grids[0];
    const columnCount = integerAttribute(grid, "aria-colcount");

    const capturePage = async () => {
      const headers = new Map();
      const rows = new Map();
      const horizontalMax = Math.max(0, grid.scrollWidth - grid.clientWidth);
      const horizontalPositions = scrollPositions(horizontalMax, grid.clientWidth);

      for (const horizontalPosition of horizontalPositions) {
        const previousLeft = grid.scrollLeft;
        grid.scrollLeft = horizontalPosition;
        await twoFrames();
        if (horizontalPosition > previousLeft + 1 && grid.scrollLeft <= previousLeft + 0.5 && previousLeft < horizontalMax - 1) {
          domain("TABLE_GRID_INCONSISTENT");
        }

        for (const header of grid.querySelectorAll('[role="columnheader"][aria-colindex]')) {
          const columnIndex = integerAttribute(header, "aria-colindex");
          if (columnIndex > columnCount) {
            domain("TABLE_GRID_INCONSISTENT");
          }
          const text = renderedText(header);
          if (headers.has(columnIndex) && headers.get(columnIndex) !== text) {
            domain("TABLE_GRID_INCONSISTENT");
          }
          headers.set(columnIndex, text);
        }

        const verticalMax = Math.max(0, grid.scrollHeight - grid.clientHeight);
        const verticalPositions = scrollPositions(verticalMax, grid.clientHeight);
        for (const verticalPosition of verticalPositions) {
          const previousTop = grid.scrollTop;
          grid.scrollTop = verticalPosition;
          await twoFrames();
          if (verticalPosition > previousTop + 1 && grid.scrollTop <= previousTop + 0.5 && previousTop < verticalMax - 1) {
            domain("TABLE_GRID_INCONSISTENT");
          }

          for (const rowElement of grid.querySelectorAll('[role="row"][aria-rowindex]')) {
            if (rowElement.querySelector('[role="columnheader"]')) {
              continue;
            }
            const cells = rowElement.querySelectorAll(':scope > [role="gridcell"][aria-colindex]');
            if (cells.length === 0) {
              continue;
            }
            const rowIndex = integerAttribute(rowElement, "aria-rowindex");
            if (rowIndex <= 1) {
              domain("TABLE_GRID_INCONSISTENT");
            }
            let row = rows.get(rowIndex);
            if (!row) {
              row = new Map();
              rows.set(rowIndex, row);
            }
            for (const cell of cells) {
              const columnIndex = integerAttribute(cell, "aria-colindex");
              if (columnIndex > columnCount) {
                domain("TABLE_GRID_INCONSISTENT");
              }
              const text = renderedText(cell);
              if (row.has(columnIndex) && row.get(columnIndex) !== text) {
                domain("TABLE_GRID_INCONSISTENT");
              }
              row.set(columnIndex, text);
            }
          }
        }
        grid.scrollTop = 0;
        await twoFrames();
      }

      grid.scrollLeft = 0;
      grid.scrollTop = 0;
      await twoFrames();

      if (headers.size !== columnCount) {
        domain("TABLE_GRID_INCONSISTENT");
      }
      const orderedHeaders = [];
      for (let columnIndex = 1; columnIndex <= columnCount; columnIndex++) {
        if (!headers.has(columnIndex)) {
          domain("TABLE_GRID_INCONSISTENT");
        }
        orderedHeaders.push(headers.get(columnIndex));
      }

      const rowIndexes = Array.from(rows.keys()).sort((a, b) => a - b);
      const orderedRows = [];
      for (let localIndex = 0; localIndex < rowIndexes.length; localIndex++) {
        if (rowIndexes[localIndex] !== localIndex + 2) {
          domain("TABLE_GRID_INCONSISTENT");
        }
        const row = rows.get(rowIndexes[localIndex]);
        if (row.size !== columnCount) {
          domain("TABLE_GRID_INCONSISTENT");
        }
        const orderedCells = [];
        for (let columnIndex = 1; columnIndex <= columnCount; columnIndex++) {
          if (!row.has(columnIndex)) {
            domain("TABLE_GRID_INCONSISTENT");
          }
          orderedCells.push(row.get(columnIndex));
        }
        orderedRows.push(orderedCells);
      }
      return { headers: orderedHeaders, rows: orderedRows };
    };

    const pagination = document.querySelector('.table-ng-pagination[role="navigation"]');
    let numberOfPages = 1;
    if (pagination && totalRows > 0) {
      const numericPages = Array.from(pagination.querySelectorAll("button"))
        .map((button) => Number((button.innerText || "").trim()))
        .filter((value) => Number.isSafeInteger(value) && value > 0);
      if (numericPages.length === 0) {
        domain("TABLE_GRID_INCONSISTENT");
      }
      numberOfPages = Math.max(...numericPages);
    }

    let columns = null;
    const rows = [];
    let rowJSONBytes = 0;
    let limitedBy = null;

    for (let pageIndex = 0; pageIndex < numberOfPages; pageIndex++) {
      const page = await capturePage();
      if (columns === null) {
        columns = page.headers;
      } else if (JSON.stringify(columns) !== JSON.stringify(page.headers)) {
        domain("TABLE_GRID_INCONSISTENT");
      }

      for (const cells of page.rows) {
        const row = { position: rows.length, cells };
        const encodedBytes = new TextEncoder().encode(JSON.stringify(row)).length;
        if (rowJSONBytes + encodedBytes > args.maxJSONBytes) {
          if (rows.length === 0) {
            return { error: { code: "TABLE_ROW_TOO_LARGE", position: 0, rowUtf8Bytes: encodedBytes } };
          }
          limitedBy = "json_bytes";
          break;
        }
        rows.push(row);
        rowJSONBytes += encodedBytes;
      }
      if (limitedBy !== null) {
        break;
      }

      if (pageIndex + 1 < numberOfPages) {
        const nav = document.querySelector('.table-ng-pagination[role="navigation"]');
        const buttons = nav ? Array.from(nav.querySelectorAll("button")) : [];
        const nextButton = buttons.at(-1);
        if (!nextButton || nextButton.disabled) {
          domain("TABLE_PAGINATION_STALLED");
        }
        const beforeRow = grid.querySelector('[role="row"][aria-rowindex] > [role="gridcell"]')?.parentElement ?? null;
        const beforeSignature = Array.from(grid.querySelectorAll('[role="gridcell"][aria-colindex]'))
          .map((cell) => cell.getAttribute("aria-colindex") + ":" + renderedText(cell))
          .join("|");
        nextButton.click();

        let changed = false;
        for (let attempt = 0; attempt < 60; attempt++) {
          await twoFrames();
          const afterRow = grid.querySelector('[role="row"][aria-rowindex] > [role="gridcell"]')?.parentElement ?? null;
          const afterSignature = Array.from(grid.querySelectorAll('[role="gridcell"][aria-colindex]'))
            .map((cell) => cell.getAttribute("aria-colindex") + ":" + renderedText(cell))
            .join("|");
          if (afterRow !== beforeRow || afterSignature !== beforeSignature) {
            changed = true;
            break;
          }
        }
        if (!changed) {
          domain("TABLE_PAGINATION_STALLED");
        }
        grid.scrollLeft = 0;
        grid.scrollTop = 0;
        await twoFrames();
      }
    }

    if (columns === null) {
      domain("TABLE_GRID_INCONSISTENT");
    }
    if (limitedBy === null && rows.length !== totalRows) {
      domain("TABLE_GRID_INCONSISTENT");
    }
    if (limitedBy !== null && rows.length >= totalRows) {
      domain("TABLE_GRID_INCONSISTENT");
    }

    return {
      payload: {
        kind: "grafana-table",
        schemaVersion: 1,
        panel: { id: args.panelId, title: String(panel?.state?.title ?? "") },
        observed: {
          dataState: "Done",
          effectiveFromMs,
          effectiveToMs,
        },
        frame: {
          index: frameIndex,
          name: String(frame.name ?? frame.refId ?? ""),
          totalFrames: frames.length,
        },
        columns,
        dataset: {
          totalRows,
          capturedRows: rows.length,
          complete: limitedBy === null && rows.length === totalRows,
          limitedBy,
        },
        rows,
      },
    };
  } catch (error) {
    if (error?.siamCapture) {
      return { error: error.siamCapture };
    }
    throw error;
  }
}`
