package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/chromedp"
)

type svgLayoutPanel struct {
	PanelID int             `json:"panelId"`
	Status  string          `json:"status"`
	Active  bool            `json:"active"`
	Error   *svgScriptError `json:"error,omitempty"`
}
type svgLayoutResult struct {
	Status     string           `json:"status"`
	ContextKey string           `json:"contextKey"`
	Panels     []svgLayoutPanel `json:"panels"`
	Error      *svgScriptError  `json:"error,omitempty"`
}
type svgPanelState struct {
	PanelID int             `json:"panelId"`
	State   svgScriptResult `json:"state"`
}
type svgBatchStep struct {
	Layout svgLayoutResult `json:"layout"`
	States []svgPanelState `json:"states"`
	Error  *svgScriptError `json:"error,omitempty"`
}
type svgStamp struct {
	Identity     *SVGIdentity `json:"identity"`
	Run          *SVGRun      `json:"run"`
	PayloadBytes int          `json:"payloadBytes"`
}
type svgBatchReader func(context.Context, int, map[int]svgStamp) (svgBatchStep, error)

func collectSVGBatch(ctx context.Context, request Request, maxBytes int, read svgBatchReader) (Collection, error) {
	ids := request.PanelIDs
	if len(ids) == 0 || maxBytes < 1 {
		return svgError("CAPTURE_PAYLOAD_INVALID"), nil
	}
	results := map[int]PanelResult{}
	stamps := map[int]svgStamp{}
	lastStatus := map[int]string{}
	active := map[int]bool{}
	focus := ids[0]
	var focusUntil time.Time
	contextKey := ""
	finish := func() Collection {
		rows := make([]PanelResult, len(ids))
		for index, id := range ids {
			r, ok := results[id]
			if !ok {
				code := "CAPTURE_TIMEOUT"
				if active[id] && lastStatus[id] == "idle" {
					code = "CAPTURE_PRODUCER_MISSING"
				}
				r = PanelResult{PanelID: id, Status: "error", Error: svgError(code).Error}
			}
			rows[index] = r
		}
		return Collection{Payload: BatchCollection{Panels: rows}}
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return finish(), nil
			}
			return Collection{}, err
		}
		step, err := read(ctx, focus, stamps)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return finish(), nil
			}
			return Collection{}, err
		}
		if step.Error != nil {
			return svgError(step.Error.Code), nil
		}
		layout := step.Layout
		if layout.Status == "error" && layout.Error != nil {
			return svgError(layout.Error.Code), nil
		}
		if layout.Status == "ready" {
			if layout.ContextKey == "" || len(layout.Panels) != len(ids) || len(step.States) != len(ids) {
				return svgError("CAPTURE_PAYLOAD_INVALID"), nil
			}
			if contextKey != "" && contextKey != layout.ContextKey {
				return svgError("CAPTURE_CONTEXT_CHANGED"), nil
			}
			contextKey = layout.ContextKey
			for i, id := range ids {
				p, state := layout.Panels[i], step.States[i]
				if p.PanelID != id || state.PanelID != id {
					return svgError("CAPTURE_PAYLOAD_INVALID"), nil
				}
				active[id] = p.Active
				if p.Status == "error" && p.Error != nil {
					results[id] = PanelResult{PanelID: id, Status: "error", Error: svgError(p.Error.Code).Error}
					delete(stamps, id)
					continue
				}
				if p.Status != "ready" || p.Error != nil {
					return svgError("CAPTURE_PAYLOAD_INVALID"), nil
				}
				lastStatus[id] = state.State.Status
				current := state.State
				if current.Unchanged {
					previous, ok := stamps[id]
					if !ok || current.Status != "terminal-ok" || current.SnapshotJSON != nil || current.Error != nil || !sameSVGStamp(previous, current) {
						return svgError("CAPTURE_PAYLOAD_INVALID"), nil
					}
					continue
				}
				if current.Status == "idle" && current.Identity == nil && !p.Active {
					continue
				}
				perPanel := request
				perPanel.PanelID = id
				collection, terminal := svgCollectionFromScript(current, perPanel, maxBytes)
				if terminal {
					result := PanelResult{PanelID: id, Status: "error", Error: collection.Error}
					if collection.Error == nil {
						result.Status = "ok"
						result.Payload = collection.Payload.(json.RawMessage)
						stamps[id] = svgStamp{current.Identity, current.Run, current.PayloadBytes}
					} else {
						delete(stamps, id)
					}
					results[id] = result
				} else {
					delete(results, id)
					delete(stamps, id)
				}
			}
			if len(results) == len(ids) {
				return finish(), nil
			}
			pending := []int{}
			for _, id := range ids {
				if _, done := results[id]; !done {
					pending = append(pending, id)
				}
			}
			if _, done := results[focus]; done || time.Now().After(focusUntil) {
				next := 0
				for i, id := range pending {
					if id == focus {
						next = (i + 1) % len(pending)
						break
					}
				}
				// A single slow/offscreen panel cannot consume the whole budget before others are visited.
				if focusUntil.IsZero() {
					next = 0
				}
				focus = pending[next]
				slice := 500 * time.Millisecond
				if deadline, ok := ctx.Deadline(); ok {
					slice = time.Until(deadline) / time.Duration(len(pending))
					if slice < 50*time.Millisecond {
						slice = 50 * time.Millisecond
					}
				}
				focusUntil = time.Now().Add(slice)
			}
		} else if layout.Status != "pending" {
			return svgError("CAPTURE_PAYLOAD_INVALID"), nil
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

func sameSVGStamp(previous svgStamp, current svgScriptResult) bool {
	return previous.Identity != nil && previous.Run != nil && current.Identity != nil && current.Run != nil && *previous.Identity == *current.Identity && *previous.Run == *current.Run && previous.PayloadBytes == current.PayloadBytes
}

func svgBatchReadScript(request Request, focusID int, stamps map[int]svgStamp) string {
	maxTotal := request.MaxResponseBytes
	if maxTotal < 1 {
		maxTotal = 16777216
	}
	args, _ := json.Marshal(map[string]any{"panelIds": request.PanelIDs, "dashboardUID": request.DashboardUID, "focusID": focusID, "stamps": stamps, "maxTotalBytes": maxTotal})
	return fmt.Sprintf(`((request)=>{
 const layoutStep=(%s),read=(%s);const layout=layoutStep({...request,focusID:-1});
 if(layout.status!=='ready')return {layout,states:[]};
 let bytes=0;const states=[];
 for(const panelId of request.panelIds){
  const state=read(panelId),old=request.stamps?.[panelId];
  if(state.status==='terminal-ok'){
   const same=old&&old.payloadBytes===state.payloadBytes&&['producerId','producerVersion','panelId','instanceId'].every(k=>old.identity[k]===state.identity[k])&&['generation','effectiveFromMs','effectiveToMs'].every(k=>old.run[k]===state.run[k]);
   if(same)states.push({panelId,state:{status:state.status,identity:state.identity,run:state.run,payloadBytes:state.payloadBytes,unchanged:true}});
   else {bytes+=state.payloadBytes;if(bytes>request.maxTotalBytes)return {error:{code:'CAPTURE_PAYLOAD_TOO_LARGE'}};
    states.push({panelId,state:{status:state.status,identity:state.identity,run:state.run,payloadBytes:state.payloadBytes,snapshotJSON:JSON.stringify(state.snapshot)}});}
  }else states.push({panelId,state});
 }
 // Capture current terminals before switching a tab can unmount their producer.
 const focused=layoutStep(request);
 if(focused.status==='error')return {error:focused.error};
 if(focused.contextKey!==layout.contextKey)return {error:{code:'CAPTURE_CONTEXT_CHANGED'}};
 const failure=focused.panels.find(p=>p.panelId===request.focusID&&p.status==='error');
 if(failure)layout.panels=layout.panels.map(p=>p.panelId===failure.panelId?failure:p);
 return {layout,states};
})(%s)`, svgLayoutScript, svgReadScript, args)
}

func readSVGBatch(ctx context.Context, request Request, focusID int, stamps map[int]svgStamp) (svgBatchStep, error) {
	var raw json.RawMessage
	if err := chromedp.Evaluate(svgBatchReadScript(request, focusID, stamps), &raw).Do(ctx); err != nil {
		return svgBatchStep{}, err
	}
	var step svgBatchStep
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&step); err != nil {
		return svgBatchStep{Error: &svgScriptError{Code: "CAPTURE_PAYLOAD_INVALID"}}, nil
	}
	return step, nil
}
