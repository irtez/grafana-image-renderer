package capture

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSVGLayoutDriver(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required")
	}
	cmd := exec.Command(node, "testdata/svg_layout_test.cjs")
	cmd.Stdin = strings.NewReader(svgLayoutScript)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	t.Logf("%s", output)
}
