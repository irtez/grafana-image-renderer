package capture

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed svg_receiver.js
var svgReceiverScript string

//go:embed svg_read.js
var svgReadScript string

//go:embed svg_layout.js
var svgLayoutScript string

// Скрипт устанавливается до навигации во всех документах, включая iframe.
func svgBootstrapScript(panelID, maxBytes int) string {
	return svgBatchBootstrapScript([]int{panelID}, maxBytes)
}

func svgBatchBootstrapScript(panelIDs []int, maxBytes int) string {
	ids, _ := json.Marshal(panelIDs)
	return fmt.Sprintf("(%s)(%s, %d);", svgReceiverScript, ids, maxBytes)
}
func svgReadExpression(panelID int) string { return fmt.Sprintf("(%s)(%d)", svgReadScript, panelID) }
