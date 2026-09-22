package capture

import (
	_ "embed"
	"fmt"
)

//go:embed svg_receiver.js
var svgReceiverScript string

//go:embed svg_read.js
var svgReadScript string

// Скрипт устанавливается до навигации во всех документах, включая iframe.
func svgBootstrapScript(panelID, maxBytes int) string {
	return fmt.Sprintf("(%s)(%d, %d);", svgReceiverScript, panelID, maxBytes)
}
