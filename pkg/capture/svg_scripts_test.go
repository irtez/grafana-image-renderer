package capture

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSVGScriptRuntime(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for browser script contract tests")
	}
	input, err := json.Marshal(map[string]string{
		"batch":     fmt.Sprintf("(%s)([7,8],1048576);", svgReceiverScript),
		"bootstrap": svgBootstrapScript(7, 1024*1024),
		"limited":   svgBootstrapScript(7, 512),
		"read":      svgReadExpression(7),
	})
	require.NoError(t, err)
	command := exec.Command(node, "testdata/svg_receiver_test.cjs")
	command.Stdin = strings.NewReader(string(input))
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	t.Logf("%s", output)
}
