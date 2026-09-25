package config

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
)

type CaptureConfig struct {
	SVGMaxPanels     int
	SVGMaxPanelBytes int
	SVGMaxJSONBytes  int
	SemanticEnabled  bool
	Timeout          time.Duration
	MaxJSONBytes     int
}

func CaptureFlags() []cli.Flag {
	flags := []cli.Flag{
		&cli.BoolFlag{
			Name:    "capture.semantic-enabled",
			Value:   false,
			Usage:   "Enable opt-in semantic panel capture. [config: capture.semantic-enabled]",
			Sources: FromConfig("capture.semantic-enabled", "CAPTURE_SEMANTIC_ENABLED"),
		},
		&cli.DurationFlag{
			Name:    "capture.timeout",
			Value:   5 * time.Second,
			Usage:   "Maximum collector time. [config: capture.timeout]",
			Sources: FromConfig("capture.timeout", "CAPTURE_TIMEOUT"),
			Validator: func(v time.Duration) error {
				if v <= 0 {
					return fmt.Errorf("capture timeout must be positive")
				}
				return nil
			},
		},
		&cli.IntFlag{
			Name:    "capture.max-json-bytes",
			Value:   1048576,
			Usage:   "Maximum serialized semantic JSON body. [config: capture.max-json-bytes]",
			Sources: FromConfig("capture.max-json-bytes", "CAPTURE_MAX_JSON_BYTES"),
			Validator: func(v int) error {
				if v <= 0 {
					return fmt.Errorf("capture max-json-bytes must be positive")
				}
				return nil
			},
		},
	}
	for _, setting := range []struct {
		name, env string
		value     int
	}{
		{"svg-max-panels", "CAPTURE_SVG_MAX_PANELS", 16},
		{"svg-max-panel-bytes", "CAPTURE_SVG_MAX_PANEL_BYTES", 4194304},
		{"svg-max-json-bytes", "CAPTURE_SVG_MAX_JSON_BYTES", 16777216},
	} {
		key := "capture." + setting.name
		flags = append(flags, &cli.IntFlag{Name: key, Value: setting.value, Usage: "SVG capture limit. [config: " + key + "]", Sources: FromConfig(key, setting.env), Validator: func(v int) error {
			if v <= 0 {
				return fmt.Errorf("%s must be positive", key)
			}
			return nil
		}})
	}
	return flags
}

func CaptureConfigFromCommand(c *cli.Command) (CaptureConfig, error) {
	return CaptureConfig{
		SVGMaxPanels:     c.Int("capture.svg-max-panels"),
		SVGMaxPanelBytes: c.Int("capture.svg-max-panel-bytes"),
		SVGMaxJSONBytes:  c.Int("capture.svg-max-json-bytes"),
		SemanticEnabled:  c.Bool("capture.semantic-enabled"),
		Timeout:          c.Duration("capture.timeout"),
		MaxJSONBytes:     c.Int("capture.max-json-bytes"),
	}, nil
}
