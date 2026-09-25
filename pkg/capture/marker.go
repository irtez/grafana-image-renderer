package capture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	markerVersion = "siamCaptureVersion"
	markerKind    = "siamCaptureKind"
	markerPanels  = "siamCapturePanels"
)

type Transport struct {
	Encoding  string
	RenderKey string
	Domain    string
}

type ProtocolError struct {
	Code    string
	Message string
}

func (e *ProtocolError) Error() string {
	return e.Code + ": " + e.Message
}

func markerInvalid() error {
	return &ProtocolError{
		Code:    "CAPTURE_MARKER_INVALID",
		Message: "semantic capture marker is invalid",
	}
}

func unsupportedKind() error {
	return &ProtocolError{
		Code:    "CAPTURE_KIND_UNSUPPORTED",
		Message: "semantic capture kind is not supported",
	}
}

func VariablesHash(values url.Values) string {
	pairs := make([][]string, 0)
	for key, entries := range values {
		if !strings.HasPrefix(key, "var-") {
			continue
		}
		for _, value := range entries {
			pairs = append(pairs, []string{key, value})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] == pairs[j][0] {
			return pairs[i][1] < pairs[j][1]
		}
		return pairs[i][0] < pairs[j][0]
	})

	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(pairs); err != nil {
		panic(fmt.Sprintf("encode string pairs: %v", err))
	}
	body := bytes.TrimSuffix(encoded.Bytes(), []byte{'\n'})
	digest := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func parseCaptureRequest(target *url.URL, transport Transport) (Request, string, string, error) {
	values, parseErr := url.ParseQuery(target.RawQuery)
	hasMarker := false
	for key := range values {
		if strings.HasPrefix(key, "siamCapture") {
			hasMarker = true
			break
		}
	}
	if !hasMarker && !strings.Contains(target.RawQuery, "siamCapture") {
		return Request{}, "", "", nil
	}
	if parseErr != nil {
		return Request{}, "", "", markerInvalid()
	}

	for key := range values {
		if strings.HasPrefix(key, "siamCapture") && key != markerVersion && key != markerKind && key != markerPanels {
			return Request{}, "", "", markerInvalid()
		}
	}
	version, ok := singleton(values, markerVersion)
	if !ok || (version != "1" && version != "2") {
		return Request{}, "", "", markerInvalid()
	}
	kind, ok := singleton(values, markerKind)
	if !ok || kind == "" {
		return Request{}, "", "", markerInvalid()
	}
	v2 := version == "2" && kind == "svgmodifier"
	if (version == "2" || kind == "svgmodifier") && !v2 {
		return Request{}, "", "", markerInvalid()
	}
	if !v2 && values.Has(markerPanels) {
		return Request{}, "", "", markerInvalid()
	}
	if transport.Encoding != "png" || transport.RenderKey == "" || transport.Domain == "" {
		return Request{}, "", "", markerInvalid()
	}

	segments := strings.Split(target.Path, "/")
	if len(segments) < 4 || segments[0] != "" || (!v2 && len(segments) != 4) {
		return Request{}, "", "", markerInvalid()
	}
	route := len(segments) - 3
	for index, part := range segments[1:] {
		if invalidPathSegment(part) || (index < route-1 && part == "render") || strings.ContainsAny(part, "\\\x00") {
			return Request{}, "", "", markerInvalid()
		}
	}
	if segments[route] != "d-solo" && !(v2 && segments[route] == "d") {
		return Request{}, "", "", markerInvalid()
	}

	render, ok := singleton(values, "render")
	if !ok || render != "1" {
		return Request{}, "", "", markerInvalid()
	}
	panelID := 0
	var panelIDs []int
	if v2 && segments[route] == "d" {
		raw, valid := nonEmptySingleton(values, markerPanels)
		if !valid || values.Has("panelId") {
			return Request{}, "", "", markerInvalid()
		}
		seen := map[int]bool{}
		for _, part := range strings.Split(raw, ",") {
			id, valid := parsePanelID(part)
			if !valid || seen[id] {
				return Request{}, "", "", markerInvalid()
			}
			seen[id] = true
			panelIDs = append(panelIDs, id)
		}
	} else {
		raw, valid := singleton(values, "panelId")
		if v2 {
			raw = strings.TrimPrefix(raw, "panel-")
		}
		var canonical bool
		panelID, canonical = parsePanelID(raw)
		if !valid || !canonical || values.Has(markerPanels) {
			return Request{}, "", "", markerInvalid()
		}
		if v2 {
			panelIDs = []int{panelID}
		}
	}
	renderFrom, ok := nonEmptySingleton(values, "from")
	if !ok {
		return Request{}, "", "", markerInvalid()
	}
	renderTo, ok := nonEmptySingleton(values, "to")
	if !ok {
		return Request{}, "", "", markerInvalid()
	}
	timezone, ok := nonEmptySingleton(values, "tz")
	if !ok {
		return Request{}, "", "", markerInvalid()
	}

	cleaned := *target
	values.Del(markerVersion)
	values.Del(markerKind)
	values.Del(markerPanels)
	if v2 {
		values.Del("refresh")
	}
	cleaned.RawQuery = values.Encode()

	return Request{
		Version: func() int {
			if v2 {
				return 2
			}
			return 0
		}(),
		PanelIDs:      panelIDs,
		DashboardUID:  segments[route+1],
		PanelID:       panelID,
		Kind:          kind,
		RenderFrom:    renderFrom,
		RenderTo:      renderTo,
		Timezone:      timezone,
		VariablesHash: VariablesHash(values),
	}, cleaned.String(), kind, nil
}

func parsePanelID(raw string) (int, bool) {
	id, err := strconv.Atoi(raw)
	return id, err == nil && id >= 0 && strconv.Itoa(id) == raw && int64(id) <= 9007199254740991
}

func singleton(values url.Values, key string) (string, bool) {
	entries := values[key]
	if len(entries) != 1 {
		return "", false
	}
	return entries[0], true
}

func nonEmptySingleton(values url.Values, key string) (string, bool) {
	value, ok := singleton(values, key)
	return value, ok && value != ""
}

func invalidPathSegment(segment string) bool {
	return segment == "" || segment == "." || segment == ".."
}
